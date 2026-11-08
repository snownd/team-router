# API 文档总览

team-router 对外暴露两类 API，分别使用不同的认证方式：

| 分类 | 前缀 | 认证方式 | 文档 |
| --- | --- | --- | --- |
| LLM 中继（OpenAI / Anthropic 兼容） | `/v1/*` | API Key：`Authorization: Bearer sk-...` | [relay.md](relay.md) |
| 登录认证 | `/api/auth/*` | 无需认证（登录本身） / SSO OAuth | [auth.md](auth.md) |
| 用户自助 | `/api/user/*` | JWT | [user.md](user.md) |
| 管理台 | `/api/admin/*` | JWT + `admin` 角色 | [admin-users.md](admin-users.md) / [admin-departments.md](admin-departments.md) / [admin-keys.md](admin-keys.md) / [admin-providers.md](admin-providers.md) / [admin-prices.md](admin-prices.md) / [admin-logs-stats.md](admin-logs-stats.md) |
| 健康检查 | `/healthz` | 无 | `GET /healthz` → `{"status": "ok"}` |

## 基础约定

- **Base URL**：`http://<host>:8080`（默认监听 `:8080`，由 `server.addr` / `TR_SERVER_ADDR` 配置）。
- **请求 ID**：每个响应携带 `X-Request-Id` 响应头；客户端可通过同名请求头传入以便链路追踪，未传入时网关自动生成（格式 `20060102T150405-<hex16>`）。
- **额度单位**：所有 `quota` / `used_quota` / `quota_cost` 均为**微美元整数**，`1e6 = $1`。
- **状态值约定**：`status` 类字段 `1 = 启用，2 = 禁用`（用户 / Key / 渠道 / 部门 / 价目通用）。

## 认证方式

### API Key（`/v1/*`）

```text
Authorization: Bearer sk-xxxxxxxx
```

Key 形如 `sk-<base64url(36字节)>`，明文仅在创建时返回一次，库中只存 sha256 哈希。
认证结果带进程内 TTL 缓存（默认约 1 分钟），禁用 / 过期在缓存到期后生效。

### JWT（`/api/*`）

```text
Authorization: Bearer <access_token>
```

通过 `POST /api/auth/login` 或 SSO 回调获取。HS256 签名，载荷含 `uid / username / role`，有效期由 `auth.token_ttl` 决定。

`/api/admin/*` 在 JWT 校验之后追加 `role == "admin"` 校验，否则返回 403：

```json
{"error": {"message": "需要管理员权限", "type": "authentication_error"}}
```

## 通用错误格式

管理台 `/api/*` 的错误统一为：

```json
{"error": {"message": "错误描述"}}
```

| 状态码 | 场景 |
| --- | --- |
| 400 | 参数错误 / 业务校验失败（message 中带原因） |
| 401 | 缺少认证令牌 / 令牌无效或已过期 |
| 403 | 非管理员访问管理接口 |
| 404 | 资源不存在（部分接口以 400 `非法 ID` 表达路径参数错误） |
| 500 | 内部错误（details 只在服务端日志） |

`/v1/*` 中继接口的错误模型按入站协议渲染，见 [relay.md](relay.md#错误模型)。

## 分页约定

列表接口统一使用：

| 参数 | 默认 | 说明 |
| --- | --- | --- |
| `page` | `1` | 页码（< 1 按 1 处理） |
| `page_size` | `20` | 每页条数，最大 500，越界回退 20 |

响应统一为：

```json
{"items": [...], "total": 123}
```

## 时间范围约定

日志 / 统计类接口统一使用 `start` / `end` 查询参数：

- 支持两种格式：RFC3339（`2025-01-01T00:00:00Z`）或日期（`2025-01-01`）；
- 缺省时为**最近 7 天**（`end` 为当前 UTC 时刻）；
- `request_logs` 为按月分区表，查询会强制带时间范围以做分区裁剪。

## 序列化注意事项

`model.APIKey`、`model.Provider`、`model.ModelPrice`、`model.RequestLog`、`model.Department` 等带 json tag 的实体按 tag 输出小写字段；但 **`model.User` 与 `model.UserDeptView` 目前没有 json tag，按 Go 导出字段名序列化**（`ID`、`Username`、`PasswordHash`、`IsPrimary` 等，见各接口文档示例）。对接时以文档示例为准，后续版本可能统一为 snake_case。
