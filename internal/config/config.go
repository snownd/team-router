// Package config 加载 team-router 网关配置（yaml + 环境变量覆盖）。
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"go.uber.org/fx"
)

// Config 全局配置根节点。
type Config struct {
	Server     ServerConfig    `yaml:"server"`
	Database   DatabaseConfig  `yaml:"database"`
	Log        LogConfig       `yaml:"log"`
	Auth       AuthConfig      `yaml:"auth"`
	Crypto     CryptoConfig    `yaml:"crypto"`
	Relay      RelayConfig     `yaml:"relay"`
	Partitions PartitionConfig `yaml:"partitions"`
	Billing    BillingConfig   `yaml:"billing"`
	Seed       SeedConfig      `yaml:"seed"`
}

type ServerConfig struct {
	// Addr HTTP 监听地址，例如 ":8080"
	Addr string `yaml:"addr"`
	// GracefulShutdownTimeout 优雅退出等待在途请求的最长时间
	GracefulShutdownTimeout time.Duration `yaml:"graceful_shutdown_timeout"`
}

type DatabaseConfig struct {
	// DSN PostgreSQL 连接串
	DSN string `yaml:"dsn"`
	// MaxOpenConns / MaxIdleConns / ConnMaxLifetime 连接池参数
	MaxOpenConns           int           `yaml:"max_open_conns"`
	MaxIdleConns           int           `yaml:"max_idle_conns"`
	ConnMaxLifetime        time.Duration `yaml:"conn_max_lifetime"`
	SlowQueryThreshold     time.Duration `yaml:"slow_query_threshold"`
	SkipDefaultTransaction bool          `yaml:"skip_default_transaction"`
}

