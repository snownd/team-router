package protocol

import (
	"encoding/json"
	"io"
	"strings"
)

// ============================================================
// Anthropic Messages wire 格式（双向）：
//   - ParseAnthropicWire: 客户端 -> canonical
//   - CanonicalToAnthropicWire: canonical -> 上游
//   - AnthropicWireToResponse: 上游非流式 -> canonical
//   - AnthropicStreamDecoder: 上游 SSE -> canonical
//   - AnthropicStreamEncoder: canonical -> 客户端 SSE
//   - AnthropicEncodeResponse: canonical -> 客户端非流式
// ============================================================

// ---- wire 结构 ----

type anthropicContentWire struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`

	// thinking
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`

	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`

	// image
	Source *anthropicImageSource `json:"source,omitempty"`

	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicCacheControl struct {
	Type string `json:"type"` // ephemeral
}

type anthropicImageSource struct {
	Type      string `json:"type"` // base64
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicMessageWire struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthropicToolWire struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	InputSchema  json.RawMessage        `json:"input_schema,omitempty"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicRequestWire struct {
	Model         string                 `json:"model"`
	System        json.RawMessage        `json:"system,omitempty"`
	Messages      []anthropicMessageWire `json:"messages"`
	MaxTokens     int                    `json:"max_tokens,omitempty"`
	Temperature   *float64               `json:"temperature,omitempty"`
	TopP          *float64               `json:"top_p,omitempty"`
	StopSequences []string               `json:"stop_sequences,omitempty"`
	Stream        bool                   `json:"stream,omitempty"`
	Tools         []anthropicToolWire    `json:"tools,omitempty"`
	ToolChoice    *anthropicToolChoice   `json:"tool_choice,omitempty"`
	Metadata      map[string]any         `json:"metadata,omitempty"`
}

type anthropicToolChoice struct {
	Type string `json:"type"` // auto | any | tool
	Name string `json:"name,omitempty"`
}

type anthropicResponseWire struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	Role         string                 `json:"role"`
	Content      []anthropicContentWire `json:"content"`
	Model        string                 `json:"model"`
	StopReason   string                 `json:"stop_reason"`
	StopSequence *string                `json:"stop_sequence"`
	Usage        *anthropicUsageWire    `json:"usage"`
}

type anthropicUsageWire struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// ---- 解析（客户端 -> canonical） ----

// ParseAnthropicWire 解析 Anthropic Messages 请求。
func ParseAnthropicWire(body []byte) (*Request, *RelayError) {
	var w anthropicRequestWire
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, ErrInvalidRequest("请求体不是合法 JSON: " + err.Error())
	}
	if w.Model == "" {
		return nil, ErrInvalidRequest("model 不能为空")
	}
	if len(w.Messages) == 0 {
		return nil, ErrInvalidRequest("messages 不能为空")
	}

	req := &Request{
		Model:         w.Model,
		Temperature:   w.Temperature,
		TopP:          w.TopP,
		MaxTokens:     w.MaxTokens,
		StopSequences: w.StopSequences,
		Stream:        w.Stream,
	}
	if md, ok := w.Metadata["user_id"].(string); ok {
		req.UserID = md
	}

	// system: string 或 blocks
	if len(w.System) > 0 && string(w.System) != "null" {
		var s string
		if err := json.Unmarshal(w.System, &s); err == nil {
			req.System = s
		} else {
			var blocks []anthropicContentWire
			if err := json.Unmarshal(w.System, &blocks); err == nil {
				var sb strings.Builder
				for _, b := range blocks {
					sb.WriteString(b.Text)
					if b.CacheControl != nil && req.SystemCacheControl == nil {
						cc := "ephemeral"
						req.SystemCacheControl = &cc
					}
				}
				req.System = sb.String()
			}
		}
	}

	for _, m := range w.Messages {
		msgs, err := parseAnthropicMessage(m)
		if err != nil {
			return nil, err
		}
		req.Messages = append(req.Messages, msgs...)
	}

	for _, t := range w.Tools {
		req.Tools = append(req.Tools, Tool{
			Name: t.Name, Description: t.Description, Parameters: t.InputSchema,
		})
	}
	if w.ToolChoice != nil {
		switch w.ToolChoice.Type {
		case "auto":
			req.ToolChoice = "auto"
		case "any":
			req.ToolChoice = "required"
		case "tool":
			req.ToolChoice = w.ToolChoice.Name
		}
	}
	return req, nil
}

