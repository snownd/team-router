# 管理台 API：用户管理（`/api/admin/users`）

需要 JWT 认证且角色为 `admin`。

> ⚠️ **序列化提示**：`user` 对象当前按 Go 导出字段名序列化（`ID`、`Username`、`PasswordHash`…，非 snake_case），且包含 `PasswordHash` 字段。前端对接请勿展示该字段；后续版本可能统一调整。

## GET /api/admin/users

用户列表（附部门信息）。

**查询参数**：`keyword`（用户名/邮箱模糊）、`page`、`page_size`。

#### 响应 200

```json
{
  "items": [
    {
      "ID": 2,
      "Username": "zhangsan",
      "DisplayName": "张三",
      "Email": "zhangsan@example.com",
      "Role": "user",
      "Status": 1,
      "Quota": 100000000,
      "UsedQuota": 12345,
      "CreatedAt": "2025-01-01T00:00:00Z",
      "UpdatedAt": "2025-01-01T00:00:00Z",
      "departments": [
        {"ID": 3, "Name": "平台研发部", "ParentID": 1, "IsPrimary": true}
      ]
    }
  ],
  "total": 1
}
```

`Quota` 单位微美元，`0` 表示不限额。

## POST /api/admin/users

创建用户（默认启用）。

#### 请求

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `username` | string | 是 | 唯一用户名 |
| `display_name` / `email` | string | 否 | 资料 |
| `password` | string | 否 | 本地登录密码（bcrypt 落库） |
| `role` | string | 否 | `admin` / `user`，其他值按 `user` 处理 |
| `quota` | int64 | 否 | 总额度（微美元） |
| `dept_ids` | int64[] | 否 | 所属部门 |
| `primary_dept_id` | int64 | 否 | 主部门，缺省取 `dept_ids[0]` |

```json
{"username": "zhangsan", "password": "secret", "dept_ids": [3]}
```

#### 响应 200

```json
{"id": 2, "username": "zhangsan"}
```

**错误**：400（`username` 缺失或用户名已存在等）。

## GET /api/admin/users/:id

用户详情。

#### 响应 200

```json
{
  "user": {"ID": 2, "Username": "zhangsan", "...": "..."},
  "departments": [{"ID": 3, "Name": "平台研发部", "ParentID": 1, "IsPrimary": true}]
}
```

**错误**：404（用户不存在）。

## PUT /api/admin/users/:id

更新用户资料（仅提交的字段生效）。

#### 请求

| 字段 | 说明 |
| --- | --- |
| `display_name` / `email` | 非空时更新 |
| `role` | `admin` / `user`，合法时更新 |
| `status` | `1` 启用 / `2` 禁用，提交时更新 |
| `quota` | 提交时重设总额度 |

#### 响应 200

```json
{"message": "ok"}
```

## PUT /api/admin/users/:id/password

重置用户密码。

#### 请求

```json
{"password": "new-secret"}
```

#### 响应 200

```json
{"message": "ok"}
```

## POST /api/admin/users/:id/quota

设置总额度或按增量充值（两者可同时提交，先设后加）。

#### 请求

| 字段 | 说明 |
| --- | --- |
| `quota` | 重设总额度（微美元） |
| `delta` | 增量调整（可为负） |

```json
{"delta": 50000000}
```

#### 响应 200

```json
{"message": "ok"}
```

## PUT /api/admin/users/:id/departments

设置用户所属部门（多部门，全量覆盖）。

#### 请求

```json
{"dept_ids": [3, 5], "primary_dept_id": 3}
```

`primary_dept_id` 缺省取 `dept_ids[0]`。

#### 响应 200

```json
{"message": "ok"}
```

**错误**：400（部门不存在等，message 带原因）。

## DELETE /api/admin/users/:id

删除用户。

#### 响应 200

```json
{"message": "ok"}
```

**错误**：400（不能删除自己）；404 复用 GET 语义。
