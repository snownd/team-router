# Plan: Web 管理台 — 前端初始化与基础功能

**Source PRD**: `.claude/prds/team-ai-token-platform.prd.md`
**Selected Milestone**: #3 Web 管理台（首个增量：初始化 + 基础功能）
**Complexity**: Large
**前端规范**: 遵循 `ecc:vue-patterns`（Composition API `<script setup>` + TS，feature-first 结构）

## Summary
在空的 `frontend/` 目录下，以 Vue 3 + TypeScript + Vite + Arco Design Vue 搭建管理台应用，主体色采用指定 cyan 色板（cyan-6 `#14C9C9`）。交付：登录与 JWT 会话、角色化导航布局、用户自助页（我的 Key / 日志 / 用量）、管理员页（用户 / 部门树 / Key / 渠道 / 价目 / 日志 / 统计），全部对接现有 `/api/*` 接口。SSO 登录（飞书）属 M4，本计划不做。

## Patterns to Mirror

后端契约（源自 `docs/api/`）：

| Category | Source | Pattern |
|---|---|---|
| API 前缀 | `internal/server/routes.go:35-93` | 自助 `/api/user/*`、管理 `/api/admin/*`、登录 `POST /api/auth/login` |
| 认证 | `docs/api/README.md` | `Authorization: Bearer <JWT>`；401 → 清 token 回登录页 |
| 错误格式 | `docs/api/README.md` | `{"error":{"message":"..."}}` → axios 拦截器统一 `Message.error` |
| 分页 | `docs/api/README.md` | 请求 `page`/`page_size`，响应 `{items,total}` → 适配 Arco Table 分页 |
| 状态值 | `docs/api/README.md` | `status`: 1=启用 2=禁用（用户/Key/渠道/部门/价目通用） |
| 额度单位 | `docs/api/README.md` | 微美元整数，`1e6 = $1`；`quota=0` 不限额；`quota_remaining=-1` 不限额 |
| 序列化例外 | `docs/api/user.md` | `/user/me` 的 `departments[]` 按 Go 字段名输出（`ID`/`Name`/`ParentID`/`IsPrimary`），按各接口文档示例逐一定义类型，不做全局假设 |
| 时间范围 | `docs/api/README.md` | 日志/统计用 `start`/`end`（RFC3339 或日期），默认近 7 天 |

前端约定（源自 `ecc:vue-patterns`，frontend/ 为空、无既有前端模式）：

| Category | Pattern |
|---|---|
| 结构 | feature-first：`api/ components/ composables/ layouts/ pages/ router/ stores/ types/ utils/` |
| 组件 | PascalCase.vue；SFC 顺序 `<script setup>` → `<template>` → `<style scoped>`；禁止 Options API |
| Props/Emits | `defineProps<Props>()` + `withDefaults`；`defineEmits<{...}>()` 类型化；v-model 用 `defineModel()` |
| 状态 | Pinia Setup Store（`defineStore("auth", () => {...})`），异步 action 含 loading/error |
| 路由 | 路由组件 `() => import(...)` 懒加载；`meta.requiresAuth` / `meta.requiresAdmin` + 全局守卫，重定向携带 `?redirect=` |
| 列表 | `v-for` 用稳定业务 ID 作 key；过滤放 computed，不与 `v-if` 同元素 |
| Composables | `use` 前缀，返回响应式值，`onUnmounted` 清理副作用 |

## Files to Change
全部为 CREATE（`frontend/` 当前为空）：

