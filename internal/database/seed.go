package database

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/auth"
	"github.com/snownd/team-router/internal/config"
	"github.com/snownd/team-router/internal/model"
)

// Seeder 启动时幂等种子数据：admin 用户、根部门兜底。
type Seeder struct {
	db  *gorm.DB
	cfg config.SeedConfig
	log *zap.Logger
}

// NewSeeder 构造。
func NewSeeder(db *gorm.DB, cfg *config.Config, log *zap.Logger) *Seeder {
	return &Seeder{db: db, cfg: cfg.Seed, log: log.Named("seed")}
}

// Run 幂等执行种子数据。
func (s *Seeder) Run(ctx context.Context) error {
	// 根部门（migration 中已建，这里兜底：任意 parent_id IS NULL 的行不存在时创建）
	var rootCount int64
	if err := s.db.WithContext(ctx).Model(&model.Department{}).
		Where("parent_id IS NULL").Count(&rootCount).Error; err != nil {
		return err
	}
	if rootCount == 0 {
		root := model.Department{Name: s.cfg.RootDepartmentName, Status: model.DeptStatusEnabled}
		if err := s.db.WithContext(ctx).Create(&root).Error; err != nil {
			return err
		}
		if err := s.db.WithContext(ctx).Exec(
			`INSERT INTO department_closure (ancestor_id, descendant_id, depth) VALUES (?, ?, 0) ON CONFLICT DO NOTHING`,
			root.ID, root.ID).Error; err != nil {
			return err
		}
		s.log.Info("root department created", zap.Int64("id", root.ID))
	}

	// admin 用户
	var admin model.User
	err := s.db.WithContext(ctx).Where("username = ?", s.cfg.AdminUsername).First(&admin).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		hash, err := auth.HashPassword(s.cfg.AdminPassword)
		if err != nil {
			return err
		}
		admin = model.User{
			Username:     s.cfg.AdminUsername,
			DisplayName:  "Administrator",
			Email:        s.cfg.AdminEmail,
			PasswordHash: hash,
			Role:         model.RoleAdmin,
			Status:       model.UserStatusEnabled,
			Quota:        1_000_000_000_000, // $1,000,000 初始额度，可后台调整
		}
		if err := s.db.WithContext(ctx).Create(&admin).Error; err != nil {
			return fmt.Errorf("create admin: %w", err)
		}
		s.log.Info("admin user created", zap.String("username", s.cfg.AdminUsername))
	case err != nil:
		return err
	}
	return nil
}
