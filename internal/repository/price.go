package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/model"
)

// PriceRepo 定价仓储。
type PriceRepo struct{ db *gorm.DB }

// NewPriceRepo 构造。
func NewPriceRepo(db *gorm.DB) *PriceRepo { return &PriceRepo{db: db} }

// Find 定价查找：优先精确 (model, provider_type)，其次通配 (model, ”)。
func (r *PriceRepo) Find(ctx context.Context, modelName, providerType string) (*model.ModelPrice, error) {
	var p model.ModelPrice
	err := r.db.WithContext(ctx).
		Where("model_name = ? AND provider_type = ? AND status = ?", modelName, providerType, model.PriceStatusEnabled).
		First(&p).Error
	if err == nil {
		return &p, nil
	}
	if err := r.db.WithContext(ctx).
		Where("model_name = ? AND provider_type = '' AND status = ?", modelName, model.PriceStatusEnabled).
		First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// List 全量。
func (r *PriceRepo) List(ctx context.Context) ([]model.ModelPrice, error) {
	var ps []model.ModelPrice
	err := r.db.WithContext(ctx).Order("model_name ASC, provider_type ASC").Find(&ps).Error
	return ps, err
}

// Upsert 新增或更新（按 model_name+provider_type 唯一约束）。
func (r *PriceRepo) Upsert(ctx context.Context, p *model.ModelPrice) error {
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO model_prices (model_name, provider_type, input_per_m, output_per_m, cache_read_per_m, cache_write_per_m, currency, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, now(), now())
		ON CONFLICT (model_name, provider_type) DO UPDATE SET
		  input_per_m = EXCLUDED.input_per_m,
		  output_per_m = EXCLUDED.output_per_m,
		  cache_read_per_m = EXCLUDED.cache_read_per_m,
		  cache_write_per_m = EXCLUDED.cache_write_per_m,
		  currency = EXCLUDED.currency,
		  status = EXCLUDED.status,
		  updated_at = now()`,
		p.ModelName, p.ProviderType, p.InputPerM, p.OutputPerM,
		p.CacheReadPerM, p.CacheWritePerM, p.Currency, p.Status).Error
}

// Delete 删除。
func (r *PriceRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.ModelPrice{}, id).Error
}
