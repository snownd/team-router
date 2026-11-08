package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/snownd/team-router/internal/repository"
)

// logFilterFromQuery 从查询参数构造日志过滤。
func logFilterFromQuery(c *gin.Context) repository.LogFilter {
	start, end := parseTimeRange(c)
	f := repository.LogFilter{
		Start: start, End: end,
		Model:    c.Query("model"),
		Protocol: c.Query("protocol"),
		Keyword:  c.Query("keyword"),
	}
	if v := c.Query("user_id"); v != "" {
		f.UserID, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := c.Query("api_key_id"); v != "" {
		f.APIKeyID, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := c.Query("provider_id"); v != "" {
		f.ProviderID, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := c.Query("is_error"); v != "" {
		b := v == "1" || v == "true"
		f.IsError = &b
	}
	f.Offset, f.Limit = pageParams(c)
	return f
}

// AdminListLogs GET /api/admin/logs：请求日志查询（强制时间范围分区裁剪）。
func (h *Handlers) AdminListLogs(c *gin.Context) {
	f := logFilterFromQuery(c)
	logs, total, err := h.LogRepo.Query(c.Request.Context(), f)
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": logs, "total": total})
}

// MyLogs GET /api/user/logs：当前用户日志。
func (h *Handlers) MyLogs(c *gin.Context) {
	f := logFilterFromQuery(c)
	f.UserID = userID(c) // 强制本人
	logs, total, err := h.LogRepo.Query(c.Request.Context(), f)
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": logs, "total": total})
}

// AdminStats GET /api/admin/stats?start&end&user_id：用量与费用统计。
func (h *Handlers) AdminStats(c *gin.Context) {
	start, end := parseTimeRange(c)
	var uid int64
	if v := c.Query("user_id"); v != "" {
		uid, _ = strconv.ParseInt(v, 10, 64)
	}
	stats, err := h.LogRepo.Stats(c.Request.Context(), start, end, uid)
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": stats, "total": len(stats)})
}

// MyStats GET /api/user/stats。
func (h *Handlers) MyStats(c *gin.Context) {
	start, end := parseTimeRange(c)
	stats, err := h.LogRepo.Stats(c.Request.Context(), start, end, userID(c))
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": stats, "total": len(stats)})
}

// TTFTStats GET /api/admin/stats/ttft：延迟看板。
func (h *Handlers) TTFTStats(c *gin.Context) {
	start, end := parseTimeRange(c)
	rows, err := h.LogRepo.TTFTStats(c.Request.Context(), start, end)
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}
