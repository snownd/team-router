// Package sso 定义 SSO 扩展机制：企业微信 / 飞书 / 钉钉等身份源通过实现
// Provider 接口接入，第一期不提供具体实现。
//
// 接入方式（见 docs/extending-sso.md）：
//  1. 实现 Provider 接口（AuthorizeURL / Exchange）；
//  2. 在 fx 中通过 sso.Register 注册到 Registry；
//  3. 通用路由 /api/auth/sso/{name}/login 与 /callback 自动生效；
//  4. 首次登录自动建号（JIT），并在 users 表记录 sso_provider / sso_external_id。
package sso

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/snownd/team-router/internal/auth"
	"github.com/snownd/team-router/internal/model"
)

// Identity SSO 换取到的用户身份。
type Identity struct {
	ExternalID  string // SSO 平台唯一 ID
	Email       string
	DisplayName string
	// Extra 附加信息（部门、手机号等）
	Extra map[string]string
}

// Provider SSO 提供方扩展接口。
type Provider interface {
	// Name 标识（路由参数）：wecom | feishu | dingtalk | ...
	Name() string
	// AuthorizeURL 生成跳转到 SSO 平台的授权地址；state 用于防 CSRF。
	AuthorizeURL(state string) string
	// Exchange 用回调 code 换取身份。
	Exchange(ctx context.Context, code string) (*Identity, error)
}

// Registry SSO 提供方注册表。
type Registry struct {
	providers map[string]Provider
}

// NewRegistry 构造（fx 注入空实现列表，后续新增 provider 时提供其构造即可）。
func NewRegistry(providers []Provider, log *zap.Logger) *Registry {
	r := &Registry{providers: map[string]Provider{}}
	for _, p := range providers {
		r.providers[p.Name()] = p
	}
	log.Info("sso registry initialized", zap.Int("providers", len(r.providers)))
	return r
}

// Get 获取 provider。
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// Names 已注册 provider 名。
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.providers))
	for n := range r.providers {
		out = append(out, n)
	}
	return out
}

// Handler 通用 SSO 路由处理器。
type Handler struct {
	reg   *Registry
	jwt   *auth.JWTManager
	users UserProvisioner
	log   *zap.Logger
}

// UserProvisioner 用户开通接口（由仓储层实现，解耦依赖方向）。
type UserProvisioner interface {
	// FindBySSO 按 SSO 身份查用户
	FindBySSO(ctx context.Context, provider, externalID string) (*model.User, error)
	// ProvisionOrCreate JIT 建号或更新资料；返回用户
	ProvisionOrCreate(ctx context.Context, providerName string, id *Identity) (*model.User, error)
}

// NewHandler 构造。
func NewHandler(reg *Registry, jwt *auth.JWTManager, users UserProvisioner, log *zap.Logger) *Handler {
	return &Handler{reg: reg, jwt: jwt, users: users, log: log.Named("sso")}
}

// RegisterRoutes 注册通用 SSO 路由。
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/:name/login", h.login)
	rg.GET("/:name/callback", h.callback)
	rg.POST("/:name/callback", h.callback)
}

func (h *Handler) login(c *gin.Context) {
	name := c.Param("name")
	p, ok := h.reg.Get(name)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": fmt.Sprintf("SSO 提供方 %q 未接入，已注册: %v", name, h.reg.Names()),
		}})
		return
	}
	state := "sso-" + name // v1 简化；生产应随机生成并缓存校验
	c.Redirect(http.StatusFound, p.AuthorizeURL(state))
}

func (h *Handler) callback(c *gin.Context) {
	name := c.Param("name")
	p, ok := h.reg.Get(name)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "SSO 提供方未接入"}})
		return
	}
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "缺少 code 参数"}})
		return
	}
	id, err := p.Exchange(c.Request.Context(), code)
	if err != nil {
		h.log.Warn("sso exchange failed", zap.String("provider", name), zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": "SSO 授权失败: " + err.Error()}})
		return
	}
	user, err := h.users.ProvisionOrCreate(c.Request.Context(), name, id)
	if err != nil {
		h.JSON500(c, err)
		return
	}
	token, err := h.jwt.Issue(user.ID, user.Username, user.Role)
	if err != nil {
		h.JSON500(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"access_token": token,
		"user":         gin.H{"id": user.ID, "username": user.Username, "role": user.Role},
	})
}

func (h *Handler) JSON500(c *gin.Context, err error) {
	h.log.Error("sso internal error", zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "内部错误"}})
}

// Module fx 模块。空 provider 列表 = 第一期无 SSO 实现；
// 接入新身份源时使用：
//
//	fx.Provide(fx.Annotate(NewWeComProvider, fx.As(new(Provider)), fx.ResultTags(`group:"sso_providers"`)))
var Module = fx.Module("sso",
	fx.Provide(
		// 兜底空组，保证无任何 SSO 实现时依赖图完整
		fx.Annotate(emptySSOProviders, fx.ResultTags(`group:"sso_providers"`)),
		fx.Annotate(NewRegistry, fx.ParamTags(`group:"sso_providers"`)),
		NewHandler,
		fx.Annotate(NewUserProvisioner, fx.As(new(UserProvisioner))),
	),
)

// emptySSOProviders 返回空 provider 集（占位组成员）。
func emptySSOProviders() []Provider { return []Provider{} }
