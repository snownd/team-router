# 管理台 API：模型定价（`/api/admin/prices`）

需要 JWT 认证且角色为 `admin`。价格按 `(model_name, provider_type)` 唯一，
`provider_type` 为空字符串表示通配价（该模型在所有渠道类型上的默认价）。
查价顺序：精确匹配 → 通配匹配，带 TTL 缓存；写接口会立即失效缓存。

**单位约定**：所有 `*_per_m` 字段均为**每百万 token 的微美元数**（`1e6` 微美元 = $1）。

## GET /api/admin/prices

全部价目。

#### 响应 200

```json
{
  "items": [
    {
      "id": 1,
      "model_name": "gpt-4o",
      "provider_type": "openai",
      "input_per_m": 2500000,
      "output_per_m": 10000000,
      "cache_read_per_m": 1250000,
      "cache_write_per_m": 0,
      "currency": "USD",
      "status": 1,
      "created_at": "2025-01-01T00:00:00Z",
      "updated_at": "2025-01-01T00:00:00Z"
    }
  ],
  "total": 1
}
```

上例即输入 $2.5 / 百万 token、输出 $10 / 百万 token。

## POST /api/admin/prices

新增或更新（upsert，按 `model_name + provider_type` 命中）。

#### 请求

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model_name` | string | 是 | 模型别名 |
| `provider_type` | string | 否 | 渠道类型，空为通配价 |
| `input_per_m` | int64 | 否 | 输入单价 |
| `output_per_m` | int64 | 否 | 输出单价 |
| `cache_read_per_m` | int64 | 否 | 缓存读单价 |
| `cache_write_per_m` | int64 | 否 | 缓存写单价 |
| `currency` | string | 否 | 缺省 `USD` |
| `status` | int | 否 | `0` 按启用处理；1 启用 / 2 禁用 |

```json
{"model_name": "gpt-4o", "provider_type": "openai", "input_per_m": 2500000, "output_per_m": 10000000}
```

#### 响应 200

```json
{"message": "ok"}
```

**错误**：400（`model_name` 缺失）。

## DELETE /api/admin/prices/:id

删除价目。

#### 响应 200

```json
{"message": "ok"}
```

> 未配置价格的模型：`billing.allow_unknown_pricing=true`（默认）时按 0 费用放行并标记估算；否则中继请求返回 400。
