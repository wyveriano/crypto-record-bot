package persistence

import (
	"CryptoRecordBot/internal/domain/model"
	"time"
)

// AlertDAO is the GORM database entity mapping the alerts table.
type AlertDAO struct {
	ChatID        int64     `gorm:"column:chat_id;primaryKey;autoIncrement:false"`
	UserID        int64     `gorm:"column:user_id;primaryKey;autoIncrement:false"`
	CoinName      string    `gorm:"column:coin_name;primaryKey"`
	IsGreaterThan bool      `gorm:"column:is_greater_than;primaryKey"`
	Price         float64   `gorm:"column:price"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

// TableName returns the table name for GORM.
func (AlertDAO) TableName() string {
	return "alerts"
}

// ToDomain converts AlertDAO to the domain Alert model.
func (dao AlertDAO) ToDomain() model.Alert {
	return model.Alert{
		ChatID:        dao.ChatID,
		UserID:        dao.UserID,
		CoinName:      dao.CoinName,
		IsGreaterThan: dao.IsGreaterThan,
		Price:         dao.Price,
		CreatedAt:     dao.CreatedAt,
	}
}

// FromDomain creates an AlertDAO from a domain Alert model.
func FromDomain(alert model.Alert) AlertDAO {
	return AlertDAO{
		ChatID:        alert.ChatID,
		UserID:        alert.UserID,
		CoinName:      alert.CoinName,
		IsGreaterThan: alert.IsGreaterThan,
		Price:         alert.Price,
		CreatedAt:     alert.CreatedAt,
	}
}