type LogConfig struct {
	// Level zap 级别：debug/info/warn/error
	Level string `yaml:"level"`
	// Dev true 时使用 console 编码，false 使用 JSON
	Dev bool `yaml:"dev"`
	// FilePath 非空时滚动写文件（lumberjack），为空写 stderr
	FilePath   string `yaml:"file_path"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxAgeDays int    `yaml:"max_age_days"`
	MaxBackups int    `yaml:"max_backups"`
}

type AuthConfig struct {
	JWTSecret string        `yaml:"jwt_secret"`
	TokenTTL  time.Duration `yaml:"token_ttl"`
	// APICacheSeconds API Key / 用户信息的进程内缓存秒数
	APICacheSeconds int `yaml:"api_cache_seconds"`
}

type CryptoConfig struct {
	// MasterKey 32 字节 hex，用于加密 provider 上游密钥；留空则使用内置开发默认值（生产必须显式配置）
	MasterKey string `yaml:"master_key"`
}

type RelayConfig struct {
	// ConnectTimeout TCP 建连超时
	ConnectTimeout time.Duration `yaml:"connect_timeout"`
	// UpstreamTimeout 上游首包（响应头）超时：非流式请求的整体处理上限；
	// 流式请求建连+上游首字节的共同上限，流建立后由 IdleStreamTimeout 接管
	UpstreamTimeout time.Duration `yaml:"upstream_timeout"`
	// MaxRetries 跨渠道重试次数（不含首次）
	MaxRetries int `yaml:"max_retries"`
	// DefaultMaxTokens 请求未指定 max_tokens 时用于配额预估的默认值
	DefaultMaxTokens int `yaml:"default_max_tokens"`
	// IdleStreamTimeout 流式响应两次上游事件之间的最大空闲时间，超时中断该次流式转发
	IdleStreamTimeout time.Duration `yaml:"idle_stream_timeout"`
	// ChannelQueueTimeout 渠道并发已满时排队等待上限；超时直接向客户端返回 429
	// （不跨渠道重试，保持各账户并发隔离）。0 使用默认 60s。
	ChannelQueueTimeout time.Duration `yaml:"channel_queue_timeout"`
	// StickySessionTTL 粘性路由（多轮会话命中相同上游账户）的会话绑定 TTL（滑动续期）。
	// 0 使用默认 30m；显式负值关闭粘性路由。
	StickySessionTTL time.Duration `yaml:"sticky_session_ttl"`
}

type PartitionConfig struct {
	// PrecreateMonths 预建未来分区的月数
	PrecreateMonths int `yaml:"precreate_months"`
	// RetentionMonths 保留月数，0 表示永久保留
	RetentionMonths int `yaml:"retention_months"`
	// MaintenanceInterval 分区维护巡检周期
	MaintenanceInterval time.Duration `yaml:"maintenance_interval"`
}

type BillingConfig struct {
	// AllowUnknownPricing 模型无价格配置时是否放行（按 0 费用计，标记 estimated）
	AllowUnknownPricing bool `yaml:"allow_unknown_pricing"`
	// PriceCacheSeconds 价格缓存秒数
	PriceCacheSeconds int `yaml:"price_cache_seconds"`
}

type SeedConfig struct {
	AdminUsername string `yaml:"admin_username"`
	AdminPassword string `yaml:"admin_password"`
	AdminEmail    string `yaml:"admin_email"`
	// RootDepartmentName 根部门名称
	RootDepartmentName string `yaml:"root_department_name"`
}

// Default 返回带默认值的配置。
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Addr:                    ":8080",
			GracefulShutdownTimeout: 30 * time.Second,
		},
		Database: DatabaseConfig{
			DSN:                    "host=localhost port=5432 user=postgres password=postgres dbname=team_router sslmode=disable",
			MaxOpenConns:           50,
			MaxIdleConns:           10,
			ConnMaxLifetime:        time.Hour,
			SlowQueryThreshold:     500 * time.Millisecond,
			SkipDefaultTransaction: true,
		},
		Log: LogConfig{
			Level:      "info",
			Dev:        true,
			MaxSizeMB:  100,
			MaxAgeDays: 30,
			MaxBackups: 10,
		},
		Auth: AuthConfig{
			JWTSecret:       "change-me-in-production",
			TokenTTL:        24 * time.Hour,
			APICacheSeconds: 60,
		},
		Crypto: CryptoConfig{},
		Relay: RelayConfig{
			ConnectTimeout:      10 * time.Second,
			UpstreamTimeout:     10 * time.Minute,
			MaxRetries:          3,
			DefaultMaxTokens:    8192,
			IdleStreamTimeout:   2 * time.Minute,
			ChannelQueueTimeout: 60 * time.Second,
			StickySessionTTL:    30 * time.Minute,
		},
		Partitions: PartitionConfig{
			PrecreateMonths:     3,
			RetentionMonths:     0,
			MaintenanceInterval: 12 * time.Hour,
		},
		Billing: BillingConfig{
			AllowUnknownPricing: true,
			PriceCacheSeconds:   60,
		},
		Seed: SeedConfig{
			AdminUsername:      "admin",
			AdminPassword:      "admin123",
			AdminEmail:         "admin@example.com",
			RootDepartmentName: "总部",
		},
	}
}

// Load 依次：默认值 -> yaml 文件 -> 环境变量覆盖。
func Load() (*Config, error) {
	cfg := Default()

	if path := os.Getenv("TR_CONFIG"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config file %s: %w", path, err)
		}
	}

	// 环境变量覆盖（部署友好）
	overrideStr(cfg)
	overrideDur(cfg)

	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func overrideStr(cfg *Config) {
	pairs := map[string]*string{
		"TR_SERVER_ADDR":         &cfg.Server.Addr,
		"TR_DATABASE_DSN":        &cfg.Database.DSN,
		"TR_LOG_LEVEL":           &cfg.Log.Level,
		"TR_LOG_FILE":            &cfg.Log.FilePath,
		"TR_AUTH_JWT_SECRET":     &cfg.Auth.JWTSecret,
		"TR_CRYPTO_MASTER_KEY":   &cfg.Crypto.MasterKey,
		"TR_SEED_ADMIN_USERNAME": &cfg.Seed.AdminUsername,
		"TR_SEED_ADMIN_PASSWORD": &cfg.Seed.AdminPassword,
	}
	for env, dst := range pairs {
		if v := os.Getenv(env); v != "" {
			*dst = v
		}
	}
	if v := os.Getenv("TR_LOG_DEV"); v != "" {
		cfg.Log.Dev = v == "true" || v == "1"
	}
	if v := os.Getenv("TR_BILLING_ALLOW_UNKNOWN_PRICING"); v != "" {
		cfg.Billing.AllowUnknownPricing = v == "true" || v == "1"
	}
}

func overrideDur(cfg *Config) {
	durs := map[string]*time.Duration{
		"TR_AUTH_TOKEN_TTL":        &cfg.Auth.TokenTTL,
		"TR_RELAY_TIMEOUT":         &cfg.Relay.UpstreamTimeout,
		"TR_RELAY_CONNECT_TIMEOUT": &cfg.Relay.ConnectTimeout,
	}
	for env, dst := range durs {
		if v := os.Getenv(env); v != "" {
			if d, err := time.ParseDuration(v); err == nil {
				*dst = d
			}
		}
	}
}

func validate(cfg *Config) error {
	if cfg.Server.Addr == "" {
		return fmt.Errorf("server.addr 不能为空")
	}
	if cfg.Database.DSN == "" {
		return fmt.Errorf("database.dsn 不能为空")
	}
	if cfg.Auth.JWTSecret == "" {
		return fmt.Errorf("auth.jwt_secret 不能为空")
	}
	if cfg.Relay.MaxRetries < 0 {
		return fmt.Errorf("relay.max_retries 不能为负")
	}
	return nil
}

// Module fx 模块。
var Module = fx.Provide(Load)
