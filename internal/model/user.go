// Package model 定义 GORM 实体。DDL 全部在 migrations/ 中维护，此处仅做读写映射。
package model

import "time"

// User 用户。v1 全局单组织，无组织表。
type User struct {
	ID            int64     `gorm:"primaryKey;column:id"`
	Username      string    `gorm:"column:username"`
	DisplayName   string    `gorm:"column:display_name"`
	Email         string    `gorm:"column:email"`
	PasswordHash  string    `gorm:"column:password_hash"` // 本地登录 bcrypt；SSO 用户可为空
	Role          string    `gorm:"column:role"`          // admin | user
	Status        int       `gorm:"column:status"`        // 1 启用 2 禁用
	Quota         int64     `gorm:"column:quota"`         // 总额度，微美元（1e6 = $1）
	UsedQuota     int64     `gorm:"column:used_quota"`    // 已消耗额度
	SSOProvider   string    `gorm:"column:sso_provider"`  // wecom|feishu|dingtalk|...，本地用户为空
	SSOExternalID string    `gorm:"column:sso_external_id"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

const (
	UserStatusEnabled  = 1
	UserStatusDisabled = 2
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

// TableName 显式表名。
func (User) TableName() string { return "users" }

// UserDepartment 用户-部门多对多关联（用户可同时属于多个部门）。
type UserDepartment struct {
	UserID    int64     `gorm:"column:user_id"`
	DeptID    int64     `gorm:"column:dept_id"`
	IsPrimary bool      `gorm:"column:is_primary"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (UserDepartment) TableName() string { return "user_departments" }

// UserDeptView 部门信息视图（联查用）。
type UserDeptView struct {
	ID        int64  `gorm:"column:id"`
	Name      string `gorm:"column:name"`
	ParentID  *int64 `gorm:"column:parent_id"`
	IsPrimary bool   `gorm:"column:is_primary"`
}
