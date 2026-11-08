package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/snownd/team-router/internal/auth"
	"github.com/snownd/team-router/internal/config"
	"github.com/snownd/team-router/internal/database/dbtest"
	"github.com/snownd/team-router/internal/model"
	"github.com/snownd/team-router/internal/repository"
)

// TestNegativeCache：确定性失败结论短 TTL 负缓存——
// 无效 key / 禁用 key 的失败结论在 TTL 内不穿透数据库；
// InvalidateCache 后立即可见最新状态（需 TEST_PG_DSN）。
func TestNegativeCache(t *testing.T) {
	db := dbtest.Prepare(t)
	ctx := context.Background()
	keyRepo := repository.NewAPIKeyRepo(db)
	userRepo := repository.NewUserRepo(db)

	a := auth.NewAuthenticator(keyRepo, userRepo, config.Default())

	u := model.User{Username: "neg-user", Role: model.RoleUser, Status: 1, Quota: 1_000}
	if err := userRepo.Create(ctx, &u); err != nil {
		t.Fatal(err)
	}
	rawKey, _, hash := auth.GenerateKey()

	// 1) key 不存在 -> ErrInvalidKey（写入负缓存）
	if _, _, err := a.Authenticate(ctx, rawKey); !errors.Is(err, auth.ErrInvalidKey) {
		t.Fatalf("无效 key 应 ErrInvalidKey: %v", err)
	}
	// 2) 随后创建同 hash 的 key：负缓存 TTL 内仍应返回无效（不穿透 DB）
	k := model.APIKey{UserID: u.ID, KeyPrefix: "sk-neg", KeyHash: hash, Status: model.APIKeyStatusEnabled}
	if err := keyRepo.Create(ctx, &k); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Authenticate(ctx, rawKey); !errors.Is(err, auth.ErrInvalidKey) {
		t.Fatalf("负缓存 TTL 内应仍返回 ErrInvalidKey: %v", err)
	}
	// 3) 清缓存后立即可认证
	a.InvalidateCache()
	if _, _, err := a.Authenticate(ctx, rawKey); err != nil {
		t.Fatalf("清缓存后应认证成功: %v", err)
	}
	// 4) 禁用 -> ErrDisabled 负缓存；重新启用后 TTL 内仍失败，清缓存后恢复
	if err := keyRepo.Update(ctx, &model.APIKey{ID: k.ID, Status: model.APIKeyStatusDisabled}, "status"); err != nil {
		t.Fatal(err)
	}
	a.InvalidateCache()
	if _, _, err := a.Authenticate(ctx, rawKey); !errors.Is(err, auth.ErrDisabled) {
		t.Fatalf("禁用 key 应 ErrDisabled: %v", err)
	}
	if err := keyRepo.Update(ctx, &model.APIKey{ID: k.ID, Status: model.APIKeyStatusEnabled}, "status"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Authenticate(ctx, rawKey); !errors.Is(err, auth.ErrDisabled) {
		t.Fatalf("解禁后负缓存 TTL 内应仍 ErrDisabled: %v", err)
	}
	a.InvalidateCache()
	if _, _, err := a.Authenticate(ctx, rawKey); err != nil {
		t.Fatalf("解禁+清缓存后应认证成功: %v", err)
	}
}
