package streaming

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCompatSession creates a fresh I2CP client and StreamManager with an
// active session, wiring cleanup to the test. Each compatibility session gets
// its own client because an I2CP connection supports exactly one session.
func newCompatSession(t *testing.T) *StreamManager {
	t.Helper()

	client := createTestClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	startProcessIO(t, client, ctx)

	manager := createTestManager(t, client)

	t.Cleanup(func() {
		manager.Close()
		client.Close()
		cancel()
	})

	return manager
}

// TestStreamingCompatability verifies a stream session can be established with
// whichever router is available.
func TestStreamingCompatability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compatibility test in short mode")
	}

	manager := newCompatSession(t)
	require.NotNil(t, manager.Destination(), "destination should be available after session start")
}

// TestCompat_StreamSessionRoundTrip runs the stream round-trip compatibility
// suite, labeled per router backend. With the i2p-provider action, the Java
// I2P job runs JavaI2P and skips I2PD; the i2pd job does the reverse.
func TestCompat_StreamSessionRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compatibility test in short mode")
	}

	t.Run("Streaming-test", func(t *testing.T) {
		runStreamRoundTripSuite(t)
	})
}

// runStreamRoundTripSuite verifies a full stream-session round trip against
// the active router: an echo server on one session, a client on a second
// session, multi-message echo integrity with RTT measurement, and a
// multi-packet payload that exceeds the streaming MTU.
func runStreamRoundTripSuite(t *testing.T) {
	t.Helper()

	serverManager := newCompatSession(t)
	clientManager := newCompatSession(t)

	// StartSession returns at session creation, before tunnels exist. Wait for
	// LeaseSet publication so the dial below doesn't race tunnel construction
	// on slow routers. Lenient on timeout: some routers may not push
	// CreateLeaseSet2 to the client, and the dial handshake is the real gate.
	for _, m := range []*StreamManager{serverManager, clientManager} {
		lsCtx, lsCancel := context.WithTimeout(context.Background(), 40*time.Second)
		if err := m.WaitForLeaseSet(lsCtx); err != nil {
			t.Logf("LeaseSet readiness not signaled within 30s, proceeding anyway: %v", err)
		}
		lsCancel()
	}

	const port = 9777
	listener, err := ListenWithManager(serverManager, port, DefaultMTU)
	require.NoError(t, err, "should create listener")
	defer listener.Close()

	serverDest := serverManager.Destination()
	require.NotNil(t, serverDest, "server destination should be available")
	t.Logf("server listening on %s:%d", serverDest.Base32()[:16], port)

	const opTimeout = 120 * time.Second

	// Echo server: reflect everything until the client disconnects.
	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		buf := make([]byte, 64*1024)
		for {
			conn.SetReadDeadline(time.Now().Add(opTimeout))
			n, err := conn.Read(buf)
			if err != nil {
				serverErr <- err
				return
			}

			conn.SetWriteDeadline(time.Now().Add(opTimeout))
			if _, err := conn.Write(buf[:n]); err != nil {
				serverErr <- err
				return
			}
		}
	}()

	t.Log("client dialing server...")
	clientConn, err := DialWithManager(clientManager, serverDest, 0, port)
	require.NoError(t, err, "client should dial server")

	// Multi-message echo round trip with RTT measurement.
	messages := []string{
		"hello i2p router",
		"stream session round-trip compatibility check",
		"third message to confirm the stream stays in sync",
	}

	var totalRTT time.Duration
	for i, msg := range messages {
		start := time.Now()

		clientConn.SetWriteDeadline(time.Now().Add(opTimeout))
		_, err := clientConn.Write([]byte(msg))
		require.NoError(t, err, "write failed for message %d", i+1)

		buf := make([]byte, len(msg))
		clientConn.SetReadDeadline(time.Now().Add(opTimeout))
		_, err = io.ReadFull(clientConn, buf)
		require.NoError(t, err, "read failed for message %d", i+1)

		rtt := time.Since(start)
		totalRTT += rtt
		assert.Equal(t, msg, string(buf), "echo mismatch for message %d", i+1)
		t.Logf("message %d echoed, RTT: %v", i+1, rtt)
	}
	t.Logf("average RTT over %d messages: %v", len(messages), totalRTT/time.Duration(len(messages)))

	// Multi-packet payload: larger than DefaultMTU, forcing fragmentation and
	// reassembly across several streaming packets in both directions.
	payload := make([]byte, 4*1024)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	clientConn.SetWriteDeadline(time.Now().Add(opTimeout))
	_, err = clientConn.Write(payload)
	require.NoError(t, err, "multi-packet write failed")

	received := make([]byte, len(payload))
	clientConn.SetReadDeadline(time.Now().Add(opTimeout))
	_, err = io.ReadFull(clientConn, received)
	require.NoError(t, err, "multi-packet read failed")
	assert.Equal(t, payload, received, "multi-packet payload should survive the round trip intact")
	t.Logf("multi-packet payload (%d bytes) echoed intact", len(payload))

	// All assertions above passed, so the server echoed every byte; anything
	// after client close is connection-teardown noise, not a round-trip failure.
	clientConn.Close()
	select {
	case err := <-serverErr:
		if err != nil && err != io.EOF {
			t.Logf("server exited after client close with: %v (non-fatal)", err)
		}
	case <-time.After(10 * time.Second):
		t.Log("server goroutine did not exit within 10s of client close (non-fatal)")
	}

	t.Log("stream session round-trip suite passed")
}
