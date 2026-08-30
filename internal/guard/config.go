package guard

import "time"

// Config holds all settings for the Guard.
type Config struct {
	RateLimit       int           // Max commands per window per user. Default: 15
	RateWindow      time.Duration // Time window for rate counting. Default: 1 minute
	WhiteList       []int64       // User IDs that bypass all limits. Default: empty
	CleanupInterval time.Duration // How often to purge stale tracking data. Default: 5 minutes
}

func (c Config) withDefaults() Config {
	if c.RateLimit <= 0 {
		c.RateLimit = 15
	}
	if c.RateWindow <= 0 {
		c.RateWindow = time.Minute
	}
	if c.CleanupInterval <= 0 {
		c.CleanupInterval = 5 * time.Minute
	}
	if c.WhiteList == nil {
		c.WhiteList = []int64{}
	}
	return c
}
