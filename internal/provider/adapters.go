package provider

// 本文件包含全部内置上游适配器：
//
//	openai           标准 OpenAI Chat Completions（/v1/chat/completions）
//	openai_responses OpenAI Responses API（/v1/responses，强制无状态）
//	anthropic        Anthropic Messages（/v1/messages）
//
// 新增适配器见 docs/extending-provider.md。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/snownd/team-router/internal/protocol"
)

// joinBaseURL 规范化上游 URL：base 以 /v1 结尾则直接拼接子路径，否则补 /v1。
func joinBaseURL(base, subPath string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + subPath
	}
	return base + "/v1" + subPath
}

// ==================== OpenAI（及兼容端点）====================

// OpenAIAdapter OpenAI Chat Completions 上游适配器。
type OpenAIAdapter struct{}

// NewOpenAIAdapter 构造。
func NewOpenAIAdapter() *OpenAIAdapter { return &OpenAIAdapter{} }

// Type 标识。
func (a *OpenAIAdapter) Type() string { return "openai" }

// BuildRequest canonical -> POST {base}/v1/chat/completions。
func (a *OpenAIAdapter) BuildRequest(ctx context.Context, req *protocol.Request, ch *Channel, stream bool) (*http.Request, error) {
	model := ch.UpstreamModel(req.Model)
	body, err := protocol.CanonicalToChatWire(req, model, stream)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		joinBaseURL(ch.BaseURL, "/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+ch.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")
	return httpReq, nil
}

// BuildModelsRequest GET {base}/v1/models（Bearer 鉴权）。
func (a *OpenAIAdapter) BuildModelsRequest(ctx context.Context, ch *Channel) (*http.Request, error) {
	return newUpstreamGet(ctx, ch, func(h http.Header) { h.Set("Authorization", "Bearer "+ch.APIKey) })
}

// DecodeNonStream 上游非流式响应。
func (a *OpenAIAdapter) DecodeNonStream(body []byte, model string) (*protocol.Response, *protocol.RelayError) {
	return protocol.ChatWireToResponse(body, model)
}

// NewStreamDecoder 上游 SSE 解码。
func (a *OpenAIAdapter) NewStreamDecoder(r io.ReadCloser) protocol.StreamDecoder {
	return protocol.NewChatStreamDecoder(r, r)
}

// ParseUpstreamError 上游错误。
func (a *OpenAIAdapter) ParseUpstreamError(status int, body []byte) *protocol.RelayError {
	return parseOpenAIError(status, body)
}

// ==================== OpenAI Responses ====================

// OpenAIResponsesAdapter OpenAI Responses API（/v1/responses）上游适配器。
// 强制无状态：store=false、完整上下文入 input；不支持 stop 序列。
type OpenAIResponsesAdapter struct{}

// NewOpenAIResponsesAdapter 构造。
func NewOpenAIResponsesAdapter() *OpenAIResponsesAdapter { return &OpenAIResponsesAdapter{} }

// Type 标识。
func (a *OpenAIResponsesAdapter) Type() string { return "openai_responses" }

