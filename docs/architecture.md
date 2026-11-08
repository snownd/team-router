# 架构说明

## 总体数据流

```
客户端（OpenAI / Anthropic 协议）
        │ wire 请求
        ▼
┌─ relay（internal/relay）────────────────────────────┐
│ 1. API Key 认证（sha256 + TTL 缓存）                  │
│ 2. 协议解析 wire → canonical（internal/protocol）     │
│ 3. 渠道路由（model → providers，priority/weight）     │
│ 4. 计价 + 配额预扣（internal/billing）                 │
│ 5. 适配器转发（internal/provider → 上游）              │
│ 6. 上游 SSE/JSON → canonical 事件/响应                │
│ 7. canonical → 客户端协议 SSE/JSON                    │
│ 8. 计量（TTFT/用量，internal/usage）                   │
│ 9. 结算 + 异步写 request_logs（分区表）                │
└──────────────────────────────────────────────────────┘
```

## Canonical 中间格式（any-to-any 的关键）

三种入站协议 × M 种上游 provider，若两两直转需要 3M 个转换器；
引入 canonical 后只需要 3 + M 个：

```
chat wire  ─┐                     ┌─ openai adapter ──→ OpenAI 兼容上游
responses ──┼─ protocol pkg ──▶ canonical ─┤
messages ──┘  （wire↔canonical）  └─ anthropic adapter → Anthropic 上游
```

- `Request`：三种协议请求字段的超集（system、消息、工具、采样参数、缓存控制…）
- `Event`：统一流事件模型（`message_start / block_start / block_delta / block_stop / message_delta / usage / done / error`）
- 每个协议包同时提供「解析器（入站）」「编码器（出站）」与「上游构建器/解码器」

## 用量语义

| 字段 | 语义 |
|---|---|
| `InputTokens` | 新鲜输入 token（**不含**缓存） |
| `CacheReadTokens` | 缓存命中 token |
| `CacheWriteTokens` | 缓存写入 token |
| `OutputTokens` | 输出 token |
| `ReasoningTokens` | 推理 token（输出的一部分） |

- OpenAI：`Input = prompt_tokens − cached_tokens`（适配器换算）
- Anthropic：`input_tokens` 本身不含缓存读，直接映射

## 计费与配额

1. **价格表** `model_prices`：`(model_name, provider_type)` 唯一；`provider_type=''` 为通配价。查价顺序：精确 → 通配，带 TTL 缓存。
2. **预扣**：`est = (输入估算 + max_tokens) × max(in价, out价)`，key 级 + 用户级两条原子 UPDATE（`quota=0` 表示不限额），用户失败回滚 key。
3. **结算**：实际费用 − 预扣额，差额原子累加（可为负）。
4. **明细**：`request_logs.billing_detail`（jsonb）包含单价、各分项费用、是否估算、币种。

## 请求日志（按月分区）

- `request_logs` 为 PG 声明式 RANGE 分区表，父表 PK `(id, created_at)`；PG17 之前分区表不支持 identity 列，故 `id` 使用独立 sequence。
- 父表上的索引自动应用到未来分区（PG11+）；另有 DEFAULT 分区兜底。
- `PartitionManager`（fx 生命周期）：启动时与每 12h 预建未来 3 个月分区（`request_logs_y2026m02` 形态），`retention_months>0` 时自动 DROP 过期分区。
- 写入走异步 writer（缓冲 channel + 批量 insert + 退避重试，满则丢弃并计数）。
- 查询 API 强制带 `created_at` 范围实现分区裁剪。

## 部门闭包表

`department_closure(ancestor_id, descendant_id, depth)`，每个节点含自引用行（depth=0）。

| 操作 | 实现 |
|---|---|
| 新建 | 插入自引用行 + `INSERT SELECT` 父链祖先行（depth+1） |
| 移动子树 | 校验不成环 → 删除「子树 × 旧外部祖先」关系 → 交叉插入新祖先链（事务） |
| 删除子树 | 单条 `DELETE ... WHERE id IN (子树)`，闭包/用户关联靠 FK 级联 |
| 查后代 | `WHERE ancestor_id = ?`（一次索引查询） |
| 面包屑 | `JOIN depth DESC` |

用户多部门：`user_departments(user_id, dept_id, is_primary)` M2M。

## fx 模块图

```
config → logging → database(迁移/分区/种子) → repository → ┬→ auth → relay ┐
                                                            ├→ billing ────┤→ server(gin)
                                                            ├→ provider ───┤
                                                            ├→ usage ──────┤
                                                            └→ sso ────────┘→ admin
```

日志：`fx.WithLogger(fxevent.ZapLogger)`；gorm 桥接 zap（慢 SQL/错误）；gin 访问日志、业务日志均带 `request_id`（由 `X-Request-Id` 中间件生成或透传，并落 `request_logs.request_id`）。

## 重试与超时

- 渠道失败（网络错误 / 408 / 429 / 5xx）→ 按 priority 顺序尝试下一渠道，最多 `max_retries` 次；已向客户端写出字节（流式已开始）则不再重试。
- 非流式：上游 2xx 但解析失败视为终态错误。
- 每次上游请求独立 `UpstreamTimeout`；客户端断开（ctx cancel）立即停止并按部分用量结算，日志记 `status_code=499`。

## 渠道并发限制与排队（单节点）

- 每渠道可设 `providers.max_concurrency`（0 = 不限制）：进程内按渠道 ID 持信号量（`provider.Limiter`），流式请求持有槽位直至流结束。
- 满载时新请求在队列等待（`relay.channel_queue_timeout`，默认 60s）；超时直接向客户端返回终止性 429（`code=channel_busy`），**不跨渠道重试**，保持各账户并发隔离与排队语义可预期。
- 上游渠道真正的 429（非排队超时）仍按可重试错误处理，跨渠道重试。

## 粘性路由（多轮会话命中相同上游账户）

- 会话键推导优先级：① 请求头 `X-Session-ID`；② `metadata.session_id`；③ 兑底 `sha256(keyID|model|首条 user 消息文本)`——多轮对话首条消息稳定，天然按会话聚合（codex / claude code / opencode 等不发自定义头的客户端适用）。
- 进程内 `provider.StickyTable`（滑动 TTL，`relay.sticky_session_ttl` 默认 30m，负值关闭）；仅转发成功后绑定，渠道下线/不再服务该模型时自动回退正常加权路由。
- 粘性与排队交互：粘住渠道满载时先排队；排队超时直接 429，不转移到其他账户。
