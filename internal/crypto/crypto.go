// Package crypto 提供 AES-256-GCM 加解密，用于 provider 上游密钥的静态加密。
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sync"

	"go.uber.org/fx"

	"github.com/snownd/team-router/internal/config"
)

// 默认开发主密钥（固定 32 字节）：生产环境必须通过 TR_CRYPTO_MASTER_KEY 显式配置。
var defaultDevKey = []byte("0123456789abcdef0123456789abcdef")

// Encryptor AES-GCM 加解密器，并发安全。
type Encryptor struct {
	aead cipher.AEAD
}

var (
	defaultEnc     *Encryptor
	defaultEncOnce sync.Once
)

// New 用 32 字节 hex 主密钥构造 Encryptor；key 为空时使用开发默认密钥。
func New(keyHex string) (*Encryptor, error) {
	key := defaultDevKey
	if keyHex != "" {
		k, err := hex.DecodeString(keyHex)
		if err != nil {
			return nil, fmt.Errorf("master key 不是合法 hex: %w", err)
		}
		if len(k) != 32 {
			return nil, fmt.Errorf("master key 必须为 32 字节（64 个 hex 字符），当前 %d 字节", len(k))
		}
		key = k
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Encryptor{aead: aead}, nil
}

// Default 返回使用开发默认密钥的实例（测试用）。
func Default() *Encryptor {
	defaultEncOnce.Do(func() {
		defaultEnc, _ = New("")
	})
	return defaultEnc
}

// IsDefaultKey 是否在使用开发默认密钥。
func IsDefaultKey(keyHex string) bool { return keyHex == "" }

// NewFromConfig 从配置构造 Encryptor。
func NewFromConfig(cfg *config.Config) (*Encryptor, error) {
	return New(cfg.Crypto.MasterKey)
}

// Module fx 模块。
var Module = fx.Provide(NewFromConfig)

// Encrypt 加密，返回 hex(nonce||ciphertext)。
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := e.aead.Seal(nil, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(append(nonce, ct...)), nil
}

// Decrypt 解密 hex(nonce||ciphertext)。
func (e *Encryptor) Decrypt(cipherHex string) (string, error) {
	if cipherHex == "" {
		return "", nil
	}
	raw, err := hex.DecodeString(cipherHex)
	if err != nil {
		return "", fmt.Errorf("cipher hex 非法: %w", err)
	}
	ns := e.aead.NonceSize()
	if len(raw) < ns {
		return "", fmt.Errorf("cipher 过短")
	}
	pt, err := e.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
