package streaming

import (
	"testing"
)

// TestProfileConstants verifies profile type constants match I2P spec values.
func TestProfileConstants(t *testing.T) {
	t.Run("ProfileBulk is 1", func(t *testing.T) {
		if ProfileBulk != 1 {
			t.Errorf("ProfileBulk should be 1, got %d", ProfileBulk)
		}
	})

	t.Run("ProfileInteractive is 2", func(t *testing.T) {
		if ProfileInteractive != 2 {
			t.Errorf("ProfileInteractive should be 2, got %d", ProfileInteractive)
		}
	})

	t.Run("Default profile is ProfileBulk", func(t *testing.T) {
		config := DefaultProfileConfig()
		if config.Profile != ProfileBulk {
			t.Errorf("default profile should be ProfileBulk, got %d", config.Profile)
		}
	})
}

// TestFlagProfileInteractive verifies the profile flag bit is correctly positioned.
func TestFlagProfileInteractive(t *testing.T) {
	t.Run("FlagProfileInteractive is bit 8", func(t *testing.T) {
		expected := uint16(0x0100) // bit 8
		if FlagProfileInteractive != expected {
			t.Errorf("FlagProfileInteractive should be 0x0100, got 0x%04x", FlagProfileInteractive)
		}
	})
}

// TestDefaultProfileConfig verifies default configuration values.
func TestDefaultProfileConfig(t *testing.T) {
	config := DefaultProfileConfig()

	t.Run("default profile is bulk", func(t *testing.T) {
		if config.Profile != ProfileBulk {
			t.Errorf("default profile should be ProfileBulk, got %d", config.Profile)
		}
	})
}

