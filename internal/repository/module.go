package repository

import "go.uber.org/fx"

// Module fx 模块。
var Module = fx.Module("repository",
	fx.Provide(
		NewUserRepo,
		NewDepartmentRepo,
		NewAPIKeyRepo,
		NewProviderRepo,
		NewPriceRepo,
		NewRequestLogRepo,
		NewQuotaRepo,
	),
)
