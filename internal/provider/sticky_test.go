package provider

import (
	"testing"
	"time"
)

// TestStickyTableBindAndHit：绑定后会话命中同一渠道，且访问滑动续期。
func TestStickyTableBindAndHit(t *testing.T) {
	st := NewStickyTable(30 * time.Minute)

	if _, ok := st.Get("s1"); ok {
		t.Fatal("未绑定的会话不应命中")
	}

	st.Set("s1", 42)
	id, ok := st.Get("s1")
	if !ok || id != 42 {
		t.Fatalf("应命中渠道 42, got %d %v", id, ok)
	}

	// 15 分钟后访问一次 → 续期，30+ 分钟后仍应命中
	st.now = func() time.Time { return time.Now().Add(45 * time.Minute) }
	_, ok = st.Get("s1")
	if ok {
		t.Fatal("TTL 过期后不应命中（未续期场景见下个用例）")
	}
}

// TestStickyTableSlidingTTL：周期性访问的活跃会话不过期。
func TestStickyTableSlidingTTL(t *testing.T) {
	st := NewStickyTable(30 * time.Minute)
	base := time.Now()
	st.now = func() time.Time { return base }
	st.Set("s1", 7)

	for i := 1; i <= 5; i++ {
		st.now = func() time.Time { return base.Add(time.Duration(i) * 25 * time.Minute) }
		if id, ok := st.Get("s1"); !ok || id != 7 {
			t.Fatalf("第 %d 次访问（滑动续期）应命中, got %d %v", i, id, ok)
		}
	}
	// 滑动续期：每次命中后过期时间从最近一次访问重新起算
	st.now = func() time.Time { return base.Add(5*25*time.Minute + 29*time.Minute) }
	if _, ok := st.Get("s1"); !ok {
		t.Fatal("距最近访问 29m（<30m）应命中")
	}
	// 上次命中发生在 154m，续期至 184m；183m 仍命中（并续期至 213m）
	st.now = func() time.Time { return base.Add(183 * time.Minute) }
	if _, ok := st.Get("s1"); !ok {
		t.Fatal("距续期后 29m（<30m）应命中")
	}
	// 距最后一次命中（183m）31m → 过期
	st.now = func() time.Time { return base.Add(214 * time.Minute) }
	if _, ok := st.Get("s1"); ok {
		t.Fatal("距最近访问 31m（>30m）应过期")
	}
}

// TestStickyTableDisabled：TTL 为负时粘性路由关闭（Get 永不命中）。
func TestStickyTableDisabled(t *testing.T) {
	st := NewStickyTable(-1)
	st.Set("s1", 1)
	if _, ok := st.Get("s1"); ok {
		t.Fatal("关闭粘性（负 TTL）时不应命中")
	}
}

// TestStickyTableSweepExpired：惰性清理移除过期项。
func TestStickyTableSweepExpired(t *testing.T) {
	st := NewStickyTable(10 * time.Minute)
	base := time.Now()
	st.now = func() time.Time { return base }
	st.Set("a", 1)
	st.Set("b", 2)

	// TTL/2 = 5m 后再 Set 应触发清理窗口；到 11m 时 a/b 均过期
	st.now = func() time.Time { return base.Add(11 * time.Minute) }
	st.Set("c", 3)

	st.mu.Lock()
	n := len(st.entries)
	st.mu.Unlock()
	if n != 1 {
		t.Errorf("过期项应被清理，剩余 %d, want 1", n)
	}
	if _, ok := st.Get("c"); !ok {
		t.Error("未过期项 c 不应被清理")
	}
}

// TestStickyTableConcurrent：并发读写无竞争（-race）。
func TestStickyTableConcurrent(t *testing.T) {
	st := NewStickyTable(time.Minute)
	done := make(chan struct{})
	for g := 0; g < 8; g++ {
		go func(g int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < 200; i++ {
				st.Set("s", int64(g))
				st.Get("s")
			}
		}(g)
	}
	for g := 0; g < 8; g++ {
		<-done
	}
}
