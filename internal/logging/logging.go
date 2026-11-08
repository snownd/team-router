// Package logging 提供全局 zap logger 及 fx 集成。
package logging

import (
	"fmt"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/snownd/team-router/internal/config"
)

// New 依据配置构造 *zap.Logger。由 fx 在依赖图最底层提供。
func New(cfg *config.Config) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(cfg.Log.Level)); err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", cfg.Log.Level, err)
	}

	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	var encoder zapcore.Encoder
	if cfg.Log.Dev {
		encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	}

	var sink zapcore.WriteSyncer = zapcore.AddSync(stderr())
	if cfg.Log.FilePath != "" {
		sink = zapcore.AddSync(&lumberjack.Logger{
			Filename:   cfg.Log.FilePath,
			MaxSize:    max(cfg.Log.MaxSizeMB, 10),
			MaxAge:     cfg.Log.MaxAgeDays,
			MaxBackups: cfg.Log.MaxBackups,
			Compress:   true,
		})
	}

	core := zapcore.NewCore(encoder, sink, level)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	return logger.Named("gateway"), nil
}

// FxLogger 将 fx 自身事件日志接入 zap。
func FxLogger(l *zap.Logger) fxevent.Logger {
	return &fxevent.ZapLogger{Logger: l.Named("fx")}
}

// Module fx 模块：提供 *zap.Logger。
var Module = fx.Provide(New)

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
