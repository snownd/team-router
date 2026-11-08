package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/model"
)

// 配额不足哨兵错误（key/user 两级区分，供上层生成差异化文案）。
var (
	// ErrKeyQuotaInsufficient API Key 层额度不足。
	ErrKeyQuotaInsufficient = errors.New("api key 额度不足")
	// ErrUserQuotaInsufficient 用户层额度不足。
	ErrUserQuotaInsufficient = errors.New("用户额度不足")
)

// QuotaRepo 配额跨表事务操作：API Key 与用户两级额度必须同进同退，
// 任一侧失败整体回滚，保证多租户账目最终一致。
type QuotaRepo struct{ db *gorm.DB }

// NewQuotaRepo 构造。
func NewQuotaRepo(db *gorm.DB) *QuotaRepo { return &QuotaRepo{db: db} }

// Reserve 预扣：单事务内先扣 key 再扣 user；user 侧额度不足或不可用时整体回滚。
// quota=0 表示该级不限额（仅累加已用）。
// 返回 (false, ErrKeyQuotaInsufficient|ErrUserQuotaInsufficient) 表示额度不足；
// 返回 (false, 其他 error) 表示数据库故障。
func (r *QuotaRepo) Reserve(ctx context.Context, keyID, userID int64, est int64) (bool, error) {
	if est <= 0 {
		return true, nil
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec(`
			UPDATE api_keys SET used_quota = used_quota + ?
			WHERE id = ? AND status = ? AND (quota = 0 OR quota - used_quota >= ?)`,
			est, keyID, model.APIKeyStatusEnabled, est)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrKeyQuotaInsufficient
		}
		res = tx.Exec(`
			UPDATE users SET used_quota = used_quota + ?
			WHERE id = ? AND status = ? AND (quota = 0 OR quota - used_quota >= ?)`,
			est, userID, model.UserStatusEnabled, est)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrUserQuotaInsufficient
		}
		return nil
	})
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, ErrKeyQuotaInsufficient), errors.Is(err, ErrUserQuotaInsufficient):
		return false, err
	default:
		return false, err
	}
}

// Settle 结算：在单事务内将差额 delta（actual - reserved，可为负，负值即退还）
// 同时记入 API Key 与用户已用额度；任一更新失败整体回滚。
func (r *QuotaRepo) Settle(ctx context.Context, keyID, userID, delta int64) error {
	if delta == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.APIKey{}).Where("id = ?", keyID).
			UpdateColumn("used_quota", gorm.Expr("used_quota + ?", delta))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("api key %d not found", keyID)
		}
		res = tx.Model(&model.User{}).Where("id = ?", userID).
			UpdateColumn("used_quota", gorm.Expr("used_quota + ?", delta))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("user %d not found", userID)
		}
		return nil
	})
}
