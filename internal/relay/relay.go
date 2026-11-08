// Package relay 网关核心管线：认证 -> 解析 -> 路由 -> 转发 -> 计量 -> 计费 -> 落日志。
//
// 路由增强（单节点内存实现）：
//   - 粘性路由：同会话（X-Session-ID / metadata.session_id / 首条 user 消息哈希）
//     的多轮请求命中相同上游账户（StickyTable，滑动 TTL）；
//   - 并发限制：每渠道 max_concurrency 在途槽位（Limiter），满载排队等待
//     （channel_queue_timeout 上限），超时直接返回终止性 429（channel_busy），
//     不跨渠道重试，保持各账户并发隔离。
package relay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/snownd/team-router/internal/auth"
	"github.com/snownd/team-router/internal/billing"
	"github.com/snownd/team-router/internal/config"
	"github.com/snownd/team-router/internal/model"
	"github.com/snownd/team-router/internal/protocol"
	"github.com/snownd/team-router/internal/provider"
	"github.com/snownd/team-router/internal/usage"
)

// streamEncoder canonical 事件 -> 客户端 SSE（三种协议通用接口）。
type streamEncoder interface {
	WriteEvent(ev *protocol.Event) error
}

// protoBinding 协议绑定：解析 / 错误渲染 / 响应编码 / 流编码工厂。
type protoBinding struct {
	id           string
	parse        func([]byte) (*protocol.Request, *protocol.RelayError)
	renderErr    func(w http.ResponseWriter, e *protocol.RelayError)
	encodeResp   func(*protocol.Response) map[string]any
	newStreamEnc func(w http.ResponseWriter) streamEncoder
}

// Service relay 服务。
type Service struct {
	auth    *auth.Authenticator
	router  *provider.Router
	reg     *provider.Registry
	limiter *provider.Limiter
	sticky  *provider.StickyTable
	pricer  *billing.Pricer
	quota   *billing.Quota
	writer  *usage.AsyncLogWriter
	cfg     *config.Config
	log     *zap.Logger
	client  *http.Client

	proxyMu      sync.Mutex
	proxyClients map[string]*http.Client
}

// NewService 构造。
func NewService(
	a *auth.Authenticator,
	router *provider.Router,
	reg *provider.Registry,
	limiter *provider.Limiter,
	pricer *billing.Pricer,
	quota *billing.Quota,
	writer *usage.AsyncLogWriter,
	cfg *config.Config,
	zapLog *zap.Logger,
) *Service {
	stickyTTL := cfg.Relay.StickySessionTTL
	if stickyTTL == 0 {
		stickyTTL = 30 * time.Minute
	}
	return &Service{
		auth: a, router: router, reg: reg, limiter: limiter,
		sticky: provider.NewStickyTable(stickyTTL),
		pricer: pricer, quota: quota,
		writer: writer, cfg: cfg, log: zapLog.Named("relay"),
		client:       &http.Client{Transport: newTransport(cfg.Relay, nil)},
		proxyClients: map[string]*http.Client{},
	}
}

// ---- HTTP handlers ----

// HandleChatCompletions POST /v1/chat/completions。
func (s *Service) HandleChatCompletions(c *gin.Context) {
	s.relay(c, protoBinding{
		id:           model.ProtocolChatCompletions,
		parse:        protocol.ParseChatWire,
		renderErr:    protocol.RenderChatError,
		encodeResp:   protocol.ChatEncodeResponse,
		newStreamEnc: func(w http.ResponseWriter) streamEncoder { return protocol.NewChatStreamEncoder(w) },
	})
}

// HandleResponses POST /v1/responses。
func (s *Service) HandleResponses(c *gin.Context) {
	s.relay(c, protoBinding{
		id:           model.ProtocolResponses,
		parse:        protocol.ParseResponsesWire,
		renderErr:    protocol.RenderResponsesError,
		encodeResp:   protocol.ResponsesEncodeResponse,
		newStreamEnc: func(w http.ResponseWriter) streamEncoder { return protocol.NewResponsesStreamEncoder(w) },
	})
}

// HandleMessages POST /v1/messages。
func (s *Service) HandleMessages(c *gin.Context) {
	s.relay(c, protoBinding{
		id:           model.ProtocolMessages,
		parse:        protocol.ParseAnthropicWire,
		renderErr:    protocol.RenderMessagesError,
		encodeResp:   protocol.AnthropicEncodeResponse,
		newStreamEnc: func(w http.ResponseWriter) streamEncoder { return protocol.NewAnthropicStreamEncoder(w) },
	})
}