// BuildRequest canonical -> POST {base}/v1/responses。
func (a *OpenAIResponsesAdapter) BuildRequest(ctx context.Context, req *protocol.Request, ch *Channel, stream bool) (*http.Request, error) {
	model := ch.UpstreamModel(req.Model)
	body, err := protocol.CanonicalToResponsesWire(req, model, stream)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		joinBaseURL(ch.BaseURL, "/responses"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+ch.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")
	return httpReq, nil
}

// BuildModelsRequest GET {base}/v1/models（Bearer 鉴权）。
func (a *OpenAIResponsesAdapter) BuildModelsRequest(ctx context.Context, ch *Channel) (*http.Request, error) {
	return newUpstreamGet(ctx, ch, func(h http.Header) { h.Set("Authorization", "Bearer "+ch.APIKey) })
}

// DecodeNonStream 上游非流式响应。
func (a *OpenAIResponsesAdapter) DecodeNonStream(body []byte, model string) (*protocol.Response, *protocol.RelayError) {
	return protocol.ResponsesWireToResponse(body, model)
}

// NewStreamDecoder 上游 SSE 解码。
func (a *OpenAIResponsesAdapter) NewStreamDecoder(r io.ReadCloser) protocol.StreamDecoder {
	return protocol.NewResponsesStreamDecoder(r, r)
}

// ParseUpstreamError 上游错误（与 Chat 同为 OpenAI 错误格式）。
func (a *OpenAIResponsesAdapter) ParseUpstreamError(status int, body []byte) *protocol.RelayError {
	return parseOpenAIError(status, body)
}

// ==================== Anthropic ====================

// AnthropicAdapter Anthropic Messages 上游适配器。
type AnthropicAdapter struct {
	// DefaultMaxTokens 上游必填 max_tokens 的默认值
	DefaultMaxTokens int
}

// NewAnthropicAdapter 构造。
func NewAnthropicAdapter() *AnthropicAdapter {
	return &AnthropicAdapter{DefaultMaxTokens: 8192}
}

// Type 标识。
func (a *AnthropicAdapter) Type() string { return "anthropic" }

// BuildRequest canonical -> POST {base}/v1/messages。
func (a *AnthropicAdapter) BuildRequest(ctx context.Context, req *protocol.Request, ch *Channel, stream bool) (*http.Request, error) {
	model := ch.UpstreamModel(req.Model)
	body, err := protocol.CanonicalToAnthropicWire(req, model, stream, a.DefaultMaxTokens)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		joinBaseURL(ch.BaseURL, "/messages"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", ch.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Accept", "text/event-stream")
	return httpReq, nil
}

// BuildModelsRequest GET {base}/v1/models（x-api-key + anthropic-version 鉴权）。
func (a *AnthropicAdapter) BuildModelsRequest(ctx context.Context, ch *Channel) (*http.Request, error) {
	return newUpstreamGet(ctx, ch, func(h http.Header) {
		h.Set("x-api-key", ch.APIKey)
		h.Set("anthropic-version", "2023-06-01")
	})
}

// DecodeNonStream 上游非流式响应。
func (a *AnthropicAdapter) DecodeNonStream(body []byte, model string) (*protocol.Response, *protocol.RelayError) {
	return protocol.AnthropicWireToResponse(body, model)
}

// NewStreamDecoder 上游 SSE 解码。
func (a *AnthropicAdapter) NewStreamDecoder(r io.ReadCloser) protocol.StreamDecoder {
	return protocol.NewAnthropicStreamDecoder(r, r)
}

// ParseUpstreamError 上游错误。
func (a *AnthropicAdapter) ParseUpstreamError(status int, body []byte) *protocol.RelayError {
	var w struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &w) == nil && w.Error.Message != "" {
		return protocol.NewRelayError(status, orStr(w.Error.Type, "api_error"), "", w.Error.Message)
	}
	return protocol.NewRelayError(status, "api_error", "", fmt.Sprintf("上游错误(HTTP %d): %s", status, truncate(body, 512)))
}

func orStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// newUpstreamGet 构造 GET {base}/v1/models 请求，鉴权头由各适配器设置。
func newUpstreamGet(ctx context.Context, ch *Channel, setAuth func(http.Header)) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, joinBaseURL(ch.BaseURL, "/models"), nil)
	if err != nil {
		return nil, err
	}
	setAuth(httpReq.Header)
	return httpReq, nil
}

// FetchUpstreamModels 调用上游模型列表接口（BuildModelsRequest 构造），
// 解析 OpenAI / Anthropic 风格的 {"data":[{"id":...}]} 响应，
// 返回按字典序排序的模型 ID 列表。管理台“拉取模型”探测用。
func FetchUpstreamModels(ctx context.Context, a Adapter, ch *Channel, client *http.Client) ([]string, error) {
	req, err := a.BuildModelsRequest(ctx, ch)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求上游失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("读取上游响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if re := a.ParseUpstreamError(resp.StatusCode, body); re != nil && re.Message != "" {
			return nil, fmt.Errorf("上游返回 HTTP %d: %s", resp.StatusCode, re.Message)
		}
		return nil, fmt.Errorf("上游返回 HTTP %d: %s", resp.StatusCode, truncate(body, 256))
	}
	var wire struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("解析上游响应失败: %w", err)
	}
	out := make([]string, 0, len(wire.Data))
	for _, m := range wire.Data {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	slices.Sort(out)
	return out, nil
}

// parseOpenAIError OpenAI 风格错误体 -> RelayError（chat 与 responses 共用）。
func parseOpenAIError(status int, body []byte) *protocol.RelayError {
	var w struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &w) == nil && w.Error.Message != "" {
		code, _ := w.Error.Code.(string)
		return protocol.NewRelayError(status, orStr(w.Error.Type, "api_error"), code, w.Error.Message)
	}
	return protocol.NewRelayError(status, "api_error", "", fmt.Sprintf("上游错误(HTTP %d): %s", status, truncate(body, 512)))
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
