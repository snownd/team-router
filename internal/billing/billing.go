// Package billing 定价、计费与配额结算。金额单位：微美元（1e6 = $1）。
package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/snownd/team-router/internal/config"
	"github.com/snownd/team-router/internal/model"
	"github.com/snownd/team-router/internal/protocol"
	"github.com/snownd/team-router/internal/repository"
)

// Pricer 模型价格查询（带 TTL 进程内缓存）。
type Pricer struct {
	repo *repository.PriceRepo
	cfg  config.BillingConfig
	log  *zap.Logger

	mu    sync.RWMutex
	cache map[string]priceEntry
}

type priceEntry struct {
	price   *model.ModelPrice
	found   bool
	expires time.Time
}

// NewPricer 构造。
func NewPricer(repo *repository.PriceRepo, cfg *config.Config, log *zap.Logger) *Pricer {
	return &Pricer{
		repo:  repo,
		cfg:   cfg.Billing,
		log:   log.Named("pricer"),
		cache: map[string]priceEntry{},
	}
}

// GetPrice 查询模型价格：优先 (model, providerType) 精确匹配，其次通配。
// 返回 nil 表示未配置价格。
func (p *Pricer) GetPrice(ctx context.Context, modelName, providerType string) (*model.ModelPrice, error) {
	ttl := time.Duration(p.cfg.PriceCacheSeconds) * time.Second
	if ttl <= 0 {
		ttl = time.Minute
	}
	key := modelName + "\x00" + providerType

	p.mu.RLock()
	if e, ok := p.cache[key]; ok && time.Now().Before(e.expires) {
		p.mu.RUnlock()
		return e.price, nil
	}
	p.mu.RUnlock()

	price, err := p.repo.Find(ctx, modelName, providerType)
	if err != nil {
		if isNotFound(err) {
			p.mu.Lock()
			p.cache[key] = priceEntry{nil, false, time.Now().Add(ttl)}
			p.mu.Unlock()
			return nil, nil
		}
		return nil, err
	}
	p.mu.Lock()
	p.cache[key] = priceEntry{price, true, time.Now().Add(ttl)}
	p.mu.Unlock()
	return price, nil
}

// Invalidate 清空价格缓存。
func (p *Pricer) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = map[string]priceEntry{}
}

// Cost 计费结果。
type Cost struct {
	QuotaCost int64 // 微美元
	Currency  string
	Detail    json.RawMessage // billing_detail
	Priced    bool            // 是否有有效定价
}

// Compute 按用量与单价计费。
func Compute(u protocol.Usage, price *model.ModelPrice, modelName, providerType string) Cost {
	if price == nil {
		detail, _ := json.Marshal(map[string]any{
			"model": modelName, "provider_type": providerType,
			"priced": false, "estimated": u.Estimated,
			"currency": "USD", "cost": 0,
			"tokens": tokenDetail(u),
		})
		return Cost{QuotaCost: 0, Currency: "USD", Detail: detail, Priced: false}
	}
	input := mulDiv(u.InputTokens, price.InputPerM)
	output := mulDiv(u.OutputTokens, price.OutputPerM)
	cacheRead := mulDiv(u.CacheReadTokens, price.CacheReadPerM)
	cacheWrite := mulDiv(u.CacheWriteTokens, price.CacheWritePerM)
	total := input + output + cacheRead + cacheWrite

	detail, _ := json.Marshal(map[string]any{
		"model": modelName, "provider_type": providerType,
		"priced": true, "estimated": u.Estimated,
		"currency": price.Currency,
		"prices": map[string]int64{
			"input_per_m": price.InputPerM, "output_per_m": price.OutputPerM,
			"cache_read_per_m": price.CacheReadPerM, "cache_write_per_m": price.CacheWritePerM,
		},
		"tokens": tokenDetail(u),
		"parts": map[string]int64{
			"input": input, "output": output, "cache_read": cacheRead, "cache_write": cacheWrite,
		},
		"cost": total,
	})
	return Cost{QuotaCost: total, Currency: price.Currency, Detail: detail, Priced: true}
}

