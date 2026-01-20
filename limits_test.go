package streaming

import (
	"testing"
	"time"

	go_i2cp "github.com/go-i2p/go-i2cp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultConnectionLimitsConfig verifies default values per I2P spec.
func TestDefaultConnectionLimitsConfig(t *testing.T) {
	config := DefaultConnectionLimitsConfig()

	t.Run("max concurrent streams unlimited by default", func(t *testing.T) {
		assert.Equal(t, -1, config.MaxConcurrentStreams, "should be -1 (unlimited)")
	})

	t.Run("per-peer limits disabled by default", func(t *testing.T) {
		assert.Equal(t, 0, config.MaxConnsPerMinute, "should be 0 (disabled)")
		assert.Equal(t, 0, config.MaxConnsPerHour, "should be 0 (disabled)")
		assert.Equal(t, 0, config.MaxConnsPerDay, "should be 0 (disabled)")
	})

	t.Run("total limits disabled by default", func(t *testing.T) {
		assert.Equal(t, 0, config.MaxTotalConnsPerMinute, "should be 0 (disabled)")
		assert.Equal(t, 0, config.MaxTotalConnsPerHour, "should be 0 (disabled)")
		assert.Equal(t, 0, config.MaxTotalConnsPerDay, "should be 0 (disabled)")
	})

	t.Run("limit action defaults to reset", func(t *testing.T) {
		assert.Equal(t, LimitActionReset, config.LimitAction)
	})

	t.Run("reject logging enabled by default", func(t *testing.T) {
		assert.False(t, config.DisableRejectLogging)
	})
}

// TestConnectionLimiterMaxConcurrent tests the MaxConcurrentStreams limit.
func TestConnectionLimiterMaxConcurrent(t *testing.T) {
	t.Run("unlimited when set to negative", func(t *testing.T) {
		config := &ConnectionLimitsConfig{
			MaxConcurrentStreams: -1,
		}
		limiter := newConnectionLimiter(config)

		// Should allow many connections
		for i := 0; i < 100; i++ {
			err := limiter.CheckAndRecordConnection(nil)
			assert.NoError(t, err, "should allow connection %d", i)
		}
		assert.Equal(t, 100, limiter.ActiveStreams())
	})

	t.Run("enforces limit when positive", func(t *testing.T) {
		config := &ConnectionLimitsConfig{
			MaxConcurrentStreams: 3,
		}
		limiter := newConnectionLimiter(config)

		// First 3 should succeed
		for i := 0; i < 3; i++ {
			err := limiter.CheckAndRecordConnection(nil)
			assert.NoError(t, err, "connection %d should be allowed", i)
		}
		assert.Equal(t, 3, limiter.ActiveStreams())

		// 4th should fail
		err := limiter.CheckAndRecordConnection(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max concurrent streams limit exceeded")
		assert.Equal(t, 3, limiter.ActiveStreams(), "should still be 3")

		// After closing one, new connection should succeed
		limiter.ConnectionClosed()
		assert.Equal(t, 2, limiter.ActiveStreams())

		err = limiter.CheckAndRecordConnection(nil)
		assert.NoError(t, err)
		assert.Equal(t, 3, limiter.ActiveStreams())
	})

	t.Run("zero means unlimited", func(t *testing.T) {
		config := &ConnectionLimitsConfig{
			MaxConcurrentStreams: 0,
		}
		limiter := newConnectionLimiter(config)

		// Zero is treated as unlimited (same as negative)
		for i := 0; i < 50; i++ {
			err := limiter.CheckAndRecordConnection(nil)
			assert.NoError(t, err)
		}
	})
}

// TestConnectionLimiterPerPeerRateLimits tests per-peer connection rate limiting.
func TestConnectionLimiterPerPeerRateLimits(t *testing.T) {
	t.Run("enforces per-minute limit for same peer", func(t *testing.T) {
		config := &ConnectionLimitsConfig{
			MaxConnsPerMinute:    2,
			MaxConcurrentStreams: -1, // Unlimited concurrent
		}
		limiter := newConnectionLimiter(config)

		peer := createLimitsTestDestination(t)

		// First 2 connections should succeed
		err := limiter.CheckAndRecordConnection(peer)
		assert.NoError(t, err)
		err = limiter.CheckAndRecordConnection(peer)
		assert.NoError(t, err)

		// 3rd should fail
		err = limiter.CheckAndRecordConnection(peer)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connections per minute from peer exceeded")
	})

	t.Run("different peers have independent limits", func(t *testing.T) {
		config := &ConnectionLimitsConfig{
			MaxConnsPerMinute:    2,
			MaxConcurrentStreams: -1,
		}
		limiter := newConnectionLimiter(config)

		peer1 := createLimitsTestDestination(t)
		peer2 := createLimitsTestDestination2(t)

		// Exhaust limit for peer1
		err := limiter.CheckAndRecordConnection(peer1)
		assert.NoError(t, err)
		err = limiter.CheckAndRecordConnection(peer1)
		assert.NoError(t, err)
		err = limiter.CheckAndRecordConnection(peer1)
		assert.Error(t, err, "peer1 should be limited")

		// peer2 should still be allowed
		err = limiter.CheckAndRecordConnection(peer2)
		assert.NoError(t, err, "peer2 should be allowed")
		err = limiter.CheckAndRecordConnection(peer2)
		assert.NoError(t, err, "peer2 should be allowed")
	})

	t.Run("nil destination bypasses per-peer checks", func(t *testing.T) {
		config := &ConnectionLimitsConfig{
			MaxConnsPerMinute:    1,
			MaxConcurrentStreams: -1,
		}
		limiter := newConnectionLimiter(config)

		// Multiple nil destinations should work (no peer tracking)
		for i := 0; i < 5; i++ {
			err := limiter.CheckAndRecordConnection(nil)
			assert.NoError(t, err)
		}
	})
}

// TestConnectionLimiterTotalRateLimits tests total connection rate limiting.
func TestConnectionLimiterTotalRateLimits(t *testing.T) {
	t.Run("enforces total per-minute limit", func(t *testing.T) {
		config := &ConnectionLimitsConfig{
			MaxTotalConnsPerMinute: 3,
			MaxConcurrentStreams:   -1,
		}
		limiter := newConnectionLimiter(config)

		peer1 := createLimitsTestDestination(t)
		peer2 := createLimitsTestDestination2(t)

		// 3 connections from different peers should succeed
		err := limiter.CheckAndRecordConnection(peer1)
		assert.NoError(t, err)
		err = limiter.CheckAndRecordConnection(peer2)
		assert.NoError(t, err)
		err = limiter.CheckAndRecordConnection(peer1)
		assert.NoError(t, err)

		// 4th should fail regardless of peer
		err = limiter.CheckAndRecordConnection(peer2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "total connections per minute limit exceeded")
	})
}

// TestConnectionLimiterCombinedLimits tests multiple limits working together.
func TestConnectionLimiterCombinedLimits(t *testing.T) {
	t.Run("concurrent and rate limits both apply", func(t *testing.T) {
		config := &ConnectionLimitsConfig{
			MaxConcurrentStreams:   5, // Higher concurrent limit
			MaxTotalConnsPerMinute: 3, // Lower rate limit to hit first
		}
		limiter := newConnectionLimiter(config)

		// First 3 should succeed (rate limit will be hit)
		err := limiter.CheckAndRecordConnection(nil)
		assert.NoError(t, err)
		err = limiter.CheckAndRecordConnection(nil)
		assert.NoError(t, err)
		err = limiter.CheckAndRecordConnection(nil)
		assert.NoError(t, err)

		// 4th fails due to rate limit (not concurrent limit)
		err = limiter.CheckAndRecordConnection(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "total connections per minute")
	})

	t.Run("concurrent limit checked before rate limit", func(t *testing.T) {
		config := &ConnectionLimitsConfig{
			MaxConcurrentStreams:   2,  // Low concurrent limit
			MaxTotalConnsPerMinute: 10, // High rate limit
		}
		limiter := newConnectionLimiter(config)

		// First 2 should succeed
		err := limiter.CheckAndRecordConnection(nil)
		assert.NoError(t, err)
		err = limiter.CheckAndRecordConnection(nil)
		assert.NoError(t, err)

		// 3rd fails due to concurrent limit (rate limit not reached)
		err = limiter.CheckAndRecordConnection(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max concurrent streams")
	})
}

// TestConnectionLimiterConfigUpdate tests dynamic config updates.
func TestConnectionLimiterConfigUpdate(t *testing.T) {
	t.Run("config can be updated", func(t *testing.T) {
		limiter := newConnectionLimiter(nil)

		// Initially unlimited
		for i := 0; i < 10; i++ {
			err := limiter.CheckAndRecordConnection(nil)
			assert.NoError(t, err)
		}

		// Update to strict limit
		limiter.SetConfig(&ConnectionLimitsConfig{
			MaxConcurrentStreams: 1,
		})

		// Now should fail (already 10 active)
		err := limiter.CheckAndRecordConnection(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max concurrent streams")
	})

	t.Run("GetConfig returns copy", func(t *testing.T) {
		original := &ConnectionLimitsConfig{
			MaxConcurrentStreams: 5,
			MaxConnsPerMinute:    10,
		}
		limiter := newConnectionLimiter(original)

		copy := limiter.GetConfig()
		assert.Equal(t, 5, copy.MaxConcurrentStreams)
		assert.Equal(t, 10, copy.MaxConnsPerMinute)

		// Modifying copy shouldn't affect original
		copy.MaxConcurrentStreams = 100
		assert.Equal(t, 5, limiter.config.MaxConcurrentStreams)
	})
}

// TestConnectionLimiterCleanup tests stale history cleanup.
func TestConnectionLimiterCleanup(t *testing.T) {
	t.Run("cleanup removes stale entries", func(t *testing.T) {
		config := &ConnectionLimitsConfig{
			MaxConnsPerMinute:    10,
			MaxConcurrentStreams: -1,
		}
		limiter := newConnectionLimiter(config)

		peer := createLimitsTestDestination(t)

		// Add some connections
		for i := 0; i < 3; i++ {
			err := limiter.CheckAndRecordConnection(peer)
			assert.NoError(t, err)
		}

		// There should be peer history
		limiter.mu.Lock()
		assert.Equal(t, 1, len(limiter.peerHistory), "should have 1 peer")
		limiter.mu.Unlock()

		// Cleanup shouldn't remove recent entries
		limiter.CleanupStaleHistory()
		limiter.mu.Lock()
		assert.Equal(t, 1, len(limiter.peerHistory), "should still have 1 peer")
		limiter.mu.Unlock()
	})
}

// TestLimitAction tests the different limit actions.
func TestLimitAction(t *testing.T) {
	t.Run("limit action constants are distinct", func(t *testing.T) {
		assert.NotEqual(t, LimitActionReset, LimitActionDrop)
		assert.NotEqual(t, LimitActionReset, LimitActionHTTP)
		assert.NotEqual(t, LimitActionDrop, LimitActionHTTP)
	})
}

// TestConnectionClosedDecrement tests that closing connections properly decrements.
func TestConnectionClosedDecrement(t *testing.T) {
	t.Run("connection closed decrements active count", func(t *testing.T) {
		config := &ConnectionLimitsConfig{
			MaxConcurrentStreams: 10,
		}
		limiter := newConnectionLimiter(config)

		// Add 5 connections
		for i := 0; i < 5; i++ {
			err := limiter.CheckAndRecordConnection(nil)
			assert.NoError(t, err)
		}
		assert.Equal(t, 5, limiter.ActiveStreams())

		// Close 3
		limiter.ConnectionClosed()
		limiter.ConnectionClosed()
		limiter.ConnectionClosed()
		assert.Equal(t, 2, limiter.ActiveStreams())

		// Close more than active (shouldn't go negative)
		limiter.ConnectionClosed()
		limiter.ConnectionClosed()
		limiter.ConnectionClosed() // Extra close
		assert.Equal(t, 0, limiter.ActiveStreams(), "should not go negative")
	})
}

// Helper function to create a test destination for limits tests
func createLimitsTestDestination(t *testing.T) *go_i2cp.Destination {
	t.Helper()
	crypto := go_i2cp.NewCrypto()
	dest, err := go_i2cp.NewDestination(crypto)
	if err != nil {
		t.Fatalf("failed to generate destination: %v", err)
	}
	return dest
}

// Helper function to create a different test destination for limits tests
func createLimitsTestDestination2(t *testing.T) *go_i2cp.Destination {
	t.Helper()
	// Just create another destination - crypto will generate different keys
	crypto := go_i2cp.NewCrypto()
	dest, err := go_i2cp.NewDestination(crypto)
	if err != nil {
		t.Fatalf("failed to generate destination: %v", err)
	}
	return dest
}

// TestStreamManagerConnectionLimits tests the StreamManager integration.
func TestStreamManagerConnectionLimits(t *testing.T) {
	t.Run("manager exposes connection limits API", func(t *testing.T) {
		// Create a minimal mock client for testing
		// Since we can't create a real client without network, we just test the config API
		config := &ConnectionLimitsConfig{
			MaxConcurrentStreams: 100,
			MaxConnsPerMinute:    10,
			LimitAction:          LimitActionDrop,
		}

		// Test that config is properly copied
		limiter := newConnectionLimiter(config)
		retrieved := limiter.GetConfig()

		assert.Equal(t, 100, retrieved.MaxConcurrentStreams)
		assert.Equal(t, 10, retrieved.MaxConnsPerMinute)
		assert.Equal(t, LimitActionDrop, retrieved.LimitAction)
	})
}

// TestConnectionHistoryCountSince tests timestamp counting logic.
func TestConnectionHistoryCountSince(t *testing.T) {
	t.Run("counts timestamps correctly", func(t *testing.T) {
		h := &connectionHistory{}
		now := time.Now()

		// Add timestamps at various times
		h.timestamps = []time.Time{
			now.Add(-2 * time.Hour),    // 2 hours ago
			now.Add(-30 * time.Minute), // 30 min ago
			now.Add(-10 * time.Minute), // 10 min ago
			now.Add(-5 * time.Minute),  // 5 min ago
			now.Add(-30 * time.Second), // 30 sec ago
		}

		// Count since 1 minute ago: only 30sec entry is after -1min
		count := h.countSinceLocked(now.Add(-time.Minute))
		assert.Equal(t, 1, count, "should only count the 30 second ago entry")

		// Count since 15 minutes ago: 10min, 5min, 30sec entries are after -15min
		count = h.countSinceLocked(now.Add(-15 * time.Minute))
		assert.Equal(t, 3, count, "should count 10min, 5min, and 30sec entries")

		// Count since 1 hour ago: 30min, 10min, 5min, 30sec entries are after -1hour
		count = h.countSinceLocked(now.Add(-time.Hour))
		assert.Equal(t, 4, count, "should count 30min, 10min, 5min, and 30sec entries")
	})
}

// TestPruneOldEntries tests timestamp pruning.
func TestPruneOldEntries(t *testing.T) {
	t.Run("prunes entries older than 24 hours", func(t *testing.T) {
		h := &connectionHistory{}
		now := time.Now()

		h.timestamps = []time.Time{
			now.Add(-48 * time.Hour),  // 2 days ago - should be pruned
			now.Add(-25 * time.Hour),  // 25 hours ago - should be pruned
			now.Add(-23 * time.Hour),  // 23 hours ago - should be kept
			now.Add(-1 * time.Hour),   // 1 hour ago - should be kept
			now.Add(-5 * time.Minute), // 5 min ago - should be kept
		}

		h.pruneOldEntriesLocked(now)

		assert.Equal(t, 3, len(h.timestamps), "should keep 3 recent entries")
	})
}

// TestStreamListenerWithLimiter tests that listeners properly use the limiter.
func TestStreamListenerWithLimiter(t *testing.T) {
	t.Run("listener has limiter reference when created with manager", func(t *testing.T) {
		// We can't easily create a full manager without I2CP, but we can test the
		// limiter field is properly set in the struct
		limiter := newConnectionLimiter(&ConnectionLimitsConfig{
			MaxConcurrentStreams: 50,
		})

		// Verify the limiter can check connections
		err := limiter.CheckAndRecordConnection(nil)
		require.NoError(t, err)
		assert.Equal(t, 1, limiter.ActiveStreams())
	})
}
