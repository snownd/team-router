# 管理台 API：部门管理（`/api/admin/departments`）

需要 JWT 认证且角色为 `admin`。部门为闭包表（closure table）实现的树结构，
根节点 `parent_id = null`；**一个用户可属于多个部门**（含主部门标记）。

## GET /api/admin/departments

全量部门树。

#### 响应 200

```json
{
  "tree": [
    {
      "id": 1,
      "parent_id": null,
      "name": "总公司",
      "sort_order": 0,
      "status": 1,
      "children": [
        {"id": 3, "parent_id": 1, "name": "平台研发部", "sort_order": 0, "status": 1, "children": []}
      ]
    }
  ],
  "total": 2
}
```

`children` 为空时省略；`status`：1 启用 / 2 禁用。

## POST /api/admin/departments

创建部门。

#### 请求

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `parent_id` | int64 | 否 | 父部门 ID，0 / 缺省为根 |
| `name` | string | 是 | 部门名 |
| `sort_order` | int | 否 | 排序值 |

```json
{"parent_id": 1, "name": "质量保障部", "sort_order": 2}
```

#### 响应 200

```json
{
  "id": 7,
  "parent_id": 1,
  "name": "质量保障部",
  "sort_order": 2,
  "status": 1,
  "created_at": "2025-01-01T00:00:00Z",
  "updated_at": "2025-01-01T00:00:00Z"
}
```

**错误**：400（`name` 缺失、父部门不存在等）。

## PUT /api/admin/departments/:id

更新部门元信息。

#### 请求

| 字段 | 说明 |
| --- | --- |
| `name` | 新名称 |
| `sort_order` | 排序值 |
| `status` | `0` 按启用处理；`1` 启用 / `2` 禁用 |

#### 响应 200

```json
{"message": "ok"}
```

## POST /api/admin/departments/:id/move

移动子树到新的父部门。

#### 请求

```json
{"new_parent_id": 1}
```

#### 响应 200

```json
{"message": "ok"}
```

**错误**：400（会形成环、目标部门不存在等，message 带原因）。

## DELETE /api/admin/departments/:id

删除部门——**整个子树连同成员关联一起删除**，请谨慎调用。

#### 响应 200

```json
{"message": "ok"}
```

## GET /api/admin/departments/:id/ancestors

祖先链（面包屑，从根到父节点）。

#### 响应 200

```json
{"breadcrumbs": [{"id": 1, "name": "总公司"}]}
```

## GET /api/admin/departments/:id/descendants

全部后代部门 ID。

#### 响应 200

```json
{"ids": [3, 4, 5], "count": 3}
```

## GET /api/admin/departments/:id/users

部门成员。`include_subtree=1`（或 `true`）时包含子树成员。

#### 响应 200

```json
{"items": [ {"ID": 2, "Username": "zhangsan", "...": "..."} ], "total": 1}
```

> 成员对象为 `model.User` 序列化（Go 字段名，含 `PasswordHash`），见 [admin-users.md](admin-users.md#️-序列化提示)。

无成员时返回 `{"items": [], "total": 0}`。
