package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/snownd/team-router/internal/crypto"
	"github.com/snownd/team-router/internal/repository"
)

// Router 渠道选择器：按模型筛选可用渠道，priority 优先、同级 weight 加权随机；
// 渠道列表带 TTL 进程内缓存。
//
// 并发说明：加权随机使用 math/rand/v2 顶层函数（stdlib 保证并发安全），
// 不持有共享的 *rand.Rand 实例，避免并发请求下的数据竞争。
type Router struct {
	repo   *repository.ProviderRepo
	enc    *crypto.Encryptor
	reg    *Registry
	log    *zap.Logger
	ttl    time.Duration
	mu     sync.RWMutex
	chans  []*Channel
	expire time.Time
}

// NewRouter 构造。
func NewRouter(repo *repository.ProviderRepo, enc *crypto.Encryptor, reg *Registry, log *zap.Logger) *Router {
	return &Router{
		repo: repo, enc: enc, reg: reg, log: log.Named("router"),
		ttl: 30 * time.Second,
	}
}

// channels 刷新缓存。
func (r *Router) channels(ctx context.Context) ([]*Channel, error) {
	r.mu.RLock()
	if r.chans != nil && time.Now().Before(r.expire) {
		out := r.chans
		r.mu.RUnlock()
		return out, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.chans != nil && time.Now().Before(r.expire) {
		return r.chans, nil
	}
	providers, err := r.repo.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	var out []*Channel
	for _, p := range providers {
		if _, ok := r.reg.Get(p.Type); !ok {
			continue // 无适配器的渠道类型跳过
		}
		apiKey, err := r.enc.Decrypt(p.APIKeyEnc)
		if err != nil {
			r.log.Warn("decrypt provider key failed, skip channel",
				zap.Int64("provider_id", p.ID), zap.Error(err))
			continue
		}
		mapping := map[string]string{}
		if len(p.ModelMapping) > 0 && string(p.ModelMapping) != "null" {
			_ = unmarshalMap(p.ModelMapping, &mapping)
		}
		out = append(out, &Channel{
			ID: p.ID, Name: p.Name, Type: p.Type, BaseURL: p.BaseURL, APIKey: apiKey,
			Models: p.ModelList(), ModelMapping: mapping,
			Priority: p.Priority, Weight: maxInt(p.Weight, 1),
			MaxConcurrency: maxInt(p.MaxConcurrency, 0), ProxyURL: p.ProxyURL,
		})
	}
	r.chans = out
	r.expire = time.Now().Add(r.ttl)
	return out, nil
}

// Invalidate 主动失效缓存（渠道配置变更后调用）。
func (r *Router) Invalidate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chans = nil
}

// AdapterTypes 已注册适配器类型透出。
func (r *Router) AdapterTypes() []string { return r.reg.Types() }

// FetchModels 探测上游渠道的模型列表（管理台“拉取模型”用；type 未注册时报错）。
func (r *Router) FetchModels(ctx context.Context, ch *Channel, client *http.Client) ([]string, error) {
	a, ok := r.reg.Get(ch.Type)
	if !ok {
		return nil, fmt.Errorf("不支持的渠道类型 %s", ch.Type)
	}
	return FetchUpstreamModels(ctx, a, ch, client)
}

// SelectForModel 返回可服务该模型的渠道列表（已按优先级+权重排序）。
func (r *Router) SelectForModel(ctx context.Context, model string) ([]*Channel, error) {
	all, err := r.channels(ctx)
	if err != nil {
		return nil, err
	}
	var matched []*Channel
	for _, c := range all {
		if channelServes(c, model) {
			matched = append(matched, c)
		}
	}
	// 按 priority 分层，层内加权随机
	ordered := make([]*Channel, 0, len(matched))
	byPriority := map[int][]*Channel{}
	var priorities []int
	for _, c := range matched {
		if _, ok := byPriority[c.Priority]; !ok {
			priorities = append(priorities, c.Priority)
		}
		byPriority[c.Priority] = append(byPriority[c.Priority], c)
	}
	sortDesc(priorities)
	for _, p := range priorities {
		ordered = append(ordered, weightedShuffle(byPriority[p])...)
	}
	return ordered, nil
}

// Models 返回网关可服务的模型清单（去重）。
func (r *Router) Models(ctx context.Context) ([]string, error) {
	all, err := r.channels(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, c := range all {
		for _, m := range c.Models {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
		for m := range c.ModelMapping {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	return out, nil
}

func channelServes(c *Channel, model string) bool {
	if _, ok := c.ModelMapping[model]; ok {
		return true
	}
	for _, m := range c.Models {
		if m == model {
			return true
		}
	}
	return false
}

func weightedShuffle(chs []*Channel) []*Channel {
	out := make([]*Channel, len(chs))
	copy(out, chs)
	// 加权随机抽取（不放回）；rand/v2 顶层函数并发安全
	for i := 0; i < len(out)-1; i++ {
		total := 0
		for _, c := range out[i:] {
			total += c.Weight
		}
		pick := rand.IntN(total)
		for j := i; j < len(out); j++ {
			pick -= out[j].Weight
			if pick < 0 {
				out[i], out[j] = out[j], out[i]
				break
			}
		}
	}
	return out
}

func sortDesc(s []int) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] > s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func unmarshalMap(raw []byte, m *map[string]string) error {
	return json.Unmarshal(raw, m)
}
