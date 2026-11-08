# 接入企业微信 / 飞书 / 钉钉 SSO

首期未实现任何 SSO，但扩展点已就绪：`/api/auth/sso/{name}/login` 与 `/api/auth/sso/{name}/callback` 是通用路由，注册了 Provider 即生效。

## 扩展接口

```go
// internal/sso/sso.go
type Provider interface {
    Name() string                                  // 路由名：wecom | feishu | dingtalk
    AuthorizeURL(state string) string              // 跳转到 SSO 平台的授权地址
    Exchange(ctx context.Context, code string) (*Identity, error)
}

type Identity struct {
    ExternalID  string            // 平台唯一 ID（userid / open_id / unionid）
    Email       string
    DisplayName string
    Extra       map[string]string // 手机号、部门等
}
```

回调成功后网关自动完成：查找/创建用户（JIT 建号，挂根部门）→ 签发与本地登录相同的 JWT。users 表的 `sso_provider` / `sso_external_id` 字段已建好并有索引。

## 接入飞书示例

```go
// internal/sso/feishu/feishu.go
package feishu

type Provider struct {
    AppID, AppSecret, RedirectURL string
}

func (p *Provider) Name() string { return "feishu" }

func (p *Provider) AuthorizeURL(state string) string {
    // https://open.feishu.cn/open-apis/authen/v1/index?app_id=...&redirect_uri=...&state=...
}

func (p *Provider) Exchange(ctx context.Context, code string) (*sso.Identity, error) {
    // 1. app_access_token（app_id + app_secret）
    // 2. POST /authen/v1/oidc/access_token(code) -> user_access_token
    // 3. GET /authen/v1/user_info -> open_id / name / email
}
```

## 注册

```go
// cmd/gateway/main.go（或独立 module）
fx.Provide(fx.Annotate(
    func(cfg *config.Config) (sso.Provider, error) { // 从配置读取 app_id/secret
        return feishu.New(cfg.SSO.Feishu), nil
    },
    fx.As(new(sso.Provider)),
    fx.ResultTags(`group:"sso_providers"`),
))
```

`internal/sso` 的 fx Module 已声明该 value group，并提供空组兜底——不注册任何 Provider 时服务正常启动，SSO 路由返回 404 与可用列表。

## 企业微信 / 钉钉要点

| 平台 | 授权码流程 | 身份字段 |
|---|---|---|
| 企业微信 | 网页授权 `/connect/oauth2/authorize` → `code` → `getuserinfo` | `userid`（配合通讯录读取部门可同步组织架构） |
| 飞书 | 上述示例 | `open_id` / `union_id` |
| 钉钉 | 扫码或账号密码授权 → `authCode` → `getuser_byunionid` | `unionid` |

## 注意事项

- `state` 首期为固定值，生产接入时应改为随机生成 + Redis/内存缓存校验（防 CSRF）
- 建议将 `Extra` 中的部门信息映射到本系统的多部门模型（`PUT /api/admin/users/:id/departments`）
- JIT 建号默认挂根部门；可在 Provisioner 中按 SSO 部门信息自动挂载（`internal/sso/provisioner.go`）
