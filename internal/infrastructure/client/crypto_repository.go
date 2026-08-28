package client

import (
	"CryptoRecordBot/internal/domain/model"
	"CryptoRecordBot/internal/domain/ports"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	coingecko "github.com/superoo7/go-gecko/v3"
)

type geckoRepository struct {
	client *coingecko.Client
}

// NewGeckoRepository creates a new CoinGecko client adapter implementing ports.CryptoRepository.
func NewGeckoRepository(client *coingecko.Client) ports.CryptoRepository {
	return &geckoRepository{
		client: client,
	}
}

type geckoPriceItem struct {
	USD          float64 `json:"usd"`
	USD24HChange float64 `json:"usd_24h_change"`
}

func (g *geckoRepository) GetPrice(_ context.Context, coinName string, currency string) (float64, error) {
	coinName = strings.ToLower(strings.TrimSpace(coinName))
	if currency == "" {
		currency = "usd"
	}

	price, err := g.client.SimpleSinglePrice(coinName, currency)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch price for %q from coingecko: %w", coinName, err)
	}
	if price == nil {
		return 0, fmt.Errorf("no price returned for coin %q", coinName)
	}

	return float64(price.MarketPrice), nil
}

func (g *geckoRepository) GetPriceWith24HChange(_ context.Context, coinName string) (*model.SimplePrice, error) {
	coinName = strings.ToLower(strings.TrimSpace(coinName))
	url := fmt.Sprintf(
		"https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd&include_24hr_change=true",
		coinName,
	)

	response, err := g.client.MakeReq(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch price with 24h change for %q: %w", coinName, err)
	}

	mappedResponse := make(map[string]geckoPriceItem)
	if err := json.Unmarshal(response, &mappedResponse); err != nil {
		return nil, fmt.Errorf("failed to parse coingecko price response: %w", err)
	}

	item, exists := mappedResponse[coinName]
	if !exists {
		return nil, fmt.Errorf("token %q not found on coingecko", coinName)
	}

	return &model.SimplePrice{
		ID: coinName,
		PriceWithChange: model.PriceWithChange{
			USD:          item.USD,
			USD24HChange: item.USD24HChange,
		},
	}, nil
}

func (g *geckoRepository) IsValidCoin(_ context.Context, coinName string) (bool, error) {
	coinName = strings.ToLower(strings.TrimSpace(coinName))
	coins, err := g.client.CoinsList()
	if err != nil {
		return false, fmt.Errorf("failed to fetch coingecko coins list: %w", err)
	}
	if coins == nil {
		return false, nil
	}

	for _, coin := range *coins {
		if strings.EqualFold(coin.ID, coinName) || strings.EqualFold(coin.Symbol, coinName) {
			return true, nil
		}
	}
	return false, nil
}
