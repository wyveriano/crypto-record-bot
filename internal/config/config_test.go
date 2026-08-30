package config

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	os.Clearenv()
	t.Setenv("TELEGRAM_TOKEN", "test-token-123")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.TelegramToken != "test-token-123" {
		t.Errorf("expected token 'test-token-123', got %q", cfg.TelegramToken)
	}
	if cfg.Profile != "prod" {
		t.Errorf("expected profile 'prod', got %q", cfg.Profile)
	}
	if cfg.DBPath != "crypto_record.db" {
		t.Errorf("expected dbPath 'crypto_record.db', got %q", cfg.DBPath)
	}
	if cfg.AlertInterval != 3*time.Minute {
		t.Errorf("expected alertInterval 3m, got %v", cfg.AlertInterval)
	}
	if cfg.RateLimit != 15 {
		t.Errorf("expected default rateLimit 15, got %d", cfg.RateLimit)
	}
	if cfg.RateWindow != time.Minute {
		t.Errorf("expected default rateWindow 1m, got %v", cfg.RateWindow)
	}
	if cfg.CleanupInterval != 5*time.Minute {
		t.Errorf("expected default cleanupInterval 5m, got %v", cfg.CleanupInterval)
	}
	if cfg.MaxAlertsPerUser != 20 {
		t.Errorf("expected default maxAlertsPerUser 20, got %d", cfg.MaxAlertsPerUser)
	}
	if len(cfg.WhiteList) != 0 {
		t.Errorf("expected empty whitelist, got %v", cfg.WhiteList)
	}
}

func TestLoad_CustomEnv(t *testing.T) {
	os.Clearenv()
	t.Setenv("TELEGRAM_TOKEN", "custom-token")
	t.Setenv("WHITE_LIST", "1001, 1002 ,1003")
	t.Setenv("PROFILE", "dev")
	t.Setenv("DB_PATH", "custom.db")
	t.Setenv("ALERT_INTERVAL", "1m")
	t.Setenv("RATE_LIMIT", "30")
	t.Setenv("RATE_WINDOW", "30s")
	t.Setenv("GUARD_CLEANUP_INTERVAL", "10m")
	t.Setenv("MAX_ALERTS_PER_USER", "50")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.RateLimit != 30 {
		t.Errorf("expected rateLimit 30, got %d", cfg.RateLimit)
	}
	if cfg.RateWindow != 30*time.Second {
		t.Errorf("expected rateWindow 30s, got %v", cfg.RateWindow)
	}
	if cfg.CleanupInterval != 10*time.Minute {
		t.Errorf("expected cleanupInterval 10m, got %v", cfg.CleanupInterval)
	}
	if cfg.MaxAlertsPerUser != 50 {
		t.Errorf("expected maxAlertsPerUser 50, got %d", cfg.MaxAlertsPerUser)
	}
	expectedWhitelist := []int64{1001, 1002, 1003}
	if !reflect.DeepEqual(cfg.WhiteList, expectedWhitelist) {
		t.Errorf("expected whitelist %v, got %v", expectedWhitelist, cfg.WhiteList)
	}
}

func TestLoad_UnlimitedAlerts(t *testing.T) {
	os.Clearenv()
	t.Setenv("TELEGRAM_TOKEN", "custom-token")
	t.Setenv("MAX_ALERTS_PER_USER", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.MaxAlertsPerUser != 0 {
		t.Errorf("expected maxAlertsPerUser 0 (unlimited), got %d", cfg.MaxAlertsPerUser)
	}
}

func TestLoad_MissingToken(t *testing.T) {
	os.Clearenv()

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error when TELEGRAM_TOKEN is missing, got nil")
	}
}
