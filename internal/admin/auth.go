package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/auth"
	"github.com/snownd/team-router/internal/model"
)

// Login POST /api/auth/login：本地账号登录。
func (h *Handlers) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误"}})
		return
	}
	user, err := h.UserRepo.GetByUsername(c.Request.Context(), req.Username)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "用户名或密码错误"}})
			return
		}
		h.serverError(c, err)
		return
	}
	if user.Status != model.UserStatusEnabled || !auth.CheckPassword(user.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "用户名或密码错误"}})
		return
	}
	token, err := h.JWT.Issue(user.ID, user.Username, user.Role)
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"access_token": token,
		"user": gin.H{
			"id": user.ID, "username": user.Username, "display_name": user.DisplayName,
			"role": user.Role, "quota": user.Quota, "used_quota": user.UsedQuota,
		},
	})
}

// Me GET /api/user/me：当前用户信息（含部门）。
func (h *Handlers) Me(c *gin.Context) {
	uid := userID(c)
	user, err := h.UserRepo.GetByID(c.Request.Context(), uid)
	if err != nil {
		h.serverError(c, err)
		return
	}
	depts, _ := h.DeptRepo.UserDepartments(c.Request.Context(), uid)
	c.JSON(http.StatusOK, gin.H{
		"id": user.ID, "username": user.Username, "display_name": user.DisplayName,
		"email": user.Email, "role": user.Role, "status": user.Status,
		"quota": user.Quota, "used_quota": user.UsedQuota,
		"quota_remaining": quotaRemaining(user.Quota, user.UsedQuota),
		"departments":     depts,
	})
}

func quotaRemaining(quota, used int64) int64 {
	if quota == 0 {
		return -1 // 不限额
	}
	r := quota - used
	if r < 0 {
		return 0
	}
	return r
}

func (h *Handlers) serverError(c *gin.Context, err error) {
	h.Log.Error("admin handler error", zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "内部错误"}})
}
