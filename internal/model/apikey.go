package model

import (
	"encoding/json"
	"time"
)

// APIKey 用户 API Key。明文仅生成时返回一次，库中存 sha256 哈希。
type APIKey struct {
	ID            int64           `gorm:"primaryKey;column:id" json:"id"`
	UserID        int64           `gorm:"column:user_id" json:"user_id"`
	Name          string          `gorm:"column:name" json:"name"`
	KeyPrefix     string          `gorm:"column:key_prefix" json:"key_prefix"` // 展示用前缀，如 sk-AbCdEf..
	KeyHash       string          `gorm:"column:key_hash" json:"-"`
	Status        int             `gorm:"column:status" json:"status"` // 1 启用 2 禁用
	Quota         int64           `gorm:"column:quota" json:"quota"`
	UsedQuota     int64           `gorm:"column:used_quota" json:"used_quota"`
	AllowedModels json.RawMessage `gorm:"column:allowed_models" json:"allowed_models"` // jsonb，null/[] 表示不限制
	ExpiresAt     *time.Time      `gorm:"column:expires_at" json:"expires_at"`
	CreatedAt     time.Time       `gorm:"column:created_at" json:"created_at"`
	UpdatedAt     time.Time       `gorm:"column:updated_at" json:"updated_at"`
}

const (
	APIKeyStatusEnabled  = 1
	APIKeyStatusDisabled = 2
)

func (APIKey) TableName() string { return "api_keys" }

// AllowedModelList 返回允许的模型列表；空表示不限制。
func (k *APIKey) AllowedModelList() []string {
	if len(k.AllowedModels) == 0 || string(k.AllowedModels) == "null" {
		return nil
	}
	var list []string
	_ = json.Unmarshal(k.AllowedModels, &list)
	return list
}

// AllowsModel 判断 key 是否允许访问某模型。
func (k *APIKey) AllowsModel(model string) bool {
	list := k.AllowedModelList()
	if len(list) == 0 {
		return true
	}
	for _, m := range list {
		if m == model {
			return true
		}
	}
	return false
}
