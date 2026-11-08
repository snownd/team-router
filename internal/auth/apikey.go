package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"go.uber.org/fx"
	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/config"
	"github.com/snownd/team-router/internal/model"
	"github.com/snownd/team-router/internal/repository"
)

// Authenticator API Key 认证器（sha256 哈希查找 + TTL 进程内缓存）。
//
// 缓存策略：
//   - 正缓存：认证成功结果，TTL 取配置 APICacheSeconds；
//   - 负缓存：无效/禁用/过期等确定性失败结论，短 TTL（negativeTTL）缓存，
//     避免无效 key 被穷举时每次请求都穿透到数据库；
//   - 负缓存条目数有上限，防止恶意随机 key 撑爆内存。
//
// 管理端变更 key/用户状态后可调用 InvalidateCache() 立即生效。
type Authenticator struct {
	keyRepo  *repository.APIKeyRepo
	userRepo *repository.UserRepo
	ttl      time.Duration

	mu    sync.RWMutex
	cache map[string]authEntry
}

// 负缓存参数。
const (
	// negativeTTL 确定性失败结论的缓存时长
	negativeTTL = 30 * time.Second
	// negativeCacheMax 负缓存条目上限
	negativeCacheMax = 100_000
)

type authEntry struct {
	key     *model.APIKey
	user    *model.User
	err     error // 非空表示负缓存（认证失败结论）
	expires time.Time
}

// NewAuthenticator 构造。
func NewAuthenticator(keyRepo *repository.APIKeyRepo, userRepo *repository.UserRepo, cfg *config.Config) *Authenticator {
	ttl := time.Duration(cfg.Auth.APICacheSeconds) * time.Second
	if ttl <= 0 {
		ttl = time.Minute
	}
	return &Authenticator{keyRepo: keyRepo, userRepo: userRepo, ttl: ttl, cache: map[string]authEntry{}}
}

// ErrInvalidKey 无效 API Key。
var ErrInvalidKey = errors.New("无效的 API Key")

// ErrDisabled key 或用户被禁用。
var ErrDisabled = errors.New("API Key 或用户已被禁用")

// ErrExpired key 已过期。
var ErrExpired = errors.New("API Key 已过期")

// HashKey 计算 key 的 sha256 hex。
func HashKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// GenerateKey 生成新 key：返回 (raw, prefix, hash)。raw 仅此一次可见。
func GenerateKey() (raw, prefix, hash string) {
	b := make([]byte, 36)
	_, _ = rand.Read(b)
	raw = "sk-" + base64.RawURLEncoding.EncodeToString(b)
	prefix = raw[:12] + "..."
	return raw, prefix, HashKey(raw)
}

// Authenticate 认证 raw key，返回 key 与所属用户。
func (a *Authenticator) Authenticate(ctx context.Context, raw string) (*model.APIKey, *model.User, error) {
	hash := HashKey(raw)

	a.mu.RLock()
	if e, ok := a.cache[hash]; ok && time.Now().Before(e.expires) {
		a.mu.RUnlock()
		if e.err != nil {
			return nil, nil, e.err
		}
		return e.key, e.user, nil
	}
	a.mu.RUnlock()

	// cacheFail 写入负缓存（达到上限时放弃，仅损失防御效果不影响正确性）
	cacheFail := func(err error) {
		a.mu.Lock()
		if len(a.cache) < negativeCacheMax {
			a.cache[hash] = authEntry{err: err, expires: time.Now().Add(negativeTTL)}
		}
		a.mu.Unlock()
	}

	key, err := a.keyRepo.FindByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			cacheFail(ErrInvalidKey)
			return nil, nil, ErrInvalidKey
		}
		return nil, nil, err // DB 临时故障不缓存
	}
	user, err := a.userRepo.GetByID(ctx, key.UserID)
	if err != nil {
		return nil, nil, err
	}
	if key.Status != model.APIKeyStatusEnabled || user.Status != model.UserStatusEnabled {
		cacheFail(ErrDisabled)
		return nil, nil, ErrDisabled
	}
	if key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now()) {
		cacheFail(ErrExpired)
		return nil, nil, ErrExpired
	}

	a.mu.Lock()
	a.cache[hash] = authEntry{key: key, user: user, expires: time.Now().Add(a.ttl)}
	a.mu.Unlock()
	return key, user, nil
}

// InvalidateCache 清空认证缓存。
func (a *Authenticator) InvalidateCache() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cache = map[string]authEntry{}
}

// Module fx 模块。
var Module = fx.Module("auth",
	fx.Provide(NewAuthenticator, NewJWTManager),
)
