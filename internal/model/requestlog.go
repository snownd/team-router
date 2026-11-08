package model

import (
	"encoding/json"
	"time"
)

// RequestLog 请求日志，底层为按月 RANGE 分区表 request_logs。
// 注意：PK 为 (id, created_at)；写入时必须显式设置 CreatedAt 以路由分区；
// 查询时必须带 created_at 范围以做分区裁剪。
type RequestLog struct {
	// 由于 PG 分区表上 GENERATED ALWAYS AS IDENTITY 兼容性（PG17 前不支持），
	// id 使用独立 sequence 作为默认值，插入时留空由数据库生成。
	ID               int64           `gorm:"column:id" json:"id"`
	CreatedAt        time.Time       `gorm:"column:created_at" json:"created_at"`
	RequestID        string          `gorm:"column:request_id" json:"request_id"`
	UserID           int64           `gorm:"column:user_id" json:"user_id"`
	Username         string          `gorm:"column:username" json:"username"`
	APIKeyID         int64           `gorm:"column:api_key_id" json:"api_key_id"`
	ProviderID       int64           `gorm:"column:provider_id" json:"provider_id"`
	ProviderName     string          `gorm:"column:provider_name" json:"provider_name"`
	UpstreamType     string          `gorm:"column:upstream_type" json:"upstream_type"`
	Protocol         string          `gorm:"column:protocol" json:"protocol"` // chat_completions|responses|messages
	Model            string          `gorm:"column:model" json:"model"`
	UpstreamModel    string          `gorm:"column:upstream_model" json:"upstream_model"`
	Stream           bool            `gorm:"column:stream" json:"stream"`
	StatusCode       int             `gorm:"column:status_code" json:"status_code"`
	IsError          bool            `gorm:"column:is_error" json:"is_error"`
	ErrMsg           string          `gorm:"column:err_msg" json:"err_msg"`
	InputTokens      int             `gorm:"column:input_tokens" json:"input_tokens"`
	OutputTokens     int             `gorm:"column:output_tokens" json:"output_tokens"`
	ReasoningTokens  int             `gorm:"column:reasoning_tokens" json:"reasoning_tokens"`
	CacheReadTokens  int             `gorm:"column:cache_read_tokens" json:"cache_read_tokens"`
	CacheWriteTokens int             `gorm:"column:cache_write_tokens" json:"cache_write_tokens"`
	TTFTMs           int             `gorm:"column:ttft_ms" json:"ttft_ms"`               // 首 token 延迟
	TotalMs          int             `gorm:"column:total_ms" json:"total_ms"`             // 总耗时
	Retries          int             `gorm:"column:retries" json:"retries"`               // 跨渠道重试次数
	QuotaCost        int64           `gorm:"column:quota_cost" json:"quota_cost"`         // 实际扣费，微美元
	BillingDetail    json.RawMessage `gorm:"column:billing_detail" json:"billing_detail"` // 计费明细
	ClientIP         string          `gorm:"column:client_ip" json:"client_ip"`
}

func (RequestLog) TableName() string { return "request_logs" }

// 三种入站协议标识。
const (
	ProtocolChatCompletions = "chat_completions"
	ProtocolResponses       = "responses"
	ProtocolMessages        = "messages"
)

// UsageStat 用量统计聚合行。
type UsageStat struct {
	Bucket           time.Time `gorm:"column:bucket" json:"bucket"` // 按天
	Model            string    `gorm:"column:model" json:"model"`
	Requests         int64     `gorm:"column:requests" json:"requests"`
	InputTokens      int64     `gorm:"column:input_tokens" json:"input_tokens"`
	OutputTokens     int64     `gorm:"column:output_tokens" json:"output_tokens"`
	CacheReadTokens  int64     `gorm:"column:cache_read_tokens" json:"cache_read_tokens"`
	CacheWriteTokens int64     `gorm:"column:cache_write_tokens" json:"cache_write_tokens"`
	QuotaCost        int64     `gorm:"column:quota_cost" json:"quota_cost"`
}
