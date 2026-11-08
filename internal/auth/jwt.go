package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/snownd/team-router/internal/config"
)

// JWTManager 管理台 JWT 签发与校验。
type JWTManager struct {
	secret []byte
	ttl    time.Duration
}

// NewJWTManager 构造。
func NewJWTManager(cfg *config.Config) *JWTManager {
	return &JWTManager{secret: []byte(cfg.Auth.JWTSecret), ttl: cfg.Auth.TokenTTL}
}

// Claims JWT 载荷。
type Claims struct {
	UserID   int64  `json:"uid"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// Issue 签发 token。
func (m *JWTManager) Issue(userID int64, username, role string) (string, error) {
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "team-router",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// Parse 校验 token。
func (m *JWTManager) Parse(token string) (*Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("非法签名算法")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	return &claims, nil
}

// gin 上下文键。
const (
	CtxUserID   = "auth_user_id"
	CtxUsername = "auth_username"
	CtxRole     = "auth_role"
)

// JWTAuth JWT 认证中间件（/api/*）。
func (m *JWTManager) JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			abortJSON(c, http.StatusUnauthorized, "缺少认证令牌")
			return
		}
		claims, err := m.Parse(token)
		if err != nil {
			abortJSON(c, http.StatusUnauthorized, "认证令牌无效或已过期")
			return
		}
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUsername, claims.Username)
		c.Set(CtxRole, claims.Role)
		c.Next()
	}
}

// AdminRequired 管理员角色校验（需在 JWTAuth 之后）。
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(CtxRole) != "admin" {
			abortJSON(c, http.StatusForbidden, "需要管理员权限")
			return
		}
		c.Next()
	}
}

func bearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(header)
}

func abortJSON(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"message": msg, "type": "authentication_error"}})
}
