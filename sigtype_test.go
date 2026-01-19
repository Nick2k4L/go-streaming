package streaming

import (
	"testing"

	"github.com/go-i2p/common/certificate"
	go_i2cp "github.com/go-i2p/go-i2cp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetSignatureTypeFromDestination tests extracting signature type from destinations.
func TestGetSignatureTypeFromDestination(t *testing.T) {
	crypto := go_i2cp.NewCrypto()

	t.Run("nil destination returns Ed25519", func(t *testing.T) {
		sigType := getSignatureTypeFromDestination(nil)
		assert.Equal(t, SignatureTypeEd25519, sigType,
			"nil destination should default to Ed25519")
	})

	t.Run("go-i2cp destination returns Ed25519", func(t *testing.T) {
		// go-i2cp creates destinations with KEY certificate (type 5) and Ed25519 signing (type 7)
		dest, err := go_i2cp.NewDestination(crypto)
		require.NoError(t, err)

		sigType := getSignatureTypeFromDestination(dest)
		// go-i2cp destinations have KEY certificate with Ed25519 signing type
		assert.Equal(t, SignatureTypeEd25519, sigType,
			"go-i2cp destination should return Ed25519")
	})
}

// TestParseSignatureTypeFromBytes tests parsing signature type from raw destination bytes.
func TestParseSignatureTypeFromBytes(t *testing.T) {
	t.Run("data too short returns false", func(t *testing.T) {
		shortData := make([]byte, 100) // Less than minimum 387 bytes
		sigType, ok := parseSignatureTypeFromBytes(shortData)
		assert.False(t, ok, "short data should return false")
		assert.Equal(t, 0, sigType)
	})

	t.Run("NULL certificate returns DSA_SHA1", func(t *testing.T) {
		// Build a minimal destination with NULL certificate
		// 256 bytes public key + 128 bytes signing key + 3 bytes NULL cert
		data := make([]byte, 387)

		// Set certificate type to NULL (0) at offset 384
		data[384] = certificate.CERT_NULL
		// Length bytes (385-386) are already 0

		sigType, ok := parseSignatureTypeFromBytes(data)
		assert.True(t, ok, "valid NULL cert should parse successfully")
		assert.Equal(t, SignatureTypeDSA_SHA1, sigType,
			"NULL certificate should return DSA_SHA1")
	})

	t.Run("KEY certificate with Ed25519 returns Ed25519", func(t *testing.T) {
		// Build a destination with KEY certificate containing Ed25519 signing type
		// 256 bytes public key + 128 bytes signing key + KEY cert header
		data := make([]byte, 391) // 384 + 7 (min KEY cert)

		// Certificate at offset 384
		data[384] = certificate.CERT_KEY       // Type: KEY (5)
		data[385] = 0                          // Length high byte
		data[386] = 4                          // Length low byte (4 = sig type + crypto type)
		data[387] = 0                          // Signing type high byte
		data[388] = byte(SignatureTypeEd25519) // Signing type low byte (7)
		data[389] = 0                          // Crypto type high byte
		data[390] = 4                          // Crypto type low byte (X25519 = 4)

		sigType, ok := parseSignatureTypeFromBytes(data)
		assert.True(t, ok, "valid KEY cert should parse successfully")
		assert.Equal(t, SignatureTypeEd25519, sigType,
			"KEY certificate with Ed25519 should return Ed25519")
	})

	t.Run("KEY certificate with ECDSA P-256 returns correct type", func(t *testing.T) {
		data := make([]byte, 391)
		data[384] = certificate.CERT_KEY
		data[385] = 0
		data[386] = 4
		data[387] = 0
		data[388] = byte(SignatureTypeECDSA_SHA256_P256) // Type 1
		data[389] = 0
		data[390] = 0

		sigType, ok := parseSignatureTypeFromBytes(data)
		assert.True(t, ok)
		assert.Equal(t, SignatureTypeECDSA_SHA256_P256, sigType)
	})

	t.Run("KEY certificate with RSA 2048 returns correct type", func(t *testing.T) {
		data := make([]byte, 391)
		data[384] = certificate.CERT_KEY
		data[385] = 0
		data[386] = 4
		data[387] = 0
		data[388] = byte(SignatureTypeRSA_SHA256_2048) // Type 4
		data[389] = 0
		data[390] = 0

		sigType, ok := parseSignatureTypeFromBytes(data)
		assert.True(t, ok)
		assert.Equal(t, SignatureTypeRSA_SHA256_2048, sigType)
	})

	t.Run("unknown certificate type falls back to Ed25519", func(t *testing.T) {
		data := make([]byte, 387)
		data[384] = 99 // Unknown certificate type
		data[385] = 0
		data[386] = 0

		sigType, ok := parseSignatureTypeFromBytes(data)
		assert.True(t, ok, "unknown cert type should still return ok")
		assert.Equal(t, SignatureTypeEd25519, sigType,
			"unknown certificate type should fall back to Ed25519")
	})
}

// TestGetSignatureLengthFromDestination tests that signature lengths are correctly determined.
func TestGetSignatureLengthFromDestination(t *testing.T) {
	t.Run("nil destination returns 0", func(t *testing.T) {
		length := getSignatureLength(nil)
		assert.Equal(t, 0, length)
	})

	t.Run("valid destination returns correct length", func(t *testing.T) {
		crypto := go_i2cp.NewCrypto()
		dest, err := go_i2cp.NewDestination(crypto)
		require.NoError(t, err)

		length := getSignatureLength(dest)
		// go-i2cp destinations have KEY cert with Ed25519 -> 64 bytes
		assert.Equal(t, 64, length,
			"Ed25519 signature should be 64 bytes")
	})
}

// TestSerializeDestination tests destination serialization.
func TestSerializeDestination(t *testing.T) {
	crypto := go_i2cp.NewCrypto()

	t.Run("serializes destination correctly", func(t *testing.T) {
		dest, err := go_i2cp.NewDestination(crypto)
		require.NoError(t, err)

		bytes := serializeDestination(dest)
		assert.NotNil(t, bytes)
		// Minimum destination size is 387 bytes (256 + 128 + 3)
		assert.GreaterOrEqual(t, len(bytes), 387,
			"serialized destination should be at least 387 bytes")
	})

	t.Run("nil destination returns nil", func(t *testing.T) {
		bytes := serializeDestination(nil)
		assert.Nil(t, bytes)
	})
}

// TestSignatureTypeConstants verifies the signature type constants match I2P spec.
func TestSignatureTypeConstants(t *testing.T) {
	// Per I2P specification: https://geti2p.net/spec/common-structures#type
	tests := []struct {
		name     string
		sigType  int
		expected int
	}{
		{"DSA_SHA1", SignatureTypeDSA_SHA1, 0},
		{"ECDSA_SHA256_P256", SignatureTypeECDSA_SHA256_P256, 1},
		{"ECDSA_SHA384_P384", SignatureTypeECDSA_SHA384_P384, 2},
		{"ECDSA_SHA512_P521", SignatureTypeECDSA_SHA512_P521, 3},
		{"RSA_SHA256_2048", SignatureTypeRSA_SHA256_2048, 4},
		{"RSA_SHA384_3072", SignatureTypeRSA_SHA384_3072, 5},
		{"RSA_SHA512_4096", SignatureTypeRSA_SHA512_4096, 6},
		{"Ed25519", SignatureTypeEd25519, 7},
		{"Ed25519ph", SignatureTypeEd25519ph, 8},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.sigType,
				"signature type constant should match I2P spec")
		})
	}
}

// TestGetPublicKeyLengthForSignatureTypes tests public key length lookup for signature types.
func TestGetPublicKeyLengthForSignatureTypes(t *testing.T) {
	tests := []struct {
		sigType     uint16
		expectedLen int
	}{
		{SignatureTypeDSA_SHA1, 128},
		{SignatureTypeECDSA_SHA256_P256, 64},
		{SignatureTypeECDSA_SHA384_P384, 96},
		{SignatureTypeECDSA_SHA512_P521, 132},
		{SignatureTypeRSA_SHA256_2048, 256},
		{SignatureTypeRSA_SHA384_3072, 384},
		{SignatureTypeRSA_SHA512_4096, 512},
		{SignatureTypeEd25519, 32},
		{SignatureTypeEd25519ph, 32},
		{99, 0}, // Unknown type
	}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			length := getPublicKeyLength(tc.sigType)
			assert.Equal(t, tc.expectedLen, length)
		})
	}
}
