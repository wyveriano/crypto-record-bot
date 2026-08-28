package telegram

import (
	"CryptoRecordBot/internal/application"
	"CryptoRecordBot/internal/domain/ports"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	telegram "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot represents the Telegram incoming & outgoing adapter.
type Bot struct {
	api          *telegram.BotAPI
	priceService *application.PriceService
	alertService *application.AlertService
	whiteList    []int64
}

// Ensure Bot implements ports.Notifier at compile-time.
var _ ports.Notifier = (*Bot)(nil)

// NewBot creates a new Telegram Bot adapter.
func NewBot(
	api *telegram.BotAPI,
	priceService *application.PriceService,
	alertService *application.AlertService,
	whiteList []int64,
) *Bot {
	return &Bot{
		api:          api,
		priceService: priceService,
		alertService: alertService,
		whiteList:    whiteList,
	}
}

// Notify sends a message to a specific Telegram chat (satisfies ports.Notifier).
func (b *Bot) Notify(_ context.Context, chatID int64, message string) error {
	msg := telegram.NewMessage(chatID, message)
	_, err := b.api.Send(msg)
	return err
}

// Start begins the long-polling loop and listens for updates until ctx is cancelled.
func (b *Bot) Start(ctx context.Context) error {
	updateConfig := telegram.NewUpdate(0)
	updateConfig.Timeout = 60

	updates := b.api.GetUpdatesChan(updateConfig)

	slog.Info("Telegram bot update listener started", slog.String("bot_username", b.api.Self.UserName))

	for {
		select {
		case <-ctx.Done():
			slog.Info("Shutting down Telegram update listener...")
			b.api.StopReceivingUpdates()
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			if update.Message == nil {
				continue
			}

			if len(b.whiteList) > 0 && !slices.Contains(b.whiteList, update.Message.From.ID) {
				slog.Warn("Unauthorized access attempt",
					slog.Int64("user_id", update.Message.From.ID),
					slog.String("username", update.Message.From.UserName),
				)
				continue
			}

			go b.handleMessage(ctx, update.Message)
		}
	}
}

func (b *Bot) handleMessage(parentCtx context.Context, msg *telegram.Message) {
	if !msg.IsCommand() {
		return
	}

	ctx, cancel := context.WithTimeout(parentCtx, 15*time.Second)
	defer cancel()

	cmd := strings.ToLower(msg.Command())
	switch cmd {
	case "price":
		b.handlePrice(ctx, msg)
	case "createalert":
		b.handleCreateAlert(ctx, msg)
	case "listalerts":
		b.handleListAlerts(ctx, msg)
	case "deletealert":
		b.handleDeleteAlert(ctx, msg)
	default:
		slog.Debug("Unknown command received", slog.String("command", cmd))
	}
}

func (b *Bot) handlePrice(ctx context.Context, msg *telegram.Message) {
	coinName := strings.TrimSpace(msg.CommandArguments())
	if coinName == "" {
		coinName = "bitcoin"
	}

	price, err := b.priceService.GetPrice(ctx, coinName)
	if err != nil {
		b.reply(msg.Chat.ID, fmt.Sprintf("❌ %s", err.Error()))
		return
	}

	response := fmt.Sprintf(
		"💰 %s: USD %s (%s %.2f%%)",
		strings.ToUpper(price.ID),
		price.FormattedUSD(),
		price.ChangeSymbol(),
		price.USD24HChange,
	)
	b.reply(msg.Chat.ID, response)
}

func (b *Bot) handleCreateAlert(ctx context.Context, msg *telegram.Message) {
	args := strings.Fields(msg.CommandArguments())
	if len(args) != 3 {
		b.reply(msg.Chat.ID, "⚠️ Usage: /createalert <coin> <operator> <price>\n\nExample:\n/createalert bitcoin > 50000\n/createalert ethereum < 2500")
		return
	}

	coinName := args[0]
	operator := args[1]
	priceStr := args[2]

	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil || price <= 0 {
		b.reply(msg.Chat.ID, fmt.Sprintf("❌ Invalid price value: %q. Must be a positive decimal number.", priceStr))
		return
	}

	alert, err := b.alertService.CreateAlert(ctx, msg.Chat.ID, msg.From.ID, coinName, operator, price)
	if err != nil {
		b.reply(msg.Chat.ID, fmt.Sprintf("❌ %s", err.Error()))
		return
	}

	b.reply(msg.Chat.ID, fmt.Sprintf("✅ Alert created: %s", alert.String()))
}

func (b *Bot) handleListAlerts(ctx context.Context, msg *telegram.Message) {
	alerts, err := b.alertService.ListAlerts(ctx, msg.Chat.ID, msg.From.ID)
	if err != nil {
		b.reply(msg.Chat.ID, fmt.Sprintf("❌ Failed to list alerts: %s", err.Error()))
		return
	}

	if len(alerts) == 0 {
		b.reply(msg.Chat.ID, "ℹ️ You do not have any active price alerts.")
		return
	}

	var sb strings.Builder
	sb.WriteString("📋 Active Price Alerts:\n\n")
	for i, alert := range alerts {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, alert.String()))
	}
	b.reply(msg.Chat.ID, sb.String())
}

func (b *Bot) handleDeleteAlert(ctx context.Context, msg *telegram.Message) {
	coinName := strings.TrimSpace(msg.CommandArguments())
	if coinName == "" {
		b.reply(msg.Chat.ID, "⚠️ Usage: /deletealert <coin>\n\nExample:\n/deletealert bitcoin")
		return
	}

	deleted, err := b.alertService.DeleteAlert(ctx, msg.Chat.ID, msg.From.ID, coinName)
	if err != nil {
		b.reply(msg.Chat.ID, fmt.Sprintf("❌ %s", err.Error()))
		return
	}

	if !deleted {
		b.reply(msg.Chat.ID, fmt.Sprintf("ℹ️ No active alerts found for %q.", coinName))
		return
	}

	b.reply(msg.Chat.ID, fmt.Sprintf("✅ Alerts for %q deleted successfully.", coinName))
}

func (b *Bot) reply(chatID int64, text string) {
	msg := telegram.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		slog.Error("Failed to send telegram reply",
			slog.Int64("chat_id", chatID),
			slog.Any("error", err),
		)
	}
}
