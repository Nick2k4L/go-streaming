package streaming

import (
	"testing"

	go_i2cp "github.com/go-i2p/go-i2cp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSignPacket tests the SignPacket function with various scenarios.
func TestSignPacket(t *testing.T) {
	crypto := go_i2cp.NewCrypto()

	t.Run("sign valid packet", func(t *testing.T) {
		// Create destination and keypair
		dest, err := go_i2cp.NewDestination(crypto)
		require.NoError(t, err)

		keyPair, err := crypto.Ed25519SignatureKeygen()
		require.NoError(t, err)

		// Create packet with FROM and signature flag
		pkt := &Packet{
			SendStreamID:    1,
			RecvStreamID:    2,
			SequenceNum:     100,
			AckThrough:      99,
			Flags:           FlagSYN | FlagFromIncluded | FlagSignatureIncluded,
			FromDestination: dest,
		}

		// Sign the packet
		err = SignPacket(pkt, keyPair)
		assert.NoError(t, err)
		assert.NotNil(t, pkt.Signature, "signature should be set")
		assert.Equal(t, 64, len(pkt.Signature), "Ed25519 signature should be 64 bytes")
	})

	t.Run("error when FromDestination is nil", func(t *testing.T) {
		keyPair, err := crypto.Ed25519SignatureKeygen()
		require.NoError(t, err)

		pkt := &Packet{
			SendStreamID: 1,
			RecvStreamID: 2,
			Flags:        FlagSYN | FlagSignatureIncluded,
			// FromDestination is nil
		}

		err = SignPacket(pkt, keyPair)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "FromDestination is nil")
	})

	t.Run("error when FlagSignatureIncluded not set", func(t *testing.T) {
		dest, err := go_i2cp.NewDestination(crypto)
		require.NoError(t, err)

		keyPair, err := crypto.Ed25519SignatureKeygen()
		require.NoError(t, err)

		pkt := &Packet{
			SendStreamID:    1,
			RecvStreamID:    2,
			Flags:           FlagSYN | FlagFromIncluded, // No FlagSignatureIncluded
			FromDestination: dest,
		}

		err = SignPacket(pkt, keyPair)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "FlagSignatureIncluded not set")
	})

	t.Run("sign packet with payload", func(t *testing.T) {
		dest, err := go_i2cp.NewDestination(crypto)
		require.NoError(t, err)

		keyPair, err := crypto.Ed25519SignatureKeygen()
		require.NoError(t, err)

		pkt := &Packet{
			SendStreamID:    1,
			RecvStreamID:    2,
			SequenceNum:     100,
			AckThrough:      99,
			Flags:           FlagACK | FlagFromIncluded | FlagSignatureIncluded,
			FromDestination: dest,
			Payload:         []byte("test payload"),
		}

		err = SignPacket(pkt, keyPair)
		assert.NoError(t, err)
		assert.NotNil(t, pkt.Signature)
		assert.Equal(t, 64, len(pkt.Signature))
	})

	t.Run("sign packet with multiple optional fields", func(t *testing.T) {
		dest, err := go_i2cp.NewDestination(crypto)
		require.NoError(t, err)

		keyPair, err := crypto.Ed25519SignatureKeygen()
		require.NoError(t, err)

		pkt := &Packet{
			SendStreamID:    1,
			RecvStreamID:    2,
			SequenceNum:     100,
			AckThrough:      99,
			Flags:           FlagSYN | FlagDelayRequested | FlagMaxPacketSizeIncluded | FlagFromIncluded | FlagSignatureIncluded,
			OptionalDelay:   1000,
			MaxPacketSize:   1500,
			FromDestination: dest,
			NACKs:           []uint32{10, 20, 30},
		}

		err = SignPacket(pkt, keyPair)
		assert.NoError(t, err)
		assert.NotNil(t, pkt.Signature)
		assert.Equal(t, 64, len(pkt.Signature))
	})
}

