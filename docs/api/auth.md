# 认证 API（`/api/auth/*`）

## POST /api/auth/login

本地账号登录，签发管理台 JWT。

#### 请求

```json
{
  "username": "admin",
  "password": "admin123"
}
```

#### 响应 200

```json
{
  "access_token": "<JWT>",
  "user": {
    "id": 1,
    "username": "admin",
    "display_name": "管理员",
    "role": "admin",
    "quota": 0,
    "used_quota": 0
  }
}
```

- `role`：`admin` | `user`；
- `quota` / `used_quota`：微美元，`quota=0` 表示不限额；
- JWT 为 HS256，有效期由 `auth.token_ttl` 配置，后续请求通过 `Authorization: Bearer <token>` 携带。

#### 错误

| 状态码 | 场景 |
| --- | --- |
| 400 | 缺少 `username` / `password` |
| 401 | 用户名或密码错误、用户被禁用 |

---

## SSO 登录（扩展点）

通用路由在 SSO 提供方（企业微信 / 飞书 / 钉钉等）注册后自动生效，路径模板：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/auth/sso/:name/login` | 跳转到 SSO 平台授权页（302） |
| GET / POST | `/api/auth/sso/:name/callback` | 授权回调，换取身份并签发 JWT |

- `:name` 为提供方标识（如 `wecom` / `feishu` / `dingtalk`），未注册时返回 404，message 中列出已注册提供方；
- 首次登录自动建号（JIT provision），用户记录 `sso_provider` / `sso_external_id`；
- 回调成功响应与本地登录一致：

```json
{
  "access_token": "<JWT>",
  "user": {"id": 2, "username": "zhangsan", "role": "user"}
}
```

#### 错误

| 状态码 | 场景 |
| --- | --- |
| 400 | 回调缺少 `code` 参数 |
| 404 | 提供方未接入 |
| 502 | SSO `code` 换身份失败 |
| 500 | 建号 / 签发内部错误 |

> 第一期未内置任何提供方实现。接入方式见 [docs/extending-sso.md](../extending-sso.md)。
