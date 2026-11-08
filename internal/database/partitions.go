package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/config"
)

// PartitionManager request_logs 按月分区管理器：预建未来分区、按保留策略清理过期分区。
// 父表上的索引（PG11+）会自动应用到新建分区，无需单独建索引。
type PartitionManager struct {
	db  *gorm.DB
	cfg config.PartitionConfig
	log *zap.Logger
}

// NewPartitionManager 构造（fx 注入用，从 Config 提取分区配置）。
func NewPartitionManager(db *gorm.DB, cfg *config.Config, log *zap.Logger) *PartitionManager {
	return newPartitionManager(db, cfg.Partitions, log)
}

func newPartitionManager(db *gorm.DB, cfg config.PartitionConfig, log *zap.Logger) *PartitionManager {
	return &PartitionManager{db: db, cfg: cfg, log: log.Named("partitions")}
}

// EnsureNow 立即执行一次巡检（幂等）。
func (pm *PartitionManager) EnsureNow(ctx context.Context) error {
	if err := pm.ensureFuture(ctx); err != nil {
		return err
	}
	return pm.dropExpired(ctx)
}

// StartLoop 启动后台巡检循环，返回停止函数。
func (pm *PartitionManager) StartLoop(ctx context.Context) {
	interval := pm.cfg.MaintenanceInterval
	if interval <= 0 {
		interval = 12 * time.Hour
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := pm.EnsureNow(ctx); err != nil {
					pm.log.Error("partition maintenance failed", zap.Error(err))
				}
			}
		}
	}()
}

// ensureFuture 预建未来 N 个月分区（含当月）。
func (pm *PartitionManager) ensureFuture(ctx context.Context) error {
	n := pm.cfg.PrecreateMonths
	if n < 0 {
		n = 0
	}
	for i := 0; i <= n; i++ {
		start := monthStart(time.Now().UTC().AddDate(0, i, 0))
		end := start.AddDate(0, 1, 0)
		name := partitionName(start)
		sql := fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s PARTITION OF request_logs FOR VALUES FROM ('%s') TO ('%s')`,
			name, start.Format(time.RFC3339), end.Format(time.RFC3339),
		)
		if err := pm.db.WithContext(ctx).Exec(sql).Error; err != nil {
			// 并发创建时的唯一冲突视为成功
			if !strings.Contains(err.Error(), "already exists") &&
				!strings.Contains(err.Error(), "42P07") &&
				!strings.Contains(err.Error(), "duplicate") {
				return fmt.Errorf("create partition %s: %w", name, err)
			}
		} else {
			pm.log.Info("partition ensured", zap.String("partition", name))
		}
	}
	return nil
}

// dropExpired 按保留月数删除过期分区。
func (pm *PartitionManager) dropExpired(ctx context.Context) error {
	if pm.cfg.RetentionMonths <= 0 {
		return nil // 永久保留
	}
	cutoff := monthStart(time.Now().UTC().AddDate(0, -pm.cfg.RetentionMonths, 0))

	var names []string
	if err := pm.db.WithContext(ctx).Raw(`
		SELECT c.relname FROM pg_class c
		JOIN pg_inherits i ON i.inhrelid = c.oid
		WHERE i.inhparent = 'request_logs'::regclass AND c.relname ~ '^request_logs_y[0-9]{4}m[0-9]{2}$'
	`).Scan(&names).Error; err != nil {
		return err
	}

	for _, name := range names {
		if name == "request_logs_default" {
			continue
		}
		pt, err := parsePartitionMonth(name)
		if err != nil {
			continue
		}
		if pt.Before(cutoff) {
			if err := pm.db.WithContext(ctx).Exec(fmt.Sprintf(`DROP TABLE IF EXISTS %s`, name)).Error; err != nil {
				pm.log.Error("drop partition failed", zap.String("partition", name), zap.Error(err))
			} else {
				pm.log.Info("partition dropped", zap.String("partition", name))
			}
		}
	}
	return nil
}

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func partitionName(start time.Time) string {
	return fmt.Sprintf("request_logs_y%dm%02d", start.Year(), int(start.Month()))
}

func parsePartitionMonth(name string) (time.Time, error) {
	var y, m int
	if _, err := fmt.Sscanf(name, "request_logs_y%dm%02d", &y, &m); err != nil {
		return time.Time{}, err
	}
	return time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC), nil
}