// TestFindSignatureOffset tests the findSignatureOffset helper function.
func TestFindSignatureOffset(t *testing.T) {
	crypto := go_i2cp.NewCrypto()

	t.Run("minimal packet (no optional fields)", func(t *testing.T) {
		dest, err := go_i2cp.NewDestination(crypto)
		require.NoError(t, err)

		pkt := &Packet{
			SendStreamID:    1,
			RecvStreamID:    2,
			Flags:           FlagSYN | FlagFromIncluded | FlagSignatureIncluded,
			FromDestination: dest,
		}

		offset := findSignatureOffset(pkt)
		// Header (22) + FROM destination (387+) = 409+
		assert.GreaterOrEqual(t, offset, 409, "offset should be at least header + destination")
	})

	t.Run("packet with NACKs", func(t *testing.T) {
		dest, err := go_i2cp.NewDestination(crypto)
		require.NoError(t, err)

		pkt := &Packet{
			SendStreamID:    1,
			RecvStreamID:    2,
			Flags:           FlagSYN | FlagFromIncluded | FlagSignatureIncluded,
			FromDestination: dest,
			NACKs:           []uint32{10, 20, 30}, // 3 * 4 = 12 bytes
		}

		offset := findSignatureOffset(pkt)
		// Header (22) + NACKs (12) + FROM destination (387+) = 421+
		assert.GreaterOrEqual(t, offset, 421, "offset should include NACK bytes")
	})

	t.Run("packet with all optional fields", func(t *testing.T) {
		dest, err := go_i2cp.NewDestination(crypto)
		require.NoError(t, err)

		pkt := &Packet{
			SendStreamID:    1,
			RecvStreamID:    2,
			Flags:           FlagSYN | FlagDelayRequested | FlagMaxPacketSizeIncluded | FlagFromIncluded | FlagSignatureIncluded,
			OptionalDelay:   1000,
			MaxPacketSize:   1500,
			FromDestination: dest,
			NACKs:           []uint32{10, 20},
		}

		offset := findSignatureOffset(pkt)
		// Header (22) + NACKs (8) + Delay (2) + MaxPacketSize (2) + FROM (387+) = 421+
		assert.GreaterOrEqual(t, offset, 421, "offset should include all optional fields")
	})
}

// TestSignAndMarshalCycle tests that signing doesn't break marshalling.
func TestSignAndMarshalCycle(t *testing.T) {
	crypto := go_i2cp.NewCrypto()

	dest, err := go_i2cp.NewDestination(crypto)
	require.NoError(t, err)

	keyPair, err := crypto.Ed25519SignatureKeygen()
	require.NoError(t, err)

	pkt := &Packet{
		SendStreamID:    1,
		RecvStreamID:    2,
		SequenceNum:     100,
		AckThrough:      99,
		Flags:           FlagSYN | FlagFromIncluded | FlagSignatureIncluded,
		FromDestination: dest,
		Payload:         []byte("test"),
	}

	// Sign the packet
	err = SignPacket(pkt, keyPair)
	require.NoError(t, err)

	// Marshal the signed packet
	data, err := pkt.Marshal()
	require.NoError(t, err)
	assert.NotNil(t, data)

	// Verify the marshaled data contains the signature
	// Signature should be at the end of the options section, before payload
	assert.Greater(t, len(data), 22+387+64, "data should include header + destination + signature")
}

// TestVerifyPacketSignaturePrerequisites tests error conditions for VerifyPacketSignature.
func TestVerifyPacketSignaturePrerequisites(t *testing.T) {
	crypto := go_i2cp.NewCrypto()

	t.Run("error when FromDestination is nil", func(t *testing.T) {
		pkt := &Packet{
			Flags:     FlagSignatureIncluded,
			Signature: make([]byte, 64),
		}

		err := VerifyPacketSignature(pkt, crypto)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no FROM destination")
	})

	t.Run("error when Signature is empty", func(t *testing.T) {
		dest, err := go_i2cp.NewDestination(crypto)
		require.NoError(t, err)

		pkt := &Packet{
			Flags:           FlagFromIncluded | FlagSignatureIncluded,
			FromDestination: dest,
			Signature:       nil,
		}

		err = VerifyPacketSignature(pkt, crypto)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no signature present")
	})

	t.Run("error when FlagSignatureIncluded not set", func(t *testing.T) {
		dest, err := go_i2cp.NewDestination(crypto)
		require.NoError(t, err)

		pkt := &Packet{
			Flags:           FlagFromIncluded, // No FlagSignatureIncluded
			FromDestination: dest,
			Signature:       make([]byte, 64),
		}

		err = VerifyPacketSignature(pkt, crypto)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "FlagSignatureIncluded not set")
	})

	t.Run("verification not fully implemented", func(t *testing.T) {
		dest, err := go_i2cp.NewDestination(crypto)
		require.NoError(t, err)

		pkt := &Packet{
			SendStreamID:    1,
			RecvStreamID:    2,
			Flags:           FlagSYN | FlagFromIncluded | FlagSignatureIncluded,
			FromDestination: dest,
			Signature:       make([]byte, 64),
		}

		err = VerifyPacketSignature(pkt, crypto)
		assert.Error(t, err)
		// Current implementation returns "not yet fully implemented" error
		assert.Contains(t, err.Error(), "not yet fully implemented")
	})
}
