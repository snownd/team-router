// Package admin 管理台与用户自助 API handlers。
package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/snownd/team-router/internal/auth"
	"github.com/snownd/team-router/internal/billing"
	"github.com/snownd/team-router/internal/config"
	"github.com/snownd/team-router/internal/crypto"
	"github.com/snownd/team-router/internal/provider"
	"github.com/snownd/team-router/internal/repository"
)

// Handlers 管理台 handler 聚合。
type Handlers struct {
	UserRepo     *repository.UserRepo
	DeptRepo     *repository.DepartmentRepo
	KeyRepo      *repository.APIKeyRepo
	ProviderRepo *repository.ProviderRepo
	PriceRepo    *repository.PriceRepo
	LogRepo      *repository.RequestLogRepo

	JWT    *auth.JWTManager
	Router *provider.Router
	Pricer *billing.Pricer
	Enc    *crypto.Encryptor
	Cfg    *config.Config
	Log    *zap.Logger
}

// NewHandlers 构造。
func NewHandlers(
	userRepo *repository.UserRepo,
	deptRepo *repository.DepartmentRepo,
	keyRepo *repository.APIKeyRepo,
	providerRepo *repository.ProviderRepo,
	priceRepo *repository.PriceRepo,
	logRepo *repository.RequestLogRepo,
	jwt *auth.JWTManager,
	router *provider.Router,
	pricer *billing.Pricer,
	enc *crypto.Encryptor,
	cfg *config.Config,
	zapLog *zap.Logger,
) *Handlers {
	return &Handlers{
		UserRepo: userRepo, DeptRepo: deptRepo, KeyRepo: keyRepo,
		ProviderRepo: providerRepo, PriceRepo: priceRepo, LogRepo: logRepo,
		JWT: jwt, Router: router, Pricer: pricer, Enc: enc, Cfg: cfg,
		Log: zapLog.Named("admin"),
	}
}

// ---- 通用辅助 ----

func pageParams(c *gin.Context) (offset, limit int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if size < 1 || size > 500 {
		size = 20
	}
	return (page - 1) * size, size
}

func idParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "非法 ID"}})
		return 0, false
	}
	return id, true
}

// parseTimeRange 解析时间范围（支持 RFC3339 与 YYYY-MM-DD），默认最近 7 天。
func parseTimeRange(c *gin.Context) (time.Time, time.Time) {
	start, end := time.Now().UTC().AddDate(0, 0, -7), time.Now().UTC()
	if s := c.Query("start"); s != "" {
		if t, err := parseTimeFlex(s); err == nil {
			start = t
		}
	}
	if e := c.Query("end"); e != "" {
		if t, err := parseTimeFlex(e); err == nil {
			end = t
		}
	}
	return start, end
}

func parseTimeFlex(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", s)
}

func userID(c *gin.Context) int64 {
	v, _ := c.Get(auth.CtxUserID)
	if id, ok := v.(int64); ok {
		return id
	}
	return 0
}

// Module fx 模块。
var Module = fx.Module("admin", fx.Provide(NewHandlers))
