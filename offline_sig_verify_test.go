package streaming

import (
	"crypto/rand"
	"encoding/binary"
	"testing"
	"time"

	go_i2cp "github.com/go-i2p/go-i2cp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createSignedOfflineSig creates a properly signed offline signature for testing.
// It signs the data: Expires || TransientSigType || TransientPublicKey with the destination's signing key.
func createSignedOfflineSig(t *testing.T, dest *go_i2cp.Destination, expires uint32, transientType uint16, transientKeyLen int) *OfflineSig {
	t.Helper()

	// Generate transient public key
	transientKey := make([]byte, transientKeyLen)
	_, err := rand.Read(transientKey)
	require.NoError(t, err)

	// Build the data to sign: Expires (4 bytes) + TransientSigType (2 bytes) + TransientPublicKey
	toSign := make([]byte, 0, 6+len(transientKey))
	var buf4 [4]byte
	binary.BigEndian.PutUint32(buf4[:], expires)
	toSign = append(toSign, buf4[:]...)
	var buf2 [2]byte
	binary.BigEndian.PutUint16(buf2[:], transientType)
	toSign = append(toSign, buf2[:]...)
	toSign = append(toSign, transientKey...)

	// Sign with the destination's signing key
	signingKeyPair, err := dest.SigningKeyPair()
	require.NoError(t, err)

	signature, err := signingKeyPair.Sign(toSign)
	require.NoError(t, err)

	return &OfflineSig{
		Expires:            expires,
		TransientSigType:   transientType,
		TransientPublicKey: transientKey,
		DestSignature:      signature,
	}
}

// TestVerifyOfflineSignatureNilInputs tests error handling for nil inputs
func TestVerifyOfflineSignatureNilInputs(t *testing.T) {
	crypto := go_i2cp.NewCrypto()
	dest, err := go_i2cp.NewDestination(crypto)
	require.NoError(t, err)

	offsig := &OfflineSig{
		Expires:            uint32(time.Now().Unix() + 3600),
		TransientSigType:   SignatureTypeEd25519,
		TransientPublicKey: make([]byte, 32),
		DestSignature:      make([]byte, 64),
	}

	t.Run("nil offline signature", func(t *testing.T) {
		err := VerifyOfflineSignature(nil, dest, crypto)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "offline signature is nil")
	})

	t.Run("nil destination", func(t *testing.T) {
		err := VerifyOfflineSignature(offsig, nil, crypto)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "destination is nil")
	})

	t.Run("nil crypto", func(t *testing.T) {
		err := VerifyOfflineSignature(offsig, dest, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "crypto is nil")
	})
}

// TestVerifyOfflineSignatureExpired tests expiration checking
func TestVerifyOfflineSignatureExpired(t *testing.T) {
	crypto := go_i2cp.NewCrypto()
	dest, err := go_i2cp.NewDestination(crypto)
	require.NoError(t, err)

	// Create offline signature that expired 1 hour ago
	offsig := &OfflineSig{
		Expires:            uint32(time.Now().Unix() - 3600),
		TransientSigType:   SignatureTypeEd25519,
		TransientPublicKey: make([]byte, 32),
		DestSignature:      make([]byte, 64),
	}
	_, err = rand.Read(offsig.TransientPublicKey)
	require.NoError(t, err)
	_, err = rand.Read(offsig.DestSignature)
	require.NoError(t, err)

	err = VerifyOfflineSignature(offsig, dest, crypto)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "offline signature expired")
}

// TestVerifyOfflineSignatureNotExpired tests that non-expired signatures pass expiration check
func TestVerifyOfflineSignatureNotExpired(t *testing.T) {
	crypto := go_i2cp.NewCrypto()
	dest, err := go_i2cp.NewDestination(crypto)
	require.NoError(t, err)

	// Create a properly signed offline signature that expires in 1 hour
	expires := uint32(time.Now().Unix() + 3600)
	offsig := createSignedOfflineSig(t, dest, expires, SignatureTypeEd25519, 32)

	// Should pass verification
	err = VerifyOfflineSignature(offsig, dest, crypto)
	assert.NoError(t, err, "valid signature should verify successfully")
}

