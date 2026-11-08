package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/snownd/team-router/internal/database/dbtest"
	"github.com/snownd/team-router/internal/model"
	"github.com/snownd/team-router/internal/repository"
)

// testDB 返回独立临时数据库（含迁移与分区）。
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	return dbtest.Prepare(t)
}

func TestClosureTableLifecycle(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	repo := repository.NewDepartmentRepo(db)

	// 结构：
	// root
	// ├─ a
	// │  └─ a1
	// └─ b
	root, err := repo.Create(ctx, 0, "root", 0)
	if err != nil {
		t.Fatal(err)
	}
	a, err := repo.Create(ctx, root.ID, "a", 0)
	if err != nil {
		t.Fatal(err)
	}
	a1, err := repo.Create(ctx, a.ID, "a1", 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := repo.Create(ctx, root.ID, "b", 0)
	if err != nil {
		t.Fatal(err)
	}

	// 自检闭包行
	assertDepth := func(ancestor, descendant int64, want int) {
		t.Helper()
		var got int
		if err := db.Raw(`SELECT depth FROM department_closure WHERE ancestor_id=? AND descendant_id=?`,
			ancestor, descendant).Scan(&got).Error; err != nil || got != want {
			t.Errorf("closure(%d->%d) depth = %d, want %d (err=%v)", ancestor, descendant, got, want, err)
		}
	}
	assertDepth(root.ID, root.ID, 0)
	assertDepth(root.ID, a1.ID, 2)
	assertDepth(a.ID, a1.ID, 1)

	// 用计数验证不存在
	var cnt int64
	db.Raw(`SELECT count(*) FROM department_closure WHERE ancestor_id=? AND descendant_id=?`, b.ID, a1.ID).Scan(&cnt)
	if cnt != 0 {
		t.Errorf("b 不应是 a1 的祖先")
	}

	// 后代查询
	desc, err := repo.DescendantIDs(ctx, root.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(desc) != 3 {
		t.Errorf("root 后代应为 3: %v", desc)
	}

	// 祖先链（面包屑）
	bc, err := repo.Ancestors(ctx, a1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bc) != 3 || bc[0].Name != "root" || bc[2].Name != "a1" {
		t.Errorf("面包屑错误: %+v", bc)
	}

	// 移动子树：a1 -> b 下
	if err := repo.Move(ctx, a1.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	assertDepth(b.ID, a1.ID, 1)
	assertDepth(root.ID, a1.ID, 2)
	db.Raw(`SELECT count(*) FROM department_closure WHERE ancestor_id=? AND descendant_id=?`, a.ID, a1.ID).Scan(&cnt)
	if cnt != 0 {
		t.Errorf("移动后 a 不应仍是 a1 的祖先")
	}
	bc2, _ := repo.Ancestors(ctx, a1.ID)
	if len(bc2) != 3 || bc2[1].Name != "b" {
		t.Errorf("移动后面包屑错误: %+v", bc2)
	}

	// 环检测：root 不能移到 a1 下
	if err := repo.Move(ctx, root.ID, a1.ID); err == nil {
		t.Error("不能把祖先移动到后代下")
	}

	// 删除子树 b（含 a1）
	if err := repo.Delete(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	var deptCnt int64
	db.Raw(`SELECT count(*) FROM departments WHERE id IN (?,?)`, b.ID, a1.ID).Scan(&deptCnt)
	if deptCnt != 0 {
		t.Errorf("子树应被删除")
	}
	db.Raw(`SELECT count(*) FROM department_closure WHERE descendant_id IN (?,?)`, b.ID, a1.ID).Scan(&cnt)
	if cnt != 0 {
		t.Errorf("闭包行应被清除")
	}
}

func TestUserMultiDepartments(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	deptRepo := repository.NewDepartmentRepo(db)
	userRepo := repository.NewUserRepo(db)

	root, _ := deptRepo.Create(ctx, 0, "root", 0)
	d1, _ := deptRepo.Create(ctx, root.ID, "d1", 0)
	d2, _ := deptRepo.Create(ctx, root.ID, "d2", 0)

	u := model.User{Username: fmt.Sprintf("u%d", time.Now().UnixNano()), Role: model.RoleUser, Status: 1}
	if err := userRepo.Create(ctx, &u); err != nil {
		t.Fatal(err)
	}
	// 用户同时属于两个部门，主部门 d2
	if err := deptRepo.SetUserDepartments(ctx, u.ID, []int64{d1.ID, d2.ID}, d2.ID); err != nil {
		t.Fatal(err)
	}
	depts, err := deptRepo.UserDepartments(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(depts) != 2 {
		t.Fatalf("应属于 2 个部门: %+v", depts)
	}
	if !depts[0].IsPrimary || depts[0].ID != d2.ID {
		t.Errorf("主部门应为 d2: %+v", depts[0])
	}

	// 部门（含子树）成员查询
	ids, err := deptRepo.DeptUserIDs(ctx, d1.ID, false)
	if err != nil || len(ids) != 1 {
		t.Errorf("d1 成员查询错误: %v %v", ids, err)
	}
	subIDs, _ := deptRepo.DeptUserIDs(ctx, root.ID, true)
	if len(subIDs) != 1 {
		t.Errorf("root 子树成员查询错误: %v", subIDs)
	}

	// 全量替换部门
	if err := deptRepo.SetUserDepartments(ctx, u.ID, []int64{d2.ID}, d2.ID); err != nil {
		t.Fatal(err)
	}
	depts, _ = deptRepo.UserDepartments(ctx, u.ID)
	if len(depts) != 1 || depts[0].ID != d2.ID {
		t.Errorf("部门替换失败: %+v", depts)
	}

	// 无效部门报错
	if err := deptRepo.SetUserDepartments(ctx, u.ID, []int64{99999}, 0); err == nil {
		t.Error("无效部门应报错")
	}
}

func TestQuotaReserve(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepo(db)
	keyRepo := repository.NewAPIKeyRepo(db)

	u := model.User{Username: fmt.Sprintf("q%d", time.Now().UnixNano()), Role: model.RoleUser, Status: 1, Quota: 1_000}
	if err := userRepo.Create(ctx, &u); err != nil {
		t.Fatal(err)
	}
	k := model.APIKey{UserID: u.ID, KeyPrefix: "sk-test", KeyHash: fmt.Sprintf("h%d", time.Now().UnixNano()), Status: 1, Quota: 1_000}
	if err := keyRepo.Create(ctx, &k); err != nil {
		t.Fatal(err)
	}

	// 预扣 400 成功
	ok, err := keyRepo.ReserveQuota(ctx, k.ID, 400)
	if err != nil || !ok {
		t.Fatalf("预扣失败: %v %v", ok, err)
	}
	if _, err := userRepo.ReserveQuota(ctx, u.ID, 400); err != nil {
		t.Fatal(err)
	}
	// 再预扣 700 失败（余 600）
	ok, _ = keyRepo.ReserveQuota(ctx, k.ID, 700)
	if ok {
		t.Error("key 预扣应不足")
	}
	// 结算退回 200（实际 200）
	if err := keyRepo.AdjustQuota(ctx, k.ID, -200); err != nil {
		t.Fatal(err)
	}
	if err := userRepo.AdjustQuota(ctx, u.ID, -200); err != nil {
		t.Fatal(err)
	}
	var k2 model.APIKey
	db.First(&k2, k.ID)
	if k2.UsedQuota != 200 {
		t.Errorf("key used = %d, want 200", k2.UsedQuota)
	}
}

func TestRequestLogPartitionedInsert(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// 分区已由 dbtest.Prepare 预建

	repo := repository.NewRequestLogRepo(db)
	now := time.Now().UTC()
	nextMonth := now.AddDate(0, 1, 0).AddDate(0, 0, 2) // 下下月初附近也行（落在预建分区）
	logs := []model.RequestLog{
		{CreatedAt: now, UserID: 1, Model: "gpt-4o", InputTokens: 10, OutputTokens: 5, TTFTMs: 120, QuotaCost: 25},
		{CreatedAt: nextMonth, UserID: 1, Model: "claude", CacheReadTokens: 100, QuotaCost: 3},
	}
	if err := repo.InsertBatch(ctx, logs); err != nil {
		t.Fatal(err)
	}

	// 查询（分区裁剪）
	got, total, err := repo.Query(ctx, repository.LogFilter{Start: now.Add(-time.Minute), End: now.AddDate(0, 2, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("应查到 2 条: %d", total)
	}
	byModel := map[string]model.RequestLog{}
	for _, l := range got {
		byModel[l.Model] = l
	}
	if byModel["gpt-4o"].TTFTMs != 120 {
		t.Errorf("TTFT 落库错误")
	}
	if byModel["claude"].CacheReadTokens != 100 {
		t.Errorf("缓存读 token 落库错误")
	}

	// 验证行确实写入对应月分区
	var partName string
	if err := db.Raw(`SELECT tableoid::regclass::text FROM request_logs WHERE model='gpt-4o'`).Scan(&partName).Error; err != nil {
		t.Fatal(err)
	}
	wantPrefix := fmt.Sprintf("request_logs_y%dm%02d", now.Year(), int(now.Month()))
	if partName != wantPrefix {
		t.Errorf("行未落入当月分区 %s: %s", wantPrefix, partName)
	}

	// 统计
	stats, err := repo.Stats(ctx, now.Add(-time.Minute), now.AddDate(0, 2, 0), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 {
		t.Errorf("统计行错误: %+v", stats)
	}
}
