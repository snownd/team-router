// Package protocol 定义网关内部统一中间格式（canonical），
// 以及三种入站协议（OpenAI Chat Completions / OpenAI Responses / Anthropic Messages）
// 与 canonical 之间的双向编解码。
//
// 数据流：
//
//	客户端 wire 请求 ──ParseXxxWire──▶ canonical.Request
//	canonical.Request ──XxxToWire──▶ 上游请求（由 provider adapter 发送）
//	上游 SSE/JSON ──XxxStreamDecoder──▶ canonical.Event 流
//	canonical.Event 流 ──XxxStreamEncoder──▶ 客户端协议 SSE
//
// 语义约定：
//   - Usage.InputTokens 为"新鲜输入" token（不含缓存命中）；
//     OpenAI 的 prompt_tokens 含 cached_tokens，由适配器换算 Input = prompt - cached；
//     Anthropic 的 input_tokens 天然不含缓存读。
//   - 停止原因统一为 canonical 取值：stop / length / tool_calls / content_filter。
package protocol

import "encoding/json"

// ---- 请求 ----

// Request canonical 请求（三种协议的超集）。
type Request struct {
	Model  string
	System string
	// SystemCacheControl Anthropic 风格 system 缓存标记（透传 anthropic 上游）
	SystemCacheControl *string

	Messages []Message
	Tools    []Tool
	// ToolChoice "auto"|"none"|"required" 或原生结构（透传时尽量归一化为字符串）
	ToolChoice    any
	Temperature   *float64
	TopP          *float64
	MaxTokens     int // 0 = 未指定
	StopSequences []string
	Stream        bool
	Metadata      map[string]any
	UserID        string // 各协议的 user 字段

	// Responses API 专有（v1 仅支持无状态子集）
	Store              bool
	PreviousResponseID string
}

// Message canonical 消息。
type Message struct {
	Role       string // system | user | assistant | tool
	Text       string // 纯文本内容（Blocks 为空时生效）
	Blocks     []Block
	ToolCalls  []ToolCall // assistant 消息的工具调用
	ToolCallID string     // role=tool 时对应的调用 ID
	Name       string
	// CacheControl Anthropic 缓存标记（作用于本消息最后一个内容块）
	CacheControl *string
}

// Block 消息内容块。
type Block struct {
	Type         string // text | image
	Text         string
	MimeType     string
	Data         string // base64（图片）
	CacheControl *string
}

// ToolCall 工具调用。
type ToolCall struct {
	ID        string
	Name      string
	Arguments string // JSON 字符串
}

// Tool 工具定义。
type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema
}

// IsEmpty 辅助判断。
func (t Tool) IsEmpty() bool { return t.Name == "" }

// ---- 响应（非流式） ----

// Response canonical 非流式响应。
type Response struct {
	ID         string
	Model      string
	Blocks     []ContentBlock
	StopReason string
	Usage      Usage
}

// ContentBlock 生成内容块。
type ContentBlock struct {
	Type     string // text | thinking | tool_use
	Text     string
	ToolCall ToolCall
}

// Usage canonical 用量。
type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	ReasoningTokens  int
	Estimated        bool // 上游未返回 usage、由网关估算
}

// TotalInput 输入总量（新鲜 + 缓存读 + 缓存写）。
func (u Usage) TotalInput() int { return u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens }

// Add 累加（流式过程中多次 usage 事件合并）。
func (u *Usage) Add(o Usage) {
	u.InputTokens += o.InputTokens
	u.OutputTokens += o.OutputTokens
	u.CacheReadTokens += o.CacheReadTokens
	u.CacheWriteTokens += o.CacheWriteTokens
	u.ReasoningTokens += o.ReasoningTokens
	u.Estimated = u.Estimated || o.Estimated
}

// ---- 流事件 ----

// 事件类型。
const (
	EventMessageStart = "message_start"
	EventBlockStart   = "block_start"
	EventBlockDelta   = "block_delta"
	EventBlockStop    = "block_stop"
	EventMessageDelta = "message_delta"
	EventUsage        = "usage"
	EventDone         = "done"
	EventError        = "error"
)

// 块类型。
const (
	BlockTypeText     = "text"
	BlockTypeImage    = "image"
	BlockTypeThinking = "thinking"
	BlockTypeToolUse  = "tool_use"
)

// 增量类型。
const (
	DeltaText      = "text"
	DeltaThinking  = "thinking"
	DeltaInputJSON = "input_json"
)

// 停止原因。
const (
	StopStop          = "stop"
	StopLength        = "length"
	StopToolCalls     = "tool_calls"
	StopContentFilter = "content_filter"
)

// Event canonical 流事件。
type Event struct {
	Type       string
	ID         string // message_start：上游消息 ID
	Model      string
	Index      int      // 块索引（canonical 全局顺序编号）
	BlockType  string   // block_start: text|thinking|tool_use
	ToolCall   ToolCall // block_start(tool_use)
	DeltaType  string
	Text       string
	StopReason string
	Usage      *Usage
	Error      *RelayError
}

// StreamDecoder 上游流 -> canonical 事件迭代器。
type StreamDecoder interface {
	// Next 返回下一个事件；流结束时返回 (nil, io.EOF)；错误包装在 EventError 中抛出。
	Next() (*Event, error)
	// Close 释放底层资源。
	Close() error
}
