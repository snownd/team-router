# 用户自助 API（`/api/user/*`）

全部接口需要 JWT 认证（`Authorization: Bearer <token>`），数据范围强制限定为当前登录用户。

## GET /api/user/me

当前用户信息（含部门）。

#### 响应 200

```json
{
  "id": 2,
  "username": "zhangsan",
  "display_name": "张三",
  "email": "zhangsan@example.com",
  "role": "user",
  "status": 1,
  "quota": 100000000,
  "used_quota": 12345,
  "quota_remaining": 99987655,
  "departments": [
    {"ID": 3, "Name": "平台研发部", "ParentID": 1, "IsPrimary": true}
  ]
}
```

- `quota` 单位微美元（1e6 = $1），`0` 表示不限额；
- `quota_remaining`：剩余额度；`-1` 表示不限额，最小为 `0`；
- `departments[].IsPrimary` 为主部门标记（该结构当前按 Go 字段名序列化，见 [总览](README.md#序列化注意事项)）。

## GET /api/user/keys

当前用户的 API Key 列表（不含明文）。

#### 响应 200

```json
{
  "items": [
    {
      "id": 10,
      "user_id": 2,
      "name": "my-key",
      "key_prefix": "sk-AbCdEf...",
      "status": 1,
      "quota": 0,
      "used_quota": 5000,
      "allowed_models": null,
      "expires_at": null,
      "created_at": "2025-01-01T00:00:00Z",
      "updated_at": "2025-01-01T00:00:00Z"
    }
  ],
  "total": 1
}
```

- `allowed_models`：jsonb 数组；`null` / `[]` 表示不限制；
- `quota`：Key 级额度（微美元），`0` 不限额。

## POST /api/user/keys

创建 API Key。**明文仅此一次返回**，请妥善保存。

#### 请求

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `name` | string | 否 | 备注名 |
| `quota` | int64 | 否 | Key 级额度（微美元），默认 0 不限额 |
| `allowed_models` | string[] | 否 | 模型白名单，空表示不限制 |
| `expires_in_days` | int | 否 | 有效天数，0 表示永不过期 |

```json
{"name": "my-key", "expires_in_days": 30}
```

#### 响应 200

```json
{
  "id": 10,
  "name": "my-key",
  "key": "sk-xxxxxxxxxxxxxxxx",
  "key_prefix": "sk-AbCdEf...",
  "quota": 0,
  "expires_at": "2025-01-31T00:00:00Z",
  "notice": "请妥善保存，密钥明文不会再次显示"
}
```

## DELETE /api/user/keys/:id

删除自己的 Key。

**响应 200** `{"message": "ok"}`

**错误**：404（Key 不存在或不属于当前用户）；400（非法 ID）。

## GET /api/user/logs

当前用户的请求日志（强制本人，忽略 `user_id` 参数）。

#### 查询参数

| 参数 | 说明 |
| --- | --- |
| `start` / `end` | 时间范围，RFC3339 或 `YYYY-MM-DD`，默认最近 7 天 |
| `model` | 按模型精确过滤 |
| `protocol` | `chat_completions` / `responses` / `messages` |
| `keyword` | 关键字（模型 / 错误信息等模糊匹配） |
| `is_error` | `1` / `true` 只看错误，`0` / `false` 只看成功 |
| `page` / `page_size` | 分页，默认第 1 页 / 20 条 |

**响应 200** `{"items": [RequestLog...], "total": 123}`，字段见 [admin-logs-stats.md](admin-logs-stats.md#requestlog-字段)。

## GET /api/user/stats

当前用户的用量统计（按天 × 模型聚合）。

**查询参数**：`start` / `end`（默认最近 7 天）。

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

`quota_cost` 单位微美元。
