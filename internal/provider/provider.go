// Package provider 定义上游渠道适配器抽象与注册表。
package provider

import (
	"context"
	"io"
	"net/http"
	"sync"

	"go.uber.org/fx"

	"github.com/snownd/team-router/internal/protocol"
)

// Channel 运行时渠道视图（含解密后的上游密钥）。
type Channel struct {
	ID             int64
	Name           string
	Type           string
	BaseURL        string
	APIKey         string
	Models         []string
	ModelMapping   map[string]string
	Priority       int
	Weight         int
	MaxConcurrency int // 0 = 不限制（见 Limiter）
	ProxyURL       string
}

// UpstreamModel 请求模型名 -> 上游模型名（考虑 mapping）。
func (c *Channel) UpstreamModel(model string) string {
	if m, ok := c.ModelMapping[model]; ok && m != "" {
		return m
	}
	return model
}

// Adapter 上游适配器：canonical <-> 某类上游协议。
type Adapter interface {
	// Type 适配器标识（对应 providers.type）
	Type() string
	// BuildRequest 构造上游 HTTP 请求
	BuildRequest(ctx context.Context, req *protocol.Request, ch *Channel, stream bool) (*http.Request, error)
	// BuildModelsRequest 构造上游“模型列表”探测请求（GET {base}/models，按协议设置鉴权头），
	// 供管理台拉取上游可服务模型（见 FetchUpstreamModels）
	BuildModelsRequest(ctx context.Context, ch *Channel) (*http.Request, error)
	// DecodeNonStream 解析上游非流式响应体
	DecodeNonStream(body []byte, model string) (*protocol.Response, *protocol.RelayError)
	// NewStreamDecoder 构造上游 SSE 解码器
	NewStreamDecoder(r io.ReadCloser) protocol.StreamDecoder
	// ParseUpstreamError 上游错误响应 -> RelayError
	ParseUpstreamError(status int, body []byte) *protocol.RelayError
}

// Registry adapter 注册表。
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

// NewRegistry 构造（fx 注入已注册的 adapter 集合）。
func NewRegistry(adapters []Adapter) *Registry {
	r := &Registry{adapters: map[string]Adapter{}}
	for _, a := range adapters {
		r.adapters[a.Type()] = a
	}
	return r
}

// Register 注册 adapter。
func (r *Registry) Register(a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[a.Type()] = a
}

// Get 按 type 获取。
func (r *Registry) Get(typ string) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[typ]
	return a, ok
}

// Types 已注册的 adapter 类型。
func (r *Registry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.adapters))
	for t := range r.adapters {
		out = append(out, t)
	}
	return out
}

// Module fx 模块。新增上游适配器时：
//
//	fx.Annotate(NewXxxAdapter, fx.As(new(Adapter)), fx.ResultTags(`group:"adapters"`))
var Module = fx.Module("provider",
	fx.Provide(
		fx.Annotate(NewRegistry, fx.ParamTags(`group:"adapters"`)),
		NewRouter,
		NewLimiter,
		fx.Annotate(NewOpenAIAdapter, fx.As(new(Adapter)), fx.ResultTags(`group:"adapters"`)),
		fx.Annotate(NewOpenAIResponsesAdapter, fx.As(new(Adapter)), fx.ResultTags(`group:"adapters"`)),
		fx.Annotate(NewAnthropicAdapter, fx.As(new(Adapter)), fx.ResultTags(`group:"adapters"`)),
	),
)
