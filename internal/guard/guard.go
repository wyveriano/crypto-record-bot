package guard

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"
)

// Guard provides per-user rate limiting with whitelist bypass.
type Guard struct {
	cfg     Config
	limiter *rateLimiter
	mu      sync.RWMutex
	cancel  context.CancelFunc
}

// New creates a Guard and starts the background cleanup goroutine.
// Call Close() when shutting down.
func New(ctx context.Context, cfg Config) *Guard {
	cfg = cfg.withDefaults()
	cleanupCtx, cancel := context.WithCancel(ctx)

	g := &Guard{
		cfg:     cfg,
		limiter: newRateLimiter(cfg.RateLimit, cfg.RateWindow),
		cancel:  cancel,
	}

	go g.cleanupLoop(cleanupCtx)
	return g
}

// Allow checks if the given userID is permitted to execute a command.
// Returns nil if allowed, or an error with a human-readable message if denied.
// Whitelisted users always return nil (bypass all limits).
func (g *Guard) Allow(userID int64) error {
	if g.IsWhiteListed(userID) {
		return nil
	}

	if !g.limiter.allow(userID) {
		retryAfter := g.limiter.retryAfter(userID)
		return fmt.Errorf("⏳ Too many requests. Please wait %s before trying again", retryAfter.Round(time.Second))
	}

	return nil
}

// IsWhiteListed returns true if the userID is in the whitelist.
func (g *Guard) IsWhiteListed(userID int64) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return slices.Contains(g.cfg.WhiteList, userID)
}

// Close stops the background cleanup goroutine.
func (g *Guard) Close() {
	if g.cancel != nil {
		g.cancel()
	}
}

func (g *Guard) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(g.cfg.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			removed := g.limiter.cleanup()
			if removed > 0 {
				slog.Debug("Guard: cleaned up stale user entries", slog.Int("removed", removed))
			}
		}
	}
}
