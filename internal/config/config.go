package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration settings for the application.
type Config struct {
	TelegramToken    string
	WhiteList        []int64
	Profile          string
	DBPath           string
	AlertInterval    time.Duration
	RateLimit        int
	RateWindow       time.Duration
	CleanupInterval  time.Duration
	MaxAlertsPerUser int
}

// Load parses environment variables and returns a validated Config struct.
func Load() (*Config, error) {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_TOKEN"))
	if token == "" {
		return nil, errors.New("TELEGRAM_TOKEN environment variable is required")
	}

	whiteList, err := parseWhiteList(os.Getenv("WHITE_LIST"))
	if err != nil {
		return nil, fmt.Errorf("invalid WHITE_LIST: %w", err)
	}

	profile := strings.ToLower(strings.TrimSpace(os.Getenv("PROFILE")))
	if profile == "" {
		profile = "prod"
	}

	dbPath := strings.TrimSpace(os.Getenv("DB_PATH"))
	if dbPath == "" {
		dbPath = "crypto_record.db"
	}

	alertInterval := 3 * time.Minute
	if intervalStr := strings.TrimSpace(os.Getenv("ALERT_INTERVAL")); intervalStr != "" {
		if parsedInterval, err := time.ParseDuration(intervalStr); err == nil && parsedInterval > 0 {
			alertInterval = parsedInterval
		}
	}

	rateLimit := 15
	if rlStr := strings.TrimSpace(os.Getenv("RATE_LIMIT")); rlStr != "" {
		if parsed, err := strconv.Atoi(rlStr); err == nil && parsed > 0 {
			rateLimit = parsed
		}
	}

	rateWindow := time.Minute
	if rwStr := strings.TrimSpace(os.Getenv("RATE_WINDOW")); rwStr != "" {
		if parsed, err := time.ParseDuration(rwStr); err == nil && parsed > 0 {
			rateWindow = parsed
		}
	}

	cleanupInterval := 5 * time.Minute
	if ciStr := strings.TrimSpace(os.Getenv("GUARD_CLEANUP_INTERVAL")); ciStr != "" {
		if parsed, err := time.ParseDuration(ciStr); err == nil && parsed > 0 {
			cleanupInterval = parsed
		}
	}

	maxAlertsPerUser := 20
	if maStr := strings.TrimSpace(os.Getenv("MAX_ALERTS_PER_USER")); maStr != "" {
		if parsed, err := strconv.Atoi(maStr); err == nil && parsed >= 0 {
			maxAlertsPerUser = parsed
		}
	}

	return &Config{
		TelegramToken:    token,
		WhiteList:        whiteList,
		Profile:          profile,
		DBPath:           dbPath,
		AlertInterval:    alertInterval,
		RateLimit:        rateLimit,
		RateWindow:       rateWindow,
		CleanupInterval:  cleanupInterval,
		MaxAlertsPerUser: maxAlertsPerUser,
	}, nil
}

func parseWhiteList(raw string) ([]int64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []int64{}, nil
	}

	parts := strings.Split(trimmed, ",")
	result := make([]int64, 0, len(parts))
	for _, part := range parts {
		cleanPart := strings.TrimSpace(part)
		if cleanPart == "" {
			continue
		}
		id, err := strconv.ParseInt(cleanPart, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid user ID %q: %w", cleanPart, err)
		}
		result = append(result, id)
	}
	return result, nil
}
