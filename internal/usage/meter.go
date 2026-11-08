// Package usage 用量计量、token 估算与异步请求日志写入。
package usage

import (
	"time"

	"github.com/snownd/team-router/internal/protocol"
)

// Meter 流式请求计量器：跟踪 TTFT、总耗时、累计用量、停止原因。
type Meter struct {
	start       time.Time
	firstToken  time.Time
	usage       protocol.Usage
	stopReason  string
	blockStarts int
	firstByte   time.Time
}

// NewMeter 构造。
func NewMeter() *Meter { return &Meter{start: time.Now()} }

// NewMeterStartedAt 以指定起始时间构造（测试用）。
func NewMeterStartedAt(start time.Time) *Meter { return &Meter{start: start} }

// Observe 观察一个 canonical 事件。
func (m *Meter) Observe(ev *protocol.Event) {
	switch ev.Type {
	case protocol.EventBlockDelta:
		if m.firstToken.IsZero() {
			m.firstToken = time.Now()
		}
	case protocol.EventBlockStart:
		m.blockStarts++
		if m.firstByte.IsZero() {
			m.firstByte = time.Now()
		}
	case protocol.EventUsage:
		if ev.Usage != nil {
			m.usage.Add(*ev.Usage)
		}
	case protocol.EventMessageDelta:
		if ev.StopReason != "" {
			m.stopReason = ev.StopReason
		}
		if ev.Usage != nil {
			m.usage.Add(*ev.Usage)
		}
	}
}

// ObserveUsage 直接注入用量（非流式路径）。
func (m *Meter) ObserveUsage(u protocol.Usage) { m.usage.Add(u) }

// SetFirstToken 外部强制设置首 token 时间。
func (m *Meter) SetFirstToken(t time.Time) { m.firstToken = t }

// TTFTMs 首 token 延迟（毫秒）。未产生内容时为 0。
func (m *Meter) TTFTMs() int {
	if m.firstToken.IsZero() {
		return 0
	}
	return int(m.firstToken.Sub(m.start).Milliseconds())
}

// TotalMs 总耗时（毫秒）。
func (m *Meter) TotalMs() int { return int(time.Since(m.start).Milliseconds()) }

// Usage 累计用量。
func (m *Meter) Usage() protocol.Usage { return m.usage }

// StopReason 停止原因。
func (m *Meter) StopReason() string { return m.stopReason }
