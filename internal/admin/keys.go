package admin

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/snownd/team-router/internal/auth"
	"github.com/snownd/team-router/internal/model"
)

// keyDTO key 创建载荷。
type keyDTO struct {
	Name          string   `json:"name"`
	Quota         int64    `json:"quota"`
	AllowedModels []string `json:"allowed_models"`
	ExpiresInDays int      `json:"expires_in_days"`
}

// ListMyKeys GET /api/user/keys：当前用户 key 列表。
func (h *Handlers) ListMyKeys(c *gin.Context) {
	keys, err := h.KeyRepo.ListByUser(c.Request.Context(), userID(c))
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": keys, "total": len(keys)})
}

// CreateMyKey POST /api/user/keys：创建 key（明文仅返回一次）。
func (h *Handlers) CreateMyKey(c *gin.Context) {
	var dto keyDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误"}})
		return
	}
	key, err := h.createKey(c, userID(c), dto)
	if err != nil {
		return
	}
	c.JSON(http.StatusOK, key)
}

func (h *Handlers) createKey(c *gin.Context, uid int64, dto keyDTO) (gin.H, error) {
	raw, prefix, hash := auth.GenerateKey()
	k := model.APIKey{
		UserID: uid, Name: dto.Name, KeyPrefix: prefix, KeyHash: hash,
		Status: model.APIKeyStatusEnabled, Quota: dto.Quota,
	}
	if len(dto.AllowedModels) > 0 {
		k.AllowedModels = json.RawMessage(mustJSONStr(dto.AllowedModels))
	}
	if dto.ExpiresInDays > 0 {
		t := time.Now().AddDate(0, 0, dto.ExpiresInDays)
		k.ExpiresAt = &t
	}
	if err := h.KeyRepo.Create(c.Request.Context(), &k); err != nil {
		h.serverError(c, err)
		return nil, err
	}
	// 明文仅此一次返回
	return gin.H{
		"id": k.ID, "name": k.Name, "key": raw, "key_prefix": prefix,
		"quota": k.Quota, "expires_at": k.ExpiresAt,
		"notice": "请妥善保存，密钥明文不会再次显示",
	}, nil
}

// DeleteMyKey DELETE /api/user/keys/:id。
func (h *Handlers) DeleteMyKey(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	k, err := h.KeyRepo.GetByID(c.Request.Context(), id)
	if err != nil || k.UserID != userID(c) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "key 不存在"}})
		return
	}
	if err := h.KeyRepo.Delete(c.Request.Context(), id); err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// ---- 管理端 ----

// AdminListKeys GET /api/admin/keys?user_id=。
func (h *Handlers) AdminListKeys(c *gin.Context) {
	offset, limit := pageParams(c)
	var uid int64
	if v := c.Query("user_id"); v != "" {
		uid = parseInt64(v)
	}
	keys, total, err := h.KeyRepo.ListAll(c.Request.Context(), uid, offset, limit)
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": keys, "total": total})
}

// AdminCreateKey POST /api/admin/keys：为任意用户创建 key。
func (h *Handlers) AdminCreateKey(c *gin.Context) {
	var dto struct {
		UserID        int64    `json:"user_id"`
		Name          string   `json:"name"`
		Quota         int64    `json:"quota"`
		AllowedModels []string `json:"allowed_models"`
		ExpiresInDays int      `json:"expires_in_days"`
	}
	if err := c.ShouldBindJSON(&dto); err != nil || dto.UserID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误：user_id 必填"}})
		return
	}
	if _, err := h.UserRepo.GetByID(c.Request.Context(), dto.UserID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "用户不存在"}})
		return
	}
	key, err := h.createKey(c, dto.UserID, keyDTO{
		Name: dto.Name, Quota: dto.Quota,
		AllowedModels: dto.AllowedModels, ExpiresInDays: dto.ExpiresInDays,
	})
	if err != nil {
		return
	}
	c.JSON(http.StatusOK, key)
}

// AdminUpdateKey PUT /api/admin/keys/:id：启用/禁用/改额度。
func (h *Handlers) AdminUpdateKey(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	k, err := h.KeyRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "key 不存在"}})
		return
	}
	var dto struct {
		Name   string `json:"name"`
		Status *int   `json:"status"`
		Quota  *int64 `json:"quota"`
	}
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误"}})
		return
	}
	if dto.Name != "" {
		k.Name = dto.Name
	}
	if dto.Status != nil {
		k.Status = *dto.Status
	}
	cols := []string{"name", "status"}
	if dto.Quota != nil {
		k.Quota = *dto.Quota
		cols = append(cols, "quota")
	}
	if err := h.KeyRepo.Update(c.Request.Context(), k, cols...); err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// AdminDeleteKey DELETE /api/admin/keys/:id。
func (h *Handlers) AdminDeleteKey(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := h.KeyRepo.Delete(c.Request.Context(), id); err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func mustJSONStr(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func parseInt64(s string) int64 {
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int64(ch-'0')
	}
	return n
}
