# 管理台 API：API Key 管理（`/api/admin/keys`）

需要 JWT 认证且角色为 `admin`。管理台侧的 Key 全生命周期管理；
用户自助创建见 [user.md](user.md)。Key 明文仅创建时返回一次，库中只存 sha256 哈希。

## GET /api/admin/keys

Key 列表。

**查询参数**：`user_id`（按用户过滤，缺省全部）、`page`、`page_size`。

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

`allowed_models`：jsonb；`null` / `[]` 表示不限制。`quota` / `used_quota` 单位微美元。

## POST /api/admin/keys

为任意用户创建 Key。

#### 请求

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `user_id` | int64 | 是 | 归属用户，须已存在 |
| `name` | string | 否 | 备注名 |
| `quota` | int64 | 否 | Key 级额度（微美元），0 不限额 |
| `allowed_models` | string[] | 否 | 模型白名单，空不限制 |
| `expires_in_days` | int | 否 | 有效天数，0 永不过期 |

```json
{"user_id": 2, "name": "ops-key", "expires_in_days": 90, "allowed_models": ["gpt-4o"]}
```

#### 响应 200

```json
{
  "id": 11,
  "name": "ops-key",
  "key": "sk-xxxxxxxxxxxxxxxx",
  "key_prefix": "sk-AbCdEf...",
  "quota": 0,
  "expires_at": "2025-04-01T00:00:00Z",
  "notice": "请妥善保存，密钥明文不会再次显示"
}
```

**错误**：400（`user_id` 缺失）；404（用户不存在）。

## PUT /api/admin/keys/:id

更新 Key（启用 / 禁用 / 改名 / 改额度）。

#### 请求

| 字段 | 说明 |
| --- | --- |
| `name` | 非空时更新 |
| `status` | `1` 启用 / `2` 禁用 |
| `quota` | 提交时更新 Key 级额度 |

#### 响应 200

```json
{"message": "ok"}
```

> 认证结果带进程内 TTL 缓存，状态变更最长延迟约一个缓存周期生效。

## DELETE /api/admin/keys/:id

删除 Key。

#### 响应 200

```json
{"message": "ok"}
```