// HandleModels GET /v1/models：网关可服务模型列表（OpenAI 格式）。
func (s *Service) HandleModels(c *gin.Context) {
	if _, _, rerr := s.authenticate(c, protocol.RenderChatError); rerr != nil {
		return
	}
	models, err := s.router.Models(c.Request.Context())
	if err != nil {
		protocol.RenderChatError(c.Writer, protocol.ErrUpstream("查询模型列表失败"))
		return
	}
	items := make([]map[string]any, 0, len(models))
	for _, m := range models {
		items = append(items, map[string]any{"id": m, "object": "model", "owned_by": "team-router"})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": items})
}

// ---- 核心管线 ----

// streamEncoder 包装器：流式模型名还原。
// 将 message_start 事件中的模型名强制改写为客户端请求的模型别名（响应模型名还原，
// 对齐 new-api 的闭环双向映射），防止渠道模型映射后的上游物理模型名漂移到客户端，
// 避免对模型名敏感的下游客户端（代码辅助工具等）发生状态错误。
type modelRewriteEncoder struct {
	inner streamEncoder
	model string
}

func (e *modelRewriteEncoder) WriteEvent(ev *protocol.Event) error {
	if e.model != "" && ev.Type == protocol.EventMessageStart {
		ev.Model = e.model
	}
	return e.inner.WriteEvent(ev)
}

func (s *Service) relay(c *gin.Context, pb protoBinding) {
	ctx := c.Request.Context()
	started := time.Now()

	// 1. API Key 认证
	rawKey := bearer(c.GetHeader("Authorization"))
	if rawKey == "" {
		pb.renderErr(c.Writer, protocol.ErrAuth("缺少 API Key（Authorization: Bearer sk-...）"))
		return
	}
	key, user, authErr := s.authenticateRaw(c, rawKey)
	if authErr != nil {
		pb.renderErr(c.Writer, authErr)
		return
	}

	// 2. 读取并解析请求
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 32<<20))
	if err != nil {
		pb.renderErr(c.Writer, protocol.ErrInvalidRequest("读取请求体失败"))
		return
	}
	req, perr := pb.parse(body)
	if perr != nil {
		pb.renderErr(c.Writer, perr)
		return
	}

	// 3. key 模型白名单
	if !key.AllowsModel(req.Model) {
		pb.renderErr(c.Writer, protocol.ErrPermission("该 API Key 无权访问模型 "+req.Model))
		return
	}

	// 4. 渠道路由
	channels, err := s.router.SelectForModel(ctx, req.Model)
	if err != nil || len(channels) == 0 {
		pb.renderErr(c.Writer, protocol.ErrNotFoundModel("无可用渠道服务模型 "+req.Model))
		return
	}
	// 粘性路由：多轮会话绑定的渠道移到候选首位（best effort，未命中/已下线则正常加权路由）
	sessionKey := s.sessionKey(c, req, key)
	if channelID, ok := s.sticky.Get(sessionKey); ok {
		if i := indexOfChannelID(channels, channelID); i > 0 {
			sticky := channels[i]
			copy(channels[1:i+1], channels[:i])
			channels[0] = sticky
		}
	}

	// 5. 计价与配额预扣
	// 按候选渠道集合的价目上界预扣：实际命中渠道由转发阶段的加权抽样与跨渠道重试决定，
	// 必须按候选全集中最贵价目预扣，避免“高价渠道低价预扣”导致超额放行；
	// 结算阶段再按实际命中渠道精确计费（见 settleAndLog）。
	price, err := s.reservePrice(ctx, req.Model, channels)
	if err != nil {
		s.log.Warn("price lookup failed", zap.Error(err))
	}
	if price == nil && !s.cfg.Billing.AllowUnknownPricing {
		pb.renderErr(c.Writer, protocol.ErrInvalidRequest("模型 "+req.Model+" 未配置价格且网关禁止未定价请求"))
		return
	}
	maxOut := req.MaxTokens
	if maxOut <= 0 {
		maxOut = s.cfg.Relay.DefaultMaxTokens
	}
	estInput := usage.EstimateRequestTokens(req)
	reserved := billing.EstimateReserve(estInput, maxOut, price)
	if rerr := s.quota.Reserve(ctx, key.ID, user.ID, reserved); rerr != nil {
		s.log.Info("quota reserve rejected",
			zap.Int64("user_id", user.ID), zap.Int64("key_id", key.ID), zap.Error(rerr))
		pb.renderErr(c.Writer, protocol.ErrQuota("额度不足: "+rerr.Error()))
		return
	}

	// 6. 转发（带跨渠道重试）；meter 从请求开始计时
	result := &forwardResult{meter: usage.NewMeter()}
	s.forward(c, pb, req, channels, result)

	// 粘性绑定：仅转发成功时绑定会话 -> 渠道，失败不绑定（下一轮自然重选）
	if result.channel != nil && result.relayErr == nil {
		s.sticky.Set(sessionKey, result.channel.ID)
	}

	// 未向客户端写出任何内容时，按协议渲染错误响应
	if result.relayErr != nil && !result.wroteToClient {
		pb.renderErr(c.Writer, result.relayErr)
	}

	// 7. 结算 + 落日志
	s.settleAndLog(c, pb, req, user, key, result, reserved, started)
}

