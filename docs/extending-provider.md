# 接入新的上游 Provider

内置适配器：`openai`（标准 Chat Completions）、`openai_responses`（OpenAI Responses API，
强制无状态：store=false、完整上下文入 input、不支持 stop 序列）、`anthropic`（Messages）。
三者均支持渠道级 `api_key`（AES-GCM 加密落库）、模型白名单（`models` + `model_mapping`）、
最大并发（`max_concurrency`，满载排队，见 docs/architecture.md）与粘性路由。

以 Gemini 为例。provider 适配器只关心 **canonical ↔ 上游协议**，与客户端用什么协议无关。

## 1. 实现 Adapter 接口

```go
// internal/provider/gemini/adapter.go
package gemini

type Adapter struct{}

func (a *Adapter) Type() string { return "gemini" }

func (a *Adapter) BuildRequest(ctx context.Context, req *protocol.Request,
    ch *provider.Channel, stream bool) (*http.Request, error) {
    // canonical -> Gemini generateContent 请求体
    // 上游模型名：ch.UpstreamModel(req.Model)（含 model_mapping 重定向）
    // 鉴权头按 Gemini 规范设置（ch.APIKey 已解密）
}

func (a *Adapter) DecodeNonStream(body []byte, model string) (*protocol.Response, *protocol.RelayError) {
    // Gemini JSON -> canonical Response（注意 usage / stop reason 映射）
}

func (a *Adapter) NewStreamDecoder(r io.ReadCloser) protocol.StreamDecoder {
    // Gemini SSE -> canonical Event 流（实现 protocol.StreamDecoder 接口）
    // 事件模型：message_start / block_start(text|thinking|tool_use) /
    //          block_delta(text|thinking|input_json) / block_stop /
    //          message_delta(stop) / usage / done
}

func (a *Adapter) ParseUpstreamError(status int, body []byte) *protocol.RelayError {
    // 上游错误体 -> RelayError（HTTP 状态保持，408/429/5xx 触发跨渠道重试）
}

func (a *Adapter) BuildModelsRequest(ctx context.Context, ch *provider.Channel) (*http.Request, error) {
    // 构造 GET {base}/models 探测请求（按 Gemini 规范设置鉴权头），
    // 供管理台“拉取模型列表”使用；响应解析复用 provider.FetchUpstreamModels
    //（若 Gemini 的 models 响应不是 {"data":[{"id":...}]} 结构，需自行实现解析）
}
```

## 2. 关键映射约定

- **用量**：canonical `Usage.InputTokens` 必须是「新鲜输入」（不含缓存读）；Gemini 无缓存字段则 CacheRead/Write 填 0
- **停止原因**：canonical 取值 `stop / length / tool_calls / content_filter`
- **usage 事件语义**：增量（Add），每个分量至多出现一次

## 3. 注册到 fx

```go
// internal/provider/provider.go 的 Module 追加：
fx.Annotate(gemini.NewAdapter, fx.As(new(Adapter)), fx.ResultTags(`group:"adapters"`)),
```

`Registry` 通过 value group 收集全部适配器；管理端 `GET /api/admin/providers/types` 自动出现 `gemini`，即可建渠道：

```json
POST /api/admin/providers
{"name":"google","type":"gemini","base_url":"https://generativelanguage.googleapis.com",
 "api_key":"AIza...","models":["gemini-2.0-flash"]}
```

## 4. 测试建议

仿照 `internal/protocol/*_test.go`：用真实 SSE 报文做 golden 文件测试 decoder；any-to-any 用例参考 `TestAnyToAnyPipeline`。
