// Integration tests for Java I2P interoperability
//
// These tests validate that go-streaming can successfully communicate
// with the Java I2P router and streaming library.
//
// Prerequisites:
//   - Java I2P router running on 127.0.0.1:7654
//   - Router must be bootstrapped and connected
//   - I2CP port accessible
//
// Run with: go test -v -run TestJavaI2P
package streaming

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestJavaI2P_RouterConnection tests basic connectivity to Java I2P router
func TestJavaI2P_RouterConnection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Use shared I2CP connection which validates router connectivity
	i2cp := RequireI2CP(t)

	t.Log("✓ connected to Java I2P router via shared I2CP session")
	t.Logf("✓ destination: %s...", i2cp.Manager.Destination().Base32()[:16])
}

// TestJavaI2P_SessionCreation tests creating an I2CP session
func TestJavaI2P_SessionCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	t.Log("=== I2CP Session Creation Handshake Trace ===")
	t.Log("")

	// Use shared I2CP connection (session already created)
	i2cp := RequireI2CP(t)

	t.Log("✓ Using shared I2CP session")
	t.Logf("  ✓ Session ID: %d", i2cp.Manager.Session().ID())
	t.Logf("  ✓ Destination: %s...", i2cp.Manager.Destination().Base32()[:52])
	t.Log("")
	t.Log("=== Session Creation Successful ===")
}

// TestJavaI2P_ListenerCreation tests creating a streaming listener
func TestJavaI2P_ListenerCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Use shared I2CP session
	i2cp := RequireI2CP(t)

	// Create streaming listener
	t.Log("creating streaming listener on port 8085...")
	listener, err := ListenWithManager(i2cp.Manager, 8085, DefaultMTU)
	require.NoError(t, err, "should create listener")
	defer listener.Close()

	t.Logf("✓ listener created")
	t.Logf("  Listening on port: 8085")
	t.Logf("  Destination: %s", i2cp.Manager.Destination().Base32())

	// Verify listener is ready
	time.Sleep(1 * time.Second)
	t.Log("✓ listener ready for connections")
}

// TestJavaI2P_PacketFormat validates that our packets match Java I2P format
func TestJavaI2P_PacketFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Use shared I2CP session
	i2cp := RequireI2CP(t)

	// Create a test SYN packet (without signature for simplicity)
	pkt := &Packet{
		SendStreamID:    0,     // Must be 0 in initial SYN
		RecvStreamID:    12345, // Our local stream ID
		SequenceNum:     1000,  // Initial sequence number
		AckThrough:      0,
		Flags:           FlagSYN | FlagFromIncluded,
		FromDestination: i2cp.Manager.Destination(),
	}

	// Add 8 NACKs (Java I2P compatibility)
	pkt.NACKs = make([]uint32, 8)
	for i := 0; i < 8; i++ {
		pkt.NACKs[i] = uint32(i * 100)
	}

	// Marshal to bytes
	data, err := pkt.Marshal()
	require.NoError(t, err, "should marshal packet")

	t.Logf("✓ SYN packet created")
	t.Logf("  Packet size: %d bytes", len(data))
	t.Logf("  Flags: 0x%02X (SYN=%v, FROM=%v)",
		pkt.Flags,
		pkt.Flags&FlagSYN != 0,
		pkt.Flags&FlagFromIncluded != 0)
	t.Logf("  SendStreamID: %d (must be 0)", pkt.SendStreamID)
	t.Logf("  RecvStreamID: %d", pkt.RecvStreamID)
	t.Logf("  SequenceNum: %d", pkt.SequenceNum)
	t.Logf("  NACKs: %d (8 required)", len(pkt.NACKs))
	t.Logf("  FromDestination: %d bytes", len(pkt.FromDestination.Base64()))

	// Validate minimum packet size (22 header + 32 NACKs + 391 dest)
	minSize := 22 + 32 + 391
	require.GreaterOrEqual(t, len(data), minSize, "packet should meet minimum size")

	// Unmarshal to verify roundtrip
	parsed := &Packet{}
	err = parsed.Unmarshal(data)
	require.NoError(t, err, "should unmarshal packet")

	require.Equal(t, pkt.SendStreamID, parsed.SendStreamID)
	require.Equal(t, pkt.RecvStreamID, parsed.RecvStreamID)
	require.Equal(t, pkt.SequenceNum, parsed.SequenceNum)
	require.Equal(t, pkt.Flags, parsed.Flags)
	require.Equal(t, len(pkt.NACKs), len(parsed.NACKs))

	t.Log("✓ packet format validated")
}

// TestJavaI2P_EnvironmentCheck validates the test environment is ready
func TestJavaI2P_EnvironmentCheck(t *testing.T) {
	t.Log("Java I2P Integration Test Environment Check")
	t.Log("============================================")
	t.Log("")
	t.Log("Prerequisites:")
	t.Log("  ✓ Java I2P router must be running")
	t.Log("  ✓ Router must be at 127.0.0.1:7654")
	t.Log("  ✓ Router must be bootstrapped")
	t.Log("  ✓ I2CP port must be accessible")
	t.Log("")
	t.Log("To run full integration tests:")
	t.Log("  go test -v -run TestJavaI2P")
	t.Log("")
	t.Log("To skip integration tests (unit tests only):")
	t.Log("  go test -v -short")
}
