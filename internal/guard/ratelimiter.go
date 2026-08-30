package guard

import (
	"sync"
	"time"
)

type userWindow struct {
	timestamps []time.Time
}

type rateLimiter struct {
	maxRequests int
	window      time.Duration
	users       map[int64]*userWindow
	mu          sync.Mutex
}

func newRateLimiter(maxRequests int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		maxRequests: maxRequests,
		window:      window,
		users:       make(map[int64]*userWindow),
	}
}

func (rl *rateLimiter) allow(userID int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	uw, exists := rl.users[userID]
	if !exists {
		uw = &userWindow{timestamps: make([]time.Time, 0, rl.maxRequests)}
		rl.users[userID] = uw
	}

	// Remove expired timestamps
	cutoff := now.Add(-rl.window)
	i := 0
	for i < len(uw.timestamps) && uw.timestamps[i].Before(cutoff) {
		i++
	}
	uw.timestamps = uw.timestamps[i:]

	// Check limit
	if len(uw.timestamps) >= rl.maxRequests {
		return false
	}

	// Record this request
	uw.timestamps = append(uw.timestamps, now)
	return true
}

func (rl *rateLimiter) retryAfter(userID int64) time.Duration {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	uw, exists := rl.users[userID]
	if !exists || len(uw.timestamps) == 0 {
		return 0
	}

	// The oldest timestamp will expire at: oldest + window
	oldest := uw.timestamps[0]
	remaining := time.Until(oldest.Add(rl.window))
	if remaining < 0 {
		return 0
	}
	return remaining
}

// cleanup removes user entries that have no recent activity.
// Returns the number of entries removed.
func (rl *rateLimiter) cleanup() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-rl.window)
	removed := 0
	for userID, uw := range rl.users {
		if len(uw.timestamps) == 0 || uw.timestamps[len(uw.timestamps)-1].Before(cutoff) {
			delete(rl.users, userID)
			removed++
		}
	}
	return removed
}
