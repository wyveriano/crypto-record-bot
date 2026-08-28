package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Alert represents a user-defined price alert threshold for a cryptocurrency.
type Alert struct {
	ChatID        int64
	UserID        int64
	CoinName      string
	IsGreaterThan bool
	Price         float64
	CreatedAt     time.Time
}

// String returns a human-readable representation of the alert.
func (a Alert) String() string {
	symbol := "<"
	if a.IsGreaterThan {
		symbol = ">"
	}
	return fmt.Sprintf("%s %s %s", a.CoinName, symbol, a.FormattedPrice())
}

// FormattedPrice formats the threshold price to a trimmed string.
func (a Alert) FormattedPrice() string {
	return strconv.FormatFloat(a.Price, 'f', -1, 64)
}

// Matches returns true if the given market price triggers this alert threshold.
func (a Alert) Matches(currentPrice float64) bool {
	if a.IsGreaterThan {
		return currentPrice > a.Price
	}
	return currentPrice < a.Price
}

// NewAlert constructs an Alert instance with normalized coin name and creation time.
func NewAlert(chatID, userID int64, coinName string, isGreaterThan bool, price float64) Alert {
	return Alert{
		ChatID:        chatID,
		UserID:        userID,
		CoinName:      strings.ToLower(strings.TrimSpace(coinName)),
		IsGreaterThan: isGreaterThan,
		Price:         price,
		CreatedAt:     time.Now().UTC(),
	}
}
