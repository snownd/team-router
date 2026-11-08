// Package database 提供 gorm 连接、迁移、分区管理与种子数据。
package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/snownd/team-router/internal/config"
)

// NewDB 构造 gorm.DB（zap 日志桥接 + 连接池）。
func NewDB(cfg *config.Config, zapLog *zap.Logger) (*gorm.DB, error) {
	gl := newGormLogger(zapLog.Named("gorm"), cfg.Database.SlowQueryThreshold)
	return openDB(cfg.Database, gl)
}

// postgresOpen 构造 postgres dialector。
func postgresOpen(dsn string) gorm.Dialector {
	return gormpostgres.New(gormpostgres.Config{DSN: dsn})
}

// openDB 按配置打开连接并设置连接池。
func openDB(dc config.DatabaseConfig, gl gormlogger.Interface) (*gorm.DB, error) {
	db, err := gorm.Open(postgresOpen(dc.DSN), &gorm.Config{
		Logger: gl,
		// 事务由仓储层按需显式使用
		SkipDefaultTransaction: dc.SkipDefaultTransaction,
		PrepareStmt:            true,
	})
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(dc.MaxOpenConns)
	sqlDB.SetMaxIdleConns(dc.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(dc.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}

// zapGormLogger 将 gorm 日志桥接到 zap。
type zapGormLogger struct {
	log                  *zap.Logger
	slowThreshold        time.Duration
	ignoreRecordNotFound bool
}

func newGormLogger(log *zap.Logger, slow time.Duration) gormlogger.Interface {
	return &zapGormLogger{log: log, slowThreshold: slow, ignoreRecordNotFound: true}
}

func (l *zapGormLogger) LogMode(gormlogger.LogLevel) gormlogger.Interface { return l }

func (l *zapGormLogger) Info(_ context.Context, msg string, data ...any) {
	l.log.Sugar().Infof(msg, data...)
}

func (l *zapGormLogger) Warn(_ context.Context, msg string, data ...any) {
	l.log.Sugar().Warnf(msg, data...)
}

func (l *zapGormLogger) Error(_ context.Context, msg string, data ...any) {
	l.log.Sugar().Errorf(msg, data...)
}

func (l *zapGormLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()
	fields := []zap.Field{
		zap.Duration("elapsed", elapsed),
		zap.Int64("rows", rows),
	}
	switch {
	case err != nil && l.ignoreRecordNotFound && errors.Is(err, gorm.ErrRecordNotFound):
		return
	case err != nil:
		l.log.Error("sql error", append(fields, zap.Error(err), zap.String("sql", sql))...)
	case l.slowThreshold > 0 && elapsed > l.slowThreshold:
		l.log.Warn("slow sql", append(fields, zap.String("sql", sql))...)
	default:
		l.log.Debug("sql", append(fields, zap.String("sql", sql))...)
	}
}
