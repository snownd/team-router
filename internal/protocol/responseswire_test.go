package protocol

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseResponsesWire(t *testing.T) {
	body := `{
		"model": "gpt-4o",
		"instructions": "be nice",
		"input": [
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "hi"}]},
			{"type": "function_call", "call_id": "c1", "name": "f", "arguments": "{\"a\":1}"},
			{"type": "function_call_output", "call_id": "c1", "output": "42"}
		],
		"tools": [{"type": "function", "name": "f", "parameters": {"type": "object"}}],
		"max_output_tokens": 300,
		"stream": true
	}`
	req, rerr := ParseResponsesWire([]byte(body))
	if rerr != nil {
		t.Fatalf("parse: %v", rerr)
	}
	if req.System != "be nice" || req.Model != "gpt-4o" || req.MaxTokens != 300 || !req.Stream {
		t.Errorf("基础字段错误: %+v", req)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("messages 数量错误: %d", len(req.Messages))
	}
	if req.Messages[0].Role != "user" || req.Messages[0].Text != "hi" {
		t.Errorf("user 消息错误")
	}
	if req.Messages[1].Role != "assistant" || req.Messages[1].ToolCalls[0].ID != "c1" {
		t.Errorf("function_call 转换错误: %+v", req.Messages[1])
	}
	if req.Messages[2].Role != "tool" || req.Messages[2].ToolCallID != "c1" || req.Messages[2].Text != "42" {
		t.Errorf("function_call_output 转换错误: %+v", req.Messages[2])
	}
}

func TestParseResponsesWireRejectsStateful(t *testing.T) {
	body := `{"model": "gpt-4o", "input": "hi", "previous_response_id": "resp_123"}`
	_, rerr := ParseResponsesWire([]byte(body))
	if rerr == nil {
		t.Fatal("应拒绝 previous_response_id")
	}
}

func TestParseResponsesWireEasyInput(t *testing.T) {
	// EasyInputMessage：省略 type（默认 message），content 可为 parts 数组或 string
	body := `{
		"model": "gpt-4o",
		"instructions": "be nice",
		"input": [
			{"role": "developer", "content": "dev prefix"},
			{"role": "user", "content": [{"type": "input_text", "text": "hi"}, {"type": "input_text", "text": " there"}]},
			{"role": "assistant", "content": "hello"},
			{"type": "reasoning", "summary": []}
		]
	}`
	req, rerr := ParseResponsesWire([]byte(body))
	if rerr != nil {
		t.Fatalf("parse: %v", rerr)
	}
	if req.System != "be nice\n\ndev prefix" {
		t.Errorf("system 合并错误: %q", req.System)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("messages 数量错误: %d", len(req.Messages))
	}
	if req.Messages[0].Role != "user" || req.Messages[0].Text != "hi there" {
		t.Errorf("user 消息错误: %+v", req.Messages[0])
	}
	if req.Messages[1].Role != "assistant" || req.Messages[1].Text != "hello" {
		t.Errorf("assistant 消息错误: %+v", req.Messages[1])
	}
}

func TestParseResponsesWireRejectsEmptyInput(t *testing.T) {
	for _, body := range []string{
		`{"model": "gpt-4o", "input": []}`,
		`{"model": "gpt-4o", "input": [{"type": "reasoning", "summary": []}]}`,
	} {
		if _, rerr := ParseResponsesWire([]byte(body)); rerr == nil {
			t.Errorf("应拒绝空 input: %s", body)
		}
	}
}

