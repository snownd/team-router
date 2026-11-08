package admin

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/snownd/team-router/internal/model"
	"github.com/snownd/team-router/internal/provider"
)

// providerDTO 渠道创建/更新载荷。
type providerDTO struct {
	Name           string            `json:"name"`
	Type           string            `json:"type"`
	BaseURL        string            `json:"base_url"`
	APIKey         string            `json:"api_key"` // 明文，加密后落库；更新时留空表示不变
	Models         []string          `json:"models"`
	ModelMapping   map[string]string `json:"model_mapping"`
	Priority       int               `json:"priority"`
	Weight         int               `json:"weight"`
	MaxConcurrency int               `json:"max_concurrency"` // 0 = 不限制
	Status         *int              `json:"status"`
	ProxyURL       string            `json:"proxy_url"`
}

// ListProviders GET /api/admin/providers。
func (h *Handlers) ListProviders(c *gin.Context) {
	ps, err := h.ProviderRepo.List(c.Request.Context())
	if err != nil {
		h.serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": ps, "total": len(ps)})
}

// CreateProvider POST /api/admin/providers。
func (h *Handlers) CreateProvider(c *gin.Context) {
	var dto providerDTO
	if err := c.ShouldBindJSON(&dto); err != nil ||
		dto.Name == "" || dto.Type == "" || dto.BaseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误：name/type/base_url 必填"}})
		return
	}
	if !containsStr(h.Router.AdapterTypes(), dto.Type) {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message":   "不支持的渠道类型 " + dto.Type,
			"supported": h.Router.AdapterTypes(),
		}})
		return
	}
	encKey, err := h.Enc.Encrypt(dto.APIKey)
	if err != nil {
		h.serverError(c, err)
		return
	}
	status := model.ProviderStatusEnabled
	if dto.Status != nil {
		status = *dto.Status
	}
	p := model.Provider{
		Name: dto.Name, Type: dto.Type, BaseURL: dto.BaseURL, APIKeyEnc: encKey,
		Models:       json.RawMessage(mustJSONStr(orDefaultList(dto.Models))),
		ModelMapping: json.RawMessage(mustJSONStr(orDefaultMap(dto.ModelMapping))),
		Priority:     dto.Priority, Weight: maxIntDef(dto.Weight, 1),
		MaxConcurrency: dto.MaxConcurrency,
		Status:         status, ProxyURL: dto.ProxyURL,
	}
	if err := h.ProviderRepo.Create(c.Request.Context(), &p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}
	h.Router.Invalidate()
	c.JSON(http.StatusOK, p)
}

// UpdateProvider PUT /api/admin/providers/:id。
func (h *Handlers) UpdateProvider(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	p, err := h.ProviderRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "渠道不存在"}})
		return
	}
	var dto providerDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误"}})
		return
	}
	if dto.Name != "" {
		p.Name = dto.Name
	}
	if dto.BaseURL != "" {
		p.BaseURL = dto.BaseURL
	}
	if dto.Models != nil {
		p.Models = json.RawMessage(mustJSONStr(dto.Models))
	}
	if dto.ModelMapping != nil {
		p.ModelMapping = json.RawMessage(mustJSONStr(dto.ModelMapping))
	}
	p.Priority = dto.Priority
	p.Weight = maxIntDef(dto.Weight, 1)
	p.MaxConcurrency = dto.MaxConcurrency
	if dto.Status != nil {
		p.Status = *dto.Status
	}
	p.ProxyURL = dto.ProxyURL

	cols := []string{"name", "base_url", "models", "model_mapping", "priority", "weight", "max_concurrency", "status", "proxy_url"}
	if dto.APIKey != "" {
		encKey, err := h.Enc.Encrypt(dto.APIKey)
		if err != nil {
			h.serverError(c, err)
			return
		}
		p.APIKeyEnc = encKey
		cols = append(cols, "api_key")
	}
	if err := h.ProviderRepo.Update(c.Request.Context(), p, cols...); err != nil {
		h.serverError(c, err)
		return
	}
	h.Router.Invalidate()
	c.JSON(http.StatusOK, p)
}

// DeleteProvider DELETE /api/admin/providers/:id。
func (h *Handlers) DeleteProvider(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	if err := h.ProviderRepo.Delete(c.Request.Context(), id); err != nil {
		h.serverError(c, err)
		return
	}
	h.Router.Invalidate()
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// ProviderTypes GET /api/admin/providers/types：已注册适配器类型。
func (h *Handlers) ProviderTypes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"types": h.Router.AdapterTypes()})
}

// fetchModelsDTO 模型列表探测载荷。
type fetchModelsDTO struct {
	Type       string `json:"type"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key"` // 留空且指定 provider_id 时用已存密钥
	ProxyURL   string `json:"proxy_url"`
	ProviderID int64  `json:"provider_id"` // 编辑已有渠道时传入
}

// FetchProviderModels POST /api/admin/providers/fetch-models：
// 按适配器协议调用上游模型列表接口（如 GET {base}/v1/models）。
func (h *Handlers) FetchProviderModels(c *gin.Context) {
	var dto fetchModelsDTO
	if err := c.ShouldBindJSON(&dto); err != nil || dto.Type == "" || dto.BaseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "参数错误：type/base_url 必填"}})
		return
	}
	if !containsStr(h.Router.AdapterTypes(), dto.Type) {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message":   "不支持的渠道类型 " + dto.Type,
			"supported": h.Router.AdapterTypes(),
		}})
		return
	}
	apiKey := dto.APIKey
	if apiKey == "" && dto.ProviderID > 0 {
		p, err := h.ProviderRepo.GetByID(c.Request.Context(), dto.ProviderID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "渠道不存在"}})
			return
		}
		if apiKey, err = h.Enc.Decrypt(p.APIKeyEnc); err != nil {
			h.serverError(c, err)
			return
		}
	}
	client := &http.Client{Timeout: 20 * time.Second}
	if dto.ProxyURL != "" {
		if pu, err := url.Parse(dto.ProxyURL); err == nil {
			client.Transport = &http.Transport{Proxy: http.ProxyURL(pu)}
		}
	}
	models, err := h.Router.FetchModels(c.Request.Context(), &provider.Channel{
		Type: dto.Type, BaseURL: dto.BaseURL, APIKey: apiKey, ProxyURL: dto.ProxyURL,
	}, client)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "拉取模型列表失败：" + err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": models})
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func orDefaultList(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

func orDefaultMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func maxIntDef(a, def int) int {
	if a <= 0 {
		return def
	}
	return a
}
