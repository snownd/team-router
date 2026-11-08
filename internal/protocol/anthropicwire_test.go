package protocol

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestParseAnthropicWire(t *testing.T) {
	body := `{
		"model": "claude-sonnet-4-5",
		"system": [{"type": "text", "text": "be nice", "cache_control": {"type": "ephemeral"}}],
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "weather?"},
				{"type": "tool_result", "tool_use_id": "t1", "content": "sunny"}
			]},
			{"role": "assistant", "content": [
				{"type": "tool_use", "id": "t1", "name": "w", "input": {"city": "BJ"}}
			]}
		],
		"max_tokens": 500,
		"tools": [{"name": "w", "description": "d", "input_schema": {"type": "object"}}],
		"tool_choice": {"type": "any"},
		"stream": true
	}`
	req, rerr := ParseAnthropicWire([]byte(body))
	if rerr != nil {
		t.Fatalf("parse: %v", rerr)
	}
	if req.System != "be nice" || req.SystemCacheControl == nil || *req.SystemCacheControl != "ephemeral" {
		t.Errorf("system 解析错误: %q %+v", req.System, req.SystemCacheControl)
	}
	// tool_result 应拆出独立 tool 消息，顺序在原 user 消息之前；assistant tool_use 保留
	if len(req.Messages) != 3 {
		t.Fatalf("messages 数量错误: %d", len(req.Messages))
	}
	if req.Messages[0].Role != "tool" || req.Messages[0].ToolCallID != "t1" || req.Messages[0].Text != "sunny" {
		t.Errorf("tool_result 拆分错误: %+v", req.Messages[0])
	}
	if req.Messages[1].Role != "user" || req.Messages[1].Text != "weather?" {
		t.Errorf("user 消息错误: %+v", req.Messages[1])
	}
	// assistant tool_use
	if len(req.Messages[2].ToolCalls) != 1 || req.Messages[2].ToolCalls[0].ID != "t1" {
		t.Errorf("assistant tool_use 解析错误: %+v", req.Messages[2])
	}
	if req.Tools[0].Name != "w" {
		t.Errorf("tools 错误")
	}
	if tc, _ := req.ToolChoice.(string); tc != "required" {
		t.Errorf("tool_choice any -> required 错误: %v", req.ToolChoice)
	}
	if req.MaxTokens != 500 || !req.Stream {
		t.Errorf("参数错误")
	}
}

func TestCanonicalToAnthropicWireMergeToolMessages(t *testing.T) {
	req := &Request{
		Model:  "claude",
		System: "sys",
		Messages: []Message{
			{Role: "user", Text: "hi"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "t1", Name: "f", Arguments: `{"a":1}`}}},
			{Role: "tool", ToolCallID: "t1", Text: "result"},
			{Role: "user", Text: "thanks"},
		},
	}
	b, err := CanonicalToAnthropicWire(req, "claude-up", false, 1024)
	if err != nil {
		t.Fatal(err)
	}
	var w anthropicRequestWire
	_ = json.Unmarshal(b, &w)
	if len(w.Messages) != 4 {
		t.Fatalf("消息数错误: %d", len(w.Messages))
	}
	// tool 消息应合并回 user tool_result
	if w.Messages[2].Role != "user" {
		t.Errorf("tool_result 应为 user 消息: %+v", w.Messages[2])
	}
	var blocks []anthropicContentWire
	_ = json.Unmarshal(w.Messages[2].Content, &blocks)
	if len(blocks) != 1 || blocks[0].Type != "tool_result" || blocks[0].ToolUseID != "t1" {
		t.Errorf("tool_result 块错误: %+v", blocks)
	}
	// assistant tool_use
	_ = json.Unmarshal(w.Messages[1].Content, &blocks)
	if blocks[0].Type != "tool_use" || blocks[0].ID != "t1" {
		t.Errorf("tool_use 块错误: %+v", blocks)
	}
	if w.MaxTokens != 1024 {
		t.Errorf("max_tokens 默认值错误: %d", w.MaxTokens)
	}
	var sys string
	_ = json.Unmarshal(w.System, &sys)
	if sys != "sys" {
		t.Errorf("system 错误")
	}
}

func TestAnthropicStreamDecoder(t *testing.T) {
	sse := strings.Join([]string{
		`event: message_start` + "\n" + `data: {"type":"message_start","message":{"id":"msg_1","model":"claude","usage":{"input_tokens":50,"cache_read_input_tokens":20,"cache_creation_input_tokens":10}}}`,
		`event: content_block_start` + "\n" + `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`,
		`event: content_block_stop` + "\n" + `data: {"type":"content_block_stop","index":0}`,
		`event: message_delta` + "\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":7}}`,
		`event: message_stop` + "\n" + `data: {"type":"message_stop"}`,
		``,
	}, "\n\n")
	d := NewAnthropicStreamDecoder(strings.NewReader(sse), io.NopCloser(nil))

	var kinds []string
	var usage Usage
	for {
		ev, err := d.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		kinds = append(kinds, ev.Type)
		if ev.Usage != nil {
			usage.Add(*ev.Usage)
		}
		if ev.Type == EventMessageStart && ev.ID != "msg_1" {
			t.Errorf("message id 错误")
		}
	}
	expect := []string{EventMessageStart, EventBlockStart, EventBlockDelta, EventBlockStop, EventMessageDelta, EventDone}
	if strings.Join(kinds, ",") != strings.Join(expect, ",") {
		t.Fatalf("事件序列错误: %v", kinds)
	}
	if usage.InputTokens != 50 || usage.CacheReadTokens != 20 || usage.CacheWriteTokens != 10 || usage.OutputTokens != 7 {
		t.Errorf("usage 累计错误: %+v", usage)
	}
}

func TestAnthropicEncodeResponse(t *testing.T) {
	resp := &Response{
		Model: "claude", StopReason: StopLength,
		Blocks: []ContentBlock{
			{Type: BlockTypeText, Text: "answer"},
			{Type: BlockTypeToolUse, ToolCall: ToolCall{ID: "t", Name: "f", Arguments: `{"x":1}`}},
		},
		Usage: Usage{InputTokens: 10, CacheReadTokens: 5, OutputTokens: 3},
	}
	out := AnthropicEncodeResponse(resp)
	if out["stop_reason"] != "max_tokens" {
		t.Errorf("stop_reason 映射错误: %v", out["stop_reason"])
	}
	content := out["content"].([]map[string]any)
	if content[0]["type"] != "text" || content[1]["type"] != "tool_use" {
		t.Errorf("content 错误: %v", content)
	}
	usage := out["usage"].(*anthropicUsageWire)
	if usage.CacheReadInputTokens != 5 {
		t.Errorf("usage 错误: %+v", usage)
	}
}
