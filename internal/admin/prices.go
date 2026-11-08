package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/snownd/team-router/internal/model"
)

// priceDTO 定价载荷。
type priceDTO struct {
	ModelName      string `json:"model_name"`
	ProviderType   string `json:"provider_type"`
	InputPerM      int64  `json:"input_per_m"`
	OutputPerM     int64  `json:"output_per_m"`
	CacheReadPerM  int64  `json:"cache_read_per_m"`
	CacheWritePerM int64  `json:"cache_write_per_m"`
	Currency       string `json:"currency"`
	Status         int    `json:"status"`
}

// ListPrices GET /api/admin/prices。
func (h *Handlers) ListPrices(c *gin.Context) {
	ps, err := h.PriceRepo.List(c.Request.Context())
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": ps, "total": len(ps)})
}

// UpsertPrice POST /api/admin/prices：新增或更新（model_name+provider_type 唯一）。
func (h *Handlers) UpsertPrice(c *gin.Context) {
	var dto priceDTO
	if err := c.ShouldBindJSON(&dto); err != nil || dto.ModelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误：model_name 必填"}})
		return
	}
	p := model.ModelPrice{
		ModelName: dto.ModelName, ProviderType: dto.ProviderType,
		InputPerM: dto.InputPerM, OutputPerM: dto.OutputPerM,
		CacheReadPerM: dto.CacheReadPerM, CacheWritePerM: dto.CacheWritePerM,
		Currency: orDefaultStr(dto.Currency, "USD"),
		Status:   intOrDefault(dto.Status, model.PriceStatusEnabled),
	}
	if err := h.PriceRepo.Upsert(c.Request.Context(), &p); err != nil {
		h.serverError(c, err)
		return
	}
	h.Pricer.Invalidate()
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// DeletePrice DELETE /api/admin/prices/:id。
func (h *Handlers) DeletePrice(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := h.PriceRepo.Delete(c.Request.Context(), id); err != nil {
		h.serverError(c, err)
		return
	}
	h.Pricer.Invalidate()
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func orDefaultStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func intOrDefault(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}
