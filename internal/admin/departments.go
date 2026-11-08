package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/snownd/team-router/internal/model"
)

// DepartmentTree GET /api/admin/departments：全量部门树（闭包表加速，此处直接用 parent 组树）。
func (h *Handlers) DepartmentTree(c *gin.Context) {
	depts, err := h.DeptRepo.List(c.Request.Context())
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tree": buildDeptTree(depts), "total": len(depts)})
}

func buildDeptTree(depts []model.Department) []*model.DeptTreeNode {
	nodes := map[int64]*model.DeptTreeNode{}
	var roots []*model.DeptTreeNode
	for _, d := range depts {
		nodes[d.ID] = &model.DeptTreeNode{
			ID: d.ID, ParentID: d.ParentID, Name: d.Name,
			SortOrder: d.SortOrder, Status: d.Status,
		}
	}
	for _, d := range depts {
		n := nodes[d.ID]
		if d.ParentID != nil {
			if p, ok := nodes[*d.ParentID]; ok {
				p.Children = append(p.Children, n)
				continue
			}
		}
		roots = append(roots, n)
	}
	return roots
}

// CreateDepartment POST /api/admin/departments。
func (h *Handlers) CreateDepartment(c *gin.Context) {
	var dto struct {
		ParentID  int64  `json:"parent_id"`
		Name      string `json:"name" binding:"required"`
		SortOrder int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误：name 必填"}})
		return
	}
	dept, err := h.DeptRepo.Create(c.Request.Context(), dto.ParentID, dto.Name, dto.SortOrder)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, dept)
}

// UpdateDepartment PUT /api/admin/departments/:id。
func (h *Handlers) UpdateDepartment(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var dto struct {
		Name      string `json:"name"`
		SortOrder int    `json:"sort_order"`
		Status    int    `json:"status"`
	}
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误"}})
		return
	}
	if dto.Status == 0 {
		dto.Status = model.DeptStatusEnabled
	}
	if err := h.DeptRepo.UpdateMeta(c.Request.Context(), id, dto.Name, dto.SortOrder, dto.Status); err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// MoveDepartment POST /api/admin/departments/:id/move：移动子树。
func (h *Handlers) MoveDepartment(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var dto struct {
		NewParentID int64 `json:"new_parent_id"`
	}
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误"}})
		return
	}
	if err := h.DeptRepo.Move(c.Request.Context(), id, dto.NewParentID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// DeleteDepartment DELETE /api/admin/departments/:id：删除整个子树。
func (h *Handlers) DeleteDepartment(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := h.DeptRepo.Delete(c.Request.Context(), id); err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// DepartmentAncestors GET /api/admin/departments/:id/ancestors：祖先链（面包屑）。
func (h *Handlers) DepartmentAncestors(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	bc, err := h.DeptRepo.Ancestors(c.Request.Context(), id)
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"breadcrumbs": bc})
}

// DepartmentDescendants GET /api/admin/departments/:id/descendants。
func (h *Handlers) DepartmentDescendants(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	ids, err := h.DeptRepo.DescendantIDs(c.Request.Context(), id, 0)
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ids": ids, "count": len(ids)})
}

// DepartmentUsers GET /api/admin/departments/:id/users?include_subtree=1：部门（含子树）成员。
func (h *Handlers) DepartmentUsers(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	includeSubtree := c.Query("include_subtree") == "1" || c.Query("include_subtree") == "true"
	ids, err := h.DeptRepo.DeptUserIDs(c.Request.Context(), id, includeSubtree)
	if err != nil {
		h.serverError(c, err)
		return
	}
	if len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"items": []any{}, "total": 0})
		return
	}
	users, err := h.UserRepo.ListByIDs(c.Request.Context(), ids)
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": users, "total": len(users)})
}