// reservePrice 取候选渠道类型集合的价目上界（各分项取最大值）。
// 返回 nil 表示候选中无任何可用价目。
func (s *Service) reservePrice(ctx context.Context, modelName string, channels []*provider.Channel) (*model.ModelPrice, error) {
	seen := map[string]bool{}
	var prices []*model.ModelPrice
	for _, ch := range channels {
		if seen[ch.Type] {
			continue
		}
		seen[ch.Type] = true
		p, err := s.pricer.GetPrice(ctx, modelName, ch.Type)
		if err != nil {
			return nil, err
		}
		if p != nil {
			prices = append(prices, p)
		}
	}
	return billing.MergePricesUpper(prices), nil
}

// forwardResult 一次网关转发的结果汇总。
type forwardResult struct {
	channel       *provider.Channel
	meter         *usage.Meter
	resp          *protocol.Response // 非流式
	relayErr      *protocol.RelayError
	statusCode    int
	retries       int
	wroteToClient bool
}

func (s *Service) forward(c *gin.Context, pb protoBinding, req *protocol.Request, channels []*provider.Channel, result *forwardResult) {
	maxAttempts := s.cfg.Relay.MaxRetries + 1
	if maxAttempts > len(channels) {
		maxAttempts = len(channels)
	}

	var lastErr *protocol.RelayError
	for i := 0; i < maxAttempts; i++ {
		ch := channels[i]
		adapter, ok := s.reg.Get(ch.Type)
		if !ok {
			continue
		}
		result.retries = i

		rerr := s.attempt(c, pb, req, ch, adapter, result)
		if rerr == nil {
			result.channel = ch
			return
		}
		lastErr = rerr
		if rerr.Code == protocol.ErrCodeChannelBusy {
			// 排队超时：终止性 429，不跨渠道重试（各账户并发隔离，行为可预期）
			result.relayErr = rerr
			return
		}
		if !retryable(rerr) || result.wroteToClient {
			result.relayErr = rerr
			return
		}
		s.log.Info("channel failed, trying next",
			zap.Int64("provider_id", ch.ID), zap.String("provider", ch.Name),
			zap.Int("attempt", i+1), zap.String("error", rerr.Message))
	}
	result.relayErr = lastErr
}

