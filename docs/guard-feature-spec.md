# Feature Specification: Bot Abuse Protection (`guard` Package)

## 1. Context & Motivation

**CryptoRecordBot** is a Go-based Telegram bot that monitors cryptocurrency prices and manages user price alerts. It is being refactored for **public deployment on a Raspberry Pi 5 (8GB RAM)**.

Opening the bot to the public introduces abuse vectors:
- **Command flooding**: A user or bot sending hundreds of commands per second, exhausting CPU, CoinGecko API rate limits, and SQLite write throughput.
- **Database bloat**: Malicious creation of thousands of alerts, filling the SD card / storage.
- **Resource starvation**: Heavy CoinGecko API usage from `/createalert` validation (which calls `GetCoinList()`) and `/price` queries.

The bot receives messages **through Telegram's servers** (long-polling), so classic network DDoS is not a direct threat. The attack surface is **application-level abuse via Telegram messages**.

### Design Goals
1. **Performance**: Near-zero overhead on a Raspberry Pi 5. All protection must be in-memory.
2. **Simplicity**: Easy to understand, configure, and maintain.
3. **Reusability**: The protection package must have **zero dependencies** on Telegram, CoinGecko, GORM, or any project-specific code. It should be copy-pasteable into any other Go bot project.
4. **Whitelist Awareness**: Users on the whitelist bypass all rate limits and quotas (they are trusted).

---

## 2. Threat Model

| Threat | Impact on Raspberry Pi | Affected Commands |
|:---|:---|:---|
| Command spam (any command) | CPU saturation, goroutine explosion | All |
| `/price` spam | CoinGecko API rate limit exhaustion (10-30 req/min free tier) | `/price` |
| `/createalert` spam | CoinGecko API calls + SQLite writes + disk usage | `/createalert` |
| Mass alert creation | SQLite database grows until disk is full | `/createalert` |
| Multiple abusive users | All of the above, multiplied | All |

---

## 3. Options Considered

### Option 1: Strict Whitelist Only (Already Implemented)

Only user IDs defined in `WHITE_LIST` can use the bot. All others are silently ignored.

| Pros | Cons |
|:---|:---|
| Zero additional code, already works | Cannot open the bot to the public |
| Complete protection from unknown users | Manual management of user IDs |
| Best performance (single `slices.Contains`) | Does not protect against authorized user abuse |

**Verdict**: Keep as a layer, but insufficient alone for a public bot.

### Option 2: Per-User Rate Limiter (In-Memory)

A counter tracks how many commands each user has executed within a sliding time window. If the count exceeds the limit, subsequent commands are rejected with a cooldown message.

| Pros | Cons |
|:---|:---|
| Very lightweight (~100 bytes per tracked user) | Tracking state is lost on restart (acceptable) |
| Protects against all forms of command flooding | Requires choosing sensible default limits |
| Completely generic and reusable | Does not protect against slow, sustained DB bloat |
| Works with or without whitelist | |

**Verdict**: **Recommended.** This is the universal "seatbelt" for any bot.

### Option 3: Per-User Resource Quota (Max Alerts)

A business rule that caps the number of active alerts a user can have in the database (e.g., max 20).

| Pros | Cons |
|:---|:---|
| Directly protects SQLite and disk space | Only applies to alert creation |
| Simple to implement (COUNT query before INSERT) | Does not protect against `/price` spam |
| Reusable pattern for any capped resource | |

**Verdict**: **Recommended.** Essential complement to rate limiting for protecting storage.

### Option 4: Dynamic Blacklist (Auto-Ban)

If a user hits the rate limit repeatedly (e.g., 3 times within 5 minutes), they are temporarily banned for a configurable duration.

| Pros | Cons |
|:---|:---|
| Punishes persistent abuse | More complex implementation |
| Deterrent effect | Risk of false positives with tight limits |
| Reduces processing of known-abusive users to zero | Needs an unban mechanism |

**Verdict**: Recommended as a future enhancement. Not needed for initial deployment.

