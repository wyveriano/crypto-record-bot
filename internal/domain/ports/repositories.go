package ports

import (
	"CryptoRecordBot/internal/domain/model"
	"context"
)

// CryptoRepository defines the outgoing port for fetching market data and validating tokens.
type CryptoRepository interface {
	GetPrice(ctx context.Context, coinName string, currency string) (float64, error)
	IsValidCoin(ctx context.Context, coinName string) (bool, error)
	GetPriceWith24HChange(ctx context.Context, coinName string) (*model.SimplePrice, error)
}

// AlertRepository defines the outgoing port for persistence of user price alerts.
type AlertRepository interface {
	FindByChatIDAndUserID(ctx context.Context, chatID int64, userID int64) ([]model.Alert, error)
	Create(ctx context.Context, alert model.Alert) error
	Delete(ctx context.Context, chatID int64, userID int64, coinName string) (bool, error)
	DeleteExact(ctx context.Context, alert model.Alert) error
	FindCoinNames(ctx context.Context) ([]string, error)
	FindByCoinName(ctx context.Context, coinName string) ([]model.Alert, error)
	CountByUserID(ctx context.Context, userID int64) (int64, error)
}
