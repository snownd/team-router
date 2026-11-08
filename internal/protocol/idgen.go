package protocol

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
)

// idGen 简单线程安全的随机 ID 生成器。
type idGen struct {
	mu sync.Mutex
}

func newIDGen() *idGen { return &idGen{} }

// Next 生成 24 字符随机 base64 ID。
func (g *idGen) Next() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "fallback-id"
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return base64.RawURLEncoding.EncodeToString(b)
}