// TestVerifyOfflineSignatureDataFormat tests the signed data format construction
func TestVerifyOfflineSignatureDataFormat(t *testing.T) {
	crypto := go_i2cp.NewCrypto()
	dest, err := go_i2cp.NewDestination(crypto)
	require.NoError(t, err)

	expires := uint32(time.Now().Unix() + 3600)
	transientType := uint16(SignatureTypeEd25519)

	// Create a properly signed offline signature
	offsig := createSignedOfflineSig(t, dest, expires, transientType, 32)

	// Verify the function works with correctly formatted and signed data
	err = VerifyOfflineSignature(offsig, dest, crypto)
	assert.NoError(t, err, "properly signed offline signature should verify successfully")
}

// TestVerifyOfflineSignatureInvalidDestSignatureLength tests invalid signature length
func TestVerifyOfflineSignatureInvalidDestSignatureLength(t *testing.T) {
	crypto := go_i2cp.NewCrypto()
	dest, err := go_i2cp.NewDestination(crypto)
	require.NoError(t, err)

	offsig := &OfflineSig{
		Expires:            uint32(time.Now().Unix() + 3600),
		TransientSigType:   SignatureTypeEd25519,
		TransientPublicKey: make([]byte, 32),
		DestSignature:      make([]byte, 32), // Wrong length (should be 64 for Ed25519)
	}

	err = VerifyOfflineSignature(offsig, dest, crypto)
	assert.Error(t, err)
	// Should error on invalid signature length
}

// TestVerifyOfflineSignatureMultipleTransientTypes tests different transient key types
func TestVerifyOfflineSignatureMultipleTransientTypes(t *testing.T) {
	crypto := go_i2cp.NewCrypto()

	tests := []struct {
		name            string
		transientType   uint16
		transientKeyLen int
	}{
		{"Ed25519 transient", SignatureTypeEd25519, 32},
		{"ECDSA P-256 transient", SignatureTypeECDSA_SHA256_P256, 64},
		{"ECDSA P-384 transient", SignatureTypeECDSA_SHA384_P384, 96},
		{"RSA 2048 transient", SignatureTypeRSA_SHA256_2048, 256},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest, err := go_i2cp.NewDestination(crypto)
			require.NoError(t, err)

			// Create a properly signed offline signature with the given transient key type
			expires := uint32(time.Now().Unix() + 3600)
			offsig := createSignedOfflineSig(t, dest, expires, tt.transientType, tt.transientKeyLen)

			// Verify the signature
			err = VerifyOfflineSignature(offsig, dest, crypto)
			assert.NoError(t, err, "properly signed signature should verify for %s", tt.name)
		})
	}
}

// TestVerifyOfflineSignatureEmptyTransientKey tests error handling for empty transient key
func TestVerifyOfflineSignatureEmptyTransientKey(t *testing.T) {
	crypto := go_i2cp.NewCrypto()
	dest, err := go_i2cp.NewDestination(crypto)
	require.NoError(t, err)

	offsig := &OfflineSig{
		Expires:            uint32(time.Now().Unix() + 3600),
		TransientSigType:   SignatureTypeEd25519,
		TransientPublicKey: []byte{}, // Empty key
		DestSignature:      make([]byte, 64),
	}

	err = VerifyOfflineSignature(offsig, dest, crypto)
	assert.Error(t, err)
}

// TestVerifyOfflineSignatureEmptyDestSignature tests error handling for empty dest signature
func TestVerifyOfflineSignatureEmptyDestSignature(t *testing.T) {
	crypto := go_i2cp.NewCrypto()
	dest, err := go_i2cp.NewDestination(crypto)
	require.NoError(t, err)

	offsig := &OfflineSig{
		Expires:            uint32(time.Now().Unix() + 3600),
		TransientSigType:   SignatureTypeEd25519,
		TransientPublicKey: make([]byte, 32),
		DestSignature:      []byte{}, // Empty signature
	}
	_, err = rand.Read(offsig.TransientPublicKey)
	require.NoError(t, err)

	err = VerifyOfflineSignature(offsig, dest, crypto)
	assert.Error(t, err)
}