### Option 5: Per-Command Rate Limits (Command Cooldowns)

Different rate limits for different commands. For example, `/price` allows 20/min (cheap), but `/createalert` allows only 5/min (expensive: writes DB + calls CoinGecko).

| Pros | Cons |
|:---|:---|
| Protects expensive resources without limiting cheap ones | More configuration surface |
| Better user experience for lightweight commands | |

**Verdict**: Good enhancement. Can be layered on top of Option 2 in the future.

---

## 4. Recommended Solution

Implement **Options 2 + 3** together:

1. **`internal/guard/` package** — Pure Go, zero external dependencies
   - In-memory per-user rate limiter (sliding window)
   - Whitelist bypass built-in
   - Background cleanup goroutine for stale user entries
   - Graceful shutdown via `context.Context`

2. **Per-user alert quota** — Business rule in `AlertService`
   - New `CountByUserID(ctx, userID)` method on `AlertRepository` port
   - Check in `AlertService.CreateAlert()` before persisting
   - Configurable via `MAX_ALERTS_PER_USER` env var

---

## 5. Detailed Design

### 5.1 Package Structure

```text
internal/guard/
├── guard.go          # Guard struct: orchestrates rate limiting + whitelist
├── ratelimiter.go    # Sliding window rate limiter implementation
└── config.go         # GuardConfig with defaults
```

> [!IMPORTANT]
> The `guard` package must NOT import anything from `CryptoRecordBot/internal/...`.
> It must only use Go standard library packages (`sync`, `time`, `context`, `log/slog`).
> This ensures it can be extracted to its own module or copied to another project.

### 5.2 `guard.go` — Main API

```go
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
    return slices.Contains(g.cfg.WhiteList, userID)
}

// Close stops the background cleanup goroutine.
func (g *Guard) Close() {
    g.cancel()
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
```

### 5.3 `ratelimiter.go` — Sliding Window Implementation

The rate limiter uses a **sliding window log** approach per user. For each user, it stores a slice of timestamps. When `allow()` is called, it:

1. Removes timestamps older than the window.
2. Checks if the count of remaining timestamps is below the limit.
3. If yes, appends the current timestamp and returns `true`.
4. If no, returns `false`.

This approach is chosen for:
- **Accuracy**: No boundary issues like fixed-window counters.
- **Simplicity**: ~40 lines of code.
- **Memory**: Each timestamp is 8 bytes. At 15 requests/minute limit, each user stores at most 15 × 8 = 120 bytes.

```go
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
    return time.Until(oldest.Add(rl.window))
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
```

### 5.4 `config.go` — Configuration with Sensible Defaults

```go
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
```

### 5.5 Per-User Alert Quota (Business Rule)

This is NOT part of the `guard` package. It is a domain/application concern.

#### 5.5.1 New Method on `AlertRepository` Port

