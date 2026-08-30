package persistence

import (
	"CryptoRecordBot/internal/domain/model"
	"CryptoRecordBot/internal/domain/ports"
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type alertRepository struct {
	db *gorm.DB
}

// NewAlertRepository instantiates a new AlertRepository backed by GORM.
func NewAlertRepository(db *gorm.DB) ports.AlertRepository {
	return &alertRepository{
		db: db,
	}
}

func (r *alertRepository) FindByChatIDAndUserID(ctx context.Context, chatID int64, userID int64) ([]model.Alert, error) {
	var daos []AlertDAO
	result := r.db.WithContext(ctx).
		Where("chat_id = ? AND user_id = ?", chatID, userID).
		Find(&daos)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to query alerts by chat and user: %w", result.Error)
	}

	alerts := make([]model.Alert, 0, len(daos))
	for _, dao := range daos {
		alerts = append(alerts, dao.ToDomain())
	}
	return alerts, nil
}

func (r *alertRepository) Create(ctx context.Context, alert model.Alert) error {
	dao := FromDomain(alert)
	result := r.db.WithContext(ctx).
		Where("chat_id = ? AND user_id = ? AND coin_name = ? AND is_greater_than = ?",
			dao.ChatID, dao.UserID, dao.CoinName, dao.IsGreaterThan).
		FirstOrCreate(&dao)

	if result.Error != nil {
		return fmt.Errorf("failed to create alert: %w", result.Error)
	}
	return nil
}

func (r *alertRepository) Delete(ctx context.Context, chatID int64, userID int64, coinName string) (bool, error) {
	coinName = strings.ToLower(strings.TrimSpace(coinName))
	result := r.db.WithContext(ctx).
		Where("chat_id = ? AND user_id = ? AND coin_name = ?", chatID, userID, coinName).
		Delete(&AlertDAO{})

	if result.Error != nil {
		return false, fmt.Errorf("failed to delete alert for %q: %w", coinName, result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *alertRepository) DeleteExact(ctx context.Context, alert model.Alert) error {
	dao := FromDomain(alert)
	result := r.db.WithContext(ctx).
		Where("chat_id = ? AND user_id = ? AND coin_name = ? AND is_greater_than = ?",
			dao.ChatID, dao.UserID, dao.CoinName, dao.IsGreaterThan).
		Delete(&AlertDAO{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete exact alert: %w", result.Error)
	}
	return nil
}

func (r *alertRepository) FindCoinNames(ctx context.Context) ([]string, error) {
	var daos []AlertDAO
	result := r.db.WithContext(ctx).
		Distinct("coin_name").
		Find(&daos)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to find distinct coin names: %w", result.Error)
	}

	coins := make([]string, 0, len(daos))
	for _, dao := range daos {
		coins = append(coins, dao.CoinName)
	}
	return coins, nil
}

func (r *alertRepository) FindByCoinName(ctx context.Context, coinName string) ([]model.Alert, error) {
	coinName = strings.ToLower(strings.TrimSpace(coinName))
	var daos []AlertDAO
	result := r.db.WithContext(ctx).
		Where("coin_name = ?", coinName).
		Find(&daos)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to find alerts for coin %q: %w", coinName, result.Error)
	}

	alerts := make([]model.Alert, 0, len(daos))
	for _, dao := range daos {
		alerts = append(alerts, dao.ToDomain())
	}
	return alerts, nil
}

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

