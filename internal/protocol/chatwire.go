package protocol

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// ============================================================
// OpenAI Chat Completions wire 格式：
//   - ParseChatWire: 客户端请求 -> canonical
//   - CanonicalToChatWire: canonical -> 上游请求（openai 兼容渠道）
//   - ChatWireToResponse: 上游非流式响应 -> canonical
//   - ChatStreamDecoder: 上游 SSE -> canonical 事件
//   - ChatStreamEncoder: canonical 事件 -> 客户端 SSE
//   - ChatEncodeResponse: canonical -> 客户端非流式响应
// ============================================================

// ---- wire 结构 ----

type chatRequestWire struct {
	Model               string             `json:"model"`
	Messages            []chatMessage      `json:"messages"`
	Tools               []chatToolWire     `json:"tools,omitempty"`
	ToolChoice          any                `json:"tool_choice,omitempty"`
	Temperature         *float64           `json:"temperature,omitempty"`
	TopP                *float64           `json:"top_p,omitempty"`
	MaxTokens           *int               `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int               `json:"max_completion_tokens,omitempty"`
	Stream              bool               `json:"stream,omitempty"`
	StreamOptions       *chatStreamOptions `json:"stream_options,omitempty"`
	Stop                []string           `json:"stop,omitempty"`
	User                string             `json:"user,omitempty"`
}

type chatStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	Name       string          `json:"name,omitempty"`
	ToolCalls  []chatToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type chatToolCall struct {
	Index    *int           `json:"index,omitempty"`
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type,omitempty"`
	Function chatToolCallFn `json:"function"`
}

type chatToolCallFn struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type chatToolWire struct {
	Type     string         `json:"type"`
	Function chatToolSchema `json:"function"`
}

type chatToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type chatContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *chatImageURL `json:"image_url,omitempty"`
}

type chatImageURL struct {
	URL string `json:"url"`
}

type chatResponseWire struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []chatChoice   `json:"choices"`
	Usage   *chatUsageWire `json:"usage,omitempty"`
}

type chatChoice struct {
	Index        int          `json:"index"`
	Message      *chatRespMsg `json:"message,omitempty"`
	Delta        *chatRespMsg `json:"delta,omitempty"`
	FinishReason *string      `json:"finish_reason"`
}

