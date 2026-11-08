// team-router：类 newapi 的 LLM API 网关。
// 支持 OpenAI Chat Completions / OpenAI Responses / Anthropic Messages 三协议接入，
// 多上游渠道（any-to-any）、按 token 计费、配额管理、请求日志（PG 按月分区）、
// 组织部门（闭包表）与 SSO 扩展机制。
package main

import (
	"os"

	"go.uber.org/fx"

	"github.com/snownd/team-router/internal/admin"
	"github.com/snownd/team-router/internal/auth"
	"github.com/snownd/team-router/internal/billing"
	"github.com/snownd/team-router/internal/config"
	"github.com/snownd/team-router/internal/crypto"
	"github.com/snownd/team-router/internal/database"
	"github.com/snownd/team-router/internal/logging"
	"github.com/snownd/team-router/internal/provider"
	"github.com/snownd/team-router/internal/relay"
	"github.com/snownd/team-router/internal/repository"
	"github.com/snownd/team-router/internal/server"
	"github.com/snownd/team-router/internal/sso"
	"github.com/snownd/team-router/internal/usage"
)

func main() {
	app := fx.New(
		config.Module,
		logging.Module,
		database.Module,
		repository.Module,
		crypto.Module,
		auth.Module,
		billing.Module,
		usage.Module,
		provider.Module,
		sso.Module,
		relay.Module,
		admin.Module,
		server.Module,
		fx.WithLogger(logging.FxLogger),
	)
	app.Run()
	os.Exit(0)
}
