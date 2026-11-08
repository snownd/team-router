package model

import "time"

// ModelPrice 模型定价：每百万 token 的单价，单位微美元（1e6 微美元 = $1）。
// provider_type 为空表示通配（该模型所有 provider 适用的默认价）。
type ModelPrice struct {
	ID             int64     `gorm:"primaryKey;column:id" json:"id"`
	ModelName      string    `gorm:"column:model_name" json:"model_name"`
	ProviderType   string    `gorm:"column:provider_type" json:"provider_type"`
	InputPerM      int64     `gorm:"column:input_per_m" json:"input_per_m"`
	OutputPerM     int64     `gorm:"column:output_per_m" json:"output_per_m"`
	CacheReadPerM  int64     `gorm:"column:cache_read_per_m" json:"cache_read_per_m"`
	CacheWritePerM int64     `gorm:"column:cache_write_per_m" json:"cache_write_per_m"`
	Currency       string    `gorm:"column:currency" json:"currency"`
	Status         int       `gorm:"column:status" json:"status"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at"`
}

const (
	PriceStatusEnabled  = 1
	PriceStatusDisabled = 2
)

func (ModelPrice) TableName() string { return "model_prices" }
