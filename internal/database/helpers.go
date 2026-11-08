package database

import (
	"go.uber.org/zap"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/snownd/team-router/internal/config"
)

// OpenTestDB 以静默日志构造 gorm.DB（测试/工具用）。
func OpenTestDB(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgresOpen(dsn), &gorm.Config{
		Logger:                 gormlogger.Discard,
		SkipDefaultTransaction: true,
	})
}

// NewPartitionManagerWith 以显式分区配置构造（测试/工具用）。
func NewPartitionManagerWith(db *gorm.DB, cfg config.PartitionConfig, log *zap.Logger) *PartitionManager {
	return newPartitionManager(db, cfg, log)
}
