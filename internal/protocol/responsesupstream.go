package protocol

import (
	"encoding/json"
	"io"
	"net/http"
)

// ============================================================
// OpenAI Responses API 上游方向（canonical -> 上游 /v1/responses）：
//   - CanonicalToResponsesWire: canonical -> 上游请求体（强制无状态：
//     store=false，完整上下文放 input；Responses API 无 stop 参数，
//     StopSequences 丢弃）
//   - ResponsesWireToResponse: 上游非流式响应 -> canonical Response
//   - NewResponsesStreamDecoder: 上游 Responses SSE -> canonical 事件流
// 用量语义：usage.input_tokens 含 cached，换算 canonical Input = input - cached
// （对齐 chat 适配器「新鲜输入」约定）。
// ============================================================

// ---- canonical -> 上游请求 ----

type responsesInputText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesInputImage struct {
	Type     string `json:"type"`
	ImageURL string `json:"image_url"`
}

type responsesInputMessage struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []any  `json:"content"`
}

type responsesInputFunctionCall struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type responsesInputFunctionOutput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

type responsesUpstreamTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      bool            `json:"strict,omitempty"`
}

type responsesUpstreamRequest struct {
	Model           string                  `json:"model"`
	Input           []any                   `json:"input"`
	Instructions    string                  `json:"instructions,omitempty"`
	Tools           []responsesUpstreamTool `json:"tools,omitempty"`
	ToolChoice      any                     `json:"tool_choice,omitempty"`
	Temperature     *float64                `json:"temperature,omitempty"`
	TopP            *float64                `json:"top_p,omitempty"`
	MaxOutputTokens int                     `json:"max_output_tokens,omitempty"`
	Store           bool                    `json:"store"`
	Stream          bool                    `json:"stream,omitempty"`
	User            string                  `json:"user,omitempty"`
}

// CanonicalToResponsesWire canonical -> OpenAI Responses 请求体（发往上游）。
//
// 限制：Responses API 不支持 stop 序列，req.StopSequences 被丢弃；
// 历史中的 thinking 块不回放（无状态下 reasoning 不可跨请求复用）。
func CanonicalToResponsesWire(req *Request, upstreamModel string, stream bool) ([]byte, error) {
	w := responsesUpstreamRequest{
		Model:           upstreamModel,
		Instructions:    req.System,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		MaxOutputTokens: req.MaxTokens,
		Store:           false, // 强制无状态：网关始终携带完整上下文
		Stream:          stream,
		User:            req.UserID,
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "user":
			w.Input = append(w.Input, responsesInputMessage{
				Type: "message", Role: "user", Content: canonicalBlocksToResponsesInput(m, "input_text"),
			})
		case "assistant":
			if text := canonicalAssistantText(m); text != "" {
				w.Input = append(w.Input, responsesInputMessage{
					Type: "message", Role: "assistant",
					Content: []any{responsesInputText{Type: "output_text", Text: text}},
				})
			}
			for _, tc := range m.ToolCalls {
				w.Input = append(w.Input, responsesInputFunctionCall{
					Type: "function_call", CallID: tc.ID, Name: tc.Name,
					Arguments: orDefault(tc.Arguments, "{}"),
				})
			}
		case "tool":
			w.Input = append(w.Input, responsesInputFunctionOutput{
				Type: "function_call_output", CallID: m.ToolCallID, Output: orDefault(m.Text, ""),
			})
		}
	}
	for _, t := range req.Tools {
		w.Tools = append(w.Tools, responsesUpstreamTool{
			Type: "function", Name: t.Name, Description: t.Description, Parameters: t.Parameters,
		})
	}
	if tc, ok := req.ToolChoice.(string); ok && tc != "" {
		switch tc {
		case "auto", "none", "required":
			w.ToolChoice = tc
		default: // 规范化后的裸函数名（见 normalizeToolChoice）
			w.ToolChoice = map[string]any{"type": "function", "name": tc}
		}
	}
	return json.Marshal(w)
}

// canonicalBlocksToResponsesInput canonical 消息 -> Responses 输入 content parts。
// textKind 为 user 消息取 "input_text"、assistant 取 "output_text"。
func canonicalBlocksToResponsesInput(m Message, textKind string) []any {
	if len(m.Blocks) == 0 {
		return []any{responsesInputText{Type: textKind, Text: m.Text}}
	}
	parts := make([]any, 0, len(m.Blocks))
	for _, b := range m.Blocks {
		switch b.Type {
		case BlockTypeText:
			parts = append(parts, responsesInputText{Type: textKind, Text: b.Text})
		case BlockTypeImage:
			url := b.Data
			if b.MimeType != "image_url" && b.Data != "" {
				url = "data:" + b.MimeType + ";base64," + b.Data
			}
			parts = append(parts, responsesInputImage{Type: "input_image", ImageURL: url})
		}
	}
	if len(parts) == 0 && m.Text != "" {
		return []any{responsesInputText{Type: textKind, Text: m.Text}}
	}
	return parts
}