// attempt 单渠道转发。
func (s *Service) attempt(
	c *gin.Context, pb protoBinding, req *protocol.Request,
	ch *provider.Channel, adapter provider.Adapter, result *forwardResult,
) *protocol.RelayError {
	// 渠道并发槽位：满载时排队等待（ChannelQueueTimeout 上限），超时返回终止性 429；
	// 流式请求持有槽位直至流结束
	qStart := time.Now()
	release, busyErr := s.limiter.Acquire(c.Request.Context(), ch, s.queueTimeout())
	if busyErr != nil {
		return busyErr
	}
	defer release()
	if waited := time.Since(qStart); waited > 100*time.Millisecond {
		s.log.Info("queued at channel",
			zap.Int64("provider_id", ch.ID), zap.String("provider", ch.Name), zap.Duration("waited", waited))
	}

	// 超时语义拆分：
	//   - 非流式：整体超时约束“建连 + 上游处理 + 响应体读取”；
	//   - 流式：上游首包（响应头）由 Transport 层 ResponseHeaderTimeout 约束，
	//     流建立后由 IdleStreamTimeout 空闲看门狗接管（见 relayStream），
	//     避免长流式响应被整体超时腰斩。
	var ctx context.Context
	var cancel context.CancelFunc
	if req.Stream {
		ctx, cancel = context.WithCancel(c.Request.Context())
	} else {
		ctx, cancel = context.WithTimeout(c.Request.Context(), s.upstreamTimeout())
	}
	defer cancel()

	httpReq, err := adapter.BuildRequest(ctx, req, ch, req.Stream)
	if err != nil {
		return protocol.ErrInvalidRequest("构造上游请求失败: " + err.Error())
	}

	client := s.clientFor(ch.ProxyURL)
	resp, err := client.Do(httpReq)
	if err != nil {
		return protocol.ErrUpstream("上游连接失败: " + err.Error())
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		uerr := adapter.ParseUpstreamError(resp.StatusCode, body)
		uerr.Upstream = ch.Name
		return uerr
	}

	result.statusCode = resp.StatusCode

	if req.Stream {
		return s.relayStream(c, pb, req, adapter, resp, cancel, result)
	}
	return s.relayNonStream(c, pb, adapter, resp, req, result)
}

// relayStream 流式转发：上游 canonical 事件 -> 客户端协议 SSE。
//
// 生命周期约束：
//   - 模型名还原：message_start 强制回写客户端请求的模型别名；
//   - 空闲看门狗：两次上游事件间隔超过 IdleStreamTimeout 则取消上游连接
//     （客户端将收到协议错误事件，用量按已收到的部分结算）。
func (s *Service) relayStream(
	c *gin.Context, pb protoBinding, req *protocol.Request, adapter provider.Adapter,
	resp *http.Response, cancel context.CancelFunc, result *forwardResult,
) *protocol.RelayError {
	decoder := adapter.NewStreamDecoder(resp.Body)
	defer decoder.Close()

	enc := &modelRewriteEncoder{inner: pb.newStreamEnc(c.Writer), model: req.Model}
	meter := result.meter
	result.wroteToClient = true

	var watchdog *time.Timer
	if idle := s.cfg.Relay.IdleStreamTimeout; idle > 0 {
		watchdog = time.AfterFunc(idle, cancel)
		defer watchdog.Stop()
	}

	for {
		ev, err := decoder.Next()
		if watchdog != nil {
			watchdog.Reset(s.cfg.Relay.IdleStreamTimeout)
		}
		if errors.Is(err, io.EOF) {
			// 上游流正常结束但未发 done（防御）：补 done
			_ = enc.WriteEvent(&protocol.Event{Type: protocol.EventDone})
			return nil
		}
		if err != nil {
			if c.Request.Context().Err() != nil {
				// 客户端主动断开：按完成处理（部分用量已计量）
				return nil
			}
			_ = enc.WriteEvent(&protocol.Event{Type: protocol.EventError, Error: protocol.ErrUpstream("上游流中断")})
			return protocol.ErrUpstream("上游流中断: " + err.Error())
		}
		if ev.Type == protocol.EventError {
			_ = enc.WriteEvent(ev)
			return ev.Error
		}
		meter.Observe(ev)
		if err := enc.WriteEvent(ev); err != nil {
			// 客户端断开
			return nil
		}
		if ev.Type == protocol.EventDone {
			return nil
		}
	}
}

// relayNonStream 非流式转发。
func (s *Service) relayNonStream(
	c *gin.Context, pb protoBinding, adapter provider.Adapter,
	resp *http.Response, req *protocol.Request, result *forwardResult,
) *protocol.RelayError {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	resp.Body.Close()
	if err != nil {
		return protocol.ErrUpstream("读取上游响应失败")
	}
	cr, derr := adapter.DecodeNonStream(body, req.Model)
	if derr != nil {
		return derr
	}
	meter := result.meter
	meter.ObserveUsage(cr.Usage)
	meter.SetFirstToken(time.Now())
	result.resp = cr
	result.wroteToClient = true

	c.JSON(http.StatusOK, pb.encodeResp(cr))
	return nil
}

