package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/model"
)

// ProviderRepo 渠道仓储。
type ProviderRepo struct{ db *gorm.DB }

// NewProviderRepo 构造。
func NewProviderRepo(db *gorm.DB) *ProviderRepo { return &ProviderRepo{db: db} }

// Create 创建。
func (r *ProviderRepo) Create(ctx context.Context, p *model.Provider) error {
	return r.db.WithContext(ctx).Create(p).Error
}

// GetByID 查询。
func (r *ProviderRepo) GetByID(ctx context.Context, id int64) (*model.Provider, error) {
	var p model.Provider
	if err := r.db.WithContext(ctx).First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// Update 指定列。
func (r *ProviderRepo) Update(ctx context.Context, p *model.Provider, cols ...string) error {
	return r.db.WithContext(ctx).Model(p).Select(cols).Updates(p).Error
}

// Delete 删除。
func (r *ProviderRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.Provider{}, id).Error
}

// List 全量。
func (r *ProviderRepo) List(ctx context.Context) ([]model.Provider, error) {
	var ps []model.Provider
	err := r.db.WithContext(ctx).Order("priority DESC, id ASC").Find(&ps).Error
	return ps, err
}

// ListEnabled 可用渠道。
func (r *ProviderRepo) ListEnabled(ctx context.Context) ([]model.Provider, error) {
	var ps []model.Provider
	err := r.db.WithContext(ctx).
		Where("status = ?", model.ProviderStatusEnabled).
		Order("priority DESC, id ASC").Find(&ps).Error
	return ps, err
}
