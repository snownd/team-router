package relay_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/auth"
	"github.com/snownd/team-router/internal/billing"
	"github.com/snownd/team-router/internal/config"
	"github.com/snownd/team-router/internal/crypto"
	"github.com/snownd/team-router/internal/database/dbtest"
	"github.com/snownd/team-router/internal/model"
	"github.com/snownd/team-router/internal/provider"
	"github.com/snownd/team-router/internal/relay"
	"github.com/snownd/team-router/internal/repository"
	"github.com/snownd/team-router/internal/usage"
)

func setupStack(t *testing.T) (
	svc *relay.Service,
	cfg *config.Config,
	db *gorm.DB,
	userRepo *repository.UserRepo,
	keyRepo *repository.APIKeyRepo,
	logRepo *repository.RequestLogRepo,
) {
	return setupStackWith(t, nil)
}

// setupStackWith setupStack 的可配置变体（mutate 在服务构建前修改 cfg）。
func setupStackWith(t *testing.T, mutate func(*config.Config)) (
	svc *relay.Service,
	cfg *config.Config,
	db *gorm.DB,
	userRepo *repository.UserRepo,
	keyRepo *repository.APIKeyRepo,
	logRepo *repository.RequestLogRepo,
) {
	t.Helper()
	db = dbtest.Prepare(t)

	userRepo = repository.NewUserRepo(db)
	keyRepo = repository.NewAPIKeyRepo(db)
	providerRepo := repository.NewProviderRepo(db)
	priceRepo := repository.NewPriceRepo(db)
	logRepo = repository.NewRequestLogRepo(db)
	deptRepo := repository.NewDepartmentRepo(db)

	// fx 同款构建
	enc, _ := crypto.New("")
	reg := provider.NewRegistry([]provider.Adapter{
		provider.NewOpenAIAdapter(),
		provider.NewOpenAIResponsesAdapter(),
		provider.NewAnthropicAdapter(),
	})
	router := provider.NewRouter(providerRepo, enc, reg, zap.NewNop())
	pricer := billing.NewPricer(priceRepo, config.Default(), zap.NewNop())
	quota := billing.NewQuota(repository.NewQuotaRepo(db), zap.NewNop())
	writer := usage.NewAsyncLogWriter(logRepo, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	writer.Start(ctx)
	t.Cleanup(func() { cancel(); writer.Wait() })

	cfg = config.Default()
	cfg.Relay.UpstreamTimeout = 30 * time.Second
	if mutate != nil {
		mutate(cfg)
	}
	svc = relay.NewService(
		auth.NewAuthenticator(keyRepo, userRepo, cfg),
		router, reg, provider.NewLimiter(), pricer, quota, writer, cfg, zap.NewNop(),
	)
	_ = deptRepo
	return svc, cfg, db, userRepo, keyRepo, logRepo
}

// TestRelayMessagesViaOpenAIUpstream：客户端用 Anthropic Messages 协议，
// 上游渠道为 OpenAI 兼容 SSE —— 验证 any-to-any 转换、计费与请求日志落库。
func TestRelayMessagesViaOpenAIUpstream(t *testing.T) {
	svc, _, db, userRepo, keyRepo, logRepo := setupStack(t)
	ctx := context.Background()

	// mock 上游：OpenAI Chat SSE
	var gotAuth, gotPath string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		time.Sleep(30 * time.Millisecond) // 模拟上游首 token 延迟
		chunks := []string{
			`data: {"id":"cmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"你好"}}]}`,
			`data: {"choices":[{"index":0,"delta":{"content":"！"}}]}`,
			`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`data: {"usage":{"prompt_tokens":120,"prompt_tokens_details":{"cached_tokens":50},"completion_tokens":8,"total_tokens":128}}`,
			`data: [DONE]`,
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "%s\n\n", c)
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	// 渠道：openai 兼容端点
	enc, _ := crypto.New("")
	encKey, _ := enc.Encrypt("sk-upstream-key")
	p := model.Provider{
		Name: "mock-openai", Type: "openai", BaseURL: upstream.URL, APIKeyEnc: encKey,
		Models: json.RawMessage(`["gpt-test"]`), Priority: 10, Weight: 1, Status: 1,
	}
	providerRepo := repository.NewProviderRepo(db)
	if err := providerRepo.Create(ctx, &p); err != nil {
		t.Fatal(err)
	}
	// 价格：输入 $3/M、输出 $15/M、缓存读 $0.3/M
	priceRepo := repository.NewPriceRepo(db)
	if err := priceRepo.Upsert(ctx, &model.ModelPrice{
		ModelName: "gpt-test", InputPerM: 3_000_000, OutputPerM: 15_000_000,
		CacheReadPerM: 300_000, Currency: "USD", Status: 1,
	}); err != nil {
		t.Fatal(err)
	}

	// 用户 + API Key
	u := model.User{Username: "e2e-user", Role: model.RoleUser, Status: 1, Quota: 10_000_000}
	if err := userRepo.Create(ctx, &u); err != nil {
		t.Fatal(err)
	}
	rawKey, _, hash := auth.GenerateKey()
	k := model.APIKey{UserID: u.ID, Name: "e2e", KeyPrefix: "sk-e2e", KeyHash: hash, Status: 1}
	if err := keyRepo.Create(ctx, &k); err != nil {
		t.Fatal(err)
	}

	// 客户端请求：Anthropic Messages 协议
	body := `{"model":"gpt-test","max_tokens":100,"stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	svc.HandleMessages(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	out := w.Body.String()
	for _, want := range []string{
		"event: message_start",
		"event: content_block_delta",
		`"text_delta"`,
		"你好",
		"event: message_delta",
		`"stop_reason":"end_turn"`,
		"event: message_stop",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("客户端 SSE 缺少 %q:\n%s", want, out)
		}
	}
	// usage：openai prompt 120（含 50 缓存）-> canonical input 70 + cache_read 50
	if !strings.Contains(out, `"input_tokens":70`) {
		t.Errorf("message_delta usage 应含 input_tokens=70:\n%s", out)
	}
	if !strings.Contains(out, `"cache_read_input_tokens":50`) {
		t.Errorf("应含 cache_read_input_tokens=50:\n%s", out)
	}

	// 上游收到的请求验证
	if gotPath != "/v1/chat/completions" {
		t.Errorf("上游路径错误: %s", gotPath)
	}
	if gotAuth != "Bearer sk-upstream-key" {
		t.Errorf("上游认证头错误: %s", gotAuth)
	}
	if gotBody["model"] != "gpt-test" {
		t.Errorf("上游模型错误: %v", gotBody["model"])
	}
	if so, ok := gotBody["stream_options"].(map[string]any); !ok || so["include_usage"] != true {
		t.Errorf("应注入 stream_options.include_usage: %v", gotBody["stream_options"])
	}

	// 等待异步日志落库
	var logRow model.RequestLog
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		logs, total, err := logRepo.Query(ctx, repository.LogFilter{
			Start: time.Now().Add(-time.Minute), End: time.Now().Add(time.Minute), Limit: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
		if total > 0 {
			logRow = logs[0]
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if logRow.ID == 0 {
		t.Fatal("请求日志未落库")
	}
	// 字段校验
	if logRow.Protocol != model.ProtocolMessages {
		t.Errorf("protocol 错误: %s", logRow.Protocol)
	}
	if logRow.UpstreamType != "openai" || logRow.ProviderName != "mock-openai" {
		t.Errorf("provider 字段错误: %s / %s", logRow.UpstreamType, logRow.ProviderName)
	}
	if logRow.InputTokens != 70 || logRow.CacheReadTokens != 50 || logRow.OutputTokens != 8 {
		t.Errorf("token 记录错误: %+v", logRow)
	}
	if logRow.TTFTMs <= 0 {
		t.Errorf("TTFT 应记录: %d", logRow.TTFTMs)
	}
	if logRow.IsError {
		t.Errorf("不应为错误请求")
	}
	// 计费：70*3M/1M + 8*15M/1M + 50*0.3M/1M = 210+120+15 = 345 微美元
	if logRow.QuotaCost != 345 {
		t.Errorf("quota_cost = %d, want 345", logRow.QuotaCost)
	}
	var detail map[string]any
	_ = json.Unmarshal(logRow.BillingDetail, &detail)
	if detail["priced"] != true {
		t.Errorf("billing_detail.priced 错误: %v", detail["priced"])
	}

	// 用户已用额度应等于实际费用
	u2, _ := userRepo.GetByID(ctx, u.ID)
	if u2.UsedQuota != 345 {
		t.Errorf("user used_quota = %d, want 345（预扣后多退少补）", u2.UsedQuota)
	}
}

func TestRelayChatNonStreamErrorMapping(t *testing.T) {
	svc, _, db, userRepo, keyRepo, _ := setupStack(t)
	ctx := context.Background()

	// 上游 401
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key","type":"invalid_request_error","code":"invalid_api_key"}}`))
	}))
	defer upstream.Close()

	enc, _ := crypto.New("")
	encKey, _ := enc.Encrypt("k")
	p := model.Provider{
		Name: "err-openai", Type: "openai", BaseURL: upstream.URL, APIKeyEnc: encKey,
		Models: json.RawMessage(`["gpt-err"]`), Priority: 10, Weight: 1, Status: 1,
	}
	if err := repository.NewProviderRepo(db).Create(ctx, &p); err != nil {
		t.Fatal(err)
	}

	u := model.User{Username: "e2e-err", Role: model.RoleUser, Status: 1, Quota: 10_000_000}
	_ = userRepo.Create(ctx, &u)
	rawKey, _, hash := auth.GenerateKey()
	_ = keyRepo.Create(ctx, &model.APIKey{UserID: u.ID, KeyPrefix: "sk-e", KeyHash: hash, Status: 1})

	body := `{"model":"gpt-err","messages":[{"role":"user","content":"x"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	svc.HandleChatCompletions(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "bad key") {
		t.Errorf("错误消息应透传: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"type":"invalid_request_error"`) {
		t.Errorf("OpenAI 错误格式错误: %s", w.Body.String())
	}
}

func TestRelayAuthErrors(t *testing.T) {
	svc, _, _, _, _, _ := setupStack(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"x"}]}`))
	req.Header.Set("Authorization", "Bearer sk-wrong")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	svc.HandleChatCompletions(c)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("无效 key 应 401: %d", w.Code)
	}

	// 缺少 key
	req2 := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"m","messages":[{"role":"user","content":"x"}]}`))
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = req2
	svc.HandleMessages(c2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("缺少 key 应 401: %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), `"type":"error"`) {
		t.Errorf("Messages 协议错误格式: %s", w2.Body.String())
	}
}

