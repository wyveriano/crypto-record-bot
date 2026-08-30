package guard

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}.withDefaults()

	if cfg.RateLimit != 15 {
		t.Errorf("expected default RateLimit 15, got %d", cfg.RateLimit)
	}
	if cfg.RateWindow != time.Minute {
		t.Errorf("expected default RateWindow 1m, got %v", cfg.RateWindow)
	}
	if cfg.CleanupInterval != 5*time.Minute {
		t.Errorf("expected default CleanupInterval 5m, got %v", cfg.CleanupInterval)
	}
	if cfg.WhiteList == nil || len(cfg.WhiteList) != 0 {
		t.Errorf("expected default WhiteList empty slice, got %v", cfg.WhiteList)
	}
}

func TestGuardAllowUnderLimit(t *testing.T) {
	ctx := context.Background()
	g := New(ctx, Config{
		RateLimit:  3,
		RateWindow: 100 * time.Millisecond,
	})
	defer g.Close()

	userID := int64(12345)
	for i := 0; i < 3; i++ {
		if err := g.Allow(userID); err != nil {
			t.Fatalf("expected request %d to be allowed, got error: %v", i+1, err)
		}
	}
}

func TestGuardRateLimitExceeded(t *testing.T) {
	ctx := context.Background()
	g := New(ctx, Config{
		RateLimit:  2,
		RateWindow: 500 * time.Millisecond,
	})
	defer g.Close()

	userID := int64(12345)

	// First 2 requests should succeed
	if err := g.Allow(userID); err != nil {
		t.Fatalf("expected request 1 to be allowed, got: %v", err)
	}
	if err := g.Allow(userID); err != nil {
		t.Fatalf("expected request 2 to be allowed, got: %v", err)
	}

	// 3rd request should fail
	err := g.Allow(userID)
	if err == nil {
		t.Fatalf("expected 3rd request to be rate-limited, but got nil error")
	}

	if !strings.Contains(err.Error(), "Too many requests") {
		t.Errorf("expected error to contain 'Too many requests', got %q", err.Error())
	}

	// Different user should still be allowed
	otherUserID := int64(99999)
	if err := g.Allow(otherUserID); err != nil {
		t.Errorf("expected different user to be allowed, got: %v", err)
	}
}

func TestGuardWhitelistBypass(t *testing.T) {
	ctx := context.Background()
	whitelistedID := int64(777)
	g := New(ctx, Config{
		RateLimit:  2,
		RateWindow: 1 * time.Minute,
		WhiteList:  []int64{whitelistedID},
	})
	defer g.Close()

	// Whitelisted user can send many requests without being blocked
	for i := 0; i < 10; i++ {
		if err := g.Allow(whitelistedID); err != nil {
			t.Fatalf("whitelisted user got rate limited on request %d: %v", i+1, err)
		}
	}

	if !g.IsWhiteListed(whitelistedID) {
		t.Errorf("expected IsWhiteListed to return true for %d", whitelistedID)
	}
	if g.IsWhiteListed(999) {
		t.Errorf("expected IsWhiteListed to return false for non-whitelisted user")
	}
}

func TestGuardWindowSliding(t *testing.T) {
	ctx := context.Background()
	g := New(ctx, Config{
		RateLimit:  2,
		RateWindow: 100 * time.Millisecond,
	})
	defer g.Close()

	userID := int64(12345)

	if err := g.Allow(userID); err != nil {
		t.Fatalf("req 1 failed: %v", err)
	}
	if err := g.Allow(userID); err != nil {
		t.Fatalf("req 2 failed: %v", err)
	}
	if err := g.Allow(userID); err == nil {
		t.Fatalf("req 3 should have been blocked")
	}

	// Wait for window to expire
	time.Sleep(120 * time.Millisecond)

	// Now should be allowed again
	if err := g.Allow(userID); err != nil {
		t.Fatalf("expected request after window expiry to be allowed, got: %v", err)
	}
}

func TestGuardCleanup(t *testing.T) {
	ctx := context.Background()
	g := New(ctx, Config{
		RateLimit:       5,
		RateWindow:      50 * time.Millisecond,
		CleanupInterval: 20 * time.Millisecond,
	})
	defer g.Close()

	// Record activity for user 1
	_ = g.Allow(101)
	_ = g.Allow(102)

	if len(g.limiter.users) != 2 {
		t.Fatalf("expected 2 users tracked, got %d", len(g.limiter.users))
	}

	// Wait for window to expire and background cleanup to run
	time.Sleep(100 * time.Millisecond)

	g.limiter.mu.Lock()
	userCount := len(g.limiter.users)
	g.limiter.mu.Unlock()

	if userCount != 0 {
		t.Errorf("expected users to be cleaned up, remaining: %d", userCount)
	}
}

func TestGuardConcurrency(t *testing.T) {
	ctx := context.Background()
	g := New(ctx, Config{
		RateLimit:  50,
		RateWindow: 1 * time.Second,
		WhiteList:  []int64{1},
	})
	defer g.Close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		userID := int64(i % 5)
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				_ = g.Allow(id)
			}
		}(userID)
	}
	wg.Wait()
}