Add to [internal/domain/ports/repositories.go](file:///C:/Users/emipo/go/crypto-record-bot/internal/domain/ports/repositories.go):

```go
type AlertRepository interface {
    // ... existing methods ...
    CountByUserID(ctx context.Context, userID int64) (int64, error)
}
```

#### 5.5.2 Implementation in Persistence Layer

Add to [internal/infrastructure/persistence/repositories.go](file:///C:/Users/emipo/go/crypto-record-bot/internal/infrastructure/persistence/repositories.go):

```go
func (r *alertRepository) CountByUserID(ctx context.Context, userID int64) (int64, error) {
    var count int64
    result := r.db.WithContext(ctx).
        Model(&AlertDAO{}).
        Where("user_id = ?", userID).
        Count(&count)
    if result.Error != nil {
        return 0, fmt.Errorf("failed to count alerts for user %d: %w", userID, result.Error)
    }
    return count, nil
}
```

#### 5.5.3 Quota Check in AlertService

Modify `CreateAlert()` in [internal/application/alert_service.go](file:///C:/Users/emipo/go/crypto-record-bot/internal/application/alert_service.go#L35-L69):

```go
// Add maxAlerts as a field in AlertService
type AlertService struct {
    alertRepo     ports.AlertRepository
    cryptoRepo    ports.CryptoRepository
    notifier      ports.Notifier
    maxAlerts     int  // <-- NEW: max alerts per user (0 = unlimited)
}

// In CreateAlert, BEFORE the CoinGecko validation:
if s.maxAlerts > 0 {
    count, err := s.alertRepo.CountByUserID(ctx, userID)
    if err != nil {
        return model.Alert{}, fmt.Errorf("failed to check alert quota: %w", err)
    }
    if count >= int64(s.maxAlerts) {
        return model.Alert{}, fmt.Errorf("you have reached the maximum of %d active alerts", s.maxAlerts)
    }
}
```

> [!NOTE]
> The quota check happens BEFORE calling CoinGecko (`IsValidCoin`), because the CoinGecko
> call is expensive (network I/O) and the quota check is cheap (local SQLite COUNT query).
> This follows the "fail fast with cheap checks first" principle.

---

## 6. Integration Points

### 6.1 Configuration Changes

Add the following fields to [internal/config/config.go](file:///C:/Users/emipo/go/crypto-record-bot/internal/config/config.go):

```go
type Config struct {
    // ... existing fields ...

    // Guard / Rate Limiting
    RateLimit       int           // Env: RATE_LIMIT, Default: 15
    RateWindow      time.Duration // Env: RATE_WINDOW, Default: 1m
    CleanupInterval time.Duration // Env: GUARD_CLEANUP_INTERVAL, Default: 5m

    // Alert Quota
    MaxAlertsPerUser int          // Env: MAX_ALERTS_PER_USER, Default: 20
}
```

| Env Variable | Type | Default | Description |
|:---|:---|:---|:---|
| `RATE_LIMIT` | `int` | `15` | Maximum commands per user per rate window |
| `RATE_WINDOW` | `duration` | `1m` | Time window for rate limit counting |
| `GUARD_CLEANUP_INTERVAL` | `duration` | `5m` | Interval for purging inactive user tracking data |
| `MAX_ALERTS_PER_USER` | `int` | `20` | Maximum active alerts a single user can have. `0` = unlimited |

### 6.2 Bootstrap Wiring

In [internal/bootstrap/app.go](file:///C:/Users/emipo/go/crypto-record-bot/internal/bootstrap/app.go), create the Guard and inject it into the Bot:

```go
import "CryptoRecordBot/internal/guard"

func NewApp(cfg *config.Config) (*App, error) {
    // ... existing setup ...

    // Initialize Guard
    g := guard.New(ctx, guard.Config{
        RateLimit:       cfg.RateLimit,
        RateWindow:      cfg.RateWindow,
        WhiteList:       cfg.WhiteList,
        CleanupInterval: cfg.CleanupInterval,
    })

    // Pass guard to bot
    bot := botAdapter.NewBot(botAPI, priceService, alertService, g)

    // ...
}
```

> [!IMPORTANT]
> The whitelist is now owned by the `Guard`. Remove the `whiteList` field from `Bot`.
> The Bot should call `g.Allow(userID)` which already handles whitelist bypass internally.

### 6.3 Telegram Bot Integration

In [internal/infrastructure/telegram/bot.go](file:///C:/Users/emipo/go/crypto-record-bot/internal/infrastructure/telegram/bot.go), modify `handleMessage()`:

```go
type Bot struct {
    api          *telegram.BotAPI
    priceService *application.PriceService
    alertService *application.AlertService
    guard        *guard.Guard  // <-- REPLACES whiteList []int64
}

func (b *Bot) handleMessage(parentCtx context.Context, msg *telegram.Message) {
    if !msg.IsCommand() {
        return
    }

    // --- GUARD CHECK (single point of enforcement) ---
    if err := b.guard.Allow(msg.From.ID); err != nil {
        b.reply(msg.Chat.ID, err.Error())
        return
    }

    ctx, cancel := context.WithTimeout(parentCtx, 15*time.Second)
    defer cancel()

    // ... switch on command as before ...
}
```

Also update the `Start()` method whitelist check. Since `guard.Allow()` already handles the whitelist, the explicit whitelist check in `Start()` should be removed:

```go
// BEFORE (current code in Start()):
if len(b.whiteList) > 0 && !slices.Contains(b.whiteList, update.Message.From.ID) {
    slog.Warn("Unauthorized access attempt", ...)
    continue
}

// AFTER: Remove this block entirely. The guard handles it in handleMessage().
// Non-command messages from non-whitelisted users will simply be ignored
// by the `if !msg.IsCommand()` check in handleMessage.
```

### 6.4 Lifecycle: Guard Shutdown

In [internal/bootstrap/app.go](file:///C:/Users/emipo/go/crypto-record-bot/internal/bootstrap/app.go), ensure the Guard's cleanup goroutine stops on shutdown:

```go
type App struct {
    cfg          *config.Config
    bot          *botAdapter.Bot
    alertService *application.AlertService
    guard        *guard.Guard  // <-- NEW
}

func (a *App) Run(ctx context.Context) error {
    defer a.guard.Close()  // <-- Ensure cleanup goroutine stops
    // ... rest of Run ...
}
```

---

## 7. User-Facing Behavior

### Rate-Limited User Experience

When a non-whitelisted user exceeds the rate limit:

```
User:  /price bitcoin
Bot:   💰 BITCOIN: USD 67500 (😎 2.50%)

User:  /price ethereum
Bot:   💰 ETHEREUM: USD 3200 (😎 1.20%)

... (13 more commands within the same minute) ...

User:  /price cardano
Bot:   ⏳ Too many requests. Please wait 23s before trying again
```

### Quota-Exceeded User Experience

When a user has reached the maximum alert count:

```
User:  /createalert bitcoin > 100000
Bot:   ❌ you have reached the maximum of 20 active alerts

User:  /deletealert dogecoin
Bot:   ✅ Alerts for "dogecoin" deleted successfully.

User:  /createalert bitcoin > 100000
Bot:   ✅ Alert created: bitcoin > 100000
```

### Whitelisted User Experience

No limits applied. All commands execute immediately regardless of rate or quota.

---

## 8. Memory & Performance Analysis (Raspberry Pi 5)

### Rate Limiter Memory

| Scenario | Users | Timestamps/User | Memory |
|:---|:---|:---|:---|
| Low traffic | 50 | 15 | ~6 KB |
| Medium traffic | 500 | 15 | ~60 KB |
| High traffic | 5,000 | 15 | ~600 KB |
| Extreme | 50,000 | 15 | ~6 MB |

**Conclusion**: Even extreme traffic uses <1% of available RAM (8 GB).

### CPU Overhead

- `Allow()`: One mutex lock + linear scan of ≤15 timestamps + slice copy = **<1μs per call**.
- `cleanup()`: One pass over the map every 5 minutes = negligible.
- `CountByUserID()`: Single SQLite `COUNT(*)` with index on `user_id` = **<1ms**.

### Cleanup Goroutine

- Runs every 5 minutes.
- Acquires mutex briefly.
- Deletes map entries with no recent activity.
- On shutdown: stopped via context cancellation.

---

## 9. Current Codebase Reference

### Project Structure (as of latest refactor)

```text
CryptoRecordBot/
├── cmd/main.go
├── internal/
│   ├── config/config.go
│   ├── bootstrap/app.go
│   ├── domain/
│   │   ├── model/alert.go
│   │   ├── model/price.go
│   │   └── ports/
│   │       ├── notifier.go
│   │       └── repositories.go
│   ├── application/
│   │   ├── price_service.go
│   │   └── alert_service.go
│   └── infrastructure/
│       ├── telegram/bot.go
│       ├── client/crypto_repository.go
│       └── persistence/
│           ├── database.go
│           ├── entities.go
│           └── repositories.go
├── go.mod
└── go.sum
```

### Key Interfaces

- **`ports.AlertRepository`** ([repositories.go](file:///C:/Users/emipo/go/crypto-record-bot/internal/domain/ports/repositories.go)): Needs new `CountByUserID` method.
- **`ports.Notifier`** ([notifier.go](file:///C:/Users/emipo/go/crypto-record-bot/internal/domain/ports/notifier.go)): No changes needed.
- **`ports.CryptoRepository`** ([repositories.go](file:///C:/Users/emipo/go/crypto-record-bot/internal/domain/ports/repositories.go)): No changes needed.

### Key Files to Modify

| File | Change |
|:---|:---|
| `internal/config/config.go` | Add `RateLimit`, `RateWindow`, `CleanupInterval`, `MaxAlertsPerUser` fields + env parsing |
| `internal/domain/ports/repositories.go` | Add `CountByUserID` to `AlertRepository` interface |
| `internal/infrastructure/persistence/repositories.go` | Implement `CountByUserID` |
| `internal/application/alert_service.go` | Add `maxAlerts` field, quota check in `CreateAlert`, update constructor |
| `internal/infrastructure/telegram/bot.go` | Replace `whiteList` with `guard`, call `guard.Allow()` in `handleMessage`, remove old whitelist check from `Start()` |
| `internal/bootstrap/app.go` | Create `Guard`, inject into `Bot`, call `guard.Close()` on shutdown |

### New Files to Create

| File | Content |
|:---|:---|
| `internal/guard/guard.go` | Guard struct with `Allow()`, `IsWhiteListed()`, `Close()`, cleanup goroutine |
| `internal/guard/ratelimiter.go` | Sliding window rate limiter: `allow()`, `retryAfter()`, `cleanup()` |
| `internal/guard/config.go` | `Config` struct with `withDefaults()` |

---

## 10. Implementation Checklist

- [ ] Create `internal/guard/config.go` with `Config` struct and `withDefaults()`
- [ ] Create `internal/guard/ratelimiter.go` with sliding window implementation
- [ ] Create `internal/guard/guard.go` with `New()`, `Allow()`, `IsWhiteListed()`, `Close()`
- [ ] Add `CountByUserID` to `ports.AlertRepository` interface
- [ ] Implement `CountByUserID` in `persistence.alertRepository`
- [ ] Add `RateLimit`, `RateWindow`, `CleanupInterval`, `MaxAlertsPerUser` to `config.Config` with env var parsing and defaults
- [ ] Add `maxAlerts` field to `application.AlertService`, update constructor `NewAlertService`
- [ ] Add quota check in `AlertService.CreateAlert()` (before CoinGecko validation)
- [ ] Replace `whiteList []int64` with `guard *guard.Guard` in `telegram.Bot`
- [ ] Update `telegram.Bot.Start()`: remove explicit whitelist filtering block
- [ ] Update `telegram.Bot.handleMessage()`: add `guard.Allow()` check as first line
- [ ] Update `telegram.NewBot()` constructor signature
- [ ] Update `bootstrap.NewApp()`: create `Guard`, pass to `Bot`, remove whitelist from Bot constructor
- [ ] Add `guard.Close()` call in `App.Run()` defer
- [ ] Run `go vet ./...` and `go build ./...` to verify compilation
- [ ] Update `README.md` configuration table with new env vars
- [ ] Commit with message: `feat: add guard rate limiter and per-user alert quota for public deployment`

---

## 11. Future Enhancements (Out of Scope for Initial Implementation)

- **Per-command rate limits**: Different limits for `/price` (cheap) vs `/createalert` (expensive).
- **Auto-ban / dynamic blacklist**: Temporarily ban users who repeatedly hit the rate limit.
- **Metrics / observability**: Expose rate limit hit counts via slog or Prometheus for monitoring on the Pi.
- **Persistent ban list**: Store banned users in SQLite to survive restarts.
