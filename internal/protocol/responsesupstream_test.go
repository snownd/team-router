package protocol

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestCanonicalToResponsesWireRoundTrip：canonical -> Responses wire，
// 产物可被 ParseResponsesWire 解析回等价 canonical（无状态子集）。
func TestCanonicalToResponsesWireRoundTrip(t *testing.T) {
	req := &Request{
		Model:  "gpt-test",
		System: "你是助手",
		Messages: []Message{
			{Role: "user", Text: "今天天气如何"},
			{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "call_1", Name: "get_weather", Arguments: `{"city":"北京"}`},
			}},
			{Role: "tool", ToolCallID: "call_1", Text: `{"temp":25}`},
		},
		Tools: []Tool{
			{Name: "get_weather", Description: "查询天气", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
		ToolChoice:    "auto",
		Temperature:   ptrFloat(0.5),
		MaxTokens:     1024,
		StopSequences: []string{"END"}, // Responses API 不支持，应被丢弃
	}

	body, err := CanonicalToResponsesWire(req, "gpt-5-upstream", false)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["model"] != "gpt-5-upstream" {
		t.Errorf("model = %v, want gpt-5-upstream", raw["model"])
	}
	if raw["instructions"] != "你是助手" {
		t.Errorf("instructions = %v", raw["instructions"])
	}
	if s, _ := raw["store"].(bool); s {
		t.Error("store 必须为 false（无状态）")
	}
	if _, ok := raw["stop"]; ok {
		t.Error("Responses wire 不应包含 stop 字段")
	}
	if f, _ := raw["max_output_tokens"].(float64); f != 1024 {
		t.Errorf("max_output_tokens = %v, want 1024", raw["max_output_tokens"])
	}
	if raw["tool_choice"] != "auto" {
		t.Errorf("tool_choice = %v, want auto", raw["tool_choice"])
	}

	input, _ := raw["input"].([]any)
	if len(input) != 3 {
		t.Fatalf("input items = %d, want 3: %s", len(input), body)
	}
	msg, _ := input[0].(map[string]any)
	if msg["type"] != "message" || msg["role"] != "user" {
		t.Errorf("input[0] 应为 user message: %v", msg)
	}
	fc, _ := input[1].(map[string]any)
	if fc["type"] != "function_call" || fc["call_id"] != "call_1" || fc["name"] != "get_weather" {
		t.Errorf("input[1] 应为 function_call: %v", fc)
	}
	fo, _ := input[2].(map[string]any)
	if fo["type"] != "function_call_output" || fo["call_id"] != "call_1" {
		t.Errorf("input[2] 应为 function_call_output: %v", fo)
	}

	// 往返：上游 wire 可被网关 Responses 解析器重新读入（语义等价）
	back, rerr := ParseResponsesWire(body)
	if rerr != nil {
		t.Fatalf("round trip parse failed: %v", rerr)
	}
	if back.Model != "gpt-5-upstream" || back.System != "你是助手" {
		t.Errorf("round trip 语义漂移: %+v", back)
	}
	if len(back.Messages) != 3 {
		t.Errorf("round trip 消息数 = %d, want 3", len(back.Messages))
	}
	if back.Messages[1].ToolCalls[0].Name != "get_weather" {
		t.Errorf("round trip 工具调用丢失: %+v", back.Messages[1])
	}
}

// TestCanonicalToResponsesWireImages：user 多模态消息映射为 input_image。
func TestCanonicalToResponsesWireImages(t *testing.T) {
	req := &Request{
		Model: "gpt-test",
		Messages: []Message{{
			Role: "user",
			Blocks: []Block{
				{Type: BlockTypeText, Text: "看图"},
				{Type: BlockTypeImage, MimeType: "image/png", Data: "aGk="},
			},
		}},
	}
	body, err := CanonicalToResponsesWire(req, "gpt-test", true)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Input []struct {
			Content []struct {
				Type     string `json:"type"`
				ImageURL string `json:"image_url"`
				Text     string `json:"text"`
			} `json:"content"`
		} `json:"input"`
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	if !raw.Stream {
		t.Error("stream 应透传")
	}
	if len(raw.Input) != 1 || len(raw.Input[0].Content) != 2 {
		t.Fatalf("content parts = %v", raw.Input)
	}
	img := raw.Input[0].Content[1]
	if img.Type != "input_image" || !strings.HasPrefix(img.ImageURL, "data:image/png;base64,") {
		t.Errorf("input_image 映射错误: %+v", img)
	}
}

