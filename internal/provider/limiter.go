package provider

import (
	"context"
	"sync"
	"time"

	"github.com/snownd/team-router/internal/protocol"
)

var _ = context.Background
var _ = sync.Mutex{}
var _ = time.Now
var _ = protocol.ErrChannelBusy

type channelSlot struct {
	sem chan struct{}
	cap int
}

type Limiter struct {
	mu    sync.Mutex
	slots map[int64]*channelSlot
}

func NewLimiter() *Limiter { return &Limiter{slots: map[int64]*channelSlot{}} }

// slotFor 取/建渠道信号量；容量与当前配置不一致时重建。
func (l *Limiter) slotFor(ch *Channel) *channelSlot {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.slots[ch.ID]
	if !ok || s.cap != ch.MaxConcurrency {
		s = &channelSlot{sem: make(chan struct{}, ch.MaxConcurrency), cap: ch.MaxConcurrency}
		l.slots[ch.ID] = s
	}
	return s
}

func (l *Limiter) Acquire(ctx context.Context, ch *Channel, wait time.Duration) (func(), *protocol.RelayError) {
	if ch.MaxConcurrency <= 0 {
		return func() {}, nil
	}
	slot := l.slotFor(ch)
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case slot.sem <- struct{}{}:
		return func() { <-slot.sem }, nil
	case <-ctx.Done():
		return nil, protocol.NewRelayError(499, "api_error", "client_closed", "客户端已取消请求")
	case <-timer.C:
		return nil, protocol.ErrChannelBusy("渠道 " + ch.Name + " 并发已满，排队超时")
	}
}
