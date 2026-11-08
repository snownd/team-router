// Package server gin 引擎、中间件与 HTTP 生命周期。
package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/snownd/team-router/internal/config"
)

// NewEngine 构造 gin 引擎（含中间件）。
func NewEngine(cfg *config.Config, zapLog *zap.Logger) *gin.Engine {
	if !cfg.Log.Dev {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	engine.Use(
		RequestID(),
		ZapAccessLog(zapLog.Named("http")),
		gin.Recovery(),
	)
	return engine
}

// NewServer 构造 http.Server。
func NewServer(cfg *config.Config, engine *gin.Engine) *http.Server {
	return &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           engine,
		ReadHeaderTimeout: 30 * time.Second,
	}
}

// RegisterLifecycle HTTP 生命周期（优雅退出）。
func RegisterLifecycle(lc fx.Lifecycle, cfg *config.Config, srv *http.Server, log *zap.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			lnErr := make(chan error, 1)
			go func() {
				log.Info("http server listening", zap.String("addr", cfg.Server.Addr))
				lnErr <- srv.ListenAndServe()
			}()
			select {
			case err := <-lnErr:
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					return err
				}
			case <-time.After(100 * time.Millisecond):
				// 正常启动
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdownTimeout)
			defer cancel()
			log.Info("http server shutting down")
			return srv.Shutdown(shutdownCtx)
		},
	})
}

// RequestID 请求 ID 中间件：生成 X-Request-Id，贯穿 zap 与 request_logs。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-Id")
		if id == "" {
			id = newRequestID()
		}
		c.Set("request_id", id)
		c.Header("X-Request-Id", id)
		c.Next()
	}
}

// ZapAccessLog zap 访问日志中间件。
func ZapAccessLog(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("access",
			zap.String("request_id", c.GetString("request_id")),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Int64("latency_ms", time.Since(start).Milliseconds()),
			zap.Int64("body_bytes", c.Request.ContentLength),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
		)
	}
}

func newRequestID() string {
	// 时间戳前缀 + 进程内计数，足够网关场景
	return time.Now().UTC().Format("20060102T150405") + "-" + randomHex(8)
}

// Module fx 模块。
var Module = fx.Module("server",
	fx.Provide(NewEngine, NewServer),
	fx.Invoke(RegisterLifecycle, RegisterRoutes),
)