```
frontend/
├── package.json / vite.config.ts / tsconfig*.json / index.html / .gitignore
├── src/
│   ├── main.ts                     # Arco 注册 + 主题样式引入 + Pinia/Router
│   ├── App.vue                     # <router-view> + a-config-provider(zh-CN)
│   ├── styles/theme.less           # cyan 色板覆盖（--primary-1..10）
│   ├── types/api.ts                # 各端点请求/响应类型（按 docs/api 逐端点定义）
│   ├── router/index.ts             # 懒加载路由 + meta 守卫 + redirect 回跳
│   ├── stores/useAuthStore.ts      # Pinia Setup Store：token + 当前用户
│   ├── api/http.ts                 # axios 实例：token 注入、错误/401 拦截、X-Request-Id
│   ├── api/auth.ts / user.ts / admin.ts   # 端点封装（返回类型化 Promise）
│   ├── composables/usePagination.ts       # page/page_size ↔ {items,total} ↔ Arco Table（≥7 个列表页复用）
│   ├── utils/quota.ts / format.ts  # 微美元格式化、时间范围
│   ├── utils/quota.test.ts         # Vitest：quota 格式化（0/-1/微美元换算）
│   ├── layouts/AdminLayout.vue     # 侧边菜单（按角色过滤）+ 头部用户下拉
│   ├── components/StatusTag.vue / QuotaText.vue   # 类型化 props，不调 API
│   └── pages/
│       ├── LoginPage.vue
│       ├── DashboardPage.vue       # admin→/admin/stats，user→/user/stats
│       ├── my/MyKeysPage.vue       # 创建后一次性明文弹窗 + 复制
│       ├── my/MyLogsPage.vue / my/MyUsagePage.vue
│       └── admin/UsersPage.vue  DepartmentsPage.vue  KeysPage.vue
│            ProvidersPage.vue  PricesPage.vue  LogsPage.vue
```

## Tasks

### Task 1: 脚手架与主题
- **Action**: `pnpm create vite frontend --template vue-ts`；安装 `@arco-design/web-vue`、`vue-router`、`pinia`、`axios`、`less`；Vitest + `vue-tsc type-check` 脚本 + `@vue/test-utils`；vite dev proxy `/api → http://localhost:8080`；在 `theme.less` 用 cyan-1..10 覆盖 Arco primary 变量。
- **Mirror**: Arco 全量引入方式以实施时官方文档为准（见 Risks：ctx7 本次不可用）。
- **Validate**: `pnpm dev` 打开页面无报错，主色为 `#14C9C9`。

### Task 2: HTTP 客户端与认证
- **Action**: `api/http.ts`（请求注入 token、响应统一解析 `error.message`、401 清会话跳登录）；`useAuthStore`（Setup Store，localStorage 持久化 token + user）；`LoginPage.vue`（用户名/密码 → `POST /api/auth/login`，成功后跳 `route.query.redirect ?? '/'`）；全局守卫：未登录 → `/login?redirect=to.fullPath`，`meta.requiresAdmin` 且非 admin → 403 处理。
- **Mirror**: 错误格式 `{"error":{"message"}}`；不做 token 自动刷新（JWT TTL 到期重新登录即可，保持简单）。
- **Validate**: 浏览器登录 admin 成功；未登录访问 `/admin/users` 跳登录页且登录后回跳原页。

### Task 3: 布局与共享组件
- **Action**: `AdminLayout.vue`（a-layout：侧边菜单按 `role` 过滤「我的」/「管理」两组）；`StatusTag`（1/2 → 启用/禁用，props 类型化）、`QuotaText`（微美元 → `$x.xx`，0/-1 显示「不限额」）。
- **Mirror**: 展示型组件只接 props 发 events，不访问 store/API。
- **Validate**: admin 与 user 两种角色登录后菜单项不同。

### Task 4: 工具函数、分页 composable 与单测
- **Action**: `utils/quota.ts`（微美元换算、不限额语义）；`usePagination.ts`（封装 page/page_size 参数、{items,total} 响应、Arco Table 分页 props + loading 状态）；`utils/quota.test.ts` 覆盖 0 / -1 / 1234567 等边界。
- **Mirror**: composable 返回响应式值；后续所有列表页复用同一分页模式。
- **Validate**: `pnpm test` 全绿。