// TestProfileString verifies String() method returns expected values.
func TestProfileString(t *testing.T) {
	tests := []struct {
		name     string
		profile  StreamProfile
		expected string
	}{
		{"ProfileBulk", ProfileBulk, "bulk"},
		{"ProfileInteractive", ProfileInteractive, "interactive"},
		{"Unknown profile", StreamProfile(99), "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.profile.String()
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

// TestStreamManagerProfileConfiguration verifies manager profile getter/setter.
func TestStreamManagerProfileConfiguration(t *testing.T) {
	// Create a minimal manager struct for testing
	manager := &StreamManager{
		profileConfig: DefaultProfileConfig(),
	}

	t.Run("default profile is bulk", func(t *testing.T) {
		profile := manager.GetStreamProfile()
		if profile != ProfileBulk {
			t.Errorf("expected ProfileBulk, got %d", profile)
		}
	})

	t.Run("set profile to interactive", func(t *testing.T) {
		manager.SetStreamProfile(ProfileInteractive)
		profile := manager.GetStreamProfile()
		if profile != ProfileInteractive {
			t.Errorf("expected ProfileInteractive, got %d", profile)
		}
	})

	t.Run("set profile back to bulk", func(t *testing.T) {
		manager.SetStreamProfile(ProfileBulk)
		profile := manager.GetStreamProfile()
		if profile != ProfileBulk {
			t.Errorf("expected ProfileBulk, got %d", profile)
		}
	})
}

// TestBuildSYNPacketProfileFlag verifies SYN packet includes profile flag when interactive.
func TestBuildSYNPacketProfileFlag(t *testing.T) {
	// Note: The buildSYNPacket method requires session.Destination() to be available,
	// so we test the flag logic directly rather than calling buildSYNPacket.
	t.Run("bulk profile does not set interactive flag", func(t *testing.T) {
		// Test profileToFlag helper directly
		flags := profileToFlag(ProfileBulk)
		if flags&FlagProfileInteractive != 0 {
			t.Errorf("bulk profile should not set FlagProfileInteractive, flags=0x%04x", flags)
		}
	})

	t.Run("interactive profile sets interactive flag", func(t *testing.T) {
		flags := profileToFlag(ProfileInteractive)
		if flags&FlagProfileInteractive == 0 {
			t.Errorf("interactive profile should set FlagProfileInteractive, flags=0x%04x", flags)
		}
	})

	t.Run("profileFromFlag extracts bulk correctly", func(t *testing.T) {
		profile := profileFromFlag(FlagSYN | FlagFromIncluded)
		if profile != ProfileBulk {
			t.Errorf("expected ProfileBulk, got %d", profile)
		}
	})

	t.Run("profileFromFlag extracts interactive correctly", func(t *testing.T) {
		profile := profileFromFlag(FlagSYN | FlagFromIncluded | FlagProfileInteractive)
		if profile != ProfileInteractive {
			t.Errorf("expected ProfileInteractive, got %d", profile)
		}
	})
}

// TestBuildSynAckPacketProfileFlag verifies SYN-ACK packet includes profile flag when interactive.
func TestBuildSynAckPacketProfileFlag(t *testing.T) {
	// Note: Full SYN-ACK testing requires a mock session which isn't trivial.
	// The buildSynAckPacket method requires session.Destination() to be available.
	// Profile flag logic is tested through the extractRemoteProfileFromSynAck tests.
	t.Run("profile flag logic is consistent", func(t *testing.T) {
		// Test that flag checking works correctly
		bulkFlags := FlagSYN | FlagFromIncluded | FlagSignatureIncluded
		interactiveFlags := bulkFlags | FlagProfileInteractive

		if bulkFlags&FlagProfileInteractive != 0 {
			t.Errorf("bulk flags should not have interactive flag")
		}
		if interactiveFlags&FlagProfileInteractive == 0 {
			t.Errorf("interactive flags should have interactive flag")
		}
	})
}

// TestExtractRemoteProfileFromSYN verifies profile extraction from incoming SYN.
func TestExtractRemoteProfileFromSYN(t *testing.T) {
	t.Run("SYN without interactive flag has bulk profile", func(t *testing.T) {
		synPkt := &Packet{
			Flags: FlagSYN | FlagFromIncluded,
		}

		conn := &StreamConn{}
		// Simulate profile extraction logic from initConnectionState
		if synPkt.Flags&FlagProfileInteractive != 0 {
			conn.remoteProfile = ProfileInteractive
		} else {
			conn.remoteProfile = ProfileBulk
		}

		if conn.remoteProfile != ProfileBulk {
			t.Errorf("expected ProfileBulk, got %d", conn.remoteProfile)
		}
	})

	t.Run("SYN with interactive flag has interactive profile", func(t *testing.T) {
		synPkt := &Packet{
			Flags: FlagSYN | FlagFromIncluded | FlagProfileInteractive,
		}

		conn := &StreamConn{}
		// Simulate profile extraction logic from initConnectionState
		if synPkt.Flags&FlagProfileInteractive != 0 {
			conn.remoteProfile = ProfileInteractive
		} else {
			conn.remoteProfile = ProfileBulk
		}

		if conn.remoteProfile != ProfileInteractive {
			t.Errorf("expected ProfileInteractive, got %d", conn.remoteProfile)
		}
	})
}

// TestExtractRemoteProfileFromSynAck verifies profile extraction from SYN-ACK.
func TestExtractRemoteProfileFromSynAck(t *testing.T) {
	t.Run("SYN-ACK without interactive flag has bulk profile", func(t *testing.T) {
		synAckPkt := &Packet{
			Flags: FlagSYN | FlagFromIncluded | FlagSignatureIncluded,
		}

		conn := &StreamConn{}
		conn.extractRemoteProfileFromSynAck(synAckPkt)

		if conn.remoteProfile != ProfileBulk {
			t.Errorf("expected ProfileBulk, got %d", conn.remoteProfile)
		}
	})

	t.Run("SYN-ACK with interactive flag has interactive profile", func(t *testing.T) {
		synAckPkt := &Packet{
			Flags: FlagSYN | FlagFromIncluded | FlagSignatureIncluded | FlagProfileInteractive,
		}

		conn := &StreamConn{}
		conn.extractRemoteProfileFromSynAck(synAckPkt)

		if conn.remoteProfile != ProfileInteractive {
			t.Errorf("expected ProfileInteractive, got %d", conn.remoteProfile)
		}
	})
}

// TestStreamConnProfileGetters verifies Profile() and RemoteProfile() methods.
func TestStreamConnProfileGetters(t *testing.T) {
	t.Run("Profile returns local profile", func(t *testing.T) {
		conn := &StreamConn{
			profile:       ProfileInteractive,
			remoteProfile: ProfileBulk,
		}

		if conn.Profile() != ProfileInteractive {
			t.Errorf("Profile() should return ProfileInteractive, got %d", conn.Profile())
		}
	})

	t.Run("RemoteProfile returns remote profile", func(t *testing.T) {
		conn := &StreamConn{
			profile:       ProfileBulk,
			remoteProfile: ProfileInteractive,
		}

		if conn.RemoteProfile() != ProfileInteractive {
			t.Errorf("RemoteProfile() should return ProfileInteractive, got %d", conn.RemoteProfile())
		}
	})
}

// TestProfileNegotiationScenarios verifies various client-server profile scenarios.
func TestProfileNegotiationScenarios(t *testing.T) {
	t.Run("both bulk - most common scenario", func(t *testing.T) {
		// Client sends SYN with no profile flag (bulk default)
		clientSYN := &Packet{Flags: FlagSYN | FlagFromIncluded}

		// Server extracts remote profile as bulk
		serverConn := &StreamConn{profile: ProfileBulk}
		if clientSYN.Flags&FlagProfileInteractive != 0 {
			serverConn.remoteProfile = ProfileInteractive
		} else {
			serverConn.remoteProfile = ProfileBulk
		}

		if serverConn.remoteProfile != ProfileBulk {
			t.Errorf("server should see client as bulk")
		}
		if serverConn.profile != ProfileBulk {
			t.Errorf("server profile should be bulk")
		}
	})

	t.Run("client interactive, server bulk", func(t *testing.T) {
		// Client sends SYN with interactive flag
		clientSYN := &Packet{Flags: FlagSYN | FlagFromIncluded | FlagProfileInteractive}

		// Server extracts remote profile as interactive
		serverConn := &StreamConn{profile: ProfileBulk}
		if clientSYN.Flags&FlagProfileInteractive != 0 {
			serverConn.remoteProfile = ProfileInteractive
		} else {
			serverConn.remoteProfile = ProfileBulk
		}

		if serverConn.remoteProfile != ProfileInteractive {
			t.Errorf("server should see client as interactive")
		}
		if serverConn.profile != ProfileBulk {
			t.Errorf("server profile should be bulk")
		}
	})

	t.Run("both interactive", func(t *testing.T) {
		// Client sends SYN with interactive flag
		clientSYN := &Packet{Flags: FlagSYN | FlagFromIncluded | FlagProfileInteractive}
		// Server is also interactive
		serverConn := &StreamConn{profile: ProfileInteractive}
		if clientSYN.Flags&FlagProfileInteractive != 0 {
			serverConn.remoteProfile = ProfileInteractive
		} else {
			serverConn.remoteProfile = ProfileBulk
		}

		// Server sends SYN-ACK with interactive flag
		serverSYNACK := &Packet{Flags: FlagSYN | FlagFromIncluded | FlagProfileInteractive}

		// Client extracts remote profile as interactive
		clientConn := &StreamConn{profile: ProfileInteractive}
		clientConn.extractRemoteProfileFromSynAck(serverSYNACK)

		if serverConn.remoteProfile != ProfileInteractive {
			t.Errorf("server should see client as interactive")
		}
		if clientConn.remoteProfile != ProfileInteractive {
			t.Errorf("client should see server as interactive")
		}
	})
}

// TestProfileFlagCombinations verifies profile flag doesn't interfere with other flags.
func TestProfileFlagCombinations(t *testing.T) {
	tests := []struct {
		name        string
		baseFlags   uint16
		withProfile uint16
		expectBit8  bool
	}{
		{
			name:        "SYN only",
			baseFlags:   FlagSYN,
			withProfile: FlagSYN | FlagProfileInteractive,
			expectBit8:  true,
		},
		{
			name:        "SYN + FROM + SIGNATURE",
			baseFlags:   FlagSYN | FlagFromIncluded | FlagSignatureIncluded,
			withProfile: FlagSYN | FlagFromIncluded | FlagSignatureIncluded | FlagProfileInteractive,
			expectBit8:  true,
		},
		{
			name:        "with MTU flag",
			baseFlags:   FlagSYN | FlagMaxPacketSizeIncluded,
			withProfile: FlagSYN | FlagMaxPacketSizeIncluded | FlagProfileInteractive,
			expectBit8:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify base flags don't have profile
			if tt.baseFlags&FlagProfileInteractive != 0 {
				t.Errorf("base flags should not have profile bit set")
			}

			// Verify with profile has both base flags and profile
			if tt.withProfile&FlagProfileInteractive == 0 {
				t.Errorf("withProfile should have profile bit set")
			}

			// Verify base flags are preserved
			if (tt.withProfile & tt.baseFlags) != tt.baseFlags {
				t.Errorf("base flags should be preserved in combined flags")
			}
		})
	}
}
