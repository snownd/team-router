package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/model"
)

// DepartmentRepo 部门仓储，闭包表维护层级。
//
// 闭包表不变式：
//   - 每个节点 (id, id, depth=0) 一行；
//   - 节点 A 是 B 的祖先 <=> 存在 (A, B, depth=路径长度)；
//   - 所有写操作在事务中同步维护 department_closure。
type DepartmentRepo struct{ db *gorm.DB }

// NewDepartmentRepo 构造。
func NewDepartmentRepo(db *gorm.DB) *DepartmentRepo { return &DepartmentRepo{db: db} }

// Create 新建部门（parent 为 0 表示根）。事务内插入节点与闭包行。
func (r *DepartmentRepo) Create(ctx context.Context, parentID int64, name string, sortOrder int) (*model.Department, error) {
	var dept model.Department
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if parentID != 0 {
			var cnt int64
			if err := tx.Model(&model.Department{}).Where("id = ?", parentID).Count(&cnt).Error; err != nil {
				return err
			}
			if cnt == 0 {
				return fmt.Errorf("父部门 %d 不存在", parentID)
			}
		}
		var p *int64
		if parentID != 0 {
			p = &parentID
		}
		dept = model.Department{ParentID: p, Name: name, SortOrder: sortOrder, Status: model.DeptStatusEnabled}
		if err := tx.Create(&dept).Error; err != nil {
			return err
		}
		// 自引用 + 祖先链
		if err := tx.Exec(
			`INSERT INTO department_closure (ancestor_id, descendant_id, depth) VALUES (?, ?, 0)`,
			dept.ID, dept.ID).Error; err != nil {
			return err
		}
		if parentID != 0 {
			return tx.Exec(`
				INSERT INTO department_closure (ancestor_id, descendant_id, depth)
				SELECT a.ancestor_id, ?, a.depth + 1
				FROM department_closure a
				WHERE a.descendant_id = ?`,
				dept.ID, parentID).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &dept, nil
}

// Rename / UpdateMeta 更新部门名称、排序、状态。
func (r *DepartmentRepo) UpdateMeta(ctx context.Context, id int64, name string, sortOrder, status int) error {
	updates := map[string]any{}
	if name != "" {
		updates["name"] = name
	}
	updates["sort_order"] = sortOrder
	updates["status"] = status
	return r.db.WithContext(ctx).Model(&model.Department{}).Where("id = ?", id).Updates(updates).Error
}

// Move 将 subtree 移动到 newParent 下（newParent 为 0 表示提升为根）。
// 不允许把子树移到自身后代下（否则成环）。
func (r *DepartmentRepo) Move(ctx context.Context, id, newParentID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var node model.Department
		if err := tx.First(&node, id).Error; err != nil {
			return err
		}
		if (node.ParentID == nil && newParentID == 0) ||
			(node.ParentID != nil && *node.ParentID == newParentID) {
			return nil // no-op
		}
		if newParentID != 0 {
			if newParentID == id {
				return fmt.Errorf("不能将部门移动到自身下")
			}
			// newParent 不能是 id 的后代
			var cnt int64
			if err := tx.Raw(`
				SELECT count(*) FROM department_closure
				WHERE ancestor_id = ? AND descendant_id = ?`,
				id, newParentID).Scan(&cnt).Error; err != nil {
				return err
			}
			if cnt > 0 {
				return fmt.Errorf("不能将部门移动到其子部门下")
			}
		}

		var newParent *int64
		if newParentID != 0 {
			newParent = &newParentID
		}
		if err := tx.Model(&model.Department{}).Where("id = ?", id).
			Update("parent_id", newParent).Error; err != nil {
			return err
		}

		// 删除子树与“旧侧外部祖先”的关系，再插入新祖先链：
		//   删除：ancestor 不属于子树 && descendant 属于子树
		//   插入：新父链上每个祖先 -> 子树中每个节点，depth = (祖先->newParent 的 depth) + (子树内部相对 depth) + 1
		if err := tx.Exec(`
			DELETE FROM department_closure AS c
			USING department_closure AS subtree
			WHERE c.descendant_id = subtree.descendant_id
			  AND c.ancestor_id NOT IN (
			      SELECT descendant_id FROM department_closure
			      WHERE ancestor_id = ?
			  )
			  AND subtree.ancestor_id = ?`,
			id, id).Error; err != nil {
			return err
		}
		if newParentID == 0 {
			return nil
		}
		return tx.Exec(`
			INSERT INTO department_closure (ancestor_id, descendant_id, depth)
			SELECT sup.ancestor_id, sub.descendant_id, sup.depth + sub.depth + 1
			FROM department_closure sup
			CROSS JOIN department_closure sub
			WHERE sup.descendant_id = ? AND sub.ancestor_id = ?
			ON CONFLICT DO NOTHING`,
			newParentID, id).Error
	})
}