// TestResponsesWireToResponse：上游非流式响应（message + reasoning + function_call）。
func TestResponsesWireToResponse(t *testing.T) {
	body := `{
		"id": "resp_1", "model": "gpt-5", "status": "completed",
		"output": [
			{"type": "reasoning", "summary": [{"type": "summary_text", "text": "思考中"}]},
			{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "你好"}]},
			{"type": "function_call", "call_id": "call_9", "name": "get_weather", "arguments": "{\"city\":\"BJ\"}"}
		],
		"usage": {"input_tokens": 100, "input_tokens_details": {"cached_tokens": 40}, "output_tokens": 20, "total_tokens": 120}
	}`
	resp, rerr := ResponsesWireToResponse([]byte(body), "gpt-alias")
	if rerr != nil {
		t.Fatal(rerr)
	}
	if len(resp.Blocks) != 3 {
		t.Fatalf("blocks = %d, want 3", len(resp.Blocks))
	}
	if resp.Blocks[0].Type != BlockTypeThinking || resp.Blocks[0].Text != "思考中" {
		t.Errorf("thinking block 错误: %+v", resp.Blocks[0])
	}
	if resp.Blocks[1].Type != BlockTypeText || resp.Blocks[1].Text != "你好" {
		t.Errorf("text block 错误: %+v", resp.Blocks[1])
	}
	if resp.Blocks[2].ToolCall.Name != "get_weather" || resp.Blocks[2].ToolCall.ID != "call_9" {
		t.Errorf("tool_use block 错误: %+v", resp.Blocks[2])
	}
	if resp.StopReason != StopToolCalls {
		t.Errorf("stop = %s, want tool_calls", resp.StopReason)
	}
	if resp.Usage.InputTokens != 60 || resp.Usage.CacheReadTokens != 40 || resp.Usage.OutputTokens != 20 {
		t.Errorf("usage 换算错误（input 应为新鲜输入 100-40）: %+v", resp.Usage)
	}
	if resp.Model != "gpt-alias" {
		t.Errorf("model 应还原为请求别名: %s", resp.Model)
	}
}

// TestResponsesWireToResponseIncompleteAndFailed：incomplete -> length、failed -> 错误。
func TestResponsesWireToResponseIncompleteAndFailed(t *testing.T) {
	inc := `{"id":"r","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","content":[{"type":"output_text","text":"截断"}]}]}`
	resp, rerr := ResponsesWireToResponse([]byte(inc), "m")
	if rerr != nil {
		t.Fatal(rerr)
	}
	if resp.StopReason != StopLength {
		t.Errorf("incomplete(max_output_tokens) 应映射 length, got %s", resp.StopReason)
	}

	fail := `{"id":"r","status":"failed","error":{"code":"server_error","message":"boom"}}`
	if _, rerr := ResponsesWireToResponse([]byte(fail), "m"); rerr == nil || !strings.Contains(rerr.Message, "boom") {
		t.Errorf("failed 应返回错误: %v", rerr)
	}
}