// settleAndLog 结算配额并异步落请求日志。
func (s *Service) settleAndLog(
	c *gin.Context, pb protoBinding, req *protocol.Request,
	user *model.User, key *model.APIKey,
	result *forwardResult, reserved int64, started time.Time,
) {
	var u protocol.Usage
	if result.meter != nil {
		u = result.meter.Usage()
	}
	providerType := ""
	if result.channel != nil {
		providerType = result.channel.Type
	}

	// 上游成功但未返回 usage 时按输入估算计费并标记
	if result.relayErr == nil && u.TotalInput() == 0 && u.OutputTokens == 0 {
		u.InputTokens = usage.EstimateRequestTokens(req)
		u.Estimated = true
	}

	var cost billing.Cost
	if price, err := s.pricer.GetPrice(c.Request.Context(), req.Model, providerType); err == nil {
		cost = billing.Compute(u, price, req.Model, providerType)
	} else {
		cost = billing.Compute(u, nil, req.Model, providerType)
	}
	s.quota.Settle(context.Background(), key.ID, user.ID, reserved, cost.QuotaCost)

	status := result.statusCode
	if result.relayErr != nil {
		status = result.relayErr.HTTPStatus
	}
	if c.Request.Context().Err() != nil {
		status = 499 // client closed request
	}

	ttft, total := 0, int(time.Since(started).Milliseconds())
	if result.meter != nil {
		ttft = result.meter.TTFTMs()
	}

	errMsg := ""
	isErr := result.relayErr != nil
	if isErr {
		errMsg = result.relayErr.Message
	}

	logRec := model.RequestLog{
		CreatedAt:        started.UTC(),
		RequestID:        requestID(c),
		UserID:           user.ID,
		Username:         user.Username,
		APIKeyID:         key.ID,
		ProviderID:       0,
		ProviderName:     "",
		UpstreamType:     providerType,
		Protocol:         pb.id,
		Model:            req.Model,
		Stream:           req.Stream,
		StatusCode:       status,
		IsError:          isErr,
		ErrMsg:           errMsg,
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		ReasoningTokens:  u.ReasoningTokens,
		CacheReadTokens:  u.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens,
		TTFTMs:           ttft,
		TotalMs:          total,
		Retries:          result.retries,
		QuotaCost:        cost.QuotaCost,
		BillingDetail:    cost.Detail,
		ClientIP:         c.ClientIP(),
	}
	if result.channel != nil {
		logRec.ProviderID = result.channel.ID
		logRec.ProviderName = result.channel.Name
		logRec.UpstreamModel = result.channel.UpstreamModel(req.Model)
	}
	s.writer.Enqueue(logRec)

	if isErr {
		s.log.Warn("relay finished with error",
			zap.String("request_id", requestID(c)), zap.String("model", req.Model),
			zap.String("provider", logRec.ProviderName), zap.Int("status", status), zap.String("error", errMsg))
	} else {
		s.log.Info("relay finished",
			zap.String("request_id", requestID(c)), zap.String("model", req.Model),
			zap.String("provider", logRec.ProviderName), zap.Bool("stream", req.Stream),
			zap.Int("input_tokens", u.TotalInput()), zap.Int("output_tokens", u.OutputTokens),
			zap.Int("ttft_ms", ttft), zap.Int64("cost_usd_micro", cost.QuotaCost))
	}
}

// ---- 辅助 ----

func (s *Service) authenticate(c *gin.Context, render protocol.ErrorRenderer) (*model.APIKey, *model.User, *protocol.RelayError) {
	raw := bearer(c.GetHeader("Authorization"))
	if raw == "" {
		rerr := protocol.ErrAuth("缺少 API Key")
		render(c.Writer, rerr)
		return nil, nil, rerr
	}
	key, user, err := s.authenticateRaw(c, raw)
	if err != nil {
		render(c.Writer, err)
		return nil, nil, err
	}
	return key, user, nil
}

func (s *Service) authenticateRaw(c *gin.Context, raw string) (*model.APIKey, *model.User, *protocol.RelayError) {
	key, user, err := s.auth.Authenticate(c.Request.Context(), raw)
	switch {
	case err == nil:
		return key, user, nil
	case errors.Is(err, auth.ErrInvalidKey):
		return nil, nil, protocol.ErrAuth("无效的 API Key")
	case errors.Is(err, auth.ErrDisabled):
		return nil, nil, protocol.ErrPermission("API Key 或账户已被禁用")
	case errors.Is(err, auth.ErrExpired):
		return nil, nil, protocol.ErrAuth("API Key 已过期")
	default:
		s.log.Error("authenticate failed", zap.Error(err))
		return nil, nil, protocol.ErrUpstream("认证失败")
	}
}

