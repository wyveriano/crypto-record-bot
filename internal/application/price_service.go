package application

import (
	"CryptoRecordBot/internal/domain/model"
	"CryptoRecordBot/internal/domain/ports"
	"context"
	"fmt"
	"strings"
)

// PriceService handles price inquiry use cases.
type PriceService struct {
	cryptoRepo ports.CryptoRepository
}

// NewPriceService creates a new PriceService.
func NewPriceService(cryptoRepo ports.CryptoRepository) *PriceService {
	return &PriceService{
		cryptoRepo: cryptoRepo,
	}
}

// GetPrice queries CoinGecko for current price and 24h change.
func (s *PriceService) GetPrice(ctx context.Context, coinName string) (*model.SimplePrice, error) {
	coinName = strings.TrimSpace(coinName)
	if coinName == "" {
		coinName = "bitcoin"
	}

	price, err := s.cryptoRepo.GetPriceWith24HChange(ctx, coinName)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve price for %q: %w", coinName, err)
	}

	return price, nil
}