// parseAnthropicMessage 将 anthropic 消息转 canonical（tool_result 会拆出独立的 tool 消息）。
func parseAnthropicMessage(m anthropicMessageWire) ([]Message, *RelayError) {
	var out []Message
	base := Message{Role: m.Role}

	// content: string 或 blocks
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		base.Text = s
		return append(out, base), nil
	}
	var blocks []anthropicContentWire
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return nil, ErrInvalidRequest("message.content 格式不支持")
	}

	var textBuf strings.Builder
	var richBlocks []Block
	for _, b := range blocks {
		cc := cacheControlPtr(b.CacheControl)
		switch b.Type {
		case "text":
			textBuf.WriteString(b.Text)
			richBlocks = append(richBlocks, Block{Type: BlockTypeText, Text: b.Text, CacheControl: cc})
		case "thinking":
			richBlocks = append(richBlocks, Block{Type: BlockTypeThinking, Text: b.Thinking})
		case "image":
			if b.Source != nil && b.Source.Type == "base64" {
				richBlocks = append(richBlocks, Block{
					Type: BlockTypeImage, MimeType: b.Source.MediaType, Data: b.Source.Data, CacheControl: cc,
				})
			}
		case "tool_use":
			base.ToolCalls = append(base.ToolCalls, ToolCall{
				ID: b.ID, Name: b.Name,
				Arguments: string(orEmptyJSON(b.Input)),
			})
		case "tool_result":
			// 拆为独立 tool 消息
			text := ""
			if len(b.Content) > 0 {
				var cs string
				if err := json.Unmarshal(b.Content, &cs); err == nil {
					text = cs
				} else {
					var cblocks []anthropicContentWire
					if err := json.Unmarshal(b.Content, &cblocks); err == nil {
						for _, cb := range cblocks {
							text += cb.Text
						}
					}
				}
			}
			out = append(out, Message{Role: "tool", ToolCallID: b.ToolUseID, Text: text})
		case "redacted_thinking":
			// 忽略
		default:
			// 未知块透传为文本（尽力而为）
		}
	}

	// 只有文本块时退化为纯文本
	if len(richBlocks) > 0 && allText(richBlocks) {
		base.Text = textBuf.String()
	} else if len(richBlocks) > 0 {
		base.Blocks = richBlocks
	}
	out = append(out, base)
	return out, nil
}

func cacheControlPtr(cc *anthropicCacheControl) *string {
	if cc == nil {
		return nil
	}
	t := cc.Type
	return &t
}

func orEmptyJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	return raw
}

// ---- canonical -> 上游 anthropic 请求 ----

// CanonicalToAnthropicWire canonical -> Anthropic Messages 请求体。
// defaultMaxTokens 在请求未指定 max_tokens 时使用（anthropic 必填）。
func CanonicalToAnthropicWire(req *Request, upstreamModel string, stream bool, defaultMaxTokens int) ([]byte, error) {
	w := anthropicRequestWire{
		Model:         upstreamModel,
		MaxTokens:     req.MaxTokens,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		StopSequences: req.StopSequences,
		Stream:        stream,
	}
	if w.MaxTokens <= 0 {
		w.MaxTokens = defaultMaxTokens
	}
	if req.System != "" {
		if req.SystemCacheControl != nil {
			w.System = json.RawMessage(mustJSON([]anthropicContentWire{{
				Type: "text", Text: req.System,
				CacheControl: &anthropicCacheControl{Type: *req.SystemCacheControl},
			}}))
		} else {
			w.System = json.RawMessage(mustJSON(req.System))
		}
	}

	// canonical tool 消息需要合并回 user 的 tool_result 块：按顺序归并
	var msgs []anthropicMessageWire
	var pendingTools []anthropicContentWire
	flushTools := func() {
		if len(pendingTools) == 0 {
			return
		}
		msgs = append(msgs, anthropicMessageWire{
			Role:    "user",
			Content: json.RawMessage(mustJSON(pendingTools)),
		})
		pendingTools = nil
	}
	for _, m := range req.Messages {
		if m.Role == "tool" {
			pendingTools = append(pendingTools, anthropicContentWire{
				Type: "tool_result", ToolUseID: m.ToolCallID,
				Content: json.RawMessage(mustJSON(m.Text)),
			})
			continue
		}
		flushTools()
		if m.Role == "system" {
			// anthropic 不接受 system 消息，并入 system 参数
			if w.System == nil {
				w.System = json.RawMessage(mustJSON(m.Text))
			}
			continue
		}
		role := m.Role
		if role != "assistant" {
			role = "user"
		}
		msgs = append(msgs, canonicalMsgToAnthropic(role, m))
	}
	flushTools()
	w.Messages = msgs

	for _, t := range req.Tools {
		w.Tools = append(w.Tools, anthropicToolWire{
			Name: t.Name, Description: t.Description,
			InputSchema: orEmptyJSON(t.Parameters),
		})
	}
	if tc, ok := req.ToolChoice.(string); ok && tc != "" {
		switch tc {
		case "auto":
			w.ToolChoice = &anthropicToolChoice{Type: "auto"}
		case "required":
			w.ToolChoice = &anthropicToolChoice{Type: "any"}
		default:
			w.ToolChoice = &anthropicToolChoice{Type: "tool", Name: tc}
		}
	}
	if req.UserID != "" {
		w.Metadata = map[string]any{"user_id": req.UserID}
	}
	return json.Marshal(w)
}

