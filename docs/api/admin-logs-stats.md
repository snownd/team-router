# 管理台 API：请求日志与统计（`/api/admin/logs`、`/api/admin/stats`）

需要 JWT 认证且角色为 `admin`。底层为 PG 按月 RANGE 分区表 `request_logs`，
查询**强制携带时间范围**以做分区裁剪。

时间参数通用约定：`start` / `end` 支持 RFC3339 或 `YYYY-MM-DD`，缺省为最近 7 天。

## GET /api/admin/logs

请求日志查询。

#### 查询参数

| 参数 | 说明 |
| --- | --- |
| `start` / `end` | 时间范围 |
| `user_id` / `api_key_id` / `provider_id` | 精确过滤 |
| `model` | 模型精确匹配 |
| `protocol` | `chat_completions` / `responses` / `messages` |
| `keyword` | 模糊匹配 |
| `is_error` | `1`/`true` 只看错误；`0`/`false` 只看成功 |
| `page` / `page_size` | 分页，默认 1 / 20 |

#### 响应 200

```json
{"items": [RequestLog...], "total": 123}
```

### RequestLog 字段

| 字段 | 说明 |
| --- | --- |
| `id` / `created_at` / `request_id` | 主键、请求时间、请求 ID（与 `X-Request-Id` 一致） |
| `user_id` / `username` / `api_key_id` | 调用方 |
| `provider_id` / `provider_name` / `upstream_type` | 实际命中的渠道 |
| `protocol` / `model` / `upstream_model` | 入站协议、请求模型别名、上游物理模型（含映射） |
| `stream` / `status_code` / `is_error` / `err_msg` | 结果状态；客户端断开记 `499` |
| `input_tokens` | 新鲜输入（不含缓存命中） |
| `output_tokens` / `reasoning_tokens` | 输出与推理（推理是输出的子集） |
| `cache_read_tokens` / `cache_write_tokens` | 缓存命中与缓存写入 |
| `ttft_ms` / `total_ms` | 首 token 延迟 / 总耗时 |
| `retries` | 跨渠道重试次数 |
| `quota_cost` | 实际扣费（微美元） |
| `billing_detail` | jsonb：单价、各分项费用、币种、是否估算 |
| `client_ip` | 客户端 IP |

## GET /api/admin/stats

用量与费用统计，按**天 × 模型**聚合。

**查询参数**：`start` / `end`、`user_id`（可选）。

#### 响应 200

```json
{
  "items": [
    {
      "bucket": "2025-01-01T00:00:00Z",
      "model": "gpt-4o",
      "requests": 42,
      "input_tokens": 12000,
      "output_tokens": 8000,
      "cache_read_tokens": 1000,
      "cache_write_tokens": 500,
      "quota_cost": 6789
    }
  ],
  "total": 3
}
```

## GET /api/admin/stats/ttft

首 token 延迟看板：按 `provider_name × model` 聚合，按请求数倒序取前 100。

**查询参数**：`start` / `end`。

#### 响应 200

```json
{
  "items": [
    {
      "provider_name": "openai",
      "model": "gpt-4o",
      "requests": 120,
      "avg_ttft_ms": 530,
      "avg_total_ms": 2100
    }
  ]
}
```

`avg_ttft_ms` 仅统计 `ttft_ms > 0` 的样本。

---

## 相关接口

用户侧同构接口（数据范围强制本人，无需 admin 角色）：
[GET /api/user/logs](user.md#get-apiuserlogs)、[GET /api/user/stats](user.md#get-apiuserstats)。
