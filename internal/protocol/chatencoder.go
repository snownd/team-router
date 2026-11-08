package protocol

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// sseWrite 写一条 SSE data（无 event 名，OpenAI Chat 风格）。
func sseWriteData(w http.ResponseWriter, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
		return err
	}
	return nil
}

func sseWriteRaw(w http.ResponseWriter, s string) error {
	_, err := fmt.Fprintf(w, "data: %s\n\n", s)
	return err
}

// sseWriteEvent 写带 event 名的 SSE（Anthropic / Responses 风格）。
func sseWriteEvent(w http.ResponseWriter, event string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b); err != nil {
		return err
	}
	return nil
}

// ---- canonical 事件 -> 客户端 Chat SSE ----

// ChatStreamEncoder canonical 事件 -> OpenAI Chat 流式响应。
type ChatStreamEncoder struct {
	w        http.ResponseWriter
	flusher  http.Flusher
	id       string
	created  int64
	model    string
	roleSent bool
	usage    Usage
	finish   string
}

// NewChatStreamEncoder 构造。
func NewChatStreamEncoder(w http.ResponseWriter) *ChatStreamEncoder {
	f, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	return &ChatStreamEncoder{w: w, flusher: f, id: "chatcmpl-" + randID(), created: time.Now().Unix()}
}

// WriteEvent 写入一个 canonical 事件。
func (e *ChatStreamEncoder) WriteEvent(ev *Event) error {
	switch ev.Type {
	case EventMessageStart:
		if ev.ID != "" {
			e.id = ev.ID
		}
		if ev.Model != "" {
			e.model = ev.Model
		}
		if ev.Usage != nil {
			e.usage.Add(*ev.Usage)
		}
	case EventBlockStart:
		if ev.BlockType == BlockTypeToolUse {
			e.ensureRole()
			if err := sseWriteData(e.w, e.chunk(&chatRespMsg{
				ToolCalls: []chatToolCall{{
					Index:    &ev.Index,
					ID:       ev.ToolCall.ID,
					Type:     "function",
					Function: chatToolCallFn{Name: ev.ToolCall.Name},
				}},
			}, nil)); err != nil {
				return err
			}
		}
	case EventBlockDelta:
		e.ensureRole()
		var msg *chatRespMsg
		switch ev.DeltaType {
		case DeltaText:
			msg = &chatRespMsg{Content: ev.Text}
		case DeltaThinking:
			msg = &chatRespMsg{ReasoningContent: ev.Text}
		case DeltaInputJSON:
			idx := ev.Index
			msg = &chatRespMsg{ToolCalls: []chatToolCall{{
				Index:    &idx,
				Function: chatToolCallFn{Arguments: ev.Text},
			}}}
		}
		if msg != nil {
			if err := sseWriteData(e.w, e.chunk(msg, nil)); err != nil {
				return err
			}
		}
	case EventBlockStop:
		// chat 协议无需块级收尾
	case EventMessageDelta:
		e.finish = canonicalToOpenAIStop(ev.StopReason)
		if ev.Usage != nil {
			e.usage.Add(*ev.Usage)
		}
	case EventUsage:
		if ev.Usage != nil {
			e.usage.Add(*ev.Usage)
		}
	case EventDone:
		// finish chunk
		if e.finish == "" {
			e.finish = "stop"
		}
		if err := sseWriteData(e.w, e.chunk(&chatRespMsg{}, &e.finish)); err != nil {
			return err
		}
		// usage chunk（对齐 include_usage 行为）
		if e.usage.TotalInput() > 0 || e.usage.OutputTokens > 0 {
			if err := sseWriteData(e.w, e.chunkWithUsage(nil, nil, &e.usage)); err != nil {
				return err
			}
		}
		if err := sseWriteRaw(e.w, "[DONE]"); err != nil {
			return err
		}
	case EventError:
		// 流中错误：OpenAI 语义下发一条 error data 后终止
		em := ev.Error
		_ = sseWriteData(e.w, map[string]any{"error": map[string]any{
			"message": em.Message, "type": em.Type, "code": em.Code,
		}})
		_ = sseWriteRaw(e.w, "[DONE]")
	}
	e.flush()
	return nil
}

func (e *ChatStreamEncoder) ensureRole() {
	if e.roleSent {
		return
	}
	e.roleSent = true
	_ = sseWriteData(e.w, e.chunk(&chatRespMsg{Role: "assistant"}, nil))
}

func (e *ChatStreamEncoder) chunk(msg *chatRespMsg, finish *string) chatResponseWire {
	return e.chunkWithUsage(msg, finish, nil)
}

func (e *ChatStreamEncoder) chunkWithUsage(msg *chatRespMsg, finish *string, u *Usage) chatResponseWire {
	w := chatResponseWire{
		ID: e.id, Object: "chat.completion.chunk", Created: e.created, Model: e.model,
	}
	if msg != nil {
		w.Choices = []chatChoice{{Index: 0, Delta: msg, FinishReason: finish}}
	} else if finish != nil {
		w.Choices = []chatChoice{{Index: 0, Delta: &chatRespMsg{}, FinishReason: finish}}
	} else {
		w.Choices = []chatChoice{} // usage 块：空 choices
	}
	if u != nil {
		w.Usage = canonicalToChatUsage(u)
	}
	return w
}

func (e *ChatStreamEncoder) flush() {
	if e.flusher != nil {
		e.flusher.Flush()
	}
}

// canonicalToChatUsage canonical -> openai usage 结构。
func canonicalToChatUsage(u *Usage) *chatUsageWire {
	if u == nil {
		return nil
	}
	prompt := u.TotalInput()
	return &chatUsageWire{
		PromptTokens:            prompt,
		CompletionTokens:        u.OutputTokens,
		TotalTokens:             prompt + u.OutputTokens,
		PromptTokensDetails:     &chatPromptTokensDetails{CachedTokens: u.CacheReadTokens},
		CompletionTokensDetails: &chatCompletionTokensDetails{ReasoningTokens: u.ReasoningTokens},
	}
}

// ---- canonical -> 客户端非流式响应 ----

// ChatEncodeResponse canonical -> OpenAI Chat 非流式响应对象。
func ChatEncodeResponse(resp *Response) map[string]any {
	msg := &chatRespMsg{Role: "assistant"}
	var toolCalls []chatToolCall
	for _, b := range resp.Blocks {
		switch b.Type {
		case BlockTypeText:
			msg.Content += b.Text
		case BlockTypeThinking:
			msg.ReasoningContent += b.Text
		case BlockTypeToolUse:
			toolCalls = append(toolCalls, chatToolCall{
				ID: b.ToolCall.ID, Type: "function",
				Function: chatToolCallFn{Name: b.ToolCall.Name, Arguments: b.ToolCall.Arguments},
			})
		}
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}
	finish := canonicalToOpenAIStop(resp.StopReason)
	return map[string]any{
		"id":      nonEmpty(resp.ID, "chatcmpl-"+randID()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   resp.Model,
		"choices": []chatChoice{{Index: 0, Message: msg, FinishReason: &finish}},
		"usage":   canonicalToChatUsage(&resp.Usage),
	}
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

var randSrc = newIDGen()

func randID() string { return randSrc.Next() }
