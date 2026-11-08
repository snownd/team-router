package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/model"
)

// LogFilter 日志查询过滤条件。
type LogFilter struct {
	Start, End    time.Time // 必填语义：End 为零值时取 Start+24h
	UserID        int64
	APIKeyID      int64
	ProviderID    int64
	Model         string
	Protocol      string
	IsError       *bool
	Keyword       string // err_msg / request_id 模糊
	Offset, Limit int
}

// RequestLogRepo 请求日志仓储（分区表）。
type RequestLogRepo struct{ db *gorm.DB }

// NewRequestLogRepo 构造。
func NewRequestLogRepo(db *gorm.DB) *RequestLogRepo { return &RequestLogRepo{db: db} }

// InsertBatch 批量写入（异步 writer 调用）。
func (r *RequestLogRepo) InsertBatch(ctx context.Context, logs []model.RequestLog) error {
	if len(logs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(logs, len(logs)).Error
}

// Query 分页查询。始终带时间范围以做分区裁剪。
func (r *RequestLogRepo) Query(ctx context.Context, f LogFilter) ([]model.RequestLog, int64, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}
	if f.End.IsZero() {
		f.End = time.Now().UTC()
	}
	if !f.Start.Before(f.End) {
		return nil, 0, nil
	}

	apply := func(q *gorm.DB) *gorm.DB {
		q = q.Where("created_at >= ? AND created_at < ?", f.Start, f.End)
		if f.UserID > 0 {
			q = q.Where("user_id = ?", f.UserID)
		}
		if f.APIKeyID > 0 {
			q = q.Where("api_key_id = ?", f.APIKeyID)
		}
		if f.ProviderID > 0 {
			q = q.Where("provider_id = ?", f.ProviderID)
		}
		if f.Model != "" {
			q = q.Where("model = ?", f.Model)
		}
		if f.Protocol != "" {
			q = q.Where("protocol = ?", f.Protocol)
		}
		if f.IsError != nil {
			q = q.Where("is_error = ?", *f.IsError)
		}
		if f.Keyword != "" {
			kw := "%" + f.Keyword + "%"
			q = q.Where("err_msg ILIKE ? OR request_id ILIKE ?", kw, kw)
		}
		return q
	}

	var total int64
	if err := apply(r.db.WithContext(ctx).Model(&model.RequestLog{})).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []model.RequestLog
	if err := apply(r.db.WithContext(ctx).Model(&model.RequestLog{})).
		Order("created_at DESC").Offset(f.Offset).Limit(f.Limit).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// Stats 按天 + 模型聚合用量与费用。
func (r *RequestLogRepo) Stats(ctx context.Context, start, end time.Time, userID int64) ([]model.UsageStat, error) {
	q := r.db.WithContext(ctx).Table("request_logs").
		Select(`
			date_trunc('day', created_at) AS bucket,
			model,
			count(*) AS requests,
			COALESCE(sum(input_tokens), 0) AS input_tokens,
			COALESCE(sum(output_tokens), 0) AS output_tokens,
			COALESCE(sum(cache_read_tokens), 0) AS cache_read_tokens,
			COALESCE(sum(cache_write_tokens), 0) AS cache_write_tokens,
			COALESCE(sum(quota_cost), 0) AS quota_cost`).
		Where("created_at >= ? AND created_at < ?", start, end)
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	var stats []model.UsageStat
	err := q.Group("bucket, model").Order("bucket ASC").Scan(&stats).Error
	return stats, err
}

// TTFTStats 平均首 token 延迟（按 provider / model），供运维看板。
func (r *RequestLogRepo) TTFTStats(ctx context.Context, start, end time.Time) ([]map[string]any, error) {
	var out []map[string]any
	err := r.db.WithContext(ctx).Table("request_logs").
		Select(`
			provider_name,
			model,
			count(*) AS requests,
			COALESCE(avg(ttft_ms) FILTER (WHERE ttft_ms > 0), 0) AS avg_ttft_ms,
			COALESCE(avg(total_ms), 0) AS avg_total_ms`).
		Where("created_at >= ? AND created_at < ?", start, end).
		Group("provider_name, model").
		Order("requests DESC").
		Limit(100).
		Scan(&out).Error
	return out, err
}
