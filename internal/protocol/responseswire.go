package protocol

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// ============================================================
// OpenAI Responses API wire 格式（v1 支持无状态子集）：
//   - ParseResponsesWire: 客户端 -> canonical（不支持 previous_response_id）
//   - ResponsesStreamEncoder: canonical 事件 -> 客户端 SSE
//   - ResponsesEncodeResponse: canonical -> 客户端非流式响应
// 上游方向（canonical -> 上游 /v1/responses 及上游 SSE/JSON 解码）见 responsesupstream.go
// （openai_responses 适配器）。
// ============================================================

// ParseResponsesWire 解析客户端 Responses 请求。
func ParseResponsesWire(body []byte) (*Request, *RelayError) {
	var raw struct {
		Model        string          `json:"model"`
		Input        json.RawMessage `json:"input"`
		Instructions string          `json:"instructions"`
		Tools        []struct {
			Type        string          `json:"type"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
		} `json:"tools"`
		ToolChoice         any            `json:"tool_choice"`
		Temperature        *float64       `json:"temperature"`
		TopP               *float64       `json:"top_p"`
		MaxOutputTokens    *int           `json:"max_output_tokens"`
		Stream             bool           `json:"stream"`
		Store              bool           `json:"store"`
		PreviousResponseID string         `json:"previous_response_id"`
		Metadata           map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, ErrInvalidRequest("请求体不是合法 JSON: " + err.Error())
	}
	if raw.Model == "" {
		return nil, ErrInvalidRequest("model 不能为空")
	}
	// v1 不支持有状态模式
	if raw.PreviousResponseID != "" {
		return nil, ErrInvalidRequest("v1 不支持 previous_response_id（有状态模式），请在 input 中携带完整上下文")
	}

	req := &Request{
		Model:              raw.Model,
		System:             raw.Instructions,
		ToolChoice:         normalizeToolChoice(raw.ToolChoice),
		Temperature:        raw.Temperature,
		TopP:               raw.TopP,
		Stream:             raw.Stream,
		Store:              raw.Store,
		PreviousResponseID: raw.PreviousResponseID,
		Metadata:           raw.Metadata,
	}
	if raw.MaxOutputTokens != nil {
		req.MaxTokens = *raw.MaxOutputTokens
	}
	for _, t := range raw.Tools {
		if t.Type == "function" {
			req.Tools = append(req.Tools, Tool{Name: t.Name, Description: t.Description, Parameters: t.Parameters})
		}
	}

	// input: string 或 items 数组
	if len(raw.Input) == 0 || string(raw.Input) == "null" {
		return nil, ErrInvalidRequest("input 不能为空")
	}
	var s string
	if err := json.Unmarshal(raw.Input, &s); err == nil {
		req.Messages = append(req.Messages, Message{Role: "user", Text: s})
		return req, nil
	}

	var items []struct {
		Type      string          `json:"type"`
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
		CallID    string          `json:"call_id"`
		Name      string          `json:"name"`
		Arguments string          `json:"arguments"`
		Output    string          `json:"output"`
	}
	if err := json.Unmarshal(raw.Input, &items); err != nil {
		return nil, ErrInvalidRequest("input.items 格式不支持")
	}
	for _, it := range items {
		switch it.Type {
		case "function_call":
			req.Messages = append(req.Messages, Message{
				Role:      "assistant",
				ToolCalls: []ToolCall{{ID: it.CallID, Name: it.Name, Arguments: orDefault(it.Arguments, "{}")}},
			})
		case "function_call_output":
			req.Messages = append(req.Messages, Message{
				Role: "tool", ToolCallID: it.CallID, Text: it.Output,
			})
		case "", "message":
			// EasyInputMessage：type 可省略（默认 message），content 可为
			// string 或 parts 数组（input_text / output_text / 图片等，仅取文本）
			if it.Role == "system" || it.Role == "developer" {
				if req.System == "" {
					req.System = responsesContentText(it.Content)
				} else {
					req.System += "\n\n" + responsesContentText(it.Content)
				}
				continue
			}
			role := it.Role
			if role != "assistant" {
				role = "user"
			}
			req.Messages = append(req.Messages, Message{Role: role, Text: responsesContentText(it.Content)})
		default:
			// reasoning / item_reference / web_search_call / file_search_call /
			// image_generation_call 等：v1 忽略
		}
	}
	if len(req.Messages) == 0 && req.System == "" {
		return nil, ErrInvalidRequest("input 中没有可用消息")
	}
	return req, nil
}

// responsesContentText 解析 message item 的 content（string 或 parts 数组），拼接文本。
func responsesContentText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.Text)
	}
	return sb.String()
}

// ---- responses 响应对象构造 ----

type responsesUsage struct {
	InputTokens        int `json:"input_tokens"`
	InputTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details,omitempty"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func canonicalToResponsesUsage(u *Usage) *responsesUsage {
	if u == nil {
		return nil
	}
	in := u.TotalInput()
	return &responsesUsage{
		InputTokens: in,
		InputTokensDetails: &struct {
			CachedTokens int `json:"cached_tokens"`
		}{u.CacheReadTokens},
		OutputTokens: u.OutputTokens,
		TotalTokens:  in + u.OutputTokens,
	}
}

// responsesMessageItem 消息输出项。
func responsesMessageItem(id string, text string, annotations []any) map[string]any {
	return map[string]any{
		"type": "message", "id": id, "role": "assistant", "status": "completed",
		"content": []map[string]any{{
			"type": "output_text", "text": text, "annotations": annotations,
		}},
	}
}

// responsesFunctionCallItem 函数调用输出项。
func responsesFunctionCallItem(id, callID, name, args string) map[string]any {
	return map[string]any{
		"type": "function_call", "id": id, "call_id": callID,
		"name": name, "arguments": args, "status": "completed",
	}
}

// ResponsesEncodeResponse canonical -> Responses 非流式响应。
func ResponsesEncodeResponse(resp *Response) map[string]any {
	var output []map[string]any
	msgIdx := 0
	for _, b := range resp.Blocks {
		switch b.Type {
		case BlockTypeText:
			output = append(output, responsesMessageItem(
				"msg_"+randID(), b.Text, []any{}))
			msgIdx++
		case BlockTypeThinking:
			// v1 将 thinking 作为独立 message item 输出
			output = append(output, responsesMessageItem(
				"msg_"+randID(), b.Text, []any{}))
			msgIdx++
		case BlockTypeToolUse:
			output = append(output, responsesFunctionCallItem(
				"fc_"+randID(), b.ToolCall.ID, b.ToolCall.Name, orDefault(b.ToolCall.Arguments, "{}")))
		}
	}
	if output == nil {
		output = []map[string]any{}
	}
	id := resp.ID
	if id == "" {
		id = "resp_" + randID()
	}
	return map[string]any{
		"id":         id,
		"object":     "response",
		"created_at": time.Now().Unix(),
		"status":     "completed",
		"model":      resp.Model,
		"output":     output,
		"usage":      canonicalToResponsesUsage(&resp.Usage),
		"error":      nil,
	}
}

// ---- canonical 事件 -> 客户端 Responses SSE ----

// ResponsesStreamEncoder canonical 事件 -> Responses API 事件流。
type ResponsesStreamEncoder struct {
	w         http.ResponseWriter
	flusher   http.Flusher
	id        string
	createdAt int64
	model     string
	seq       int

	// 每个 canonical 块索引对应的 item 状态
	items       map[int]*responsesItemState
	outputCount int
	usage       Usage
	stopReason  string
}

type responsesItemState struct {
	itemID    string
	callID    string
	name      string
	text      strings.Builder
	isFunc    bool
	closed    bool
	outputIdx int
}

// NewResponsesStreamEncoder 构造。
func NewResponsesStreamEncoder(w http.ResponseWriter) *ResponsesStreamEncoder {
	f, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	return &ResponsesStreamEncoder{
		w: w, flusher: f,
		id: "resp_" + randID(), createdAt: time.Now().Unix(),
		items: map[int]*responsesItemState{},
	}
}

// WriteEvent 写入一个 canonical 事件。
func (e *ResponsesStreamEncoder) WriteEvent(ev *Event) error {
	switch ev.Type {
	case EventMessageStart:
		if ev.Model != "" {
			e.model = ev.Model
		}
		if err := e.emit("response.created", map[string]any{
			"type": "response.created", "response": e.baseResponse("in_progress", nil),
		}); err != nil {
			return err
		}
		return e.emit("response.in_progress", map[string]any{
			"type": "response.in_progress", "response": e.baseResponse("in_progress", nil),
		})
	case EventBlockStart:
		st := &responsesItemState{outputIdx: e.outputCount}
		e.outputCount++
		switch ev.BlockType {
		case BlockTypeToolUse:
			st.isFunc = true
			st.itemID = "fc_" + randID()
			st.callID = orDefault(ev.ToolCall.ID, st.itemID)
			st.name = ev.ToolCall.Name
			if err := e.emit("response.output_item.added", map[string]any{
				"type": "response.output_item.added", "output_index": st.outputIdx,
				"item": map[string]any{
					"type": "function_call", "id": st.itemID, "call_id": st.callID,
					"name": st.name, "arguments": "", "status": "in_progress",
				},
			}); err != nil {
				return err
			}
		default: // text / thinking -> message item
			st.itemID = "msg_" + randID()
			if err := e.emit("response.output_item.added", map[string]any{
				"type": "response.output_item.added", "output_index": st.outputIdx,
				"item": map[string]any{
					"type": "message", "id": st.itemID, "role": "assistant",
					"status": "in_progress", "content": []any{},
				},
			}); err != nil {
				return err
			}
			if err := e.emit("response.content_part.added", map[string]any{
				"type": "response.content_part.added", "item_id": st.itemID,
				"output_index": st.outputIdx, "content_index": 0,
				"part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}},
			}); err != nil {
				return err
			}
		}
		e.items[ev.Index] = st
	case EventBlockDelta:
		st := e.items[ev.Index]
		if st == nil {
			return nil
		}
		switch ev.DeltaType {
		case DeltaText:
			st.text.WriteString(ev.Text)
			if err := e.emit("response.output_text.delta", map[string]any{
				"type": "response.output_text.delta", "item_id": st.itemID,
				"output_index": st.outputIdx, "content_index": 0, "delta": ev.Text,
			}); err != nil {
				return err
			}
		case DeltaThinking:
			st.text.WriteString(ev.Text)
			if err := e.emit("response.output_text.delta", map[string]any{
				"type": "response.output_text.delta", "item_id": st.itemID,
				"output_index": st.outputIdx, "content_index": 0, "delta": ev.Text,
			}); err != nil {
				return err
			}
		case DeltaInputJSON:
			st.text.WriteString(ev.Text)
			if err := e.emit("response.function_call_arguments.delta", map[string]any{
				"type": "response.function_call_arguments.delta", "item_id": st.itemID,
				"output_index": st.outputIdx, "delta": ev.Text,
			}); err != nil {
				return err
			}
		}
	case EventBlockStop:
		st := e.items[ev.Index]
		if st == nil || st.closed {
			return nil
		}
		st.closed = true
		if st.isFunc {
			if err := e.emit("response.function_call_arguments.done", map[string]any{
				"type": "response.function_call_arguments.done", "item_id": st.itemID,
				"output_index": st.outputIdx, "arguments": orDefault(st.text.String(), "{}"),
			}); err != nil {
				return err
			}
			if err := e.emit("response.output_item.done", map[string]any{
				"type": "response.output_item.done", "output_index": st.outputIdx,
				"item": responsesFunctionCallItem(st.itemID, st.callID, st.name, orDefault(st.text.String(), "{}")),
			}); err != nil {
				return err
			}
		} else {
			if err := e.emit("response.output_text.done", map[string]any{
				"type": "response.output_text.done", "item_id": st.itemID,
				"output_index": st.outputIdx, "content_index": 0, "text": st.text.String(),
			}); err != nil {
				return err
			}
			if err := e.emit("response.content_part.done", map[string]any{
				"type": "response.content_part.done", "item_id": st.itemID,
				"output_index": st.outputIdx, "content_index": 0,
				"part": map[string]any{"type": "output_text", "text": st.text.String(), "annotations": []any{}},
			}); err != nil {
				return err
			}
			if err := e.emit("response.output_item.done", map[string]any{
				"type": "response.output_item.done", "output_index": st.outputIdx,
				"item": responsesMessageItem(st.itemID, st.text.String(), []any{}),
			}); err != nil {
				return err
			}
		}
	case EventMessageDelta:
		e.stopReason = ev.StopReason
		if ev.Usage != nil {
			e.usage.Add(*ev.Usage)
		}
	case EventUsage:
		if ev.Usage != nil {
			e.usage.Add(*ev.Usage)
		}
	case EventDone:
		if err := e.emit("response.completed", map[string]any{
			"type": "response.completed", "response": e.baseResponse("completed", e.completedOutput()),
		}); err != nil {
			return err
		}
	case EventError:
		em := ev.Error
		return e.emit("response.failed", map[string]any{
			"type": "response.failed",
			"response": map[string]any{
				"id": e.id, "object": "response", "created_at": e.createdAt,
				"status": "failed", "error": map[string]any{
					"code": em.Code, "message": em.Message, "param": em.Param, "type": em.Type,
				},
			},
		})
	}
	e.flush()
	return nil
}

// baseResponse 响应对象骨架。
func (e *ResponsesStreamEncoder) baseResponse(status string, output []map[string]any) map[string]any {
	if output == nil {
		output = []map[string]any{}
	}
	return map[string]any{
		"id": e.id, "object": "response", "created_at": e.createdAt,
		"status": status, "model": e.model, "output": output, "error": nil,
		"usage": canonicalToResponsesUsage(&e.usage),
	}
}

// completedOutput 汇总已完成 item。
func (e *ResponsesStreamEncoder) completedOutput() []map[string]any {
	out := make([]map[string]any, 0, len(e.items))
	for i := 0; i < e.outputCount; i++ {
		for _, st := range e.items {
			if st.outputIdx != i {
				continue
			}
			if st.isFunc {
				out = append(out, responsesFunctionCallItem(st.itemID, st.callID, st.name, orDefault(st.text.String(), "{}")))
			} else {
				out = append(out, responsesMessageItem(st.itemID, st.text.String(), []any{}))
			}
			if !st.closed {
				// 未收到 block_stop 的兜底
				st.closed = true
			}
		}
	}
	return out
}

func (e *ResponsesStreamEncoder) emit(event string, payload map[string]any) error {
	e.seq++
	payload["sequence_number"] = e.seq
	if err := sseWriteEvent(e.w, event, payload); err != nil {
		return err
	}
	e.flush()
	return nil
}

func (e *ResponsesStreamEncoder) flush() {
	if e.flusher != nil {
		e.flusher.Flush()
	}
}