// TestResponsesStreamDecoder：真实风格 SSE -> canonical 事件序列。
func TestResponsesStreamDecoder(t *testing.T) {
	sse := strings.Join([]string{
		`event: response.created`,
		`data: {"response":{"id":"resp_1","model":"gpt-5"}}`,
		``,
		`event: response.in_progress`,
		`data: {"response":{"id":"resp_1"}}`,
		``,
		`event: response.output_item.added`,
		`data: {"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant"}}`,
		``,
		`event: response.output_text.delta`,
		`data: {"item_id":"msg_1","output_index":0,"delta":"你"}`,
		``,
		`event: response.output_text.delta`,
		`data: {"item_id":"msg_1","output_index":0,"delta":"好"}`,
		``,
		`event: response.output_item.done`,
		`data: {"output_index":0,"item":{"type":"message","id":"msg_1"}}`,
		``,
		`event: response.completed`,
		`data: {"response":{"id":"resp_1","usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":30},"output_tokens":2,"total_tokens":102}}}`,
		``,
	}, "\n")

	d := NewResponsesStreamDecoder(strings.NewReader(sse), io.NopCloser(nil))
	var events []*Event
	for {
		ev, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if ev != nil {
			events = append(events, ev)
		}
	}
	_ = d.Close()

	// 期望序列：message_start / block_start(text) / delta / delta / block_stop / usage / message_delta(stop) / done
	wantTypes := []string{
		EventMessageStart, EventBlockStart, EventBlockDelta, EventBlockDelta, EventBlockStop,
		EventUsage, EventMessageDelta, EventDone,
	}
	if len(events) != len(wantTypes) {
		t.Fatalf("事件数 = %d, want %d:\n%+v", len(events), len(wantTypes), events)
	}
	for i, want := range wantTypes {
		if events[i].Type != want {
			t.Errorf("events[%d].Type = %s, want %s", i, events[i].Type, want)
		}
	}
	if events[0].ID != "resp_1" || events[0].Model != "gpt-5" {
		t.Errorf("message_start 应携带上游 id/model: %+v", events[0])
	}
	if events[1].BlockType != BlockTypeText {
		t.Errorf("block_start 应为 text: %+v", events[1])
	}
	if events[2].Text != "你" || events[3].Text != "好" {
		t.Errorf("文本增量错误: %+v %+v", events[2], events[3])
	}
	if u := events[5].Usage; u.InputTokens != 70 || u.CacheReadTokens != 30 {
		t.Errorf("usage 换算错误: %+v", u)
	}
	if events[6].StopReason != StopStop {
		t.Errorf("stop = %s, want stop", events[6].StopReason)
	}
}