func canonicalMsgToAnthropic(role string, m Message) anthropicMessageWire {
	var blocks []anthropicContentWire
	if len(m.Blocks) > 0 {
		for _, b := range m.Blocks {
			switch b.Type {
			case BlockTypeText:
				blocks = append(blocks, anthropicContentWire{
					Type: "text", Text: b.Text, CacheControl: cacheControlWire(b.CacheControl),
				})
			case BlockTypeThinking:
				blocks = append(blocks, anthropicContentWire{Type: "thinking", Thinking: b.Text})
			case BlockTypeImage:
				blocks = append(blocks, anthropicContentWire{
					Type:         "image",
					Source:       &anthropicImageSource{Type: "base64", MediaType: b.MimeType, Data: b.Data},
					CacheControl: cacheControlWire(b.CacheControl),
				})
			}
		}
	} else if m.Text != "" {
		blocks = append(blocks, anthropicContentWire{Type: "text", Text: m.Text})
	}
	for _, tc := range m.ToolCalls {
		args := json.RawMessage(orEmptyJSON(json.RawMessage(tc.Arguments)))
		blocks = append(blocks, anthropicContentWire{
			Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: args,
		})
	}
	// 消息级 cache_control 应用于最后一个块
	if m.CacheControl != nil && len(blocks) > 0 && blocks[len(blocks)-1].CacheControl == nil {
		blocks[len(blocks)-1].CacheControl = cacheControlWire(m.CacheControl)
	}
	if len(blocks) == 0 {
		blocks = []anthropicContentWire{{Type: "text", Text: ""}}
	}
	return anthropicMessageWire{Role: role, Content: json.RawMessage(mustJSON(blocks))}
}

func cacheControlWire(cc *string) *anthropicCacheControl {
	if cc == nil {
		return nil
	}
	return &anthropicCacheControl{Type: *cc}
}

// ---- 停止原因映射 ----

func normalizeAnthropicStop(s string) string {
	switch s {
	case "end_turn", "stop_sequence", "":
		return StopStop
	case "max_tokens":
		return StopLength
	case "tool_use":
		return StopToolCalls
	case "refusal":
		return StopContentFilter
	default:
		return StopStop
	}
}

func canonicalToAnthropicStop(s string) string {
	switch s {
	case StopLength:
		return "max_tokens"
	case StopToolCalls:
		return "tool_use"
	case StopContentFilter:
		return "refusal"
	default:
		return "end_turn"
	}
}

func anthropicUsageToCanonical(u *anthropicUsageWire) Usage {
	if u == nil {
		return Usage{}
	}
	return Usage{
		InputTokens:      u.InputTokens,
		CacheReadTokens:  u.CacheReadInputTokens,
		CacheWriteTokens: u.CacheCreationInputTokens,
		OutputTokens:     u.OutputTokens,
	}
}

func canonicalToAnthropicUsage(u *Usage) *anthropicUsageWire {
	if u == nil {
		return nil
	}
	return &anthropicUsageWire{
		InputTokens:              u.InputTokens,
		OutputTokens:             u.OutputTokens,
		CacheCreationInputTokens: u.CacheWriteTokens,
		CacheReadInputTokens:     u.CacheReadTokens,
	}
}

// ---- 上游非流式响应 -> canonical ----

