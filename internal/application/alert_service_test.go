package application

import (
	"CryptoRecordBot/internal/domain/model"
	"context"
	"errors"
	"strings"
	"testing"
)

type mockAlertRepo struct {
	countByUserIDFn         func(ctx context.Context, userID int64) (int64, error)
	createFn                func(ctx context.Context, alert model.Alert) error
	findByChatIDAndUserIDFn func(ctx context.Context, chatID int64, userID int64) ([]model.Alert, error)
	deleteFn                func(ctx context.Context, chatID int64, userID int64, coinName string) (bool, error)
	deleteExactFn           func(ctx context.Context, alert model.Alert) error
	findCoinNamesFn         func(ctx context.Context) ([]string, error)
	findByCoinNameFn        func(ctx context.Context, coinName string) ([]model.Alert, error)
}

func (m *mockAlertRepo) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	if m.countByUserIDFn != nil {
		return m.countByUserIDFn(ctx, userID)
	}
	return 0, nil
}

func (m *mockAlertRepo) Create(ctx context.Context, alert model.Alert) error {
	if m.createFn != nil {
		return m.createFn(ctx, alert)
	}
	return nil
}

func (m *mockAlertRepo) FindByChatIDAndUserID(ctx context.Context, chatID int64, userID int64) ([]model.Alert, error) {
	if m.findByChatIDAndUserIDFn != nil {
		return m.findByChatIDAndUserIDFn(ctx, chatID, userID)
	}
	return nil, nil
}

func (m *mockAlertRepo) Delete(ctx context.Context, chatID int64, userID int64, coinName string) (bool, error) {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, chatID, userID, coinName)
	}
	return true, nil
}

func (m *mockAlertRepo) DeleteExact(ctx context.Context, alert model.Alert) error {
	if m.deleteExactFn != nil {
		return m.deleteExactFn(ctx, alert)
	}
	return nil
}

func (m *mockAlertRepo) FindCoinNames(ctx context.Context) ([]string, error) {
	if m.findCoinNamesFn != nil {
		return m.findCoinNamesFn(ctx)
	}
	return nil, nil
}

func (m *mockAlertRepo) FindByCoinName(ctx context.Context, coinName string) ([]model.Alert, error) {
	if m.findByCoinNameFn != nil {
		return m.findByCoinNameFn(ctx, coinName)
	}
	return nil, nil
}

type mockCryptoRepo struct {
	isValidCoinFn           func(ctx context.Context, coinName string) (bool, error)
	getPriceFn              func(ctx context.Context, coinName string, currency string) (float64, error)
	getPriceWith24HChangeFn func(ctx context.Context, coinName string) (*model.SimplePrice, error)
}

func (m *mockCryptoRepo) IsValidCoin(ctx context.Context, coinName string) (bool, error) {
	if m.isValidCoinFn != nil {
		return m.isValidCoinFn(ctx, coinName)
	}
	return true, nil
}

func (m *mockCryptoRepo) GetPrice(ctx context.Context, coinName string, currency string) (float64, error) {
	if m.getPriceFn != nil {
		return m.getPriceFn(ctx, coinName, currency)
	}
	return 100.0, nil
}

func (m *mockCryptoRepo) GetPriceWith24HChange(ctx context.Context, coinName string) (*model.SimplePrice, error) {
	if m.getPriceWith24HChangeFn != nil {
		return m.getPriceWith24HChangeFn(ctx, coinName)
	}
	return &model.SimplePrice{
		ID: coinName,
		PriceWithChange: model.PriceWithChange{
			USD:          100.0,
			USD24HChange: 1.5,
		},
	}, nil
}

type mockNotifier struct {
	notifyFn func(ctx context.Context, chatID int64, message string) error
}

func (m *mockNotifier) Notify(ctx context.Context, chatID int64, message string) error {
	if m.notifyFn != nil {
		return m.notifyFn(ctx, chatID, message)
	}
	return nil
}

func TestAlertService_CreateAlert_UnderQuota(t *testing.T) {
	ctx := context.Background()
	alertRepo := &mockAlertRepo{
		countByUserIDFn: func(ctx context.Context, userID int64) (int64, error) {
			return 2, nil
		},
		createFn: func(ctx context.Context, alert model.Alert) error {
			return nil
		},
	}
	cryptoRepo := &mockCryptoRepo{
		isValidCoinFn: func(ctx context.Context, coinName string) (bool, error) {
			return true, nil
		},
	}

	service := NewAlertService(alertRepo, cryptoRepo, &mockNotifier{}, 5)
	alert, err := service.CreateAlert(ctx, 100, 200, "bitcoin", ">", 50000)
	if err != nil {
		t.Fatalf("expected successful alert creation, got: %v", err)
	}

	if alert.CoinName != "bitcoin" || !alert.IsGreaterThan || alert.Price != 50000 {
		t.Errorf("unexpected alert values: %+v", alert)
	}
}