// TestResponsesStreamDecoderToolCallAndIncomplete：
// function_call 流 -> tool_use 块 + stop=tool_calls；incomplete(max_output_tokens) -> length。
func TestResponsesStreamDecoderToolCallAndIncomplete(t *testing.T) {
	sse := strings.Join([]string{
		`event: response.created`,
		`data: {"response":{"id":"resp_2","model":"gpt-5"}}`,
		``,
		`event: response.output_item.added`,
		`data: {"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"get_weather"}}`,
		``,
		`event: response.function_call_arguments.delta`,
		`data: {"item_id":"fc_1","delta":"{\"city\":"}`,
		``,
		`event: response.function_call_arguments.delta`,
		`data: {"item_id":"fc_1","delta":"\"BJ\"}"}`,
		``,
		`event: response.output_item.done`,
		`data: {"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1"}}`,
		``,
		`event: response.incomplete`,
		`data: {"response":{"id":"resp_2","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":10,"output_tokens":9}}}`,
		``,
	}, "\n")

	d := NewResponsesStreamDecoder(strings.NewReader(sse), nil)
	var events []*Event
	for {
		ev, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if ev != nil {
			events = append(events, ev)
		}
	}

	var args strings.Builder
	sawToolStart, sawToolStop := false, false
	for _, ev := range events {
		if ev.Type == EventBlockStart && ev.BlockType == BlockTypeToolUse {
			sawToolStart = true
			if ev.ToolCall.ID != "call_1" || ev.ToolCall.Name != "get_weather" {
				t.Errorf("tool_use block_start 错误: %+v", ev)
			}
		}
		if ev.Type == EventBlockDelta && ev.DeltaType == DeltaInputJSON {
			args.WriteString(ev.Text)
		}
		if ev.Type == EventBlockStop && ev.Index == 0 {
			sawToolStop = true
		}
	}
	if !sawToolStart || !sawToolStop {
		t.Fatalf("缺少 tool_use 块事件: %+v", events)
	}
	if args.String() != `{"city":"BJ"}` {
		t.Errorf("参数增量拼接错误: %s", args.String())
	}
	var lastDelta *Event
	for _, ev := range events {
		if ev.Type == EventMessageDelta {
			lastDelta = ev
		}
	}
	// incomplete + max_output_tokens：截断信号优先于 tool_calls
	// （客户端不应执行可能被截断的函数调用）
	if lastDelta == nil || lastDelta.StopReason != StopLength {
		t.Fatalf("incomplete(max_output_tokens) 应映射 length: %+v", lastDelta)
	}
	// completed 场景下含 function_call 才映射 tool_calls：补充验证
	sseDone := strings.Join([]string{
		`event: response.created`,
		`data: {"response":{"id":"r"}}`,
		``,
		`event: response.output_item.added`,
		`data: {"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"f"}}`,
		``,
		`event: response.output_item.done`,
		`data: {"output_index":0,"item":{"type":"function_call","id":"fc_1"}}`,
		``,
		`event: response.completed`,
		`data: {"response":{"id":"r","usage":{"input_tokens":5,"output_tokens":3}}}`,
		``,
	}, "\n")
	d3 := NewResponsesStreamDecoder(strings.NewReader(sseDone), nil)
	stopDone := ""
	for {
		ev, err := d3.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if ev != nil && ev.Type == EventMessageDelta {
			stopDone = ev.StopReason
		}
	}
	if stopDone != StopToolCalls {
		t.Errorf("completed 含 function_call 应映射 tool_calls, got %s", stopDone)
	}

	// incomplete + max_output_tokens（无 tool call）-> length
	sse2 := strings.Join([]string{
		`event: response.created`,
		`data: {"response":{"id":"r"}}`,
		``,
		`event: response.output_item.added`,
		`data: {"output_index":0,"item":{"type":"message","id":"m1"}}`,
		``,
		`event: response.output_text.delta`,
		`data: {"item_id":"m1","delta":"部分输出"}`,
		``,
		`event: response.output_item.done`,
		`data: {"output_index":0,"item":{"type":"message","id":"m1"}}`,
		``,
		`event: response.incomplete`,
		`data: {"response":{"id":"r","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":5,"output_tokens":10}}}`,
		``,
	}, "\n")
	d2 := NewResponsesStreamDecoder(strings.NewReader(sse2), nil)
	var stop string
	for {
		ev, err := d2.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if ev != nil && ev.Type == EventMessageDelta {
			stop = ev.StopReason
		}
	}
	if stop != StopLength {
		t.Errorf("incomplete(max_output_tokens) 应映射 length, got %s", stop)
	}
}

// TestResponsesStreamDecoderFailed：response.failed -> error 事件。
func TestResponsesStreamDecoderFailed(t *testing.T) {
	sse := strings.Join([]string{
		`event: response.created`,
		`data: {"response":{"id":"r"}}`,
		``,
		`event: response.failed`,
		`data: {"response":{"id":"r","error":{"code":"server_error","message":"boom"}}}`,
		``,
	}, "\n")
	d := NewResponsesStreamDecoder(strings.NewReader(sse), nil)
	var gotErr *RelayError
	for {
		ev, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if ev != nil && ev.Type == EventError {
			gotErr = ev.Error
		}
	}
	if gotErr == nil || !strings.Contains(gotErr.Message, "boom") {
		t.Fatalf("应产生 error 事件: %v", gotErr)
	}
}

// TestResponsesStreamDecoderAbruptEOF：上游流中断（无 completed）时兜底补 start/done。
func TestResponsesStreamDecoderAbruptEOF(t *testing.T) {
	d := NewResponsesStreamDecoder(strings.NewReader(""), nil)
	var types []string
	for {
		ev, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if ev != nil {
			types = append(types, ev.Type)
		}
	}
	if len(types) != 2 || types[0] != EventMessageStart || types[1] != EventDone {
		t.Errorf("空流兜底事件应为 [message_start done]: %v", types)
	}
}

func ptrFloat(f float64) *float64 { return &f }