// EstimateReserve 预扣估算：输入估算 + 最大输出，按各分项单价上界的较大者（output 单价）粗算。
func EstimateReserve(inputTokens, maxOutputTokens int, price *model.ModelPrice) int64 {
	if price == nil {
		return 0
	}
	unit := price.OutputPerM
	if price.InputPerM > unit {
		unit = price.InputPerM
	}
	if unit <= 0 {
		return 0
	}
	total := inputTokens + maxOutputTokens
	if total <= 0 {
		return 0
	}
	return mulDiv(total, unit)
}

// MergePricesUpper 将多个价目合并为“上界价目”：各分项取最大值。
// 用于预扣：候选渠道可能命中任意一个（加权抽样 + 跨渠道重试），
// 预扣必须按候选全集中最贵价目计算，结算时再按实际命中渠道精确计费。
// 全部为 nil 时返回 nil。
func MergePricesUpper(prices []*model.ModelPrice) *model.ModelPrice {
	var out *model.ModelPrice
	for _, p := range prices {
		if p == nil {
			continue
		}
		if out == nil {
			cp := *p
			out = &cp
			continue
		}
		if p.InputPerM > out.InputPerM {
			out.InputPerM = p.InputPerM
		}
		if p.OutputPerM > out.OutputPerM {
			out.OutputPerM = p.OutputPerM
		}
		if p.CacheReadPerM > out.CacheReadPerM {
			out.CacheReadPerM = p.CacheReadPerM
		}
		if p.CacheWritePerM > out.CacheWritePerM {
			out.CacheWritePerM = p.CacheWritePerM
		}
	}
	return out
}

func mulDiv(tokens int, pricePerM int64) int64 {
	if tokens <= 0 || pricePerM <= 0 {
		return 0
	}
	return int64(tokens) * pricePerM / 1_000_000
}

func tokenDetail(u protocol.Usage) map[string]int {
	return map[string]int{
		"input": u.InputTokens, "output": u.OutputTokens,
		"cache_read": u.CacheReadTokens, "cache_write": u.CacheWriteTokens,
		"reasoning": u.ReasoningTokens,
	}
}

func isNotFound(err error) bool {
	return err != nil && (err.Error() == "record not found")
}

// ---- 配额 ----

// Quota 配额管理：预扣 + 结算。
// 预扣与结算均通过 QuotaRepo 在单事务内同进同退 key/user 两级额度，
// 任一侧失败整体回滚，避免两级账目出现永久性偏差。
type Quota struct {
	quotaRepo *repository.QuotaRepo
	log       *zap.Logger
}

// NewQuota 构造。
func NewQuota(quotaRepo *repository.QuotaRepo, log *zap.Logger) *Quota {
	return &Quota{quotaRepo: quotaRepo, log: log.Named("quota")}
}

// Reserve 预扣：key 与用户两级在单事务内原子预扣，任一级额度不足整体回滚。
// quota = 0 表示不限额。返回 error 时上层应返回配额不足错误。
func (q *Quota) Reserve(ctx context.Context, keyID, userID int64, est int64) error {
	ok, err := q.quotaRepo.Reserve(ctx, keyID, userID, est)
	if err != nil {
		return err // 含 ErrKeyQuotaInsufficient / ErrUserQuotaInsufficient
	}
	if !ok {
		return fmt.Errorf("额度不足")
	}
	return nil
}

// Settle 结算：actual - reserved 的差额多退少补（负值即退还多扣部分）。
// 单事务同时调整 key 与 user，任一失败整体回滚并记录错误日志；
// 事务化保证即使进程中途崩溃也不会出现只调一级的两级账目偏差。
func (q *Quota) Settle(ctx context.Context, keyID, userID, reserved, actual int64) {
	delta := actual - reserved
	if delta == 0 {
		return
	}
	if err := q.quotaRepo.Settle(ctx, keyID, userID, delta); err != nil {
		q.log.Error("settle quota failed",
			zap.Int64("key_id", keyID), zap.Int64("user_id", userID),
			zap.Int64("delta", delta), zap.Error(err))
	}
}

// Module fx 模块。
var Module = fx.Module("billing",
	fx.Provide(NewPricer, NewQuota),
)