// canonicalAssistantText 提取 assistant 消息文本（忽略 thinking 块）。
func canonicalAssistantText(m Message) string {
	if len(m.Blocks) == 0 {
		return m.Text
	}
	var sb []byte
	for _, b := range m.Blocks {
		if b.Type == BlockTypeText {
			sb = append(sb, b.Text...)
		}
	}
	return string(sb)
}

// ---- 上游非流式响应 -> canonical ----

type responsesUpstreamOutputItem struct {
	Type    string `json:"type"` // message | function_call | reasoning | ...
	Role    string `json:"role"`
	Status  string `json:"status"`
	Content []struct {
		Type string `json:"type"` // output_text | refusal | ...
		Text string `json:"text"`
	} `json:"content"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Summary   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"summary"`
}

type responsesUpstreamResponse struct {
	ID                string                        `json:"id"`
	Model             string                        `json:"model"`
	Status            string                        `json:"status"` // completed | incomplete | failed
	Output            []responsesUpstreamOutputItem `json:"output"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Error *struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Usage *responsesUsage `json:"usage"`
}

// ResponsesWireToResponse 上游 Responses 非流式响应 -> canonical Response。
func ResponsesWireToResponse(body []byte, model string) (*Response, *RelayError) {
	var w responsesUpstreamResponse
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, ErrUpstream("上游响应解析失败: " + err.Error())
	}
	if w.Status == "failed" {
		msg := "上游返回 failed"
		if w.Error != nil && w.Error.Message != "" {
			msg = w.Error.Message
		}
		return nil, ErrUpstream(msg)
	}
	resp := &Response{ID: w.ID, Model: model, StopReason: StopStop}
	hasToolCall := false
	for _, it := range w.Output {
		switch it.Type {
		case "message":
			var sb []byte
			for _, c := range it.Content {
				sb = append(sb, c.Text...)
			}
			if len(sb) > 0 {
				resp.Blocks = append(resp.Blocks, ContentBlock{Type: BlockTypeText, Text: string(sb)})
			}
		case "reasoning":
			var sb []byte
			for _, s := range it.Summary {
				sb = append(sb, s.Text...)
			}
			if len(sb) > 0 {
				resp.Blocks = append(resp.Blocks, ContentBlock{Type: BlockTypeThinking, Text: string(sb)})
			}
		case "function_call":
			hasToolCall = true
			resp.Blocks = append(resp.Blocks, ContentBlock{
				Type:     BlockTypeToolUse,
				ToolCall: ToolCall{ID: it.CallID, Name: it.Name, Arguments: orDefault(it.Arguments, "{}")},
			})
		}
	}
	if hasToolCall {
		resp.StopReason = StopToolCalls
	}
	if w.Status == "incomplete" && w.IncompleteDetails != nil && w.IncompleteDetails.Reason == "max_output_tokens" {
		resp.StopReason = StopLength
	}
	resp.Usage = responsesUsageToCanonical(w.Usage)
	return resp, nil
}

// responsesUsageToCanonical Responses usage（input 含 cached）-> canonical。
func responsesUsageToCanonical(u *responsesUsage) Usage {
	if u == nil {
		return Usage{}
	}
	cached := 0
	if u.InputTokensDetails != nil {
		cached = u.InputTokensDetails.CachedTokens
	}
	in := u.InputTokens - cached
	if in < 0 {
		in = 0
	}
	return Usage{InputTokens: in, CacheReadTokens: cached, OutputTokens: u.OutputTokens}
}

// ---- 上游 SSE -> canonical 事件 ----

// ResponsesStreamDecoder 上游 Responses SSE -> canonical 事件流。
type ResponsesStreamDecoder struct {
	reader    *SSEReader
	closer    io.Closer
	queue     []*Event
	itemIdx   map[string]int // item_id -> canonical 块索引
	nextIdx   int
	hasTool   bool
	started   bool
	done      bool
	finalized bool
}

// NewResponsesStreamDecoder 构造。
func NewResponsesStreamDecoder(r io.Reader, closer io.Closer) *ResponsesStreamDecoder {
	return &ResponsesStreamDecoder{reader: NewSSEReader(r), closer: closer, itemIdx: map[string]int{}}
}

// Next 实现 StreamDecoder。
func (d *ResponsesStreamDecoder) Next() (*Event, error) {
	for {
		if len(d.queue) > 0 {
			ev := d.queue[0]
			d.queue = d.queue[1:]
			if ev.Type == EventDone {
				d.done = true
			}
			return ev, nil
		}
		if d.done {
			return nil, io.EOF
		}
		event, data, err := d.reader.Next()
		if err != nil {
			if d.finalized {
				return nil, io.EOF
			}
			d.finalized = true
			return d.finalize(), nil
		}
		if data == "" || data == "[DONE]" {
			continue
		}
		d.handleEvent(event, data)
	}
}

// handleEvent 单条上游事件 -> canonical（复杂收尾进队列）。
func (d *ResponsesStreamDecoder) handleEvent(event, data string) {
	switch event {
	case "response.created", "response.in_progress":
		var w struct {
			Response struct {
				ID    string `json:"id"`
				Model string `json:"model"`
			} `json:"response"`
		}
		if json.Unmarshal([]byte(data), &w) != nil {
			return
		}
		if !d.started {
			d.started = true
			d.queue = append(d.queue, &Event{Type: EventMessageStart, ID: w.Response.ID, Model: w.Response.Model})
		}
	case "response.output_item.added":
		var w struct {
			Item struct {
				Type   string `json:"type"`
				ID     string `json:"id"`
				CallID string `json:"call_id"`
				Name   string `json:"name"`
			} `json:"item"`
		}
		if json.Unmarshal([]byte(data), &w) != nil {
			return
		}
		var blockType string
		var tc ToolCall
		switch w.Item.Type {
		case "message":
			blockType = BlockTypeText
		case "reasoning":
			blockType = BlockTypeThinking
		case "function_call":
			blockType = BlockTypeToolUse
			d.hasTool = true
			tc = ToolCall{ID: orDefault(w.Item.CallID, w.Item.ID), Name: w.Item.Name}
		default: // web_search_call 等内置工具项：忽略
			return
		}
		idx := d.nextIdx
		d.nextIdx++
		d.itemIdx[w.Item.ID] = idx
		d.queue = append(d.queue, &Event{Type: EventBlockStart, Index: idx, BlockType: blockType, ToolCall: tc})
	case "response.output_text.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
		var w struct {
			ItemID string `json:"item_id"`
			Delta  string `json:"delta"`
		}
		if json.Unmarshal([]byte(data), &w) != nil {
			return
		}
		deltaType := DeltaText
		if event != "response.output_text.delta" {
			deltaType = DeltaThinking
		}
		d.queue = append(d.queue, &Event{Type: EventBlockDelta, Index: d.itemIdx[w.ItemID], DeltaType: deltaType, Text: w.Delta})
	case "response.function_call_arguments.delta":
		var w struct {
			ItemID string `json:"item_id"`
			Delta  string `json:"delta"`
		}
		if json.Unmarshal([]byte(data), &w) != nil {
			return
		}
		d.queue = append(d.queue, &Event{Type: EventBlockDelta, Index: d.itemIdx[w.ItemID], DeltaType: DeltaInputJSON, Text: w.Delta})
	case "response.output_item.done":
		var w struct {
			Item struct {
				ID string `json:"id"`
			} `json:"item"`
		}
		if json.Unmarshal([]byte(data), &w) != nil {
			return
		}
		if idx, ok := d.itemIdx[w.Item.ID]; ok {
			d.queue = append(d.queue, &Event{Type: EventBlockStop, Index: idx})
		}
	case "response.completed", "response.incomplete":
		var w struct {
			Response responsesUpstreamResponse `json:"response"`
		}
		if json.Unmarshal([]byte(data), &w) != nil {
			return
		}
		usage := responsesUsageToCanonical(w.Response.Usage)
		d.queue = append(d.queue, &Event{Type: EventUsage, Usage: &usage})
		stop := StopStop
		if d.hasTool {
			stop = StopToolCalls
		}
		if event == "response.incomplete" &&
			w.Response.IncompleteDetails != nil && w.Response.IncompleteDetails.Reason == "max_output_tokens" {
			stop = StopLength
		}
		d.queue = append(d.queue, &Event{Type: EventMessageDelta, StopReason: stop})
		d.queue = append(d.queue, &Event{Type: EventDone})
	case "response.failed":
		var w struct {
			Response struct {
				Error *struct {
					Code    any    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			} `json:"response"`
		}
		if json.Unmarshal([]byte(data), &w) != nil {
			return
		}
		msg := "上游响应失败"
		if w.Response.Error != nil && w.Response.Error.Message != "" {
			msg = w.Response.Error.Message
		}
		d.queue = append(d.queue, &Event{Type: EventError, Error: NewRelayError(http.StatusBadGateway, "api_error", "", msg)})
	case "error":
		var w struct {
			Code    any    `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(data), &w) != nil || w.Message == "" {
			return
		}
		d.queue = append(d.queue, &Event{Type: EventError, Error: NewRelayError(http.StatusBadGateway, "api_error", "", w.Message)})
	}
}

// finalize 流结束（正常或中断）时的兜底收尾：补 message_start / done。
func (d *ResponsesStreamDecoder) finalize() *Event {
	if !d.started {
		d.started = true
		d.queue = append([]*Event{{Type: EventMessageStart}}, d.queue...)
	}
	d.queue = append(d.queue, &Event{Type: EventDone})
	ev := d.queue[0]
	d.queue = d.queue[1:]
	return ev
}

// Close 实现 StreamDecoder。
func (d *ResponsesStreamDecoder) Close() error {
	if d.closer != nil {
		return d.closer.Close()
	}
	return nil
}