type chatRespMsg struct {
	Role             string         `json:"role,omitempty"`
	Content          string         `json:"content,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []chatToolCall `json:"tool_calls,omitempty"`
}

type chatUsageWire struct {
	PromptTokens            int                          `json:"prompt_tokens"`
	CompletionTokens        int                          `json:"completion_tokens"`
	TotalTokens             int                          `json:"total_tokens"`
	PromptTokensDetails     *chatPromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *chatCompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}

type chatPromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type chatCompletionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// ---- 解析（客户端 -> canonical） ----

// ParseChatWire 解析客户端 Chat Completions 请求。
func ParseChatWire(body []byte) (*Request, *RelayError) {
	var raw struct {
		chatRequestWire
		RawMessages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, ErrInvalidRequest("请求体不是合法 JSON: " + err.Error())
	}
	if len(raw.RawMessages) == 0 {
		return nil, ErrInvalidRequest("messages 不能为空")
	}

	req := &Request{
		Model:         raw.Model,
		Tools:         parseChatTools(raw.Tools),
		ToolChoice:    normalizeToolChoice(raw.ToolChoice),
		Temperature:   raw.Temperature,
		TopP:          raw.TopP,
		Stream:        raw.Stream,
		StopSequences: raw.Stop,
		UserID:        raw.User,
	}
	if raw.MaxCompletionTokens != nil {
		req.MaxTokens = *raw.MaxCompletionTokens
	}
	if raw.MaxTokens != nil {
		req.MaxTokens = *raw.MaxTokens
	}

	for _, rawMsg := range raw.RawMessages {
		msg, err := parseChatMessage(rawMsg)
		if err != nil {
			return nil, err
		}
		if msg.Role == "system" || msg.Role == "developer" {
			if req.System == "" {
				req.System = msg.Text
			} else {
				req.System += "\n\n" + msg.Text
			}
			continue
		}
		req.Messages = append(req.Messages, *msg)
	}
	if req.Model == "" {
		return nil, ErrInvalidRequest("model 不能为空")
	}
	return req, nil
}

// parseChatMessage 解析单条消息（content 可能是 string 或 parts 数组）。
func parseChatMessage(raw json.RawMessage) (*Message, *RelayError) {
	var m chatMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, ErrInvalidRequest("message 解析失败: " + err.Error())
	}
	msg := &Message{Role: m.Role, Name: m.Name, ToolCallID: m.ToolCallID}
	for _, tc := range m.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{
			ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
		})
	}

	if len(m.Content) == 0 || string(m.Content) == "null" {
		return msg, nil
	}
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		msg.Text = s
		return msg, nil
	}
	// 多模态数组
	var parts []chatContentPart
	if err := json.Unmarshal(m.Content, &parts); err != nil {
		return nil, ErrInvalidRequest("message.content 格式不支持")
	}
	var textBuf strings.Builder
	for _, p := range parts {
		switch p.Type {
		case "text":
			textBuf.WriteString(p.Text)
			msg.Blocks = append(msg.Blocks, Block{Type: BlockTypeText, Text: p.Text})
		case "image_url":
			if p.ImageURL != nil {
				msg.Blocks = append(msg.Blocks, Block{Type: BlockTypeImage, Data: p.ImageURL.URL, MimeType: "image_url"})
			}
		default:
			return nil, ErrInvalidRequest("不支持的 content part 类型: " + p.Type)
		}
	}
	// 只有文本块时退化为纯文本，简化下游
	if len(msg.Blocks) > 0 && allText(msg.Blocks) {
		msg.Text = textBuf.String()
		msg.Blocks = nil
	}
	return msg, nil
}

func allText(blocks []Block) bool {
	for _, b := range blocks {
		if b.Type != BlockTypeText {
			return false
		}
	}
	return true
}

func parseChatTools(tools []chatToolWire) []Tool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]Tool, 0, len(tools))
	for _, t := range tools {
		if t.Type != "function" && t.Type != "" {
			continue // v1 仅支持 function 工具
		}
		out = append(out, Tool{
			Name: t.Function.Name, Description: t.Function.Description, Parameters: t.Function.Parameters,
		})
	}
	return out
}

// normalizeToolChoice 归一化工具选择（结构体形态转字符串）。
func normalizeToolChoice(v any) any {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		if fn, ok := t["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok {
				return name
			}
		}
		return "auto"
	}
	return nil
}

// ---- canonical -> 上游 openai 请求 ----

// CanonicalToChatWire canonical -> OpenAI Chat 请求体（发往上游）。
func CanonicalToChatWire(req *Request, upstreamModel string, stream bool) ([]byte, error) {
	w := chatRequestWire{
		Model:       upstreamModel,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      stream,
		Stop:        req.StopSequences,
		User:        req.UserID,
	}
	if req.MaxTokens > 0 {
		mt := req.MaxTokens
		w.MaxTokens = &mt
	}
	if stream {
		// 始终要求上游返回 usage
		w.StreamOptions = &chatStreamOptions{IncludeUsage: true}
	}

	if req.System != "" {
		w.Messages = append(w.Messages, chatMessage{Role: "system", Content: json.RawMessage(mustJSON(req.System))})
	}
	for _, m := range req.Messages {
		w.Messages = append(w.Messages, canonicalMsgToChat(m))
	}
	for _, t := range req.Tools {
		w.Tools = append(w.Tools, chatToolWire{Type: "function", Function: chatToolSchema{
			Name: t.Name, Description: t.Description, Parameters: t.Parameters,
		}})
	}
	if tc, ok := req.ToolChoice.(string); ok && tc != "" {
		switch tc {
		case "auto", "none", "required":
			w.ToolChoice = tc
		default:
			w.ToolChoice = map[string]any{"type": "function", "function": map[string]any{"name": tc}}
		}
	}
	return json.Marshal(w)
}

func canonicalMsgToChat(m Message) chatMessage {
	cm := chatMessage{Role: m.Role, Name: m.Name, ToolCallID: m.ToolCallID}
	for _, tc := range m.ToolCalls {
		cm.ToolCalls = append(cm.ToolCalls, chatToolCall{
			ID: tc.ID, Type: "function", Function: chatToolCallFn{Name: tc.Name, Arguments: tc.Arguments},
		})
	}
	switch {
	case len(m.Blocks) > 0:
		parts := make([]chatContentPart, 0, len(m.Blocks))
		for _, b := range m.Blocks {
			switch b.Type {
			case BlockTypeText:
				parts = append(parts, chatContentPart{Type: "text", Text: b.Text})
			case BlockTypeImage:
				url := b.Data
				if b.MimeType != "image_url" && b.Data != "" && !strings.HasPrefix(url, "http") {
					url = "data:" + b.MimeType + ";base64," + b.Data
				}
				parts = append(parts, chatContentPart{Type: "image_url", ImageURL: &chatImageURL{URL: url}})
			}
		}
		cm.Content = json.RawMessage(mustJSON(parts))
	case m.Text != "":
		cm.Content = json.RawMessage(mustJSON(m.Text))
	default:
		cm.Content = json.RawMessage(`""`)
	}
	return cm
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("null")
	}
	return b
}

// ---- 上游非流式响应 -> canonical ----

// ChatWireToResponse 上游 Chat 非流式响应 -> canonical Response。
func ChatWireToResponse(body []byte, model string) (*Response, *RelayError) {
	var w chatResponseWire
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, ErrUpstream("上游响应解析失败: " + err.Error())
	}
	resp := &Response{ID: w.ID, Model: model}
	if len(w.Choices) > 0 && w.Choices[0].Message != nil {
		m := w.Choices[0].Message
		if m.Content != "" {
			resp.Blocks = append(resp.Blocks, ContentBlock{Type: BlockTypeText, Text: m.Content})
		}
		if m.ReasoningContent != "" {
			resp.Blocks = append(resp.Blocks, ContentBlock{Type: BlockTypeThinking, Text: m.ReasoningContent})
		}
		for _, tc := range m.ToolCalls {
			resp.Blocks = append(resp.Blocks, ContentBlock{
				Type:     BlockTypeToolUse,
				ToolCall: ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments},
			})
		}
		if w.Choices[0].FinishReason != nil {
			resp.StopReason = normalizeOpenAIStop(*w.Choices[0].FinishReason)
		}
	}
	if resp.StopReason == "" {
		resp.StopReason = StopStop
	}
	resp.Usage = chatUsageToCanonical(w.Usage)
	return resp, nil
}

func chatUsageToCanonical(u *chatUsageWire) Usage {
	if u == nil {
		return Usage{}
	}
	cached := 0
	if u.PromptTokensDetails != nil {
		cached = u.PromptTokensDetails.CachedTokens
	}
	reasoning := 0
	if u.CompletionTokensDetails != nil {
		reasoning = u.CompletionTokensDetails.ReasoningTokens
	}
	input := u.PromptTokens - cached
	if input < 0 {
		input = 0
	}
	return Usage{
		InputTokens:     input,
		CacheReadTokens: cached,
		OutputTokens:    u.CompletionTokens,
		ReasoningTokens: reasoning,
	}
}

func normalizeOpenAIStop(s string) string {
	switch s {
	case "stop", "":
		return StopStop
	case "length", "max_tokens":
		return StopLength
	case "tool_calls", "function_call":
		return StopToolCalls
	case "content_filter":
		return StopContentFilter
	default:
		return StopStop
	}
}

func canonicalToOpenAIStop(s string) string {
	switch s {
	case StopLength:
		return "length"
	case StopToolCalls:
		return "tool_calls"
	case StopContentFilter:
		return "content_filter"
	default:
		return "stop"
	}
}

// ---- 上游 SSE -> canonical ----

// ChatStreamDecoder 上游 OpenAI Chat SSE -> canonical 事件流。
type ChatStreamDecoder struct {
	reader       *SSEReader
	closer       io.Closer
	queue        []*Event // 待吐出的事件队列（收尾时填充）
	textOpen     bool
	textIdx      int
	thinkingOpen bool
	thinkingIdx  int
	toolIndex    map[int]int // openai tool index -> canonical block index
	blockCounter int
	usage        *Usage
	stop         string
	model        string
	doneEmitted  bool
	startSent    bool
}

// NewChatStreamDecoder 构造。
func NewChatStreamDecoder(r io.Reader, closer io.Closer) *ChatStreamDecoder {
	return &ChatStreamDecoder{reader: NewSSEReader(r), closer: closer, toolIndex: map[int]int{}}
}

// Next 实现 StreamDecoder。
func (d *ChatStreamDecoder) Next() (*Event, error) {
	if len(d.queue) > 0 {
		ev := d.queue[0]
		d.queue = d.queue[1:]
		if ev.Type == EventDone {
			d.doneEmitted = true
		}
		return ev, nil
	}
	if d.doneEmitted {
		return nil, io.EOF
	}
	for {
		_, data, err := d.reader.Next()
		if err != nil {
			return d.nextFromFinalize()
		}
		if data == "[DONE]" {
			return d.nextFromFinalize()
		}

		var chunk struct {
			Choices []chatChoice   `json:"choices"`
			Usage   *chatUsageWire `json:"usage"`
			Model   string         `json:"model"`
			Error   *struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal([]byte(data), &chunk); jsonErr != nil {
			continue // 忽略无法解析的块
		}
		if chunk.Model != "" {
			d.model = chunk.Model
		}
		if chunk.Error != nil {
			return d.withStart(&Event{Type: EventError, Error: NewRelayError(http.StatusBadGateway, "api_error", "", chunk.Error.Message)}), nil
		}
		if chunk.Usage != nil {
			u := chatUsageToCanonical(chunk.Usage)
			d.usage = &u
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		ch := chunk.Choices[0]
		if ch.Delta != nil {
			if ev, ok := d.handleDelta(ch.Delta); ok {
				return d.withStart(ev), nil
			}
		}
		if ch.FinishReason != nil && *ch.FinishReason != "" {
			d.stop = normalizeOpenAIStop(*ch.FinishReason)
		}
	}
}

// withStart 首个真实事件前补发 message_start（对齐 anthropic 事件模型）。
// 保持队列中已有事件（如 block_start 同批携带的增量）顺序不变。
func (d *ChatStreamDecoder) withStart(ev *Event) *Event {
	if d.startSent || ev == nil {
		return ev
	}
	d.startSent = true
	items := append([]*Event{{Type: EventMessageStart, Model: d.model}}, ev)
	d.queue = append(items, d.queue...)
	first := d.queue[0]
	d.queue = d.queue[1:]
	return first
}

// nextFromFinalize 流结束时填充收尾队列并弹出首事件。
// usage 位于 message_delta 之前，保证客户端编码器在渲染停止原因时可见完整用量。
func (d *ChatStreamDecoder) nextFromFinalize() (*Event, error) {
	if d.textOpen {
		d.queue = append(d.queue, &Event{Type: EventBlockStop, Index: d.textIdx})
		d.textOpen = false
	}
	if d.thinkingOpen {
		d.queue = append(d.queue, &Event{Type: EventBlockStop, Index: d.thinkingIdx})
		d.thinkingOpen = false
	}
	if !d.startSent {
		d.queue = append(d.queue, &Event{Type: EventMessageStart})
		d.startSent = true
	}
	if d.usage != nil {
		d.queue = append(d.queue, &Event{Type: EventUsage, Usage: d.usage})
	}
	if d.stop != "" {
		d.queue = append(d.queue, &Event{Type: EventMessageDelta, StopReason: d.stop})
	}
	d.queue = append(d.queue, &Event{Type: EventDone})

	if len(d.queue) == 1 {
		d.doneEmitted = true
	}
	ev := d.queue[0]
	d.queue = d.queue[1:]
	return ev, nil
}

func (d *ChatStreamDecoder) handleDelta(delta *chatRespMsg) (*Event, bool) {
	if delta.ReasoningContent != "" {
		if !d.thinkingOpen {
			d.thinkingOpen = true
			d.thinkingIdx = d.blockCounter
			d.blockCounter++
			d.queue = append(d.queue, &Event{Type: EventBlockDelta, Index: d.thinkingIdx, DeltaType: DeltaThinking, Text: delta.ReasoningContent})
			return &Event{Type: EventBlockStart, Index: d.thinkingIdx, BlockType: BlockTypeThinking}, true
		}
		return &Event{Type: EventBlockDelta, Index: d.thinkingIdx, DeltaType: DeltaThinking, Text: delta.ReasoningContent}, true
	}
	if delta.Content != "" {
		if !d.textOpen {
			d.textOpen = true
			d.textIdx = d.blockCounter
			d.blockCounter++
			// 同批携带的文本随 block_start 后补发，避免丢失
			d.queue = append(d.queue, &Event{Type: EventBlockDelta, Index: d.textIdx, DeltaType: DeltaText, Text: delta.Content})
			return &Event{Type: EventBlockStart, Index: d.textIdx, BlockType: BlockTypeText}, true
		}
		return &Event{Type: EventBlockDelta, Index: d.textIdx, DeltaType: DeltaText, Text: delta.Content}, true
	}
	if len(delta.ToolCalls) > 0 {
		for _, tc := range delta.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			blockIdx, seen := d.toolIndex[idx]
			if !seen {
				blockIdx = d.blockCounter
				d.blockCounter++
				d.toolIndex[idx] = blockIdx
				if tc.Function.Arguments != "" {
					d.queue = append(d.queue, &Event{Type: EventBlockDelta, Index: blockIdx, DeltaType: DeltaInputJSON, Text: tc.Function.Arguments})
				}
				return &Event{
					Type: EventBlockStart, Index: blockIdx, BlockType: BlockTypeToolUse,
					ToolCall: ToolCall{ID: tc.ID, Name: tc.Function.Name},
				}, true
			}
			if tc.Function.Arguments != "" {
				return &Event{Type: EventBlockDelta, Index: blockIdx, DeltaType: DeltaInputJSON, Text: tc.Function.Arguments}, true
			}
		}
	}
	return nil, false
}

// Close 实现 StreamDecoder。
func (d *ChatStreamDecoder) Close() error {
	if d.closer != nil {
		return d.closer.Close()
	}
	return nil
}
