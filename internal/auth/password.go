// Package auth 提供 JWT、API Key 认证、密码哈希与 SSO 注册中心。
package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword 生成 bcrypt 哈希。
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword 校验密码。
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
