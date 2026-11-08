package usage

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/snownd/team-router/internal/model"
	"github.com/snownd/team-router/internal/repository"
)

// AsyncLogWriter 异步请求日志写入器：缓冲 channel + 定时/定量批量落库。
// 丢弃策略：channel 满时丢弃并计数（不影响请求路径）。
type AsyncLogWriter struct {
	repo    *repository.RequestLogRepo
	log     *zap.Logger
	ch      chan model.RequestLog
	batch   []model.RequestLog
	mu      sync.Mutex
	dropped atomic.Int64
	wg      sync.WaitGroup
}

// NewAsyncLogWriter 构造。
func NewAsyncLogWriter(repo *repository.RequestLogRepo, zapLog *zap.Logger) *AsyncLogWriter {
	return &AsyncLogWriter{
		repo: repo,
		log:  zapLog.Named("usage-writer"),
		ch:   make(chan model.RequestLog, 4096),
	}
}

// Start 启动后台写入循环（fx lifecycle 调用）。
func (w *AsyncLogWriter) Start(ctx context.Context) {
	w.wg.Add(1)
	go w.loop(ctx)
}

// Enqueue 非阻塞入队；返回是否成功。
func (w *AsyncLogWriter) Enqueue(l model.RequestLog) bool {
	if l.CreatedAt.IsZero() {
		l.CreatedAt = time.Now().UTC()
	}
	select {
	case w.ch <- l:
		return true
	default:
		d := w.dropped.Add(1)
		if d%100 == 1 {
			w.log.Warn("request log dropped (queue full)", zap.Int64("total_dropped", d))
		}
		return false
	}
}

// Dropped 累计丢弃数。
func (w *AsyncLogWriter) Dropped() int64 { return w.dropped.Load() }

func (w *AsyncLogWriter) loop(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.flush()
			return
		case l := <-w.ch:
			w.batch = append(w.batch, l)
			if len(w.batch) >= 100 {
				w.flush()
			}
		case <-ticker.C:
			w.flush()
		}
	}
}

// flush 批量写入（带简单重试）。
func (w *AsyncLogWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.batch) == 0 {
		return
	}
	logs := w.batch
	w.batch = nil

	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err = w.repo.InsertBatch(context.Background(), logs); err == nil {
			return
		}
		time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
	}
	w.log.Error("flush request logs failed", zap.Int("count", len(logs)), zap.Error(err))
}

// Wait 等待后台退出（测试用）。
func (w *AsyncLogWriter) Wait() { w.wg.Wait() }

// RegisterLifecycle fx 生命周期注册。
func RegisterLifecycle(lc fx.Lifecycle, w *AsyncLogWriter, log *zap.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			w.Start(ctx)
			return nil
		},
		OnStop: func(_ context.Context) error {
			cancel()
			w.Wait()
			log.Info("usage writer stopped")
			return nil
		},
	})
}

// Module fx 模块。
var Module = fx.Module("usage",
	fx.Provide(NewAsyncLogWriter),
	fx.Invoke(RegisterLifecycle),
)
