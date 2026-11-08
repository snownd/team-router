// Package repository 提供基于 GORM 的数据访问层。
package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/model"
)

// UserRepo 用户仓储。
type UserRepo struct{ db *gorm.DB }

// NewUserRepo 构造。
func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

// Create 创建用户。
func (r *UserRepo) Create(ctx context.Context, u *model.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

// GetByID 按 ID 查询。
func (r *UserRepo) GetByID(ctx context.Context, id int64) (*model.User, error) {
	var u model.User
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByUsername 按用户名查询。
func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// FindBySSO 按 SSO 提供方与外部 ID 查询。
func (r *UserRepo) FindBySSO(ctx context.Context, provider, externalID string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).
		Where("sso_provider = ? AND sso_external_id = ?", provider, externalID).
		First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Update 更新（全字段保存指定的列集合）。
func (r *UserRepo) Update(ctx context.Context, u *model.User, cols ...string) error {
	return r.db.WithContext(ctx).Model(u).Select(cols).Updates(u).Error
}

// Delete 删除用户。
func (r *UserRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.User{}, id).Error
}

// List 分页 + 关键字（用户名/邮箱）检索。
func (r *UserRepo) List(ctx context.Context, keyword string, offset, limit int) ([]model.User, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.User{})
	if keyword != "" {
		kw := "%" + keyword + "%"
		q = q.Where("username ILIKE ? OR email ILIKE ? OR display_name ILIKE ?", kw, kw, kw)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []model.User
	if err := q.Order("id ASC").Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// AdjustQuota 原子调整用户已用额度（delta 可为负）。
func (r *UserRepo) AdjustQuota(ctx context.Context, userID int64, delta int64) error {
	res := r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", userID).
		UpdateColumn("used_quota", gorm.Expr("used_quota + ?", delta))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("user %d not found", userID)
	}
	return nil
}

// ReserveQuota 用户级原子预扣：quota=0 表示不限额，仅累加已用。
func (r *UserRepo) ReserveQuota(ctx context.Context, userID int64, est int64) (bool, error) {
	res := r.db.WithContext(ctx).Exec(`
		UPDATE users SET used_quota = used_quota + ?
		WHERE id = ? AND status = ? AND (quota = 0 OR quota - used_quota >= ?)`,
		est, userID, model.UserStatusEnabled, est)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ChangeQuota 管理员调整总额度（设置绝对值）。
func (r *UserRepo) ChangeQuota(ctx context.Context, userID int64, quota int64) error {
	return r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", userID).
		UpdateColumn("quota", quota).Error
}

// SetPassword 重置密码哈希。
func (r *UserRepo) SetPassword(ctx context.Context, userID int64, hash string) error {
	return r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", userID).UpdateColumn("password_hash", hash).Error
}

// ListByIDs 批量查询。
func (r *UserRepo) ListByIDs(ctx context.Context, ids []int64) ([]model.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var users []model.User
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error
	return users, err
}

// ErrNotFound 统一的未找到错误。
var ErrNotFound = errors.New("record not found")