// TestVerifyOfflineSignatureBoundaryTimestamp tests boundary conditions for expiration
func TestVerifyOfflineSignatureBoundaryTimestamp(t *testing.T) {
	crypto := go_i2cp.NewCrypto()
	dest, err := go_i2cp.NewDestination(crypto)
	require.NoError(t, err)

	t.Run("expires exactly now", func(t *testing.T) {
		// Create a properly signed offline signature
		expires := uint32(time.Now().Unix())
		offsig := createSignedOfflineSig(t, dest, expires, SignatureTypeEd25519, 32)

		// This might pass or fail depending on exact timing, but shouldn't panic
		_ = VerifyOfflineSignature(offsig, dest, crypto)
	})

	t.Run("expires 1 second in future", func(t *testing.T) {
		// Create a properly signed offline signature
		expires := uint32(time.Now().Unix() + 1)
		offsig := createSignedOfflineSig(t, dest, expires, SignatureTypeEd25519, 32)

		err = VerifyOfflineSignature(offsig, dest, crypto)
		// Should pass since not expired
		assert.NoError(t, err)
	})

	t.Run("expired 1 second ago", func(t *testing.T) {
		// Create a properly signed offline signature (but expired)
		expires := uint32(time.Now().Unix() - 1)
		offsig := createSignedOfflineSig(t, dest, expires, SignatureTypeEd25519, 32)

		err = VerifyOfflineSignature(offsig, dest, crypto)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expired")
	})
}

// TestVerifyOfflineSignatureMaxTimestamp tests maximum timestamp value
func TestVerifyOfflineSignatureMaxTimestamp(t *testing.T) {
	crypto := go_i2cp.NewCrypto()
	dest, err := go_i2cp.NewDestination(crypto)
	require.NoError(t, err)

	// Use max uint32 value (year 2106)
	expires := ^uint32(0) // Max uint32
	offsig := createSignedOfflineSig(t, dest, expires, SignatureTypeEd25519, 32)

	err = VerifyOfflineSignature(offsig, dest, crypto)
	// Should pass since year 2106 is far future
	assert.NoError(t, err)
}

// TestVerifyOfflineSignatureInvalidSignature tests that invalid cryptographic signatures are rejected
func TestVerifyOfflineSignatureInvalidSignature(t *testing.T) {
	crypto := go_i2cp.NewCrypto()
	dest, err := go_i2cp.NewDestination(crypto)
	require.NoError(t, err)

	t.Run("random signature", func(t *testing.T) {
		// Create an offline signature with a random (invalid) signature
		transientKey := make([]byte, 32)
		_, err = rand.Read(transientKey)
		require.NoError(t, err)

		randomSig := make([]byte, 64)
		_, err = rand.Read(randomSig)
		require.NoError(t, err)

		offsig := &OfflineSig{
			Expires:            uint32(time.Now().Unix() + 3600),
			TransientSigType:   SignatureTypeEd25519,
			TransientPublicKey: transientKey,
			DestSignature:      randomSig,
		}

		err = VerifyOfflineSignature(offsig, dest, crypto)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature does not match")
	})

	t.Run("tampered signature", func(t *testing.T) {
		// Create a valid signature then tamper with it
		expires := uint32(time.Now().Unix() + 3600)
		offsig := createSignedOfflineSig(t, dest, expires, SignatureTypeEd25519, 32)

		// Tamper with the signature
		offsig.DestSignature[0] ^= 0xFF

		err = VerifyOfflineSignature(offsig, dest, crypto)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature does not match")
	})

	t.Run("tampered data", func(t *testing.T) {
		// Create a valid signature then tamper with the data
		expires := uint32(time.Now().Unix() + 3600)
		offsig := createSignedOfflineSig(t, dest, expires, SignatureTypeEd25519, 32)

		// Tamper with the expires field
		offsig.Expires += 1

		err = VerifyOfflineSignature(offsig, dest, crypto)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature does not match")
	})

	t.Run("wrong destination", func(t *testing.T) {
		// Sign with one destination, verify with another
		dest2, err := go_i2cp.NewDestination(crypto)
		require.NoError(t, err)

		expires := uint32(time.Now().Unix() + 3600)
		offsig := createSignedOfflineSig(t, dest, expires, SignatureTypeEd25519, 32)

		// Verify with wrong destination
		err = VerifyOfflineSignature(offsig, dest2, crypto)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signature does not match")
	})
}
