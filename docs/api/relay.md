# LLM 中继 API（`/v1/*`）

OpenAI / Anthropic 兼容入口，三种协议任意互转（any-to-any）：入站协议与上游渠道协议解耦，
网关内部统一转换为 canonical 中间格式后再适配到具体上游。

## 认证

```text
Authorization: Bearer sk-xxxxxxxx
```

缺少、无效、过期或被禁用的 Key 分别返回 401 / 403，见 [错误模型](#错误模型)。

## 端点一览

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/v1/chat/completions` | OpenAI Chat Completions（流式/非流式） |
| POST | `/v1/responses` | OpenAI Responses API（无状态子集） |
| POST | `/v1/messages` | Anthropic Messages（含 prompt cache 透传） |
| GET | `/v1/models` | 网关可服务模型列表（OpenAI 格式） |

## 处理管线

每个中继请求按以下顺序处理，任一步失败即按入站协议渲染错误并终止：

1. **API Key 认证**（sha256 查找 + TTL 缓存，key 与所属用户均须启用、未过期）；
2. **协议解析**：wire → canonical，非法 JSON / 缺字段返回 400；
3. **模型白名单**：Key 的 `allowed_models` 非空时校验请求模型；
4. **渠道路由**：按启用渠道的 `models` / `model_mapping` 匹配，priority 优先、weight 加权随机排序出候选队列；
5. **计价与配额预扣**：按候选渠道类型的价目上界预扣（最贵价目），Key 级与用户级额度都须足够，不足返回 429；
6. **转发**：跨渠道重试（见下）；
7. **结算与日志**：按实际命中渠道价目精确计费，多退少补，异步写入 `request_logs`。

### 转发与重试语义

- 请求体上限 **32 MB**；
- 重试条件：上游 408 / 429 / 5xx，且**尚未向客户端写出任何字节**；最多尝试 `max_retries + 1` 个渠道（不超过候选数）；
- 非流式：整体受 `relay.upstream_timeout`（默认 10m）约束；
- 流式：上游首包（响应头）受 `upstream_timeout` 约束，流建立后由 `relay.idle_stream_timeout`（默认 2m）空闲看门狗接管——两次上游事件间隔超时则中断，客户端收到协议错误事件，用量按已收到部分结算；
- 客户端主动断开：按完成处理（状态记 499），已产生的用量照常结算；
- 上游未返回 usage 时按输入估算计费，日志标记 `estimated`；
- 流式响应中 `message_start` 的模型名会强制还原为客户端请求的别名（消除渠道 `model_mapping` 导致的上游物理模型名漂移）。

### 计费语义

- 价格表按 `(model_name, provider_type)` 精确匹配，`provider_type=''` 为通配价；
- 四项单价：输入 / 输出 / 缓存读 / 缓存写（每百万 token，微美元整数）；
- `InputTokens` 语义为**新鲜输入**（不含缓存命中）：OpenAI 的 `prompt_tokens` 含 `cached_tokens`，由适配器换算；Anthropic 的 `input_tokens` 天然不含缓存读；
- 计费明细（单价、各分项费用、币种、是否估算）记录在日志 `billing_detail`（jsonb）；
- 模型未配置价格时：`billing.allow_unknown_pricing=true`（默认）按 0 费用放行并标记估算，否则 400 拒绝。

---

## POST /v1/chat/completions

OpenAI Chat Completions 兼容接口，请求即 OpenAI 格式：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `model` | 是 | 网关模型别名 |
| `messages` | 是 | 消息数组；`role=system/developer` 合并为 system；content 支持 string 或 parts 数组（含 `image_url`） |
| `tools` / `tool_choice` | 否 | function calling；`tool_choice` 支持 `auto/none/required` 或 `{"type":"function","function":{"name":...}}` |
| `temperature` / `top_p` / `stop` / `user` | 否 | 采样参数 |
| `max_tokens` / `max_completion_tokens` | 否 | 输出上限（影响配额预估） |
| `stream` | 否 | 默认 false；流式时强制要求上游返回 usage（`stream_options.include_usage`） |

非流式响应为标准 OpenAI `chat.completion`；流式为 `data: {chunk}\n\n` 序列，以 `data: [DONE]` 结尾，最后一个含 usage 的 chunk 携带 token 统计。

```bash
curl -s -X POST localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-..." -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}'
```

## POST /v1/responses

OpenAI Responses API 兼容接口，**仅支持无状态子集**：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `model` | 是 | 网关模型别名 |
| `input` | 是 | string 或消息数组 |
| `instructions` | 否 | 作为 system 注入 |
| `tools` / `tool_choice` | 否 | function 工具 |
| `temperature` / `top_p` / `max_output_tokens` | 否 | 采样参数 |
| `stream` / `store` / `metadata` | 否 | 流式 / 存储 / 元数据 |
| `previous_response_id` | — | **不支持**，传入返回 400（请在 `input` 携带完整上下文） |

上游统一走 Chat / Messages 协议转发，网关负责 Responses 编解码。流式事件为
`response.created` / `response.in_progress` / `response.output_item.added` /
`response.content_part.added` / 增量 delta / `response.completed` 系列（`event:` + `data:` 风格）。

## POST /v1/messages

Anthropic Messages 兼容接口，请求即 Anthropic 格式：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `model` | 是 | 网关模型别名 |
| `messages` | 是 | 消息数组；`tool_result` 块会拆为独立 tool 消息 |
| `max_tokens` | 建议 | Anthropic 必填；缺省时按 `8192` 转发 |
| `system` | 否 | string 或 blocks；blocks 的 `cache_control` 透传上游（prompt cache） |
| `temperature` / `top_p` / `stop_sequences` | 否 | 采样参数 |
| `tools` / `tool_choice` | 否 | 工具定义与选择 |
| `metadata.user_id` | 否 | 映射为请求用户标识 |
| `stream` | 否 | 默认 false |

消息 blocks 的 `cache_control`（`{"type":"ephemeral"}`）原样透传 Anthropic 上游；非 Anthropic 上游时忽略。
非流式响应为标准 Anthropic `message` 对象；流式为 `message_start` / `content_block_start` /
`content_block_delta` / `content_block_stop` / `message_delta` / `message_stop` 事件序列（`event:` + `data:` 风格）。

```bash
curl -s -X POST localhost:8080/v1/messages \
  -H "Authorization: Bearer sk-..." -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o","max_tokens":1024,"messages":[{"role":"user","content":"hello"}]}'
```

> 同一个 API Key 可用任意协议调用任意模型，网关负责协议互转。

## GET /v1/models

返回所有启用渠道可服务模型的并集（OpenAI 格式）：

```json
{
  "object": "list",
  "data": [
    {"id": "gpt-4o", "object": "model", "owned_by": "team-router"}
  ]
}
```

---

## 错误模型

统一内部错误 `RelayError`，按入站协议渲染请求体：

| HTTP | type | code | 触发场景 |
| --- | --- | --- | --- |
| 400 | `invalid_request_error` | — | 非法 JSON、缺 `model`/`messages`、模型未定价且禁止放行等 |
| 401 | `authentication_error` | `invalid_api_key` | 缺少 Key / Key 无效 / 已过期 |
| 403 | `permission_error` | — | Key 或用户被禁用、模型不在 Key 白名单 |
| 404 | `not_found_error` | `model_not_found` | 无可用渠道服务该模型 |
| 429 | `insufficient_quota` | `insufficient_quota` | 配额预扣失败（额度不足） |
| 502 | `api_error` | `upstream_error` | 上游连接失败 / 上游流中断 / 认证内部失败 |
| 503 | `overloaded_error` | — | 预留语义（上游过载） |

上游错误（非 2xx）原样透传上游的 status / type / message，并附上游渠道标识用于内部排查（不返回客户端）。

三种协议的错误体形态：

```json
{"error": {"message": "...", "type": "...", "code": "...", "param": ""}}
```

```json
{"type": "error", "error": {"type": "...", "message": "..."}}
```

- Chat / Responses 端点使用第一种（Responses 字段顺序为 `code/message/param/type`，内容一致）；
- Messages 端点使用第二种（Anthropic 风格）。

响应头始终携带 `X-Request-Id`，与 `request_logs.request_id` 一致，可用于对账与排查。
