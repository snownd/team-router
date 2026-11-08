package protocol

import (
	"encoding/json"
	"net/http"
)

// ---- canonical 事件 -> 客户端 Anthropic Messages SSE ----

// AnthropicStreamEncoder canonical 事件 -> Anthropic Messages 流式响应。
type AnthropicStreamEncoder struct {
	w       http.ResponseWriter
	flusher http.Flusher
	msgID   string
	model   string
	usage   Usage
	started bool
}

// NewAnthropicStreamEncoder 构造。
func NewAnthropicStreamEncoder(w http.ResponseWriter) *AnthropicStreamEncoder {
	f, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	return &AnthropicStreamEncoder{w: w, flusher: f, msgID: "msg_" + randID()}
}

// WriteEvent 写入一个 canonical 事件。
func (e *AnthropicStreamEncoder) WriteEvent(ev *Event) error {
	switch ev.Type {
	case EventMessageStart:
		if ev.ID != "" {
			e.msgID = ev.ID
		}
		if ev.Model != "" {
			e.model = ev.Model
		}
		if ev.Usage != nil {
			e.usage.Add(*ev.Usage)
		}
		e.started = true
		u := e.usage
		return e.writeEvent("message_start", map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id": e.msgID, "type": "message", "role": "assistant",
				"content": []any{}, "model": e.model, "stop_reason": nil, "stop_sequence": nil,
				"usage": map[string]any{
					"input_tokens": u.InputTokens, "output_tokens": 0,
				},
			},
		})
	case EventBlockStart:
		var block map[string]any
		switch ev.BlockType {
		case BlockTypeText:
			block = map[string]any{"type": "text", "text": ""}
		case BlockTypeThinking:
			block = map[string]any{"type": "thinking", "thinking": ""}
		case BlockTypeToolUse:
			block = map[string]any{"type": "tool_use", "id": ev.ToolCall.ID, "name": ev.ToolCall.Name, "input": map[string]any{}}
		default:
			return nil
		}
		return e.writeEvent("content_block_start", map[string]any{
			"type": "content_block_start", "index": ev.Index, "content_block": block,
		})
	case EventBlockDelta:
		var delta map[string]any
		switch ev.DeltaType {
		case DeltaText:
			delta = map[string]any{"type": "text_delta", "text": ev.Text}
		case DeltaThinking:
			delta = map[string]any{"type": "thinking_delta", "thinking": ev.Text}
		case DeltaInputJSON:
			delta = map[string]any{"type": "input_json_delta", "partial_json": ev.Text}
		default:
			return nil
		}
		return e.writeEvent("content_block_delta", map[string]any{
			"type": "content_block_delta", "index": ev.Index, "delta": delta,
		})
	case EventBlockStop:
		return e.writeEvent("content_block_stop", map[string]any{
			"type": "content_block_stop", "index": ev.Index,
		})
	case EventMessageDelta:
		if ev.Usage != nil {
			e.usage.Add(*ev.Usage)
		}
		usage := map[string]any{"output_tokens": e.usage.OutputTokens}
		// 上游若在结束后才给出输入用量（如 openai），补到 message_delta
		if e.usage.InputTokens > 0 {
			usage["input_tokens"] = e.usage.InputTokens
		}
		if e.usage.CacheReadTokens > 0 {
			usage["cache_read_input_tokens"] = e.usage.CacheReadTokens
		}
		if e.usage.CacheWriteTokens > 0 {
			usage["cache_creation_input_tokens"] = e.usage.CacheWriteTokens
		}
		return e.writeEvent("message_delta", map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": canonicalToAnthropicStop(ev.StopReason), "stop_sequence": nil},
			"usage": usage,
		})
	case EventUsage:
		if ev.Usage != nil {
			e.usage.Add(*ev.Usage)
		}
	case EventDone:
		return e.writeEvent("message_stop", map[string]any{"type": "message_stop"})
	case EventError:
		em := ev.Error
		return e.writeEvent("error", map[string]any{
			"type":  "error",
			"error": map[string]any{"type": em.Type, "message": em.Message},
		})
	}
	e.flush()
	return nil
}

func (e *AnthropicStreamEncoder) writeEvent(name string, payload any) error {
	if err := sseWriteEvent(e.w, name, payload); err != nil {
		return err
	}
	e.flush()
	return nil
}

func (e *AnthropicStreamEncoder) flush() {
	if e.flusher != nil {
		e.flusher.Flush()
	}
}

// ---- canonical -> 客户端非流式响应 ----

// AnthropicEncodeResponse canonical -> Anthropic Messages 非流式响应。
func AnthropicEncodeResponse(resp *Response) map[string]any {
	var content []map[string]any
	for _, b := range resp.Blocks {
		switch b.Type {
		case BlockTypeText:
			content = append(content, map[string]any{"type": "text", "text": b.Text})
		case BlockTypeThinking:
			content = append(content, map[string]any{"type": "thinking", "thinking": b.Text})
		case BlockTypeToolUse:
			input := map[string]any{}
			_ = json.Unmarshal([]byte(b.ToolCall.Arguments), &input)
			content = append(content, map[string]any{
				"type": "tool_use", "id": b.ToolCall.ID, "name": b.ToolCall.Name, "input": input,
			})
		}
	}
	if content == nil {
		content = []map[string]any{}
	}
	id := resp.ID
	if id == "" {
		id = "msg_" + randID()
	}
	return map[string]any{
		"id":            id,
		"type":          "message",
		"role":          "assistant",
		"content":       content,
		"model":         resp.Model,
		"stop_reason":   canonicalToAnthropicStop(resp.StopReason),
		"stop_sequence": nil,
		"usage":         canonicalToAnthropicUsage(&resp.Usage),
	}
}