// AnthropicWireToResponse 上游非流式响应 -> canonical。
func AnthropicWireToResponse(body []byte, model string) (*Response, *RelayError) {
	var w anthropicResponseWire
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, ErrUpstream("上游响应解析失败: " + err.Error())
	}
	resp := &Response{ID: w.ID, Model: model, StopReason: normalizeAnthropicStop(w.StopReason)}
	for _, b := range w.Content {
		switch b.Type {
		case "text":
			resp.Blocks = append(resp.Blocks, ContentBlock{Type: BlockTypeText, Text: b.Text})
		case "thinking":
			resp.Blocks = append(resp.Blocks, ContentBlock{Type: BlockTypeThinking, Text: b.Thinking})
		case "tool_use":
			resp.Blocks = append(resp.Blocks, ContentBlock{
				Type:     BlockTypeToolUse,
				ToolCall: ToolCall{ID: b.ID, Name: b.Name, Arguments: string(orEmptyJSON(b.Input))},
			})
		}
	}
	resp.Usage = anthropicUsageToCanonical(w.Usage)
	return resp, nil
}

// ---- 上游 SSE -> canonical ----

// AnthropicStreamDecoder 上游 Anthropic SSE -> canonical 事件。
type AnthropicStreamDecoder struct {
	reader *SSEReader
	closer io.Closer
}

// NewAnthropicStreamDecoder 构造。
func NewAnthropicStreamDecoder(r io.Reader, closer io.Closer) *AnthropicStreamDecoder {
	return &AnthropicStreamDecoder{reader: NewSSEReader(r), closer: closer}
}

// Next 实现 StreamDecoder。
func (d *AnthropicStreamDecoder) Next() (*Event, error) {
	for {
		event, data, err := d.reader.Next()
		if err != nil {
			return nil, err
		}
		switch event {
		case "message_start":
			var w struct {
				Message anthropicResponseWire `json:"message"`
			}
			if json.Unmarshal([]byte(data), &w) != nil {
				continue
			}
			u := anthropicUsageToCanonical(w.Message.Usage)
			return &Event{Type: EventMessageStart, ID: w.Message.ID, Model: w.Message.Model, Usage: &u}, nil
		case "content_block_start":
			var w struct {
				Index        int                  `json:"index"`
				ContentBlock anthropicContentWire `json:"content_block"`
			}
			if json.Unmarshal([]byte(data), &w) != nil {
				continue
			}
			ev := &Event{Type: EventBlockStart, Index: w.Index}
			switch w.ContentBlock.Type {
			case "text":
				ev.BlockType = BlockTypeText
			case "thinking":
				ev.BlockType = BlockTypeThinking
			case "tool_use":
				ev.BlockType = BlockTypeToolUse
				ev.ToolCall = ToolCall{ID: w.ContentBlock.ID, Name: w.ContentBlock.Name}
			default:
				continue
			}
			return ev, nil
		case "content_block_delta":
			var w struct {
				Index int `json:"index"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					Thinking    string `json:"thinking"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			}
			if json.Unmarshal([]byte(data), &w) != nil {
				continue
			}
			ev := &Event{Type: EventBlockDelta, Index: w.Index, Text: w.Delta.Text}
			switch w.Delta.Type {
			case "text_delta":
				ev.DeltaType = DeltaText
				ev.Text = w.Delta.Text
			case "thinking_delta":
				ev.DeltaType = DeltaThinking
				ev.Text = w.Delta.Thinking
			case "input_json_delta":
				ev.DeltaType = DeltaInputJSON
				ev.Text = w.Delta.PartialJSON
			default:
				continue
			}
			return ev, nil
		case "content_block_stop":
			var w struct {
				Index int `json:"index"`
			}
			if json.Unmarshal([]byte(data), &w) != nil {
				continue
			}
			return &Event{Type: EventBlockStop, Index: w.Index}, nil
		case "message_delta":
			var w struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
				Usage *anthropicUsageWire `json:"usage"`
			}
			if json.Unmarshal([]byte(data), &w) != nil {
				continue
			}
			ev := &Event{Type: EventMessageDelta, StopReason: normalizeAnthropicStop(w.Delta.StopReason)}
			if w.Usage != nil {
				u := anthropicUsageToCanonical(w.Usage)
				ev.Usage = &u
			}
			return ev, nil
		case "message_stop":
			return &Event{Type: EventDone}, nil
		case "error":
			var w struct {
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal([]byte(data), &w)
			return &Event{Type: EventError, Error: NewRelayError(500, orDefault(w.Error.Type, "api_error"), "", w.Error.Message)}, nil
		default:
			// ping 等事件忽略
		}
	}
}

// Close 实现 StreamDecoder。
func (d *AnthropicStreamDecoder) Close() error {
	if d.closer != nil {
		return d.closer.Close()
	}
	return nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
