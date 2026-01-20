package streaming

import (
	"testing"
	"time"

	go_i2cp "github.com/go-i2p/go-i2cp"
)

// createMockDestination creates a mock destination for testing.
func createMockDestination(t *testing.T) *go_i2cp.Destination {
	t.Helper()
	return createMockDestinationWithSeed(t, 12345)
}

// createMockDestinationWithSeed creates a mock destination with a specific random seed.
// Using different seeds produces destinations with different hashes.
func createMockDestinationWithSeed(t *testing.T, seed int64) *go_i2cp.Destination {
	t.Helper()
	crypto := go_i2cp.NewCrypto()
	dest, err := go_i2cp.NewDestination(crypto)
	if err != nil {
		t.Fatalf("Failed to create mock destination: %v", err)
	}
	// For different hashes, we just create new destinations (random each time)
	// The seed parameter is kept for API compatibility
	_ = seed
	return dest
}

// TestDefaultTCBCacheConfig verifies the default configuration values per I2P spec.
func TestDefaultTCBCacheConfig(t *testing.T) {
	config := DefaultTCBCacheConfig()

	// Verify dampening factors per I2P spec
	if config.RTTDampening != 0.75 {
		t.Errorf("RTTDampening = %v, want 0.75", config.RTTDampening)
	}
	if config.RTTDevDampening != 0.75 {
		t.Errorf("RTTDevDampening = %v, want 0.75", config.RTTDevDampening)
	}
	if config.WindowDampening != 0.75 {
		t.Errorf("WindowDampening = %v, want 0.75", config.WindowDampening)
	}

	// Verify TTL (spec says "a few minutes")
	if config.EntryTTL != 5*time.Minute {
		t.Errorf("EntryTTL = %v, want 5m", config.EntryTTL)
	}

	// Verify enabled by default
	if !config.Enabled {
		t.Error("Enabled = false, want true")
	}
}

// TestTCBCacheGetPut verifies basic cache put and get operations.
func TestTCBCacheGetPut(t *testing.T) {
	cache := newTCBCache(DefaultTCBCacheConfig())

	// Create a mock destination
	dest := createMockDestination(t)

	// Initially cache should be empty
	_, _, _, found := cache.Get(dest)
	if found {
		t.Error("expected cache miss for new destination")
	}

	// Put some data
	cache.Put(dest, 100*time.Millisecond, 20*time.Millisecond, 12)

	// Get should return dampened values
	rtt, rttVar, window, found := cache.Get(dest)
	if !found {
		t.Error("expected cache hit after Put")
	}

	// Values should be dampened by 0.75
	expectedRTT := time.Duration(float64(100*time.Millisecond) * 0.75)
	expectedRTTVar := time.Duration(float64(20*time.Millisecond) * 0.75)
	expectedWindow := uint32(float64(12) * 0.75) // 9

	if rtt != expectedRTT {
		t.Errorf("RTT = %v, want %v", rtt, expectedRTT)
	}
	if rttVar != expectedRTTVar {
		t.Errorf("RTTVar = %v, want %v", rttVar, expectedRTTVar)
	}
	if window != expectedWindow {
		t.Errorf("Window = %d, want %d", window, expectedWindow)
	}
}

// TestTCBCacheDisabled verifies that disabled cache returns no data.
func TestTCBCacheDisabled(t *testing.T) {
	config := DefaultTCBCacheConfig()
	config.Enabled = false
	cache := newTCBCache(config)

	dest := createMockDestination(t)

	// Put should not store anything when disabled
	cache.Put(dest, 100*time.Millisecond, 20*time.Millisecond, 12)

	// Get should return not found when disabled
	_, _, _, found := cache.Get(dest)
	if found {
		t.Error("expected cache miss when disabled")
	}
}

// TestTCBCacheExpiry verifies that entries expire after TTL.
func TestTCBCacheExpiry(t *testing.T) {
	config := DefaultTCBCacheConfig()
	config.EntryTTL = 50 * time.Millisecond // Short TTL for testing
	cache := newTCBCache(config)

	dest := createMockDestination(t)

	// Put data
	cache.Put(dest, 100*time.Millisecond, 20*time.Millisecond, 12)

	// Should be found immediately
	_, _, _, found := cache.Get(dest)
	if !found {
		t.Error("expected cache hit before expiry")
	}

	// Wait for expiry
	time.Sleep(60 * time.Millisecond)

	// Should be expired now
	_, _, _, found = cache.Get(dest)
	if found {
		t.Error("expected cache miss after expiry")
	}
}

