// Package dbtest 为数据库集成测试提供隔离环境：每个测试（或测试包）使用独立的
// 临时数据库，自动迁移 + 建分区，测试结束自动清理。
//
// 依赖环境变量 TEST_PG_DSN（未设置时调用方应 t.Skip）。
package dbtest

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"net/url"
	"regexp"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/config"
	"github.com/snownd/team-router/internal/database"

	// pgx stdlib driver（migrator 需要）
	_ "github.com/jackc/pgx/v5/stdlib"
)

// DSN 返回管理库连接串；未设置 TEST_PG_DSN 时跳过测试。
func DSN(t *testing.T) string {
	t.Helper()
	dsn := testingEnv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN 未设置，跳过数据库测试")
	}
	return dsn
}

func testingEnv(k string) string { return getEnv(k) }

// Prepare 创建独立临时数据库并完成迁移 + 分区初始化，返回 gorm.DB。
func Prepare(t *testing.T) *gorm.DB {
	t.Helper()
	adminDSN := DSN(t)

	dbName := fmt.Sprintf("tr_test_%d", rand.New(rand.NewSource(time.Now().UnixNano())).Int63())
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Fatalf("open admin: %v", err)
	}
	if _, err := admin.Exec(fmt.Sprintf(`CREATE DATABASE %s`, dbName)); err != nil {
		t.Fatalf("create test db: %v", err)
	}
	admin.Close()

	testDSN := rewriteDSNDatabase(adminDSN, dbName)

	db, err := database.OpenTestDB(testDSN)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}

	// 清理：先关连接再删库
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		drop, err := sql.Open("pgx", adminDSN)
		if err == nil {
			_, _ = drop.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, dbName))
			_ = drop.Close()
		}
	})

	if err := database.NewMigratorRaw(testDSN, zap.NewNop()).Up(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := database.NewPartitionManagerWith(db, config.PartitionConfig{
		PrecreateMonths: 3, MaintenanceInterval: time.Hour,
	}, zap.NewNop()).EnsureNow(context.Background()); err != nil {
		t.Fatalf("ensure partitions: %v", err)
	}
	return db
}

// rewriteDSNDatabase 将 DSN 中的库名替换为 testDB（支持 key=value 与 URL 两种形态）。
func rewriteDSNDatabase(dsn, dbName string) string {
	// postgres://user:pass@host:port/db?...
	if u, err := url.Parse(dsn); err == nil && u.Scheme != "" && u.Path != "" {
		u.Path = "/" + dbName
		return u.String()
	}
	// host=... dbname=xxx ...
	re := regexp.MustCompile(`(?i)(\bdbname=)[^\s]+`)
	if re.MatchString(dsn) {
		return re.ReplaceAllString(dsn, "${1}"+dbName)
	}
	return dsn + " dbname=" + dbName
}
