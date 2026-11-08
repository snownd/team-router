package model

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// Provider 上游渠道。
type Provider struct {
	ID           int64           `gorm:"primaryKey;column:id" json:"id"`
	Name         string          `gorm:"column:name" json:"name"`
	Type         string          `gorm:"column:type" json:"type"` // openai | anthropic | custom_openai ...
	BaseURL      string          `gorm:"column:base_url" json:"base_url"`
	APIKeyEnc    string          `gorm:"column:api_key" json:"-"`                   // AES-GCM 加密后的上游密钥
	Models       json.RawMessage `gorm:"column:models" json:"models"`               // 可服务模型列表 ["gpt-4o", ...]
	ModelMapping json.RawMessage `gorm:"column:model_mapping" json:"model_mapping"` // {"请求模型":"上游模型"}
	Priority     int             `gorm:"column:priority" json:"priority"`
	Weight       int             `gorm:"column:weight" json:"weight"`
	// MaxConcurrency 渠道最大并发在途请求数；0 = 不限制。达到上限时新请求在该渠道队列排队，
	// 排队超时直接向客户端返回 429（见 provider.Limiter / relay 集成）。
	MaxConcurrency int       `gorm:"column:max_concurrency" json:"max_concurrency"`
	Status         int       `gorm:"column:status" json:"status"` // 1 启用 2 禁用
	ProxyURL       string    `gorm:"column:proxy_url" json:"proxy_url"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at"`
}

const (
	ProviderStatusEnabled  = 1
	ProviderStatusDisabled = 2
)

func (Provider) TableName() string { return "providers" }

// BeforeCreate 确保 jsonb 字段非 NULL（表列带有 NOT NULL 约束）。
func (p *Provider) BeforeCreate(_ *gorm.DB) error {
	if len(p.Models) == 0 || string(p.Models) == "null" {
		p.Models = json.RawMessage("[]")
	}
	if len(p.ModelMapping) == 0 || string(p.ModelMapping) == "null" {
		p.ModelMapping = json.RawMessage("{}")
	}
	if p.Weight <= 0 {
		p.Weight = 1
	}
	if p.Status == 0 {
		p.Status = ProviderStatusEnabled
	}
	return nil
}

// ModelList 可服务模型列表。
func (p *Provider) ModelList() []string {
	if len(p.Models) == 0 || string(p.Models) == "null" {
		return nil
	}
	var list []string
	_ = json.Unmarshal(p.Models, &list)
	return list
}

// ServesModel 是否可服务请求模型（考虑 model_mapping 重定向）。
func (p *Provider) ServesModel(model string) bool {
	if m := p.MappingFor(model); m != "" {
		return true
	}
	for _, s := range p.ModelList() {
		if s == model {
			return true
		}
	}
	return false
}

// MappingFor 请求模型名 -> 上游模型名；无映射时原样返回空。
func (p *Provider) MappingFor(model string) string {
	if len(p.ModelMapping) == 0 || string(p.ModelMapping) == "null" {
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal(p.ModelMapping, &m); err != nil {
		return ""
	}
	if v, ok := m[model]; ok {
		return v
	}
	return ""
}
