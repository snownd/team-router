# 管理台 API：渠道管理（`/api/admin/providers`）

需要 JWT 认证且角色为 `admin`。上游渠道（provider）定义网关可转发到的 LLM 服务；
上游密钥以 AES-256-GCM 加密落库，任何接口**不会**返回密钥明文。

## GET /api/admin/providers

全部渠道列表。

#### 响应 200

```json
{
  "items": [
    {
      "id": 1,
      "name": "openai",
      "type": "openai",
      "base_url": "https://api.openai.com",
      "models": ["gpt-4o", "gpt-4o-mini"],
      "model_mapping": {},
      "priority": 0,
      "weight": 1,
      "status": 1,
      "proxy_url": "",
      "created_at": "2025-01-01T00:00:00Z",
      "updated_at": "2025-01-01T00:00:00Z"
    }
  ],
  "total": 1
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `type` | 适配器类型，见 `GET /providers/types` |
| `models` | 可服务模型列表（网关模型别名） |
| `model_mapping` | 请求模型 → 上游模型的重定向，如 `{"gpt-4o": "gpt-4o-2024-11-20"}` |
| `priority` | 路由优先级，数值大者优先消耗 |
| `weight` | 同优先级内加权随机，`<=0` 按 1 处理 |
| `status` | 1 启用 / 2 禁用 |
| `proxy_url` | 可选，访问该渠道使用的 HTTP 代理 |

## GET /api/admin/providers/types

已注册的适配器类型。

#### 响应 200

```json
{"types": ["openai", "anthropic"]}
```

## POST /api/admin/providers/fetch-models

按适配器协议调用上游模型列表接口（如 `GET {base}/v1/models`），返回上游可服务的模型 ID 列表，
供管理台建渠道时直接勾选。

#### 请求

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `type` | string | 是 | 适配器类型，须在 `types` 列表中 |
| `base_url` | string | 是 | 上游地址；不以 `/v1` 结尾时自动补全 |
| `api_key` | string | 否 | 上游密钥明文；留空且指定 `provider_id` 时用已存密钥 |
| `proxy_url` | string | 否 | 访问上游使用的 HTTP 代理 |
| `provider_id` | int | 否 | 编辑已有渠道时传入，配合留空的 `api_key` 复用落库密钥 |

```json
{"type": "openai", "base_url": "https://api.openai.com", "api_key": "sk-xxx"}
```

#### 响应 200

```json
{"models": ["gpt-4o", "gpt-4o-mini"]}
```

**错误**：400（参数缺失、`type` 不受支持或上游返回非 200 —— message 附上游错误信息）、
404（`provider_id` 对应渠道不存在）。

## POST /api/admin/providers

创建渠道。

#### 请求

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `name` | string | 是 | 渠道名 |
| `type` | string | 是 | 适配器类型，须在 `types` 列表中 |
| `base_url` | string | 是 | 上游地址；不以 `/v1` 结尾时自动补全 |
| `api_key` | string | 否 | 上游密钥明文（加密落库） |
| `models` | string[] | 否 | 可服务模型，缺省 `[]` |
| `model_mapping` | object | 否 | 模型重定向，缺省 `{}` |
| `priority` / `weight` | int | 否 | 路由参数，`weight<=0` 按 1 |
| `status` | int | 否 | 缺省启用 |
| `proxy_url` | string | 否 | HTTP 代理 |

```json
{"name": "openai", "type": "openai", "base_url": "https://api.openai.com", "api_key": "sk-xxx", "models": ["gpt-4o", "gpt-4o-mini"]}
```

#### 响应 200

返回创建后的渠道对象（同 GET 列表元素，无 `api_key` 字段）。

**错误**：400（`name`/`type`/`base_url` 缺失，或 `type` 不受支持——message 附 `supported` 列表）。

## PUT /api/admin/providers/:id

更新渠道。`api_key` 留空表示保持不变；`models` / `model_mapping` 提交时全量覆盖；
`priority` / `weight` / `proxy_url` 按提交值直接覆盖。

#### 响应 200

返回更新后的渠道对象。

**错误**：404（渠道不存在）。

## DELETE /api/admin/providers/:id

删除渠道。

#### 响应 200

```json
{"message": "ok"}
```

> 渠道的增删改会使路由表立即失效重建，无需重启。
