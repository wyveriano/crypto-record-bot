package model

import "strconv"

// PriceWithChange represents the cryptocurrency price in USD and its 24-hour percentage change.
type PriceWithChange struct {
	USD          float64 `json:"usd"`
	USD24HChange float64 `json:"usd_24h_change"`
}

// ChangeSymbol returns an emoji indicating price sentiment based on 24h change.
func (p PriceWithChange) ChangeSymbol() string {
	switch {
	case p.USD24HChange >= 15:
		return "🚀"
	case p.USD24HChange > 0:
		return "😎"
	case p.USD24HChange < 0:
		return "😓"
	default:
		return ""
	}
}

// SimplePrice represents the coin identifier and its market price data.
type SimplePrice struct {
	ID string
	PriceWithChange
}

// FormattedUSD returns the USD price formatted as a string.
func (s SimplePrice) FormattedUSD() string {
	return strconv.FormatFloat(s.USD, 'f', -1, 64)
}
