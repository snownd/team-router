package database

import (
	"context"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/config"
	"github.com/snownd/team-router/internal/crypto"
)

// RegisterLifecycle 数据库生命周期：启动时 迁移 -> 分区巡检 -> 种子，退出时停巡检、关连接。
func RegisterLifecycle(
	lc fx.Lifecycle,
	cfg *config.Config,
	log *zap.Logger,
	db *gorm.DB,
	mig *Migrator,
	pm *PartitionManager,
	seeder *Seeder,
) {
	var loopCancel context.CancelFunc

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := mig.Up(); err != nil {
				return err
			}
			if err := pm.EnsureNow(ctx); err != nil {
				return err
			}
			if err := seeder.Run(ctx); err != nil {
				return err
			}
			if crypto.IsDefaultKey(cfg.Crypto.MasterKey) {
				log.Warn("crypto.master_key 未配置，使用开发默认密钥加密 provider 上游密钥，生产环境必须显式配置")
			}
			loopCtx, cancel := context.WithCancel(context.Background())
			loopCancel = cancel
			pm.StartLoop(loopCtx)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if loopCancel != nil {
				loopCancel()
			}
			if sqlDB, err := db.DB(); err == nil {
				return sqlDB.Close()
			}
			return nil
		},
	})
}

// Module fx 模块。
var Module = fx.Module("database",
	fx.Provide(NewDB, NewMigrator, NewPartitionManager, NewSeeder),
	fx.Invoke(RegisterLifecycle),
)
