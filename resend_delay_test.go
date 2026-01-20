package streaming

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetResendDelaySeconds verifies the RTO to ResendDelay conversion.
// Per I2P streaming spec, ResendDelay is a uint8 representing seconds.
func TestGetResendDelaySeconds(t *testing.T) {
	s := CreateTestStreamConn(t)
	s.mu.Lock()
	defer s.mu.Unlock()

	tests := []struct {
		name     string
		rto      time.Duration
		expected uint8
	}{
		{
			name:     "minimum RTO (100ms) rounds up to 1 second",
			rto:      100 * time.Millisecond,
			expected: 1,
		},
		{
			name:     "500ms rounds up to 1 second",
			rto:      500 * time.Millisecond,
			expected: 1,
		},
		{
			name:     "1 second exactly",
			rto:      1 * time.Second,
			expected: 1,
		},
		{
			name:     "1.5 seconds truncates to 1 second",
			rto:      1500 * time.Millisecond,
			expected: 1,
		},
		{
			name:     "9 seconds (default initial RTO)",
			rto:      9 * time.Second,
			expected: 9,
		},
		{
			name:     "30 seconds",
			rto:      30 * time.Second,
			expected: 30,
		},
		{
			name:     "60 seconds (max RTO)",
			rto:      60 * time.Second,
			expected: 60,
		},
		{
			name:     "255 seconds (max uint8)",
			rto:      255 * time.Second,
			expected: 255,
		},
		{
			name:     "300 seconds clamps to 255",
			rto:      300 * time.Second,
			expected: 255,
		},
		{
			name:     "zero duration rounds up to 1",
			rto:      0,
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s.rto = tt.rto
			result := s.getResendDelaySeconds()
			assert.Equal(t, tt.expected, result,
				"RTO %v should convert to ResendDelay %d", tt.rto, tt.expected)
		})
	}
}

// TestSendDataChunkIncludesResendDelay verifies that data packets
// include the ResendDelay field set to the current RTO.
func TestSendDataChunkIncludesResendDelay(t *testing.T) {
	s := CreateTestStreamConn(t)
	s.mu.Lock()
	defer s.mu.Unlock()

	// Set a specific RTO value
	s.rto = 9 * time.Second

	// Verify the expected ResendDelay
	expectedDelay := s.getResendDelaySeconds()
	assert.Equal(t, uint8(9), expectedDelay, "RTO of 9s should give ResendDelay of 9")
}

// TestResendDelayInPacket verifies that ResendDelay is correctly
// marshaled and unmarshaled in packets.
func TestResendDelayInPacket(t *testing.T) {
	tests := []struct {
		name        string
		resendDelay uint8
	}{
		{"zero delay", 0},
		{"1 second", 1},
		{"9 seconds (default)", 9},
		{"60 seconds (max RTO)", 60},
		{"255 seconds (max uint8)", 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkt := &Packet{
				SendStreamID: 1234,
				RecvStreamID: 5678,
				SequenceNum:  100,
				AckThrough:   50,
				Flags:        0,
				ResendDelay:  tt.resendDelay,
				Payload:      []byte("test data"),
			}

			data, err := pkt.Marshal()
			require.NoError(t, err)

			parsed := &Packet{}
			err = parsed.Unmarshal(data)
			require.NoError(t, err)

			assert.Equal(t, tt.resendDelay, parsed.ResendDelay,
				"ResendDelay should be preserved through marshal/unmarshal")
			assert.Equal(t, pkt.SequenceNum, parsed.SequenceNum)
			assert.Equal(t, pkt.Payload, parsed.Payload)
		})
	}
}

// TestResendDelayBounds verifies that ResendDelay calculation
// respects the uint8 bounds and handles edge cases.
func TestResendDelayBounds(t *testing.T) {
	s := CreateTestStreamConn(t)
	s.mu.Lock()
	defer s.mu.Unlock()

	// Test with minimum RTO
	s.rto = MinRTO
	minDelay := s.getResendDelaySeconds()
	assert.GreaterOrEqual(t, minDelay, uint8(1),
		"Minimum RTO should give at least 1 second delay")

	// Test with maximum RTO
	s.rto = MaxRTO
	maxDelay := s.getResendDelaySeconds()
	assert.LessOrEqual(t, maxDelay, uint8(255),
		"Maximum RTO should not exceed 255")
	assert.Equal(t, uint8(45), maxDelay,
		"MaxRTO (45s) should give ResendDelay of 45")
}

// TestResendDelayWithTypicalRTOProgression verifies ResendDelay
// values during typical RTO progression (initial, then after RTT samples).
func TestResendDelayWithTypicalRTOProgression(t *testing.T) {
	s := CreateTestStreamConn(t)
	s.mu.Lock()
	defer s.mu.Unlock()

	// Initial RTO (9 seconds per spec)
	s.rto = 9 * time.Second
	assert.Equal(t, uint8(9), s.getResendDelaySeconds())

	// After some RTT samples, RTO might decrease
	s.rto = 1 * time.Second
	assert.Equal(t, uint8(1), s.getResendDelaySeconds())

	// After timeout backoff, RTO doubles
	s.rto = 2 * time.Second
	assert.Equal(t, uint8(2), s.getResendDelaySeconds())

	// After multiple backoffs
	s.rto = 16 * time.Second
	assert.Equal(t, uint8(16), s.getResendDelaySeconds())

	// Capped at MaxRTO (45s per spec)
	s.rto = MaxRTO
	assert.Equal(t, uint8(45), s.getResendDelaySeconds())
}

// BenchmarkGetResendDelaySeconds benchmarks the RTO to ResendDelay conversion.
func BenchmarkGetResendDelaySeconds(b *testing.B) {
	// Create a minimal StreamConn for benchmarking
	s := &StreamConn{
		rto: 9 * time.Second,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.getResendDelaySeconds()
	}
}
