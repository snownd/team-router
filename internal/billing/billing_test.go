package billing

import (
	"context"
	"encoding/json"
	"testing"

	"go.uber.org/zap"

	"github.com/snownd/team-router/internal/database/dbtest"
	"github.com/snownd/team-router/internal/model"
	"github.com/snownd/team-router/internal/protocol"
	"github.com/snownd/team-router/internal/repository"
)

func TestCompute(t *testing.T) {
	price := &model.ModelPrice{
		InputPerM:      3_000_000, // $3/M
		OutputPerM:     15_000_000,
		CacheReadPerM:  300_000,
		CacheWritePerM: 3_750_000,
		Currency:       "USD",
	}
	u := protocol.Usage{
		InputTokens:      1_000_000,
		OutputTokens:     1_000_000,
		CacheReadTokens:  1_000_000,
		CacheWriteTokens: 1_000_000,
	}
	cost := Compute(u, price, "claude", "anthropic")
	want := int64(3_000_000 + 15_000_000 + 300_000 + 3_750_000)
	if cost.QuotaCost != want {
		t.Errorf("cost = %d, want %d", cost.QuotaCost, want)
	}
	var d map[string]any
	_ = json.Unmarshal(cost.Detail, &d)
	parts := d["parts"].(map[string]any)
	if parts["cache_read"].(float64) != 300_000 {
		t.Errorf("cache_read 分项错误: %v", parts)
	}
	if !cost.Priced {
		t.Errorf("应标记 priced")
	}
}

func TestComputeUnpriced(t *testing.T) {
	cost := Compute(protocol.Usage{InputTokens: 100}, nil, "m", "openai")
	if cost.QuotaCost != 0 || cost.Priced {
		t.Errorf("未定价应计 0 费用")
	}
}

func TestEstimateReserve(t *testing.T) {
	price := &model.ModelPrice{InputPerM: 300_000, OutputPerM: 3_000_000}
	// 1000 输入 + 8000 输出（默认）* 上界单价 3M
	est := EstimateReserve(1000, 8000, price)
	want := int64(9000) * 3_000_000 / 1_000_000
	if est != want {
		t.Errorf("est = %d, want %d", est, want)
	}
	if EstimateReserve(100, 100, nil) != 0 {
		t.Errorf("无定价应预估 0")
	}
}

func TestComputeRoundingDown(t *testing.T) {
	price := &model.ModelPrice{InputPerM: 2_500_000}
	// 100 token * $2.5/M = 250 microUSD（精确整除）
	cost := Compute(protocol.Usage{InputTokens: 100}, price, "gpt-4o", "openai")
	if cost.QuotaCost != 250 {
		t.Errorf("cost = %d, want 250", cost.QuotaCost)
	}
}

func TestMergePricesUpper(t *testing.T) {
	a := &model.ModelPrice{InputPerM: 100, OutputPerM: 900, CacheReadPerM: 10, CacheWritePerM: 20}
	b := &model.ModelPrice{InputPerM: 300, OutputPerM: 500, CacheReadPerM: 0, CacheWritePerM: 50}
	m := MergePricesUpper([]*model.ModelPrice{a, b})
	if m == nil {
		t.Fatal("合并结果不应为 nil")
	}
	if m.InputPerM != 300 || m.OutputPerM != 900 || m.CacheReadPerM != 10 || m.CacheWritePerM != 50 {
		t.Errorf("各分项应取上界: %+v", m)
	}
	// 含 nil 与全 nil
	if MergePricesUpper([]*model.ModelPrice{nil, b}).InputPerM != 300 {
		t.Error("应跳过 nil 价目")
	}
	if MergePricesUpper(nil) != nil || MergePricesUpper([]*model.ModelPrice{nil}) != nil {
		t.Error("全 nil 应返回 nil")
	}
	// 不应修改入参
	if a.OutputPerM != 900 {
		t.Error("合并不应篡改入参")
	}
}

// TestQuotaReserveAndSettleAtomic：预扣/结算两级额度同进同退（需 TEST_PG_DSN）。
func TestQuotaReserveAndSettleAtomic(t *testing.T) {
	db := dbtest.Prepare(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepo(db)
	keyRepo := repository.NewAPIKeyRepo(db)

	u := model.User{Username: "q-user", Role: model.RoleUser, Status: 1, Quota: 100}
	if err := userRepo.Create(ctx, &u); err != nil {
		t.Fatal(err)
	}
	k := model.APIKey{UserID: u.ID, KeyPrefix: "sk-q", KeyHash: "hash-q", Status: 1, Quota: 10_000}
	if err := keyRepo.Create(ctx, &k); err != nil {
		t.Fatal(err)
	}

	q := NewQuota(repository.NewQuotaRepo(db), zap.NewNop())

	// 用户额度不足：key 已扣部分必须整体回滚（两级一致）
	if err := q.Reserve(ctx, k.ID, u.ID, 400); err == nil {
		t.Fatal("用户额度不足应失败")
	}
	k2, _ := keyRepo.GetByID(ctx, k.ID)
	u2, _ := userRepo.GetByID(ctx, u.ID)
	if k2.UsedQuota != 0 || u2.UsedQuota != 0 {
		t.Fatalf("预扣失败必须整体回滚: key=%d user=%d", k2.UsedQuota, u2.UsedQuota)
	}

	// 正常预扣
	if err := q.Reserve(ctx, k.ID, u.ID, 80); err != nil {
		t.Fatal(err)
	}

	// 结算：实际 30，退还 50，两级同步
	q.Settle(ctx, k.ID, u.ID, 80, 30)
	k3, _ := keyRepo.GetByID(ctx, k.ID)
	u3, _ := userRepo.GetByID(ctx, u.ID)
	if k3.UsedQuota != 30 || u3.UsedQuota != 30 {
		t.Fatalf("结算后两级均应为 30: key=%d user=%d", k3.UsedQuota, u3.UsedQuota)
	}

	// key 额度不足
	if err := q.Reserve(ctx, k.ID, u.ID, 999_999); err == nil {
		t.Fatal("key 额度不足应失败")
	}
}