// Delete 递归删除子树。
// 闭包表与 user_departments 的 FK 均带 ON DELETE CASCADE，
// 删除子树节点后关联行由数据库自动级联清理。
func (r *DepartmentRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Exec(`
		DELETE FROM departments
		WHERE id IN (
		    SELECT descendant_id FROM department_closure WHERE ancestor_id = ?
		)`, id).Error
}

// List 全量部门（用于构建树）。
func (r *DepartmentRepo) List(ctx context.Context) ([]model.Department, error) {
	var depts []model.Department
	err := r.db.WithContext(ctx).Order("sort_order ASC, id ASC").Find(&depts).Error
	return depts, err
}

// DescendantIDs 某部门的全部后代 ID（不含自身），depthLimit<=0 表示不限。
func (r *DepartmentRepo) DescendantIDs(ctx context.Context, id int64, depthLimit int) ([]int64, error) {
	q := r.db.WithContext(ctx).Table("department_closure").
		Select("descendant_id").Where("ancestor_id = ? AND depth > 0", id)
	if depthLimit > 0 {
		q = q.Where("depth <= ?", depthLimit)
	}
	var ids []int64
	err := q.Scan(&ids).Error
	return ids, err
}

// Ancestors 某部门的祖先链（根在前），即面包屑。
func (r *DepartmentRepo) Ancestors(ctx context.Context, id int64) ([]model.DeptBreadcrumb, error) {
	var out []model.DeptBreadcrumb
	err := r.db.WithContext(ctx).Raw(`
		SELECT d.id, d.name
		FROM department_closure c
		JOIN departments d ON d.id = c.ancestor_id
		WHERE c.descendant_id = ?
		ORDER BY c.depth DESC`,
		id).Scan(&out).Error
	return out, err
}

// Children 直接子部门。
func (r *DepartmentRepo) Children(ctx context.Context, id int64) ([]model.Department, error) {
	var depts []model.Department
	q := r.db.WithContext(ctx).Model(&model.Department{})
	if id == 0 {
		q = q.Where("parent_id IS NULL")
	} else {
		q = q.Where("parent_id = ?", id)
	}
	err := q.Order("sort_order ASC, id ASC").Find(&depts).Error
	return depts, err
}

// SetUserDepartments 设置用户所属部门集合（全量替换），并保证至多一个主部门。
func (r *DepartmentRepo) SetUserDepartments(ctx context.Context, userID int64, deptIDs []int64, primaryID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 校验部门存在
		if len(deptIDs) > 0 {
			var cnt int64
			if err := tx.Model(&model.Department{}).Where("id IN ?", deptIDs).Count(&cnt).Error; err != nil {
				return err
			}
			if cnt != int64(len(deptIDs)) {
				return fmt.Errorf("存在无效部门 ID")
			}
		}
		if err := tx.Where("user_id = ?", userID).Delete(&model.UserDepartment{}).Error; err != nil {
			return err
		}
		for _, d := range deptIDs {
			ud := model.UserDepartment{UserID: userID, DeptID: d, IsPrimary: d == primaryID}
			if err := tx.Create(&ud).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// UserDepartments 查询用户所属部门。
func (r *DepartmentRepo) UserDepartments(ctx context.Context, userID int64) ([]model.UserDeptView, error) {
	var out []model.UserDeptView
	err := r.db.WithContext(ctx).Raw(`
		SELECT d.id, d.name, d.parent_id, ud.is_primary
		FROM user_departments ud
		JOIN departments d ON d.id = ud.dept_id
		WHERE ud.user_id = ?
		ORDER BY ud.is_primary DESC, d.id ASC`,
		userID).Scan(&out).Error
	return out, err
}

// DeptUserIDs 查询部门（可选含子树）下的用户 ID。
func (r *DepartmentRepo) DeptUserIDs(ctx context.Context, deptID int64, includeSubtree bool) ([]int64, error) {
	q := `SELECT DISTINCT ud.user_id FROM user_departments ud`
	if includeSubtree {
		q += ` JOIN department_closure c ON c.descendant_id = ud.dept_id AND c.ancestor_id = ?`
	} else {
		q += ` WHERE ud.dept_id = ?`
	}
	var ids []int64
	err := r.db.WithContext(ctx).Raw(q, deptID).Scan(&ids).Error
	return ids, err
}
