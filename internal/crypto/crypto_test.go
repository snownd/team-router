package crypto

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	enc, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	ct, err := enc.Encrypt("sk-my-upstream-secret")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ct, "sk-my-upstream-secret") {
		t.Fatal("密文包含明文")
	}
	pt, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}
	if pt != "sk-my-upstream-secret" {
		t.Errorf("解密错误: %s", pt)
	}
}

func TestEmptyString(t *testing.T) {
	enc, _ := New("")
	ct, _ := enc.Encrypt("")
	if ct != "" {
		t.Errorf("空串应返回空")
	}
}

func TestInvalidKey(t *testing.T) {
	if _, err := New("nothex"); err == nil {
		t.Error("非法 hex 应报错")
	}
	if _, err := New("aabb"); err == nil {
		t.Error("长度不足应报错")
	}
}

func TestDeterministicDevKey(t *testing.T) {
	a, _ := New("")
	b, _ := New("")
	ctA, _ := a.Encrypt("x")
	// 相同 nonce 不应出现（GCM 随机 nonce）
	ctB, _ := b.Encrypt("x")
	if ctA == ctB {
		t.Error("两次加密密文不应相同（nonce 应随机）")
	}
}
