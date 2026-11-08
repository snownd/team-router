package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/snownd/team-router/internal/admin"
	"github.com/snownd/team-router/internal/auth"
	"github.com/snownd/team-router/internal/relay"
	"github.com/snownd/team-router/internal/sso"
)

// RegisterRoutes 注册全部路由。
func RegisterRoutes(
	engine *gin.Engine,
	relaySvc *relay.Service,
	adminH *admin.Handlers,
	jwt *auth.JWTManager,
	ssoH *sso.Handler,
) {
	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// ===== OpenAI / Anthropic 兼容 API =====
	v1 := engine.Group("/v1")
	{
		v1.POST("/chat/completions", relaySvc.HandleChatCompletions)
		v1.POST("/responses", relaySvc.HandleResponses)
		v1.POST("/messages", relaySvc.HandleMessages)
		v1.GET("/models", relaySvc.HandleModels)
	}

	api := engine.Group("/api")
	{
		// 本地登录
		api.POST("/auth/login", adminH.Login)

		// SSO 扩展点（企业微信/飞书/钉钉接入后自动生效）
		ssoH.RegisterRoutes(api.Group("/auth/sso"))

		// 用户自助
		user := api.Group("/user", jwt.JWTAuth())
		{
			user.GET("/me", adminH.Me)
			user.GET("/keys", adminH.ListMyKeys)
			user.POST("/keys", adminH.CreateMyKey)
			user.DELETE("/keys/:id", adminH.DeleteMyKey)
			user.GET("/logs", adminH.MyLogs)
			user.GET("/stats", adminH.MyStats)
		}

		// 管理台
		adm := api.Group("/admin", jwt.JWTAuth(), auth.AdminRequired())
		{
			adm.GET("/users", adminH.ListUsers)
			adm.POST("/users", adminH.CreateUser)
			adm.GET("/users/:id", adminH.GetUser)
			adm.PUT("/users/:id", adminH.UpdateUser)
			adm.DELETE("/users/:id", adminH.DeleteUser)
			adm.PUT("/users/:id/password", adminH.SetUserPassword)
			adm.POST("/users/:id/quota", adminH.SetUserQuota)
			adm.PUT("/users/:id/departments", adminH.SetUserDepartments)

			adm.GET("/departments", adminH.DepartmentTree)
			adm.POST("/departments", adminH.CreateDepartment)
			adm.PUT("/departments/:id", adminH.UpdateDepartment)
			adm.POST("/departments/:id/move", adminH.MoveDepartment)
			adm.DELETE("/departments/:id", adminH.DeleteDepartment)
			adm.GET("/departments/:id/ancestors", adminH.DepartmentAncestors)
			adm.GET("/departments/:id/descendants", adminH.DepartmentDescendants)
			adm.GET("/departments/:id/users", adminH.DepartmentUsers)

			adm.GET("/keys", adminH.AdminListKeys)
			adm.POST("/keys", adminH.AdminCreateKey)
			adm.PUT("/keys/:id", adminH.AdminUpdateKey)
			adm.DELETE("/keys/:id", adminH.AdminDeleteKey)

			adm.GET("/providers", adminH.ListProviders)
			adm.GET("/providers/types", adminH.ProviderTypes)
			adm.POST("/providers", adminH.CreateProvider)
			adm.POST("/providers/fetch-models", adminH.FetchProviderModels)
			adm.PUT("/providers/:id", adminH.UpdateProvider)
			adm.DELETE("/providers/:id", adminH.DeleteProvider)

			adm.GET("/prices", adminH.ListPrices)
			adm.POST("/prices", adminH.UpsertPrice)
			adm.DELETE("/prices/:id", adminH.DeletePrice)

			adm.GET("/logs", adminH.AdminListLogs)
			adm.GET("/stats", adminH.AdminStats)
			adm.GET("/stats/ttft", adminH.TTFTStats)
		}
	}
}
