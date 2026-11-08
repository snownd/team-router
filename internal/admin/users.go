package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/snownd/team-router/internal/auth"
	"github.com/snownd/team-router/internal/model"
)

// userDTO 用户创建/更新载荷。
type userDTO struct {
	Username      string  `json:"username"`
	DisplayName   string  `json:"display_name"`
	Email         string  `json:"email"`
	Password      string  `json:"password"`
	Role          string  `json:"role"`
	Status        *int    `json:"status"`
	Quota         *int64  `json:"quota"`
	DeptIDs       []int64 `json:"dept_ids"`
	PrimaryDeptID int64   `json:"primary_dept_id"`
}

// ListUsers GET /api/admin/users。
func (h *Handlers) ListUsers(c *gin.Context) {
	offset, limit := pageParams(c)
	users, total, err := h.UserRepo.List(c.Request.Context(), c.Query("keyword"), offset, limit)
	if err != nil {
		h.serverError(c, err)
		return
	}
	// 附加部门信息
	type userView struct {
		model.User
		Departments []model.UserDeptView `json:"departments"`
	}
	out := make([]userView, 0, len(users))
	for _, u := range users {
		depts, _ := h.DeptRepo.UserDepartments(c.Request.Context(), u.ID)
		out = append(out, userView{User: u, Departments: depts})
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "total": total})
}

// CreateUser POST /api/admin/users。
func (h *Handlers) CreateUser(c *gin.Context) {
	var dto userDTO
	if err := c.ShouldBindJSON(&dto); err != nil || dto.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误：username 必填"}})
		return
	}
	role := dto.Role
	if role != model.RoleAdmin {
		role = model.RoleUser
	}
	hash, err := auth.HashPassword(dto.Password)
	if err != nil {
		h.serverError(c, err)
		return
	}
	u := model.User{
		Username: dto.Username, DisplayName: dto.DisplayName, Email: dto.Email,
		PasswordHash: hash, Role: role, Status: model.UserStatusEnabled,
	}
	if dto.Quota != nil {
		u.Quota = *dto.Quota
	}
	if err := h.UserRepo.Create(c.Request.Context(), &u); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "创建失败: " + err.Error()}})
		return
	}
	if len(dto.DeptIDs) > 0 {
		primary := dto.PrimaryDeptID
		if primary == 0 {
			primary = dto.DeptIDs[0]
		}
		if err := h.DeptRepo.SetUserDepartments(c.Request.Context(), u.ID, dto.DeptIDs, primary); err != nil {
			h.serverError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"id": u.ID, "username": u.Username})
}

// GetUser GET /api/admin/users/:id。
func (h *Handlers) GetUser(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	u, err := h.UserRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "用户不存在"}})
		return
	}
	depts, _ := h.DeptRepo.UserDepartments(c.Request.Context(), id)
	c.JSON(http.StatusOK, gin.H{"user": u, "departments": depts})
}

// UpdateUser PUT /api/admin/users/:id。
func (h *Handlers) UpdateUser(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	u, err := h.UserRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "用户不存在"}})
		return
	}
	var dto userDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误"}})
		return
	}
	if dto.DisplayName != "" {
		u.DisplayName = dto.DisplayName
	}
	if dto.Email != "" {
		u.Email = dto.Email
	}
	if dto.Role == model.RoleAdmin || dto.Role == model.RoleUser {
		u.Role = dto.Role
	}
	if dto.Status != nil {
		u.Status = *dto.Status
	}
	if err := h.UserRepo.Update(c.Request.Context(), u, "display_name", "email", "role", "status"); err != nil {
		h.serverError(c, err)
		return
	}
	if dto.Quota != nil {
		if err := h.UserRepo.ChangeQuota(c.Request.Context(), id, *dto.Quota); err != nil {
			h.serverError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// SetUserPassword PUT /api/admin/users/:id/password。
func (h *Handlers) SetUserPassword(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var dto struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误"}})
		return
	}
	hash, err := auth.HashPassword(dto.Password)
	if err != nil {
		h.serverError(c, err)
		return
	}
	if err := h.UserRepo.SetPassword(c.Request.Context(), id, hash); err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// SetUserQuota POST /api/admin/users/:id/quota：设置总额度或按增量充值。
func (h *Handlers) SetUserQuota(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var dto struct {
		Quota *int64 `json:"quota"`
		Delta *int64 `json:"delta"`
	}
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误"}})
		return
	}
	if dto.Quota != nil {
		if err := h.UserRepo.ChangeQuota(c.Request.Context(), id, *dto.Quota); err != nil {
			h.serverError(c, err)
			return
		}
	}
	if dto.Delta != nil {
		if err := h.UserRepo.AdjustQuota(c.Request.Context(), id, *dto.Delta); err != nil {
			h.serverError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// SetUserDepartments PUT /api/admin/users/:id/departments：用户多部门设置。
func (h *Handlers) SetUserDepartments(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var dto struct {
		DeptIDs       []int64 `json:"dept_ids"`
		PrimaryDeptID int64   `json:"primary_dept_id"`
	}
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误"}})
		return
	}
	primary := dto.PrimaryDeptID
	if primary == 0 && len(dto.DeptIDs) > 0 {
		primary = dto.DeptIDs[0]
	}
	if err := h.DeptRepo.SetUserDepartments(c.Request.Context(), id, dto.DeptIDs, primary); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// DeleteUser DELETE /api/admin/users/:id。
func (h *Handlers) DeleteUser(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if id == userID(c) {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "不能删除自己"}})
		return
	}
	if err := h.UserRepo.Delete(c.Request.Context(), id); err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}
