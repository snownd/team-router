package protocol

import (
	"encoding/json"
	"net/http"
)

// RelayError 统一错误模型（可按协议渲染为 OpenAI / Anthropic / Responses 错误体）。
type RelayError struct {
	HTTPStatus int
	Type       string // invalid_request_error | authentication_error | permission_error | not_found_error | rate_limit_error | api_error | overloaded_error | insufficient_quota
	Code       string // 细分错误码（openai 风格）
	Message    string
	Param      string
	Upstream   string // 上游渠道标识（内部排查用，不返回客户端）
}

// Error 实现 error。
func (e *RelayError) Error() string { return e.Message }

// NewRelayError 快捷构造。
func NewRelayError(status int, typ, code, msg string) *RelayError {
	return &RelayError{HTTPStatus: status, Type: typ, Code: code, Message: msg}
}

// ErrInvalidRequest 400。
func ErrInvalidRequest(msg string) *RelayError {
	return NewRelayError(http.StatusBadRequest, "invalid_request_error", "", msg)
}

// ErrAuth 401。
func ErrAuth(msg string) *RelayError {
	return NewRelayError(http.StatusUnauthorized, "authentication_error", "invalid_api_key", msg)
}

// ErrPermission 403。
func ErrPermission(msg string) *RelayError {
	return NewRelayError(http.StatusForbidden, "permission_error", "", msg)
}

// ErrNotFoundModel 404。
func ErrNotFoundModel(msg string) *RelayError {
	return NewRelayError(http.StatusNotFound, "not_found_error", "model_not_found", msg)
}

// ErrQuota 429。
func ErrQuota(msg string) *RelayError {
	return NewRelayError(http.StatusTooManyRequests, "insufficient_quota", "insufficient_quota", msg)
}

// ErrUpstream 502。
func ErrUpstream(msg string) *RelayError {
	return NewRelayError(http.StatusBadGateway, "api_error", "upstream_error", msg)
}

// ErrOverloaded 529（anthropic 语义）。
func ErrOverloaded(msg string) *RelayError {
	return NewRelayError(http.StatusServiceUnavailable, "overloaded_error", "", msg)
}

// ErrCodeChannelBusy 渠道并发满且排队超时的错误码（终止性：不跨渠道重试）。
const ErrCodeChannelBusy = "channel_busy"

// ErrChannelBusy 429：渠道并发已满且排队超时。
// relay 将其视为终止性错误：直接向客户端渲染 429，不跨渠道重试，
// 保持各账户并发隔离与排队语义可预期。
func ErrChannelBusy(msg string) *RelayError {
	return NewRelayError(http.StatusTooManyRequests, "rate_limit_error", ErrCodeChannelBusy, msg)
}

// 写 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// RenderChatError 渲染 OpenAI Chat 风格错误。
func RenderChatError(w http.ResponseWriter, e *RelayError) {
	writeJSON(w, e.HTTPStatus, map[string]any{
		"error": map[string]any{
			"message": e.Message,
			"type":    e.Type,
			"code":    e.Code,
			"param":   e.Param,
		},
	})
}

// RenderResponsesError 渲染 OpenAI Responses 风格错误。
func RenderResponsesError(w http.ResponseWriter, e *RelayError) {
	writeJSON(w, e.HTTPStatus, map[string]any{
		"error": map[string]any{
			"code":    e.Code,
			"message": e.Message,
			"param":   e.Param,
			"type":    e.Type,
		},
	})
}

// RenderMessagesError 渲染 Anthropic Messages 风格错误。
func RenderMessagesError(w http.ResponseWriter, e *RelayError) {
	writeJSON(w, e.HTTPStatus, map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    e.Type,
			"message": e.Message,
		},
	})
}

// ErrorRenderer 按协议渲染错误。
type ErrorRenderer func(w http.ResponseWriter, e *RelayError)