### Task 5: 用户自助页
- **Action**: `MyKeysPage`（列表 + 创建表单：name/quota/allowed_models/expires_in_days；创建成功后弹窗展示一次性明文 `key` 与复制按钮；删除二次确认）；`MyLogsPage`（时间范围/模型/错误过滤 + 分页表格，过滤条件放 computed）；`MyUsagePage`（按天×模型统计表格 + 额度余量卡片，来自 `/user/me`）。
- **Mirror**: 明文仅创建时返回一次（`docs/api/user.md`）；列表页统一走 `usePagination`。
- **Validate**: 浏览器走通 创建 Key → 用 Key 调一次 `/v1/chat/completions` → 日志与统计可见。

### Task 6: 管理页 — 用户与部门
- **Action**: `UsersPage`（分页列表/创建/编辑/重置密码/配额设置/所属部门多选，PUT `/users/:id/departments`）；`DepartmentsPage`（a-tree 展示树 + 创建/编辑/移动/删除 + 查看部门成员）。
- **Mirror**: `status` 1/2；用户类型按文档示例（注意 PascalCase 字段例外）。
- **Validate**: 建部门 → 建用户并挂到部门 → 部门树与成员正确显示。

### Task 7: 管理页 — Key / 渠道 / 价目
- **Action**: `KeysPage`（全平台 Key 列表 + 创建/编辑/删除）；`ProvidersPage`（渠道 CRUD + `/providers/types` 类型下拉）；`PricesPage`（价目列表 + upsert 表单 + 删除）。
- **Validate**: 各页 CRUD 走通一遍。

### Task 8: 管理页 — 日志与统计 + Dashboard
- **Action**: `LogsPage`（时间范围/model/keyword/is_error 过滤 + 详情展开）；`DashboardPage`（admin 显示平台统计 + TTFT 卡片；user 显示个人统计）；表格先行，图表（echarts）留作后续增量。
- **Validate**: 产生若干请求后，Dashboard 与 Logs 数据正确。

### Task 9: 端到端验证
- **Action**: 启动 `go run ./cmd/gateway` + `pnpm dev`，用 chrome-devtools 走查：登录 → 自助三页 → 管理七页的黄金路径与 401/403 分支。
- **Validate**: `pnpm type-check && pnpm test && pnpm build` 全部通过。

## Validation
```bash
cd frontend && pnpm install
pnpm type-check
pnpm test
pnpm build
go run ./cmd/gateway   # 另一终端，需 config.yaml
pnpm dev               # 浏览器走查
```

## Risks
| Risk | Likelihood | Mitigation |
|---|---|---|
| ctx7 本次 fetch 失败，Arco Design 安装/主题细节基于既有知识 | 高 | Task 1 实施时对照官方文档校验；主题定制优先用 CSS 变量 `--primary-*` 方案（文档化程度最高） |
| `User`/`UserDeptView` 字段 PascalCase 例外导致类型对不上 | 中 | 每个端点类型独立按文档示例定义，发现偏差只改对应模块 |
| 角色/权限分支遗漏（user 可见管理页） | 中 | 路由 meta 守卫 + Task 9 用双角色走查 |
| 管理接口现返回 `PasswordHash`（PRD 已知风险，M4 修复） | 已存在 | 前端类型中不声明、不展示该字段 |
| 价目/配额单位混乱（微美元 vs 美元） | 中 | 统一走 `utils/quota.ts`，单测锁语义 |

## Out of Scope（后续增量）
- 图表可视化（echarts）、SSO 登录（M4）、审计日志（M4）、部署集成（网关托管静态资源 / nginx）、国际化。

## Acceptance
- [ ] 所有任务完成
- [ ] `pnpm type-check` / `pnpm test` / `pnpm build` 通过
- [ ] 浏览器双角色走查黄金路径通过
- [ ] 遵循后端契约（分页/错误/状态值/额度单位），未自造接口格式
- [ ] 代码符合 vue-patterns 规范（Composition API、类型化 props/emits、Setup Store、稳定 v-for key）
