package usage

import (
	"strings"
	"testing"
	"time"

	"github.com/snownd/team-router/internal/protocol"
)

func TestEstimateTokens(t *testing.T) {
	if EstimateTokens("") != 0 {
		t.Error("空串应为 0")
	}
	if EstimateTokens("abcd") != 1 {
		t.Error("4 字符 = 1 token")
	}
	if EstimateTokens("abcde") != 2 {
		t.Error("5 字符 = 2 token（向上取整）")
	}
}

func TestEstimateRequestTokens(t *testing.T) {
	req := &protocol.Request{
		System:   "12345678",                                       // 2
		Messages: []protocol.Message{{Role: "user", Text: "abcd"}}, // 1
	}
	if got := EstimateRequestTokens(req); got != 3 {
		t.Errorf("got %d, want 3", got)
	}
}

func TestMeterTTFTAndUsage(t *testing.T) {
	past := time.Now().Add(-100 * time.Millisecond)
	m := NewMeterStartedAt(past)
	u := &protocol.Usage{InputTokens: 10}
	m.Observe(&protocol.Event{Type: protocol.EventUsage, Usage: u})
	m.Observe(&protocol.Event{Type: protocol.EventBlockStart, Index: 0, BlockType: protocol.BlockTypeText})
	// TTFT 在首个 block_delta 记录
	if m.TTFTMs() != 0 {
		t.Error("无 delta 时 TTFT 应为 0")
	}
	m.Observe(&protocol.Event{Type: protocol.EventBlockDelta, DeltaType: protocol.DeltaText, Text: "x"})
	if m.TTFTMs() < 90 {
		t.Errorf("TTFT 应约等于 100ms: %d", m.TTFTMs())
	}
	m.Observe(&protocol.Event{Type: protocol.EventUsage, Usage: &protocol.Usage{OutputTokens: 5}})
	m.Observe(&protocol.Event{Type: protocol.EventMessageDelta, StopReason: protocol.StopStop, Usage: &protocol.Usage{OutputTokens: 3}})
	got := m.Usage()
	if got.InputTokens != 10 || got.OutputTokens != 8 {
		t.Errorf("usage 累计错误: %+v", got)
	}
	if m.StopReason() != protocol.StopStop {
		t.Errorf("stop 错误")
	}
	if m.TotalMs() < 100 {
		t.Errorf("总耗时错误")
	}
}

func TestEstimateRequestTokensImage(t *testing.T) {
	// base64 图片：400K 字符 ≈ 300K 原始字节 / 150B per token = 2000
	b64 := strings.Repeat("QUJD", 100_000) // 400,000 字符
	req := &protocol.Request{
		Messages: []protocol.Message{{
			Role: "user",
			Blocks: []protocol.Block{
				{Type: protocol.BlockTypeImage, MimeType: "image/png", Data: b64},
				{Type: protocol.BlockTypeImage, MimeType: "image_url", Data: "https://example.com/a.png"}, // URL 固定估值
				{Type: protocol.BlockTypeText, Text: "abcd"},                                              // 文本 1
			},
		}},
	}
	want := (400_000*3/4)/imageBytesPerToken + urlImageTokens + 1
	if got := EstimateRequestTokens(req); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	// 小图不应低于下限
	tiny := &protocol.Request{Messages: []protocol.Message{{
		Role:   "user",
		Blocks: []protocol.Block{{Type: protocol.BlockTypeImage, MimeType: "image/png", Data: "QUJD"}},
	}}}
	if got := EstimateRequestTokens(tiny); got != imageTokenFloor {
		t.Errorf("小图应按下限 %d 估算, got %d", imageTokenFloor, got)
	}
}