// TestRelayStreamModelRewrite：渠道配置模型映射（别名 -> 物理名）时，
// 上游 SSE 回显物理模型名，客户端流式响应中的 model 必须还原为请求别名（P0-1）。
func TestRelayStreamModelRewrite(t *testing.T) {
	svc, _, db, userRepo, keyRepo, _ := setupStack(t)
	ctx := context.Background()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "gpt-physical" {
			t.Errorf("上游应收到物理模型名: %v", body["model"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		chunks := []string{
			`data: {"id":"cmpl-1","model":"gpt-physical","choices":[{"index":0,"delta":{"role":"assistant","content":"hi"}}]}`,
			`data: {"model":"gpt-physical","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "%s\n\n", c)
			f.Flush()
		}
	}))
	defer upstream.Close()

	enc, _ := crypto.New("")
	encKey, _ := enc.Encrypt("sk-up")
	p := model.Provider{
		Name: "map-openai", Type: "openai", BaseURL: upstream.URL, APIKeyEnc: encKey,
		Models:       json.RawMessage(`["gpt-physical"]`),
		ModelMapping: json.RawMessage(`{"gpt-alias":"gpt-physical"}`),
		Priority:     10, Weight: 1, Status: 1,
	}
	if err := repository.NewProviderRepo(db).Create(ctx, &p); err != nil {
		t.Fatal(err)
	}
	if err := repository.NewPriceRepo(db).Upsert(ctx, &model.ModelPrice{
		ModelName: "gpt-alias", InputPerM: 1_000_000, OutputPerM: 1_000_000, Currency: "USD", Status: 1,
	}); err != nil {
		t.Fatal(err)
	}

	u := model.User{Username: "map-user", Role: model.RoleUser, Status: 1, Quota: 1_000_000}
	if err := userRepo.Create(ctx, &u); err != nil {
		t.Fatal(err)
	}
	rawKey, _, hash := auth.GenerateKey()
	if err := keyRepo.Create(ctx, &model.APIKey{UserID: u.ID, KeyPrefix: "sk-m", KeyHash: hash, Status: 1}); err != nil {
		t.Fatal(err)
	}

	body := `{"model":"gpt-alias","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	svc.HandleChatCompletions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	out := w.Body.String()
	if !strings.Contains(out, `"model":"gpt-alias"`) {
		t.Errorf("流式响应应还原为请求别名模型名:\n%s", out)
	}
	if strings.Contains(out, "gpt-physical") {
		t.Errorf("上游物理模型名不应漂移到客户端:\n%s", out)
	}
}

// ---- openai_responses 上游 / 粘性路由 / 并发排队 e2e ----

// newUpstreamFixtureUser 创建测试用户与 API Key，返回原始 key。
func newUpstreamFixtureUser(t *testing.T, userRepo *repository.UserRepo, keyRepo *repository.APIKeyRepo, name string) string {
	t.Helper()
	ctx := context.Background()
	u := model.User{Username: name, Role: model.RoleUser, Status: 1, Quota: 10_000_000}
	if err := userRepo.Create(ctx, &u); err != nil {
		t.Fatal(err)
	}
	rawKey, _, hash := auth.GenerateKey()
	k := model.APIKey{UserID: u.ID, Name: name, KeyPrefix: "sk-t", KeyHash: hash, Status: 1}
	if err := keyRepo.Create(ctx, &k); err != nil {
		t.Fatal(err)
	}
	return rawKey
}

// TestRelayChatViaResponsesUpstream：Chat 协议客户端 -> openai_responses 上游（SSE），
// 验证 /v1/responses 路径、鉴权头、流式协议转换、usage（cached 换算）与计费。
func TestRelayChatViaResponsesUpstream(t *testing.T) {
	svc, _, db, userRepo, keyRepo, logRepo := setupStack(t)
	ctx := context.Background()

	var gotPath, gotAuth string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		events := []string{
			`event: response.created` + "\n" + `data: {"response":{"id":"resp_1","model":"gpt-5"}}`,
			`event: response.output_item.added` + "\n" + `data: {"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant"}}`,
			`event: response.output_text.delta` + "\n" + `data: {"item_id":"msg_1","delta":"你"}`,
			`event: response.output_text.delta` + "\n" + `data: {"item_id":"msg_1","delta":"好"}`,
			`event: response.output_item.done` + "\n" + `data: {"output_index":0,"item":{"type":"message","id":"msg_1"}}`,
			`event: response.completed` + "\n" + `data: {"response":{"id":"resp_1","usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":30},"output_tokens":5,"total_tokens":105}}}`,
		}
		for _, e := range events {
			fmt.Fprintf(w, "%s\n\n", e)
			f.Flush()
		}
	}))
	defer upstream.Close()

	enc, _ := crypto.New("")
	encKey, _ := enc.Encrypt("sk-responses-key")
	p := model.Provider{
		Name: "mock-responses", Type: "openai_responses", BaseURL: upstream.URL, APIKeyEnc: encKey,
		Models: json.RawMessage(`["gpt-test"]`), Priority: 10, Weight: 1, Status: 1,
	}
	if err := repository.NewProviderRepo(db).Create(ctx, &p); err != nil {
		t.Fatal(err)
	}
	if err := repository.NewPriceRepo(db).Upsert(ctx, &model.ModelPrice{
		ModelName: "gpt-test", InputPerM: 3_000_000, OutputPerM: 15_000_000,
		CacheReadPerM: 300_000, Currency: "USD", Status: 1,
	}); err != nil {
		t.Fatal(err)
	}
	rawKey := newUpstreamFixtureUser(t, userRepo, keyRepo, "responses-user")

	body := `{"model":"gpt-test","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+rawKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	svc.HandleChatCompletions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if gotPath != "/v1/responses" {
		t.Errorf("上游路径错误: %s", gotPath)
	}
	if gotAuth != "Bearer sk-responses-key" {
		t.Errorf("上游认证头错误: %s", gotAuth)
	}
	if gotBody["model"] != "gpt-test" {
		t.Errorf("上游模型错误: %v", gotBody["model"])
	}
	if s, _ := gotBody["store"].(bool); s {
		t.Error("上游请求必须 store=false")
	}
	out := w.Body.String()
	for _, want := range []string{`"role":"assistant"`, "你", "好", `"finish_reason":"stop"`} {
		if !strings.Contains(out, want) {
			t.Errorf("客户端 SSE 缺少 %q:\n%s", want, out)
		}
	}

	// 日志与计费：input 新鲜 70（100-30 缓存）+ cache_read 30 + output 5
	var logRow model.RequestLog
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		logs, total, err := logRepo.Query(ctx, repository.LogFilter{
			Start: time.Now().Add(-time.Minute), End: time.Now().Add(time.Minute), Limit: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
		if total > 0 {
			logRow = logs[0]
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if logRow.ID == 0 {
		t.Fatal("请求日志未落库")
	}
	if logRow.UpstreamType != "openai_responses" || logRow.ProviderName != "mock-responses" {
		t.Errorf("provider 字段错误: %s / %s", logRow.UpstreamType, logRow.ProviderName)
	}
	if logRow.InputTokens != 70 || logRow.CacheReadTokens != 30 || logRow.OutputTokens != 5 {
		t.Errorf("token 记录错误: %+v", logRow)
	}
	// 70*3 + 5*15 + 30*0.3 = 210+75+9 = 294 微美元
	if logRow.QuotaCost != 294 {
		t.Errorf("quota_cost = %d, want 294", logRow.QuotaCost)
	}
}

// TestRelayStickyRouting：同 X-Session-ID 的多轮请求命中相同上游账户。
func TestRelayStickyRouting(t *testing.T) {
	svc, _, db, userRepo, keyRepo, _ := setupStack(t)
	ctx := context.Background()

	var hits [2]atomic.Int64
	newUpstream := func(i int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits[i].Add(1)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"cmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11}}`)
		}))
	}
	upA, upB := newUpstream(0), newUpstream(1)
	defer upA.Close()
	defer upB.Close()

	enc, _ := crypto.New("")
	for i, up := range []*httptest.Server{upA, upB} {
		encKey, _ := enc.Encrypt("sk-up")
		p := model.Provider{
			Name: fmt.Sprintf("sticky-%d", i), Type: "openai", BaseURL: up.URL, APIKeyEnc: encKey,
			Models: json.RawMessage(`["gpt-sticky"]`), Priority: 10, Weight: 1, Status: 1,
		}
		if err := repository.NewProviderRepo(db).Create(ctx, &p); err != nil {
			t.Fatal(err)
		}
	}
	rawKey := newUpstreamFixtureUser(t, userRepo, keyRepo, "sticky-user")

	doRequest := func(sessionID string) int {
		body := `{"model":"gpt-sticky","messages":[{"role":"user","content":"多轮对话"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+rawKey)
		if sessionID != "" {
			req.Header.Set("X-Session-ID", sessionID)
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		svc.HandleChatCompletions(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
		}
		return w.Code
	}

	// 同一会话两次请求（多轮）：必须命中同一上游
	doRequest("conv-1")
	doRequest("conv-1")
	a, b := hits[0].Load(), hits[1].Load()
	if a+b != 2 {
		t.Fatalf("上游命中总数 = %d, want 2", a+b)
	}
	if a != 2 && b != 2 {
		t.Errorf("同一会话应粘住同一上游: hitsA=%d hitsB=%d", a, b)
	}
}

// TestRelayChannelBusy429：渠道并发满时排队等待，超时直接向客户端返回 429。
func TestRelayChannelBusy429(t *testing.T) {
	svc, _, db, userRepo, keyRepo, _ := setupStackWith(t, func(cfg *config.Config) {
		cfg.Relay.ChannelQueueTimeout = 300 * time.Millisecond
	})
	ctx := context.Background()

	block := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block // 第一个请求阻塞占用并发槽位
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"cmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11}}`)
	}))
	defer upstream.Close()

	enc, _ := crypto.New("")
	encKey, _ := enc.Encrypt("sk-up")
	p := model.Provider{
		Name: "busy-openai", Type: "openai", BaseURL: upstream.URL, APIKeyEnc: encKey,
		Models: json.RawMessage(`["gpt-busy"]`), Priority: 10, Weight: 1, Status: 1,
		MaxConcurrency: 1,
	}
	if err := repository.NewProviderRepo(db).Create(ctx, &p); err != nil {
		t.Fatal(err)
	}
	rawKey := newUpstreamFixtureUser(t, userRepo, keyRepo, "busy-user")

	newReq := func() (*httptest.ResponseRecorder, *gin.Context) {
		body := `{"model":"gpt-busy","messages":[{"role":"user","content":"x"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+rawKey)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		return w, c
	}

	// 第一个请求：占住唯一槽位（上游阻塞）
	w1, c1 := newReq()
	go func() { svc.HandleChatCompletions(c1) }()
	time.Sleep(200 * time.Millisecond) // 等待第一个请求完成路由并进入上游

	// 第二个请求：排队 300ms 超时 -> 终止性 429（不换渠道）
	w2, c2 := newReq()
	start := time.Now()
	svc.HandleChatCompletions(c2)
	waited := time.Since(start)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s", w2.Code, w2.Body.String())
	}
	if !strings.Contains(w2.Body.String(), "channel_busy") {
		t.Errorf("429 应携带 channel_busy 错误码: %s", w2.Body.String())
	}
	if waited < 250*time.Millisecond {
		t.Errorf("应在队列中等待约 300ms 才超时, 实际 %v", waited)
	}

	// 释放第一个请求，验证其正常完成
	close(block)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && w1.Code == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	if w1.Code != http.StatusOK {
		t.Errorf("第一个请求应正常完成: %d %s", w1.Code, w1.Body.String())
	}
}
