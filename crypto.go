package streaming

import (
	"fmt"

	go_i2cp "github.com/go-i2p/go-i2cp"
)

// SignPacket signs a packet with the given Ed25519 signing key pair.
// The signature covers the entire marshaled packet with the signature field zeroed.
//
// Requirements:
//   - packet.FromDestination must be set (required for signature length calculation)
//   - packet.Flags must have FlagSignatureIncluded set
//   - keyPair must match the destination's public key
//
// Process:
//  1. Marshal the packet (signature field will be zeros or reserved space)
//  2. Zero out the signature bytes in the marshaled data
//  3. Sign the modified data
//  4. Update packet.Signature with the signature
//
// Returns error if marshalling fails or signing fails.
func SignPacket(pkt *Packet, keyPair *go_i2cp.Ed25519KeyPair) error {
	if pkt.FromDestination == nil {
		return fmt.Errorf("cannot sign packet: FromDestination is nil")
	}
	if pkt.Flags&FlagSignatureIncluded == 0 {
		return fmt.Errorf("cannot sign packet: FlagSignatureIncluded not set")
	}

	// Marshal packet - signature will be zeros or pre-allocated space
	data, err := pkt.Marshal()
	if err != nil {
		return fmt.Errorf("marshal for signing: %w", err)
	}

	// Find signature location in marshaled data and ensure it's zeroed
	sigOffset := findSignatureOffset(pkt)
	sigLen := getSignatureLength(pkt.FromDestination)
	if sigOffset+sigLen > len(data) {
		return fmt.Errorf("signature offset+length (%d) exceeds data length (%d)", sigOffset+sigLen, len(data))
	}

	// Zero the signature field in the data we'll sign
	for i := 0; i < sigLen; i++ {
		data[sigOffset+i] = 0
	}

	// Sign the data
	signature, err := keyPair.Sign(data)
	if err != nil {
		return fmt.Errorf("sign packet: %w", err)
	}

	if len(signature) != sigLen {
		return fmt.Errorf("signature length mismatch: expected %d, got %d", sigLen, len(signature))
	}

	// Update packet with signature
	pkt.Signature = signature
	return nil
}

// VerifyPacketSignature verifies a packet's signature using the public key
// from the packet's FromDestination field.
//
// Requirements:
//   - packet.FromDestination must be set
//   - packet.Signature must be set
//   - packet.Flags must have FlagSignatureIncluded set
//
// Process:
//  1. Extract signing public key from FromDestination
//  2. Marshal packet with signature field zeroed
//  3. Verify signature against the marshaled data
//
// Returns error if verification fails or prerequisites are not met.
func VerifyPacketSignature(pkt *Packet, crypto *go_i2cp.Crypto) error {
	if pkt.FromDestination == nil {
		return fmt.Errorf("cannot verify: no FROM destination")
	}
	if len(pkt.Signature) == 0 {
		return fmt.Errorf("cannot verify: no signature present")
	}
	if pkt.Flags&FlagSignatureIncluded == 0 {
		return fmt.Errorf("cannot verify: FlagSignatureIncluded not set")
	}

	// Get the signing public key from the destination
	// We need to reconstruct the key pair from the destination data
	// Since go-i2cp doesn't expose the public key directly from Destination,
	// we need to use the destination's encoded data

	// Save original signature
	originalSig := pkt.Signature

	// Temporarily zero the signature for verification
	pkt.Signature = make([]byte, len(originalSig))
	_, err := pkt.Marshal()

	// Restore original signature
	pkt.Signature = originalSig

	if err != nil {
		return fmt.Errorf("marshal for verification: %w", err)
	}

	// Extract the signing key from the destination
	// The destination contains the public key, we need to create a key pair
	// to access the Verify method

	// TODO: Implement public key extraction from destination
	// The I2P destination format is:
	// - public_key (256 bytes for encryption, or variable for newer types)
	// - signing_public_key (variable length based on signature type)
	// - certificate (variable length)
	//
	// For Ed25519, the signing public key is 32 bytes
	// We need to parse the destination to extract this key
	//
	// For now, verification requires the caller to pass the signing keypair
	// This is a known limitation that will be addressed in a future task

	return fmt.Errorf("signature verification not yet fully implemented - needs public key extraction from destination")
}

// findSignatureOffset calculates the byte offset where the signature field
// begins in a marshaled packet.
//
// The signature is always the last optional field, appearing after:
//   - Fixed header (22 bytes)
//   - NACKs (if present)
//   - ResendDelay (if FlagDelayRequested)
//   - MaxPacketSize (if FlagMaxPacketSizeIncluded)
//   - FROM destination (if FlagFromIncluded)
//   - Signature comes last
//
// This does NOT include the payload, as the signature is in the options section.
func findSignatureOffset(pkt *Packet) int {
	// Start after fixed header (22 bytes)
	offset := 22

	// Add NACK bytes
	offset += len(pkt.NACKs) * 4

	// Add optional delay (2 bytes if FlagDelayRequested set)
	if pkt.Flags&FlagDelayRequested != 0 {
		offset += 2
	}

	// Add MaxPacketSize (2 bytes if FlagMaxPacketSizeIncluded set)
	if pkt.Flags&FlagMaxPacketSizeIncluded != 0 {
		offset += 2
	}

	// Add FROM destination (387+ bytes if FlagFromIncluded set)
	if pkt.Flags&FlagFromIncluded != 0 && pkt.FromDestination != nil {
		// Calculate destination size by marshaling it
		stream := go_i2cp.NewStream(make([]byte, 0, 512))
		err := pkt.FromDestination.WriteToMessage(stream)
		if err == nil {
			offset += len(stream.Bytes())
		} else {
			// Fallback to standard size if marshaling fails
			offset += 387
		}
	}

	// Signature offset is now at the current position
	return offset
}
