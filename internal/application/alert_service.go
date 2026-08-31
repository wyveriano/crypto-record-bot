package application

import (
	"CryptoRecordBot/internal/domain/model"
	"CryptoRecordBot/internal/domain/ports"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// AlertService orchestrates alert creation, querying, deletion, and background evaluation.
type AlertService struct {
	alertRepo  ports.AlertRepository
	cryptoRepo ports.CryptoRepository
	notifier   ports.Notifier
	maxAlerts  int
}

// NewAlertService creates a new AlertService.
func NewAlertService(
	alertRepo ports.AlertRepository,
	cryptoRepo ports.CryptoRepository,
	notifier ports.Notifier,
	maxAlerts int,
) *AlertService {
	return &AlertService{
		alertRepo:  alertRepo,
		cryptoRepo: cryptoRepo,
		notifier:   notifier,
		maxAlerts:  maxAlerts,
	}
}

// CreateAlert validates input, checks coin existence with CoinGecko, and stores the alert.
func (s *AlertService) CreateAlert(
	ctx context.Context,
	chatID, userID int64,
	coinName, operator string,
	price float64,
) (model.Alert, error) {
	coinName = strings.ToLower(strings.TrimSpace(coinName))
	if coinName == "" {
		return model.Alert{}, errors.New("coin name cannot be empty")
	}

	operator = strings.TrimSpace(operator)
	if operator != ">" && operator != "<" {
		return model.Alert{}, fmt.Errorf("invalid operator %q, allowed operators are '>' or '<'", operator)
	}

	if price <= 0 {
		return model.Alert{}, errors.New("price must be greater than zero")
	}

	if s.maxAlerts > 0 {
		count, err := s.alertRepo.CountByUserID(ctx, userID)
		if err != nil {
			return model.Alert{}, fmt.Errorf("failed to check alert quota: %w", err)
		}
		if count >= int64(s.maxAlerts) {
			return model.Alert{}, fmt.Errorf("you have reached the maximum of %d active alerts", s.maxAlerts)
		}
	}

	isValid, err := s.cryptoRepo.IsValidCoin(ctx, coinName)
	if err != nil {
		return model.Alert{}, fmt.Errorf("failed to validate coin with market provider: %w", err)
	}
	if !isValid {
		return model.Alert{}, fmt.Errorf("coin %q is not recognized by CoinGecko", coinName)
	}

	alert := model.NewAlert(chatID, userID, coinName, operator == ">", price)
	if err := s.alertRepo.Create(ctx, alert); err != nil {
		return model.Alert{}, fmt.Errorf("failed to persist alert: %w", err)
	}

	return alert, nil
}

// ListAlerts retrieves all active alerts for a given chat and user.
func (s *AlertService) ListAlerts(ctx context.Context, chatID, userID int64) ([]model.Alert, error) {
	return s.alertRepo.FindByChatIDAndUserID(ctx, chatID, userID)
}

// DeleteAlert removes alerts for a specific coin belonging to the given user and chat.
func (s *AlertService) DeleteAlert(ctx context.Context, chatID, userID int64, coinName string) (bool, error) {
	coinName = strings.ToLower(strings.TrimSpace(coinName))
	if coinName == "" {
		return false, errors.New("coin name cannot be empty")
	}
	return s.alertRepo.Delete(ctx, chatID, userID, coinName)
}

// EvaluateAndTriggerAlerts checks all distinct coins against current market prices and sends notifications.
func (s *AlertService) EvaluateAndTriggerAlerts(ctx context.Context) error {
	coinNames, err := s.alertRepo.FindCoinNames(ctx)
	if err != nil {
		return fmt.Errorf("failed to retrieve active coin names: %w", err)
	}

	for _, coinName := range coinNames {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		currentPrice, err := s.cryptoRepo.GetPrice(ctx, coinName, "usd")
		if err != nil {
			slog.ErrorContext(ctx, "Failed to fetch price for coin evaluation",
				slog.String("coin", coinName),
				slog.Any("error", err),
			)
			continue // Continue to next coin instead of breaking the entire loop!
		}

		alerts, err := s.alertRepo.FindByCoinName(ctx, coinName)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to find alerts for coin",
				slog.String("coin", coinName),
				slog.Any("error", err),
			)
			continue
		}

		for _, alert := range alerts {
			if !alert.Matches(currentPrice) {
				continue
			}

			priceStr := strconv.FormatFloat(currentPrice, 'f', -1, 64)
			var conditionText string
			if alert.IsGreaterThan {
				conditionText = "higher than"
			} else {
				conditionText = "lower than"
			}

			msg := fmt.Sprintf(
				"🔔 %s price is %s USD (it is now %s your target of %s USD)",
				strings.ToUpper(alert.CoinName),
				priceStr,
				conditionText,
				alert.FormattedPrice(),
			)

			if s.notifier != nil {
				if err := s.notifier.Notify(ctx, alert.ChatID, msg); err != nil {
					slog.ErrorContext(ctx, "Failed to send alert notification to user",
						slog.Int64("chat_id", alert.ChatID),
						slog.String("coin", alert.CoinName),
						slog.Any("error", err),
					)
				}
			}

			if err := s.alertRepo.DeleteExact(ctx, alert); err != nil {
				slog.ErrorContext(ctx, "Failed to delete triggered alert",
					slog.String("coin", alert.CoinName),
					slog.Any("error", err),
				)
			}
		}
	}

	return nil
}