// clientFor 直连或按代理地址缓存的 http client。
func (s *Service) clientFor(proxyURL string) *http.Client {
	if proxyURL == "" {
		return s.client
	}
	s.proxyMu.Lock()
	defer s.proxyMu.Unlock()
	if c, ok := s.proxyClients[proxyURL]; ok {
		return c
	}
	pu, err := url.Parse(proxyURL)
	if err != nil {
		return s.client
	}
	c := &http.Client{Transport: newTransport(s.cfg.Relay, pu)}
	s.proxyClients[proxyURL] = c
	return c
}

// upstreamTimeout 非流式请求整体超时（配置缺失时的兑底默认值）。
func (s *Service) upstreamTimeout() time.Duration {
	if s.cfg.Relay.UpstreamTimeout > 0 {
		return s.cfg.Relay.UpstreamTimeout
	}
	return 10 * time.Minute
}

// queueTimeout 渠道并发满时排队等待上限（配置缺失时的兑底默认值）。
func (s *Service) queueTimeout() time.Duration {
	if s.cfg.Relay.ChannelQueueTimeout > 0 {
		return s.cfg.Relay.ChannelQueueTimeout
	}
	return 60 * time.Second
}

// sessionKey 会话键推导（粘性路由）：
// ① X-Session-ID 请求头；② metadata.session_id；
// ③ 兑底 hash(keyID|model|首条 user 消息文本)——多轮对话首条消息稳定，
// 天然按会话聚合；混入 keyID 避免不同用户同文案互串。
func (s *Service) sessionKey(c *gin.Context, req *protocol.Request, key *model.APIKey) string {
	if v := strings.TrimSpace(c.GetHeader("X-Session-ID")); v != "" {
		return "hdr:" + v
	}
	if v, _ := req.Metadata["session_id"].(string); v != "" {
		return "meta:" + v
	}
	sum := sha256.Sum256(fmt.Appendf(nil, "%d|%s|%s", key.ID, req.Model, firstUserText(req)))
	return "auto:" + hex.EncodeToString(sum[:])
}

// firstUserText 首条 user 消息文本（粘性会话键兑底输入）。
func firstUserText(req *protocol.Request) string {
	for _, m := range req.Messages {
		if m.Role != "user" {
			continue
		}
		if m.Text != "" {
			return m.Text
		}
		for _, b := range m.Blocks {
			if b.Type == protocol.BlockTypeText && b.Text != "" {
				return b.Text
			}
		}
	}
	return ""
}

// indexOfChannelID 查找渠道在候选列表中的下标（-1 不存在）。
func indexOfChannelID(chs []*provider.Channel, id int64) int {
	for i, c := range chs {
		if c.ID == id {
			return i
		}
	}
	return -1
}

func retryable(e *protocol.RelayError) bool {
	switch e.HTTPStatus {
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	}
	return e.HTTPStatus >= 500
}

func bearer(header string) string {
	if len(header) > 7 && (header[:7] == "Bearer " || header[:7] == "bearer ") {
		return header[7:]
	}
	return header
}

func requestID(c *gin.Context) string {
	if v, ok := c.Get("request_id"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// newTransport 构造 HTTP 传输：
//   - 建连超时取 RelayConfig.ConnectTimeout；
//   - ResponseHeaderTimeout 取 RelayConfig.UpstreamTimeout，作为流式/非流式共同的
//     “上游首包（响应头）”上限；流式在响应头返回后由空闲看门狗接管（见 relayStream）。
func newTransport(rc config.RelayConfig, proxyURL *url.URL) *http.Transport {
	connectTimeout := rc.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = 10 * time.Second
	}
	ttfbTimeout := rc.UpstreamTimeout
	if ttfbTimeout <= 0 {
		ttfbTimeout = 10 * time.Minute
	}
	tr := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: connectTimeout}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: ttfbTimeout,
	}
	if proxyURL != nil {
		tr.Proxy = http.ProxyURL(proxyURL)
	}
	return tr
}

// Module fx 模块。
var Module = fx.Module("relay", fx.Provide(NewService))
