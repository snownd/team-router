package protocol

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestParseChatWireBasic(t *testing.T) {
	body := `{
		"model": "gpt-4o",
		"messages": [
			{"role": "system", "content": "you are helpful"},
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": "hi", "tool_calls": [
				{"id": "call_1", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"BJ\"}"}}
			]},
			{"role": "tool", "tool_call_id": "call_1", "content": "sunny"}
		],
		"tools": [{"type": "function", "function": {"name": "get_weather", "parameters": {"type": "object"}}}],
		"tool_choice": {"type": "function", "function": {"name": "get_weather"}},
		"temperature": 0.7,
		"max_tokens": 100,
		"stream": true
	}`
	req, rerr := ParseChatWire([]byte(body))
	if rerr != nil {
		t.Fatalf("parse: %v", rerr)
	}
	if req.Model != "gpt-4o" || req.System != "you are helpful" {
		t.Errorf("model/system 错误: %s / %s", req.Model, req.System)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("messages 数量错误: %d", len(req.Messages))
	}
	if req.Messages[1].Role != "assistant" || len(req.Messages[1].ToolCalls) != 1 {
		t.Errorf("assistant tool_calls 解析错误")
	}
	tc := req.Messages[1].ToolCalls[0]
	if tc.ID != "call_1" || tc.Name != "get_weather" || tc.Arguments != `{"city":"BJ"}` {
		t.Errorf("tool call 内容错误: %+v", tc)
	}
	if req.Messages[2].Role != "tool" || req.Messages[2].ToolCallID != "call_1" {
		t.Errorf("tool 消息解析错误")
	}
	if req.MaxTokens != 100 || !req.Stream || req.Temperature == nil || *req.Temperature != 0.7 {
		t.Errorf("参数解析错误")
	}
	if tc2, _ := req.ToolChoice.(string); tc2 != "get_weather" {
		t.Errorf("tool_choice 归一化错误: %v", req.ToolChoice)
	}
	if len(req.Tools) != 1 || req.Tools[0].Name != "get_weather" {
		t.Errorf("tools 解析错误")
	}
}

func TestParseChatWireImageParts(t *testing.T) {
	body := `{"model": "m", "messages": [{"role": "user", "content": [
		{"type": "text", "text": "look"},
		{"type": "image_url", "image_url": {"url": "https://x/y.png"}}
	]}]}`
	req, rerr := ParseChatWire([]byte(body))
	if rerr != nil {
		t.Fatalf("parse: %v", rerr)
	}
	m := req.Messages[0]
	if len(m.Blocks) != 2 || m.Blocks[1].Type != "image" {
		t.Fatalf("多模态块解析错误: %+v", m.Blocks)
	}
}

func TestChatWireToCanonicalRoundTrip(t *testing.T) {
	// 客户端 chat 请求 -> canonical -> 上游 chat 请求
	in := `{"model": "gpt-4o", "messages": [
		{"role": "system", "content": "sys"},
		{"role": "user", "content": "hi"}], "stream": false}`
	req, _ := ParseChatWire([]byte(in))
	up, err := CanonicalToChatWire(req, "gpt-4o-upstream", false)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(up, &m)
	if m["model"] != "gpt-4o-upstream" {
		t.Errorf("上游模型名错误: %v", m["model"])
	}
	msgs := m["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" {
		t.Errorf("上游消息错误: %v", msgs)
	}
}

func TestChatStreamDecoderTextAndUsage(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"role":"assistant","content":"He"}}]}`,
		`data: {"choices":[{"index":0,"delta":{"content":"llo"}}]}`,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: {"usage":{"prompt_tokens":100,"prompt_tokens_details":{"cached_tokens":40},"completion_tokens":10,"total_tokens":110}}`,
		`data: [DONE]`,
		``,
	}, "\n\n")
	d := NewChatStreamDecoder(strings.NewReader(sse), io.NopCloser(nil))

	var evts []*Event
	for {
		ev, err := d.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if ev.Type == EventDone && len(evts) > 0 && evts[len(evts)-1].Type == EventDone {
			break
		}
		evts = append(evts, ev)
	}

	// 期望序列: block_start(text), delta(He), delta(llo), block_stop, message_delta(stop), usage, done
	var kinds []string
	for _, e := range evts {
		kinds = append(kinds, e.Type)
	}
	// usage 位于 message_delta 之前，保证编码器渲染停止原因时可见完整用量
	expect := []string{EventMessageStart, EventBlockStart, EventBlockDelta, EventBlockDelta, EventBlockStop, EventUsage, EventMessageDelta, EventDone}
	if strings.Join(kinds, ",") != strings.Join(expect, ",") {
		t.Fatalf("事件序列错误:\n got %v\nwant %v", kinds, expect)
	}
	var lastUsage *Usage
	var stopEvent *Event
	for _, e := range evts {
		if e.Type == EventUsage {
			lastUsage = e.Usage
		}
		if e.Type == EventMessageDelta {
			stopEvent = e
		}
	}
	if lastUsage == nil || lastUsage.InputTokens != 60 || lastUsage.CacheReadTokens != 40 || lastUsage.OutputTokens != 10 {
		t.Errorf("usage 换算错误（openai prompt 含 cached 需扣除）: %+v", lastUsage)
	}
	if stopEvent == nil || stopEvent.StopReason != StopStop {
		t.Errorf("stop 错误: %+v", stopEvent)
	}
}

func TestChatStreamDecoderToolCalls(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"c\":"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"BJ\"}"}}]}}]}`,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
		``,
	}, "\n\n")
	d := NewChatStreamDecoder(strings.NewReader(sse), io.NopCloser(nil))
	var args strings.Builder
	var sawStart bool
	var stop string
	for {
		ev, err := d.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch ev.Type {
		case EventBlockStart:
			if ev.BlockType == BlockTypeToolUse {
				sawStart = true
				if ev.ToolCall.ID != "call_1" || ev.ToolCall.Name != "get_weather" {
					t.Errorf("tool_use start 错误: %+v", ev.ToolCall)
				}
			}
		case EventBlockDelta:
			args.WriteString(ev.Text)
		case EventMessageDelta:
			stop = ev.StopReason
		}
	}
	if !sawStart {
		t.Fatal("未收到 tool_use block_start")
	}
	if args.String() != `{"c":"BJ"}` {
		t.Errorf("参数拼接错误: %s", args.String())
	}
	if stop != StopToolCalls {
		t.Errorf("stop 错误: %v", stop)
	}
}

func TestChatEncodeResponse(t *testing.T) {
	resp := &Response{
		Model: "gpt-4o", StopReason: StopToolCalls,
		Blocks: []ContentBlock{
			{Type: BlockTypeText, Text: "let me check"},
			{Type: BlockTypeToolUse, ToolCall: ToolCall{ID: "c1", Name: "f", Arguments: `{"a":1}`}},
		},
		Usage: Usage{InputTokens: 60, CacheReadTokens: 40, OutputTokens: 5},
	}
	out := ChatEncodeResponse(resp)
	b, _ := json.Marshal(out)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	choices := m["choices"].([]any)
	c := choices[0].(map[string]any)
	msg := c["message"].(map[string]any)
	if msg["content"] != "let me check" {
		t.Errorf("content 错误")
	}
	if c["finish_reason"] != "tool_calls" {
		t.Errorf("finish_reason 错误: %v", c["finish_reason"])
	}
	usage := m["usage"].(map[string]any)
	if usage["prompt_tokens"].(float64) != 100 {
		t.Errorf("usage 错误: %v", usage)
	}
}
