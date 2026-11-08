package provider

import (
	"sync"
	"time"
)

// stickyEntry 粘性会话绑定项。
type stickyEntry struct {
	channelID int64
	expireAt  time.Time
}

// StickyTable 粘性路由表（进程内，单节点部署前提）：
// 会话键 -> 渠道 ID 绑定，多轮会话命中相同上游账户。
// 命中时滑动续期；惰性清理过期项（距上次清理超过 TTL/2 时顺带扫描，无后台 goroutine）。
// ttl<=0 表示关闭粘性路由（Get 永不命中）。
type StickyTable struct {
	mu        sync.Mutex
	entries   map[string]stickyEntry
	ttl       time.Duration
	lastSweep time.Time
	now       func() time.Time // 时钟注入（测试用）
}

// NewStickyTable 构造；ttl<=0 关闭粘性路由。
func NewStickyTable(ttl time.Duration) *StickyTable {
	return &StickyTable{
		entries:   map[string]stickyEntry{},
		ttl:       ttl,
		lastSweep: time.Now(),
		now:       time.Now,
	}
}

// Get 查询会话绑定的渠道；命中时滑动续期，过期项即时移除。
func (s *StickyTable) Get(sessionKey string) (int64, bool) {
	if s.ttl <= 0 {
		return 0, false // 粘性路由已关闭
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[sessionKey]
	if !ok {
		return 0, false
	}
	now := s.now()
	if !now.Before(e.expireAt) {
		delete(s.entries, sessionKey)
		return 0, false
	}
	e.expireAt = now.Add(s.ttl) // 滑动续期
	s.entries[sessionKey] = e
	return e.channelID, true
}

// Set 绑定/更新会话 -> 渠道；顺带执行惰性清理。
func (s *StickyTable) Set(sessionKey string, channelID int64) {
	if s.ttl <= 0 {
		return // 粘性路由已关闭
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if now.Sub(s.lastSweep) > s.ttl/2 {
		s.sweepLocked(now)
	}
	s.entries[sessionKey] = stickyEntry{channelID: channelID, expireAt: now.Add(s.ttl)}
}

// sweepLocked 清理全部过期项（调用方持锁）。
func (s *StickyTable) sweepLocked(now time.Time) {
	s.lastSweep = now
	for k, e := range s.entries {
		if !now.Before(e.expireAt) {
			delete(s.entries, k)
		}
	}
}
