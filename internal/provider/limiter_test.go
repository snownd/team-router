package provider

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testChannel(id int64, maxConc int) *Channel {
	return &Channel{ID: id, Name: fmt.Sprintf("ch-%d", id), MaxConcurrency: maxConc}
}

// TestLimiterEnforcesCap：MaxConcurrency=N 时同时在途请求绝不超 N。
func TestLimiterEnforcesCap(t *testing.T) {
	l := NewLimiter()
	ch := testChannel(1, 2)

	var inFlight, max atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := l.Acquire(context.Background(), ch, 5*time.Second)
			if err != nil {
				t.Errorf("acquire 失败: %v", err)
				return
			}
			cur := inFlight.Add(1)
			for {
				old := max.Load()
				if cur <= old || max.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			inFlight.Add(-1)
			release()
		}()
	}
	wg.Wait()
	if got := max.Load(); got > 2 {
		t.Errorf("在途峰值 = %d, 超过上限 2", got)
	}
	if got := max.Load(); got < 2 {
		t.Errorf("在途峰值 = %d, 应达到过上限 2（排队后放行）", got)
	}
}

// TestLimiterZeroUnlimited：MaxConcurrency=0 直接放行，不排队。
func TestLimiterZeroUnlimited(t *testing.T) {
	l := NewLimiter()
	ch := testChannel(2, 0)

	var inFlight, max atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := l.Acquire(context.Background(), ch, 0)
			if err != nil {
				t.Errorf("acquire 失败: %v", err)
				return
			}
			cur := inFlight.Add(1)
			for {
				old := max.Load()
				if cur <= old || max.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			inFlight.Add(-1)
			release()
		}()
	}
	wg.Wait()
	if got := max.Load(); got != 8 {
		t.Errorf("不限流时应全部并发（峰值 8）, got %d", got)
	}
}

// TestLimiterQueueTimeout：满载时排队等待超时返回错误，槽位释放后排队者获得。
func TestLimiterQueueTimeout(t *testing.T) {
	l := NewLimiter()
	ch := testChannel(3, 1)

	rel1, err := l.Acquire(context.Background(), ch, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, err = l.Acquire(context.Background(), ch, 100*time.Millisecond)
	if err == nil {
		t.Fatal("满载排队应超时报错")
	}
	if elapsed := time.Since(start); elapsed < 90*time.Millisecond {
		t.Errorf("应等待约 100ms 才超时, 实际 %v", elapsed)
	}

	// 释放后排队者应立即获得
	go func() {
		time.Sleep(50 * time.Millisecond)
		rel1()
	}()
	rel2, err := l.Acquire(context.Background(), ch, 2*time.Second)
	if err != nil {
		t.Fatalf("释放后应获得槽位: %v", err)
	}
	rel2()
}

// TestLimiterContextCancel：排队期间 ctx 取消立即返回错误。
func TestLimiterContextCancel(t *testing.T) {
	l := NewLimiter()
	ch := testChannel(4, 1)

	rel1, err := l.Acquire(context.Background(), ch, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer rel1()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	if _, err := l.Acquire(ctx, ch, 10*time.Second); err == nil {
		t.Fatal("ctx 取消应返回错误")
	} else if time.Since(start) > time.Second {
		t.Error("ctx 取消应立即返回而非等到超时")
	}
}

// TestLimiterCapacityChange：渠道并发配置热更后按新容量放行。
func TestLimiterCapacityChange(t *testing.T) {
	l := NewLimiter()
	old := testChannel(5, 1)
	rel1, err := l.Acquire(context.Background(), old, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// 配置热更：同 ID 渠道容量变为 2。旧在途槽位（rel1）留在旧实例，不占新实例名额。
	updated := testChannel(5, 2)
	rel2, err := l.Acquire(context.Background(), updated, time.Second)
	if err != nil {
		t.Fatalf("容量提升后应获得槽位: %v", err)
	}
	rel3, err := l.Acquire(context.Background(), updated, time.Second)
	if err != nil {
		t.Fatalf("新容量 2 内第二个槽位应可获得: %v", err)
	}

	// 新实例满载（2/2）时第四个请求应排队超时
	if _, err := l.Acquire(context.Background(), updated, 50*time.Millisecond); err == nil {
		t.Fatal("超过新容量应排队超时")
	}
	rel3()
	rel2()
	rel1() // 旧实例的 release 不应 panic
}

// TestLimiterConcurrentAcquireRelease：并发获取/释放无竞争（-race）。
func TestLimiterConcurrentAcquireRelease(t *testing.T) {
	l := NewLimiter()
	chans := []*Channel{testChannel(10, 2), testChannel(11, 3), testChannel(12, 0)}
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := chans[g%len(chans)]
			for i := 0; i < 50; i++ {
				release, err := l.Acquire(context.Background(), ch, 2*time.Second)
				if err != nil {
					t.Errorf("acquire: %v", err)
					return
				}
				release()
			}
		}()
	}
	wg.Wait()
}
