package bootstrap

import (
	"CryptoRecordBot/internal/application"
	"CryptoRecordBot/internal/config"
	"CryptoRecordBot/internal/infrastructure/client"
	"CryptoRecordBot/internal/infrastructure/persistence"
	botAdapter "CryptoRecordBot/internal/infrastructure/telegram"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	telegram "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	gecko "github.com/superoo7/go-gecko/v3"
)

// App manages the application lifecycle and dependencies.
type App struct {
	cfg          *config.Config
	bot          *botAdapter.Bot
	alertService *application.AlertService
}

// NewApp initializes all database connections, clients, and services.
func NewApp(cfg *config.Config) (*App, error) {
	setupLogger(cfg.Profile)

	slog.Info("Initializing CryptoRecordBot...", slog.String("profile", cfg.Profile))

	// 1. Initialize SQLite Database
	db, err := persistence.NewDB(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("database initialization error: %w", err)
	}
	alertRepo := persistence.NewAlertRepository(db)

	// 2. Initialize CoinGecko Client & Repository
	geckoClient := newGeckoClient()
	cryptoRepo := client.NewGeckoRepository(geckoClient)

	// 3. Initialize Telegram Bot API
	botAPI, err := telegram.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		return nil, fmt.Errorf("failed to authorize telegram bot token: %w", err)
	}
	botAPI.Debug = strings.EqualFold("dev", cfg.Profile)
	slog.Info("Telegram authorized", slog.String("account", botAPI.Self.UserName))

	// 4. Initialize Application Services
	priceService := application.NewPriceService(cryptoRepo)

	// 5. Initialize Telegram Adapter & Alert Service
	var alertService *application.AlertService
	bot := botAdapter.NewBot(botAPI, priceService, nil, cfg.WhiteList)
	alertService = application.NewAlertService(alertRepo, cryptoRepo, bot)
	// Inject alertService into bot
	bot = botAdapter.NewBot(botAPI, priceService, alertService, cfg.WhiteList)

	return &App{
		cfg:          cfg,
		bot:          bot,
		alertService: alertService,
	}, nil
}

// Run executes the bot and the background monitoring loop until ctx is cancelled.
func (a *App) Run(ctx context.Context) error {
	slog.Info("Application running. Press CTRL+C to terminate.")

	// Launch background alert evaluation worker
	go a.runAlertWorker(ctx)

	// Run Telegram bot long-polling in the main routine
	return a.bot.Start(ctx)
}

func (a *App) runAlertWorker(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.AlertInterval)
	defer ticker.Stop()

	slog.Info("Background alert monitor started", slog.Duration("interval", a.cfg.AlertInterval))

	for {
		select {
		case <-ctx.Done():
			slog.Info("Stopping background alert monitor...")
			return
		case <-ticker.C:
			evalCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
			if err := a.alertService.EvaluateAndTriggerAlerts(evalCtx); err != nil {
				slog.ErrorContext(evalCtx, "Error during alert evaluation cycle", slog.Any("error", err))
			}
			cancel()
		}
	}
}

func newGeckoClient() *gecko.Client {
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}
	return gecko.NewClient(httpClient)
}

func setupLogger(profile string) {
	var handler slog.Handler
	if strings.EqualFold(profile, "dev") {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	}
	slog.SetDefault(slog.New(handler))
}
