package database

import (
	"database/sql"
	"fmt"

	migrate "github.com/golang-migrate/migrate/v4"
	pgxv5 "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"

	"github.com/snownd/team-router/internal/config"
	"github.com/snownd/team-router/migrations"
)

// Migrator 版本化 SQL 迁移器（golang-migrate，embed 文件系统）。
type Migrator struct {
	dsn string
	log *zap.Logger
}

// NewMigrator 构造迁移器（fx 注入用）。独立于 gorm 连接，使用 pgx 标准库驱动。
func NewMigrator(cfg *config.Config, log *zap.Logger) *Migrator {
	return NewMigratorRaw(cfg.Database.DSN, log)
}

// NewMigratorRaw 直接以 DSN 构造迁移器。
func NewMigratorRaw(dsn string, log *zap.Logger) *Migrator {
	return &Migrator{dsn: dsn, log: log.Named("migrate")}
}

// Up 执行所有待应用的迁移；无 dirty 修复逻辑（失败请人工介入）。
func (m *Migrator) Up() error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("open migrations: %w", err)
	}
	defer src.Close()

	sqlDB, err := sql.Open("pgx", m.dsn)
	if err != nil {
		return fmt.Errorf("open migrate db: %w", err)
	}
	defer sqlDB.Close()

	driver, err := pgxv5.WithInstance(sqlDB, &pgxv5.Config{})
	if err != nil {
		return fmt.Errorf("init migrate driver: %w", err)
	}

	mg, err := migrate.NewWithInstance("iofs", src, "pgx", driver)
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer mg.Close()

	if err := mg.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("apply migrations: %w", err)
	}
	version, dirty, _ := mg.Version()
	m.log.Info("migrations applied", zap.Uint64("version", uint64(version)), zap.Bool("dirty", dirty))
	return nil
}