// TestTCBCacheUpdate verifies that updating an entry smooths values.
func TestTCBCacheUpdate(t *testing.T) {
	cache := newTCBCache(DefaultTCBCacheConfig())

	dest := createMockDestination(t)

	// Put initial data
	cache.Put(dest, 100*time.Millisecond, 20*time.Millisecond, 10)

	// Update with different values
	cache.Put(dest, 200*time.Millisecond, 40*time.Millisecond, 20)

	// Get should return averaged values (smoothed)
	// With 0.5 weight: (100+200)/2 = 150ms, (20+40)/2 = 30ms, (10+20)/2 = 15
	// Then dampened by 0.75
	rtt, rttVar, window, found := cache.Get(dest)
	if !found {
		t.Error("expected cache hit after update")
	}

	// Values should be between initial and updated, dampened
	// 150ms * 0.75 = 112.5ms
	expectedRTT := time.Duration(float64(150*time.Millisecond) * 0.75)
	if rtt != expectedRTT {
		t.Errorf("RTT = %v, want %v", rtt, expectedRTT)
	}

	// 30ms * 0.75 = 22.5ms
	expectedRTTVar := time.Duration(float64(30*time.Millisecond) * 0.75)
	if rttVar != expectedRTTVar {
		t.Errorf("RTTVar = %v, want %v", rttVar, expectedRTTVar)
	}

	// 15 * 0.75 = 11.25 → 11
	var windowVal float64 = 15.0 * 0.75
	expectedWindow := uint32(windowVal)
	if window != expectedWindow {
		t.Errorf("Window = %d, want %d", window, expectedWindow)
	}
}

// TestTCBCacheClear verifies that Clear removes all entries.
func TestTCBCacheClear(t *testing.T) {
	cache := newTCBCache(DefaultTCBCacheConfig())

	dest := createMockDestination(t)
	cache.Put(dest, 100*time.Millisecond, 20*time.Millisecond, 12)

	if cache.Size() != 1 {
		t.Errorf("Size = %d, want 1", cache.Size())
	}

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("Size after Clear = %d, want 0", cache.Size())
	}

	_, _, _, found := cache.Get(dest)
	if found {
		t.Error("expected cache miss after Clear")
	}
}

// TestTCBCacheCleanupExpired verifies that cleanup removes expired entries.
func TestTCBCacheCleanupExpired(t *testing.T) {
	config := DefaultTCBCacheConfig()
	config.EntryTTL = 50 * time.Millisecond
	cache := newTCBCache(config)

	dest := createMockDestination(t)
	cache.Put(dest, 100*time.Millisecond, 20*time.Millisecond, 12)

	if cache.Size() != 1 {
		t.Errorf("Size = %d, want 1", cache.Size())
	}

	// Wait for expiry
	time.Sleep(60 * time.Millisecond)

	// Cleanup should remove expired entries
	removed := cache.CleanupExpired()
	if removed != 1 {
		t.Errorf("CleanupExpired returned %d, want 1", removed)
	}

	if cache.Size() != 0 {
		t.Errorf("Size after cleanup = %d, want 0", cache.Size())
	}
}

// TestTCBCacheNilDestination verifies handling of nil destination.
func TestTCBCacheNilDestination(t *testing.T) {
	cache := newTCBCache(DefaultTCBCacheConfig())

	// Get with nil destination should return not found
	_, _, _, found := cache.Get(nil)
	if found {
		t.Error("expected cache miss for nil destination")
	}

	// Put with nil destination should not panic
	cache.Put(nil, 100*time.Millisecond, 20*time.Millisecond, 12)

	// Size should still be 0
	if cache.Size() != 0 {
		t.Errorf("Size = %d, want 0 after Put with nil dest", cache.Size())
	}
}

// TestTCBCacheSkipsDefaultValues verifies that default values are not cached.
func TestTCBCacheSkipsDefaultValues(t *testing.T) {
	cache := newTCBCache(DefaultTCBCacheConfig())

	dest := createMockDestination(t)

	// Put with zero RTT values (no useful data learned)
	cache.Put(dest, 0, 0, 10)

	// Should not be cached since RTT is 0
	if cache.Size() != 0 {
		t.Errorf("Size = %d, want 0 for default values", cache.Size())
	}
}

// TestTCBCacheWindowMinimum verifies that window is at least 1 after dampening.
func TestTCBCacheWindowMinimum(t *testing.T) {
	cache := newTCBCache(DefaultTCBCacheConfig())

	dest := createMockDestination(t)

	// Put with window size of 1
	cache.Put(dest, 100*time.Millisecond, 20*time.Millisecond, 1)

	// 1 * 0.75 = 0.75 → should be clamped to 1
	_, _, window, found := cache.Get(dest)
	if !found {
		t.Error("expected cache hit")
	}
	if window < 1 {
		t.Errorf("Window = %d, want at least 1", window)
	}
}