func TestResponsesStreamEncoder(t *testing.T) {
	w := httptest.NewRecorder()
	enc := NewResponsesStreamEncoder(w)

	events := []*Event{
		{Type: EventMessageStart, Model: "gpt-4o"},
		{Type: EventBlockStart, Index: 0, BlockType: BlockTypeText},
		{Type: EventBlockDelta, Index: 0, DeltaType: DeltaText, Text: "Hel"},
		{Type: EventBlockDelta, Index: 0, DeltaType: DeltaText, Text: "lo"},
		{Type: EventBlockStop, Index: 0},
		{Type: EventBlockStart, Index: 1, BlockType: BlockTypeToolUse, ToolCall: ToolCall{ID: "c1", Name: "f"}},
		{Type: EventBlockDelta, Index: 1, DeltaType: DeltaInputJSON, Text: `{"a":1}`},
		{Type: EventBlockStop, Index: 1},
		{Type: EventUsage, Usage: &Usage{InputTokens: 10, CacheReadTokens: 5, OutputTokens: 8}},
		{Type: EventDone},
	}
	for _, ev := range events {
		if err := enc.WriteEvent(ev); err != nil {
			t.Fatal(err)
		}
	}

	out := w.Body.String()
	for _, want := range []string{
		"event: response.created",
		"event: response.output_item.added",
		"event: response.output_text.delta",
		"event: response.output_text.done",
		"event: response.function_call_arguments.delta",
		"event: response.function_call_arguments.done",
		"event: response.output_item.done",
		"event: response.completed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("缺少事件 %s", want)
		}
	}
	if !strings.Contains(out, `"delta":"Hel"`) || !strings.Contains(out, `"delta":"lo"`) {
		t.Errorf("文本增量错误")
	}
	if !strings.Contains(out, `"arguments":"{\"a\":1}"`) {
		t.Errorf("工具参数错误")
	}
	if !strings.Contains(out, `"cached_tokens":5`) || !strings.Contains(out, `"total_tokens":23`) {
		t.Errorf("usage 错误:\n%s", out)
	}
}

// TestAnyToAnyPipeline 客户端用 Chat 协议，上游为 Anthropic SSE，验证 canonical 全链路。
func TestAnyToAnyPipeline(t *testing.T) {
	// 1) 客户端 chat 请求（含 system + tool 定义）
	chatReq := `{"model": "claude-sonnet-4-5", "messages": [
		{"role": "system", "content": "sys"},
		{"role": "user", "content": "hi"}], "stream": true}`
	req, rerr := ParseChatWire([]byte(chatReq))
	if rerr != nil {
		t.Fatal(rerr)
	}

	// 2) canonical -> anthropic 上游请求
	up, err := CanonicalToAnthropicWire(req, "claude-upstream", true, 1024)
	if err != nil {
		t.Fatal(err)
	}
	var w anthropicRequestWire
	_ = json.Unmarshal(up, &w)
	if w.Model != "claude-upstream" || !w.Stream || w.MaxTokens != 1024 {
		t.Errorf("上游请求错误: %s", up)
	}
	var sys string
	_ = json.Unmarshal(w.System, &sys)
	if sys != "sys" {
		t.Errorf("system 传递错误")
	}

	// 3) 上游 anthropic SSE -> canonical
	upstreamSSE := strings.Join([]string{
		`event: message_start` + "\n" + `data: {"type":"message_start","message":{"id":"msg_9","model":"claude","usage":{"input_tokens":42,"cache_read_input_tokens":12,"cache_creation_input_tokens":3}}}`,
		`event: content_block_start` + "\n" + `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"你好"}}`,
		`event: content_block_stop` + "\n" + `data: {"type":"content_block_stop","index":0}`,
		`event: message_delta` + "\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":9}}`,
		`event: message_stop` + "\n" + `data: {"type":"message_stop"}`,
		``,
	}, "\n\n")
	dec := NewAnthropicStreamDecoder(strings.NewReader(upstreamSSE), io.NopCloser(nil))

	// 4) canonical -> 客户端 chat SSE
	w2 := httptest.NewRecorder()
	enc := NewChatStreamEncoder(w2)
	var usage Usage
	for {
		ev, err := dec.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if ev.Usage != nil {
			usage.Add(*ev.Usage)
		}
		if err := enc.WriteEvent(ev); err != nil {
			t.Fatal(err)
		}
	}

	out := w2.Body.String()
	// role chunk + content + finish + usage + [DONE]
	if !strings.Contains(out, `"role":"assistant"`) {
		t.Errorf("缺少 role chunk:\n%s", out)
	}
	if !strings.Contains(out, `"content":"你好"`) {
		t.Errorf("缺少内容增量:\n%s", out)
	}
	if !strings.Contains(out, `"finish_reason":"stop"`) {
		t.Errorf("缺少 finish_reason:\n%s", out)
	}
	if !strings.Contains(out, `"prompt_tokens":57`) { // 42+12+3
		t.Errorf("usage prompt_tokens 错误:\n%s", out)
	}
	if !strings.Contains(out, `"cached_tokens":12`) {
		t.Errorf("usage cached_tokens 错误:\n%s", out)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "data: [DONE]") {
		t.Errorf("缺少 [DONE]:\n%s", out)
	}
	if usage.InputTokens != 42 || usage.CacheReadTokens != 12 || usage.CacheWriteTokens != 3 || usage.OutputTokens != 9 {
		t.Errorf("usage 累计错误: %+v", usage)
	}
}
