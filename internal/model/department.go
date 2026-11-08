package model

import "time"

// Department 部门。第一期全局单组织：树根 parent_id = NULL。
type Department struct {
	ID        int64     `gorm:"primaryKey;column:id" json:"id"`
	ParentID  *int64    `gorm:"column:parent_id" json:"parent_id"`
	Name      string    `gorm:"column:name" json:"name"`
	SortOrder int       `gorm:"column:sort_order" json:"sort_order"`
	Status    int       `gorm:"column:status" json:"status"` // 1 启用 2 禁用
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

const (
	DeptStatusEnabled  = 1
	DeptStatusDisabled = 2
)

func (Department) TableName() string { return "departments" }

// DepartmentClosure 闭包表：任意两个有祖先-后代关系的节点各一行（含自身 depth=0）。
type DepartmentClosure struct {
	AncestorID   int64 `gorm:"column:ancestor_id"`
	DescendantID int64 `gorm:"column:descendant_id"`
	Depth        int   `gorm:"column:depth"`
}

func (DepartmentClosure) TableName() string { return "department_closure" }

// DeptTreeNode 部门树节点。
type DeptTreeNode struct {
	ID        int64           `json:"id"`
	ParentID  *int64          `json:"parent_id"`
	Name      string          `json:"name"`
	SortOrder int             `json:"sort_order"`
	Status    int             `json:"status"`
	Children  []*DeptTreeNode `json:"children,omitempty"`
}

// DeptBreadcrumb 部门面包屑（祖先链）。
type DeptBreadcrumb struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