// TestTCBCacheSetConfig verifies that config can be updated.
func TestTCBCacheSetConfig(t *testing.T) {
	cache := newTCBCache(DefaultTCBCacheConfig())

	newConfig := TCBCacheConfig{
		RTTDampening:    0.5,
		RTTDevDampening: 0.5,
		WindowDampening: 0.5,
		EntryTTL:        10 * time.Minute,
		Enabled:         true,
	}
	cache.SetConfig(newConfig)

	config := cache.GetConfig()
	if config.RTTDampening != 0.5 {
		t.Errorf("RTTDampening = %v, want 0.5", config.RTTDampening)
	}
	if config.EntryTTL != 10*time.Minute {
		t.Errorf("EntryTTL = %v, want 10m", config.EntryTTL)
	}
}

// TestCalculateRTOFromValues verifies RTO calculation per RFC 6298.
func TestCalculateRTOFromValues(t *testing.T) {
	tests := []struct {
		name        string
		srtt        time.Duration
		rttVariance time.Duration
		wantMin     time.Duration
		wantMax     time.Duration
	}{
		{
			name:        "normal values",
			srtt:        100 * time.Millisecond,
			rttVariance: 25 * time.Millisecond,
			wantMin:     200 * time.Millisecond, // 100 + 4*25
			wantMax:     200 * time.Millisecond,
		},
		{
			name:        "very low RTT",
			srtt:        10 * time.Millisecond,
			rttVariance: 5 * time.Millisecond,
			wantMin:     MinRTO, // Should be clamped to minimum
			wantMax:     MinRTO,
		},
		{
			name:        "zero values",
			srtt:        0,
			rttVariance: 0,
			wantMin:     MinRTO, // Should use minimum
			wantMax:     MinRTO,
		},
		{
			name:        "very high RTT",
			srtt:        50 * time.Second,
			rttVariance: 10 * time.Second,
			wantMin:     MaxRTO, // Should be clamped to maximum
			wantMax:     MaxRTO,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rto := calculateRTOFromValues(tt.srtt, tt.rttVariance)
			if rto < tt.wantMin || rto > tt.wantMax {
				t.Errorf("calculateRTOFromValues(%v, %v) = %v, want between %v and %v",
					tt.srtt, tt.rttVariance, rto, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestTCBDataApplication verifies that TCB data is correctly applied to connections.
func TestTCBDataApplication(t *testing.T) {
	// Create a minimal connection for testing
	conn := &StreamConn{
		cwnd:        DefaultWindowSize,
		ssthresh:    MaxWindowSize,
		srtt:        8 * time.Second,
		rtt:         8 * time.Second,
		rttVariance: 0,
		rto:         9 * time.Second,
	}

	// Apply cached data
	data := TCBData{
		RTT:         200 * time.Millisecond,
		RTTVariance: 50 * time.Millisecond,
		WindowSize:  20,
		FromCache:   true,
	}
	applyTCBDataToConnection(conn, data)

	// Verify RTT was applied
	if conn.srtt != 200*time.Millisecond {
		t.Errorf("srtt = %v, want 200ms", conn.srtt)
	}
	if conn.rtt != 200*time.Millisecond {
		t.Errorf("rtt = %v, want 200ms", conn.rtt)
	}

	// Verify RTT variance was applied
	if conn.rttVariance != 50*time.Millisecond {
		t.Errorf("rttVariance = %v, want 50ms", conn.rttVariance)
	}

	// Verify window was applied (20 > default 6)
	if conn.cwnd != 20 {
		t.Errorf("cwnd = %d, want 20", conn.cwnd)
	}

	// Verify ssthresh was updated
	if conn.ssthresh != 40 { // 20 * 2
		t.Errorf("ssthresh = %d, want 40", conn.ssthresh)
	}

	// Verify RTO was recalculated
	expectedRTO := calculateRTOFromValues(200*time.Millisecond, 50*time.Millisecond)
	if conn.rto != expectedRTO {
		t.Errorf("rto = %v, want %v", conn.rto, expectedRTO)
	}
}

// TestTCBDataNotAppliedWhenNotFromCache verifies that non-cache data is ignored.
func TestTCBDataNotAppliedWhenNotFromCache(t *testing.T) {
	conn := &StreamConn{
		cwnd:        DefaultWindowSize,
		ssthresh:    MaxWindowSize,
		srtt:        8 * time.Second,
		rtt:         8 * time.Second,
		rttVariance: 0,
		rto:         9 * time.Second,
	}

	// Data with FromCache=false should not be applied
	data := TCBData{
		RTT:         200 * time.Millisecond,
		RTTVariance: 50 * time.Millisecond,
		WindowSize:  20,
		FromCache:   false,
	}
	applyTCBDataToConnection(conn, data)

	// Values should remain at defaults
	if conn.srtt != 8*time.Second {
		t.Errorf("srtt = %v, want 8s (unchanged)", conn.srtt)
	}
	if conn.cwnd != DefaultWindowSize {
		t.Errorf("cwnd = %d, want %d (unchanged)", conn.cwnd, DefaultWindowSize)
	}
}

// TestTCBCacheMultipleDestinations verifies isolation between destinations.
func TestTCBCacheMultipleDestinations(t *testing.T) {
	cache := newTCBCache(DefaultTCBCacheConfig())

	dest1 := createMockDestination(t)
	dest2 := createMockDestinationWithSeed(t, 9999) // Different seed for different hash

	// Put different data for each destination
	cache.Put(dest1, 100*time.Millisecond, 20*time.Millisecond, 10)
	cache.Put(dest2, 200*time.Millisecond, 40*time.Millisecond, 20)

	// Verify data is isolated
	rtt1, _, window1, found1 := cache.Get(dest1)
	rtt2, _, window2, found2 := cache.Get(dest2)

	if !found1 || !found2 {
		t.Error("expected both destinations to be cached")
	}

	// Values should be different (dampened)
	expectedRTT1 := time.Duration(float64(100*time.Millisecond) * 0.75)
	expectedRTT2 := time.Duration(float64(200*time.Millisecond) * 0.75)

	if rtt1 != expectedRTT1 {
		t.Errorf("dest1 RTT = %v, want %v", rtt1, expectedRTT1)
	}
	if rtt2 != expectedRTT2 {
		t.Errorf("dest2 RTT = %v, want %v", rtt2, expectedRTT2)
	}

	var wVal1 float64 = 10.0 * 0.75 // 7
	var wVal2 float64 = 20.0 * 0.75 // 15
	expectedWindow1 := uint32(wVal1)
	expectedWindow2 := uint32(wVal2)

	if window1 != expectedWindow1 {
		t.Errorf("dest1 window = %d, want %d", window1, expectedWindow1)
	}
	if window2 != expectedWindow2 {
		t.Errorf("dest2 window = %d, want %d", window2, expectedWindow2)
	}
}

// TestSaveTCBDataFromConnection verifies extraction of TCB data from connection.
func TestSaveTCBDataFromConnection(t *testing.T) {
	conn := &StreamConn{
		srtt:        150 * time.Millisecond,
		rttVariance: 30 * time.Millisecond,
		cwnd:        15,
	}

	data := saveTCBDataFromConnection(conn)

	if data.RTT != 150*time.Millisecond {
		t.Errorf("RTT = %v, want 150ms", data.RTT)
	}
	if data.RTTVariance != 30*time.Millisecond {
		t.Errorf("RTTVariance = %v, want 30ms", data.RTTVariance)
	}
	if data.WindowSize != 15 {
		t.Errorf("WindowSize = %d, want 15", data.WindowSize)
	}
	if !data.FromCache {
		t.Error("FromCache = false, want true")
	}
}

// TestStreamManagerTCBCacheIntegration verifies TCB cache integration with StreamManager.
func TestStreamManagerTCBCacheIntegration(t *testing.T) {
	// Use the real I2CP test helper to get a valid StreamManager
	i2cp := RequireI2CP(t)
	sm := i2cp.Manager

	// Verify TCB cache is initialized
	if sm.TCBCache() == nil {
		t.Error("TCBCache() returned nil")
	}

	// Verify default config
	config := sm.GetTCBCacheConfig()
	if config.RTTDampening != 0.75 {
		t.Errorf("RTTDampening = %v, want 0.75", config.RTTDampening)
	}

	// Test Enable/Disable
	sm.EnableTCBCache(false)
	config = sm.GetTCBCacheConfig()
	if config.Enabled {
		t.Error("Enabled = true after disable, want false")
	}

	sm.EnableTCBCache(true)
	config = sm.GetTCBCacheConfig()
	if !config.Enabled {
		t.Error("Enabled = false after enable, want true")
	}

	// Test SetConfig
	newConfig := TCBCacheConfig{
		RTTDampening:    0.5,
		RTTDevDampening: 0.5,
		WindowDampening: 0.5,
		EntryTTL:        10 * time.Minute,
		Enabled:         true,
	}
	sm.SetTCBCacheConfig(newConfig)

	config = sm.GetTCBCacheConfig()
	if config.RTTDampening != 0.5 {
		t.Errorf("RTTDampening after SetConfig = %v, want 0.5", config.RTTDampening)
	}
}
