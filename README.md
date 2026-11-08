# team-router

类 [new-api](https://github.com/QuantumNous/new-api) 的 LLM API 网关，基于 **Gin + Uber FX + GORM + PostgreSQL + Zap**。

## 特性

- **三种入站协议，任意互转（any-to-any）**
  - `POST /v1/chat/completions`（OpenAI Chat Completions，流式/非流式）
  - `POST /v1/responses`（OpenAI Responses API，v1 支持无状态子集）
  - `POST /v1/messages`（Anthropic Messages，含 prompt cache 字段透传）
- **多上游渠道**：OpenAI 官方及兼容端点、Anthropic；按 `models` / `model_mapping` 匹配，priority 优先、weight 加权随机，失败自动跨渠道重试
- **计费与配额**：按 token 计费（输入/输出/缓存读/缓存写 四单价，微美元整数运算），key 与用户两级额度、预扣 + 多退少补，明细存入日志 `billing_detail`
- **请求日志**：PG 原生按月 RANGE 分区，记录输入/输出/缓存命中/缓存写入 token、首 token 延迟（TTFT）、总耗时、重试次数、API Key、渠道、计费明细等；分区自动预建与过期清理
- **用户与部门**：闭包表（closure table）部门树，支持移动子树/删除子树/面包屑查询；**一个用户可同时属于多个部门**（含主部门标记）
- **SSO 扩展机制**：预留企业微信 / 飞书 / 钉钉接入接口（`internal/sso`），首期不实现
- 上游密钥 AES-256-GCM 加密落库；全链路 zap 结构化日志 + request-id 贯穿

## 快速开始

```bash
# 1) 起 PG + 网关（自动迁移 + 建分区 + 种子数据）
cd deploy && docker compose up -d --build

# 2) 登录（默认 admin/admin123，种子数据）
curl -s -X POST localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}'
# => {"access_token":"...", "user":{...}}

# 3) 配置渠道（以任意 OpenAI 兼容端点为例）
curl -s -X POST localhost:8080/api/admin/providers \
  -H "Authorization: Bearer <JWT>" -H 'Content-Type: application/json' \
  -d '{"name":"openai","type":"openai","base_url":"https://api.openai.com","api_key":"sk-xxx","models":["gpt-4o","gpt-4o-mini"]}'

# 4) 给自己发一把 API Key
curl -s -X POST localhost:8080/api/user/keys \
  -H "Authorization: Bearer <JWT>" -H 'Content-Type: application/json' \
  -d '{"name":"my-key"}'
# => {"key":"sk-...","notice":"请妥善保存，密钥明文不会再次显示"}

# 5) 像调 OpenAI 一样调用网关
curl -s -X POST localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-..." -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}'

# 同一个 key 也可以直接用 Anthropic / Responses 协议调用：
curl -s -X POST localhost:8080/v1/messages \
  -H "Authorization: Bearer sk-..." -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o","max_tokens":1024,"messages":[{"role":"user","content":"hello"}]}'
```

本地开发（不走 Docker）：

```bash
cp config.example.yaml config.yaml   # 按需修改 DSN
go run ./cmd/gateway

# 跑全部测试（数据库测试需要 TEST_PG_DSN）
docker run -d --name tr-pg -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=team_router -p 5432:5432 postgres:18-alpine
TEST_PG_DSN="host=localhost user=postgres password=postgres dbname=team_router sslmode=disable" go test ./...
```

## 配置

见 `config.example.yaml`；关键环境变量：`TR_CONFIG`、`TR_DATABASE_DSN`、`TR_SERVER_ADDR`、`TR_AUTH_JWT_SECRET`、`TR_CRYPTO_MASTER_KEY`（32 字节 hex，`openssl rand -hex 32`）、`TR_LOG_LEVEL`、`TR_LOG_DEV`。

## 架构

见 [docs/architecture.md](docs/architecture.md)（canonical 中间格式、relay 管线、计费模型、分区表设计）。

## 扩展指南

- 接入新上游（Gemini、Bedrock、vLLM…）：[docs/extending-provider.md](docs/extending-provider.md)
- 接入企业微信 / 飞书 / 钉钉 SSO：[docs/extending-sso.md](docs/extending-sso.md)

## API 一览

| 分组 | 端点 | 说明 |
|---|---|---|
| 网关 | `POST /v1/chat/completions` / `POST /v1/responses` / `POST /v1/messages` | API Key 认证，三协议 |
| 网关 | `GET /v1/models` | 可服务模型列表 |
| 认证 | `POST /api/auth/login` | 本地账号登录，发 JWT |
| 认证 | `GET /api/auth/sso/{name}/login` `/callback` | SSO 扩展点（首期无实现） |
| 用户自助 | `GET/POST/DELETE /api/user/keys` | API Key 自助管理 |
| 用户自助 | `GET /api/user/me` `GET /api/user/logs` `GET /api/user/stats` | 本人信息 / 日志 / 用量 |
| 管理 | `/api/admin/users*` | 用户 CRUD、额度、密码、多部门绑定 |
| 管理 | `/api/admin/departments*` | 部门树、增删改、移动子树、面包屑、成员 |
| 管理 | `/api/admin/keys*` | 全部 Key 管理 |
| 管理 | `/api/admin/providers*` | 渠道 CRUD（`/types` 列出已注册适配器） |
| 管理 | `/api/admin/prices` | 模型定价 |
| 管理 | `GET /api/admin/logs` `GET /api/admin/stats` `GET /api/admin/stats/ttft` | 日志查询、用量与延迟看板 |

## 计费说明

- 单价单位：**微美元 / 每百万 token**（`1e6 = $1`）；额度（quota）同为微美元
- `cost = input×in价 + output×out价 + cache_read×读价 + cache_write×写价`
- 语义对齐：OpenAI `prompt_tokens` 含 `cached_tokens`，网关换算为「新鲜输入 = prompt − cached」；Anthropic `input_tokens` 天然不含缓存读
- 请求前按「输入估算 + max_tokens × 上界单价」预扣，结束后按实际用量多退少补（均为原子 SQL）
- 上游未返回 usage 时按估算计费并在 `billing_detail.estimated` 标记
- 模型无定价时：`billing.allow_unknown_pricing=true` 放行并按 0 计费；`false` 拒绝

## 首期边界（v1）

- Responses API 不支持有状态模式（`previous_response_id` / 服务端 `store`）
- 上游仅实现 OpenAI 兼容与 Anthropic 两类适配器（新增见扩展文档）
- SSO 仅提供接口与通用路由，无具体实现
- 单组织（无组织表），部门树根即全局根