func TestAlertService_CreateAlert_QuotaExceeded(t *testing.T) {
	ctx := context.Background()
	isValidCoinCalled := false

	alertRepo := &mockAlertRepo{
		countByUserIDFn: func(ctx context.Context, userID int64) (int64, error) {
			return 5, nil
		},
	}
	cryptoRepo := &mockCryptoRepo{
		isValidCoinFn: func(ctx context.Context, coinName string) (bool, error) {
			isValidCoinCalled = true
			return true, nil
		},
	}

	service := NewAlertService(alertRepo, cryptoRepo, &mockNotifier{}, 5)
	_, err := service.CreateAlert(ctx, 100, 200, "bitcoin", ">", 50000)
	if err == nil {
		t.Fatalf("expected quota exceeded error, got nil")
	}

	if !strings.Contains(err.Error(), "you have reached the maximum of 5 active alerts") {
		t.Errorf("expected error message to mention quota, got %q", err.Error())
	}

	if isValidCoinCalled {
		t.Errorf("expected IsValidCoin NOT to be called when quota is exceeded (fail-fast)")
	}
}

func TestAlertService_CreateAlert_UnlimitedQuota(t *testing.T) {
	ctx := context.Background()
	alertRepo := &mockAlertRepo{
		countByUserIDFn: func(ctx context.Context, userID int64) (int64, error) {
			return 999, nil
		},
	}
	cryptoRepo := &mockCryptoRepo{
		isValidCoinFn: func(ctx context.Context, coinName string) (bool, error) {
			return true, nil
		},
	}

	// maxAlerts = 0 means unlimited
	service := NewAlertService(alertRepo, cryptoRepo, &mockNotifier{}, 0)
	_, err := service.CreateAlert(ctx, 100, 200, "bitcoin", ">", 50000)
	if err != nil {
		t.Fatalf("expected successful alert creation with unlimited quota, got: %v", err)
	}
}

func TestAlertService_CreateAlert_InputValidation(t *testing.T) {
	ctx := context.Background()
	service := NewAlertService(&mockAlertRepo{}, &mockCryptoRepo{}, &mockNotifier{}, 20)

	// Empty coin
	if _, err := service.CreateAlert(ctx, 100, 200, "", ">", 50000); err == nil {
		t.Errorf("expected error for empty coin")
	}

	// Invalid operator
	if _, err := service.CreateAlert(ctx, 100, 200, "bitcoin", "=", 50000); err == nil {
		t.Errorf("expected error for invalid operator")
	}

	// Non-positive price
	if _, err := service.CreateAlert(ctx, 100, 200, "bitcoin", ">", 0); err == nil {
		t.Errorf("expected error for price <= 0")
	}
}

func TestAlertService_EvaluateAndTriggerAlerts_Resilience(t *testing.T) {
	ctx := context.Background()
	var notifiedMessages []string

	alertRepo := &mockAlertRepo{
		findCoinNamesFn: func(ctx context.Context) ([]string, error) {
			return []string{"badcoin", "goodcoin"}, nil
		},
		findByCoinNameFn: func(ctx context.Context, coinName string) ([]model.Alert, error) {
			if coinName == "goodcoin" {
				return []model.Alert{
					model.NewAlert(100, 200, "goodcoin", true, 50.0),
				}, nil
			}
			return nil, nil
		},
		deleteExactFn: func(ctx context.Context, alert model.Alert) error {
			return nil
		},
	}

	cryptoRepo := &mockCryptoRepo{
		getPriceFn: func(ctx context.Context, coinName string, currency string) (float64, error) {
			if coinName == "badcoin" {
				return 0, errors.New("API rate limit error")
			}
			return 60.0, nil
		},
	}

	notifier := &mockNotifier{
		notifyFn: func(ctx context.Context, chatID int64, message string) error {
			notifiedMessages = append(notifiedMessages, message)
			return nil
		},
	}

	service := NewAlertService(alertRepo, cryptoRepo, notifier, 20)
	err := service.EvaluateAndTriggerAlerts(ctx)
	if err != nil {
		t.Fatalf("expected EvaluateAndTriggerAlerts to succeed despite badcoin error, got: %v", err)
	}

	if len(notifiedMessages) != 1 {
		t.Errorf("expected 1 notification for goodcoin, got %d", len(notifiedMessages))
	}
}
