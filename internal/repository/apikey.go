package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/model"
)

// APIKeyRepo API Key 仓储。
type APIKeyRepo struct{ db *gorm.DB }

// NewAPIKeyRepo 构造。
func NewAPIKeyRepo(db *gorm.DB) *APIKeyRepo { return &APIKeyRepo{db: db} }

// Create 创建。
func (r *APIKeyRepo) Create(ctx context.Context, k *model.APIKey) error {
	return r.db.WithContext(ctx).Create(k).Error
}

// GetByID 查询。
func (r *APIKeyRepo) GetByID(ctx context.Context, id int64) (*model.APIKey, error) {
	var k model.APIKey
	if err := r.db.WithContext(ctx).First(&k, id).Error; err != nil {
		return nil, err
	}
	return &k, nil
}

// FindByHash 按密钥哈希查询（认证路径）。
func (r *APIKeyRepo) FindByHash(ctx context.Context, hash string) (*model.APIKey, error) {
	var k model.APIKey
	if err := r.db.WithContext(ctx).Where("key_hash = ?", hash).First(&k).Error; err != nil {
		return nil, err
	}
	return &k, nil
}

// ListByUser 用户的所有 key。
func (r *APIKeyRepo) ListByUser(ctx context.Context, userID int64) ([]model.APIKey, error) {
	var keys []model.APIKey
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("id DESC").Find(&keys).Error
	return keys, err
}

// ListAll 分页查询全部（管理端）。
func (r *APIKeyRepo) ListAll(ctx context.Context, userID int64, offset, limit int) ([]model.APIKey, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.APIKey{})
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var keys []model.APIKey
	err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&keys).Error
	return keys, total, err
}

// Update 更新指定列。
func (r *APIKeyRepo) Update(ctx context.Context, k *model.APIKey, cols ...string) error {
	return r.db.WithContext(ctx).Model(k).Select(cols).Updates(k).Error
}

// Delete 删除。
func (r *APIKeyRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.APIKey{}, id).Error
}

// AdjustQuota 原子调整 key 已用额度（delta 可为负）。
func (r *APIKeyRepo) AdjustQuota(ctx context.Context, keyID int64, delta int64) error {
	res := r.db.WithContext(ctx).Model(&model.APIKey{}).
		Where("id = ?", keyID).
		UpdateColumn("used_quota", gorm.Expr("used_quota + ?", delta))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("api key %d not found", keyID)
	}
	return nil
}

// ReserveQuota 预扣额度：used_quota 增加 est，要求剩余额度充足（quota=0 的 key 不限额，仅累加）。
// 返回是否成功。
func (r *APIKeyRepo) ReserveQuota(ctx context.Context, keyID int64, est int64) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE api_keys SET used_quota = used_quota + ?
		WHERE id = ? AND status = ? AND (quota = 0 OR quota - used_quota >= ?)`,
		est, keyID, model.APIKeyStatusEnabled, est)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ExpiresSoon 是否已过期（辅助）。
func (r *APIKeyRepo) ExpiresSoon(k *model.APIKey, now time.Time) bool {
	return k.ExpiresAt != nil && !k.ExpiresAt.After(now)
}
