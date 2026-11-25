// G2 真库批③：team 域真库面——三表生命周期 / 转让原子与竞态 / 分发状态机 /
// 团码与成员软上限。门控与隔离纪律见 store/dbtest 包头。
package team_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/changzhi777/openGym/backend-go/internal/store"
	"github.com/changzhi777/openGym/backend-go/internal/store/dbtest"
	"github.com/changzhi777/openGym/backend-go/internal/team"
)

func seedUsers(t *testing.T, pool *pgxpool.Pool, uids ...string) {
	t.Helper()
	ctx := t.Context()
	for _, uid := range uids {
		if err := store.InsertUser(ctx, pool, uid, uid); err != nil {
			t.Fatalf("InsertUser %s: %v", uid, err)
		}
	}
}

// TestRealTeamLifecycle 三表生命周期:建团事务 / 部分唯一索引 / 软删复活 /
// 成员列表 JOIN 昵称 / 团名与确认卡。
func TestRealTeamLifecycle(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Clean(t, pool)
	ctx := t.Context()
	seedUsers(t, pool, "coach", "m1", "m2")

	// 建团(单事务:teams + owner 行)
	id, err := team.CreateTeam(ctx, pool, "coach", "沐禾一班")
	if err != nil || id == "" {
		t.Fatalf("CreateTeam: %v id=%s", err, id)
	}

	// W7 一教练一活团:部分唯一索引,二建活团必冲突
	if _, err := team.CreateTeam(ctx, pool, "coach", "二团"); err == nil {
		t.Fatal("同 coach 二建活团应唯一冲突")
	}

	// 教练/学员双视角
	if tr, err := team.TeamOfOwner(ctx, pool, "coach"); err != nil || tr.Name != "沐禾一班" {
		t.Fatalf("TeamOfOwner: %+v err=%v", tr, err)
	}
	if _, err := team.TeamOfOwner(ctx, pool, "m1"); !errors.Is(err, team.ErrNoLiveTeam) {
		t.Fatalf("非教练应 ErrNoLiveTeam, got %v", err)
	}

	// 入团 → 成员列表(JOIN display_name + scope 默认 basic)
	if err := team.AddMember(ctx, pool, id, "m1"); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if !team.IsLiveMember(ctx, pool, id, "m1") {
		t.Fatal("m1 应为活成员")
	}
	ms, err := team.Members(ctx, pool, id)
	if err != nil || len(ms) != 2 {
		t.Fatalf("Members: %v (n=%d)", err, len(ms))
	}
	// owner 在首(joined_at 早),display_name 为注册昵称
	if ms[0].Role != "owner" || ms[0].DisplayName == "" {
		t.Fatalf("owner 行不符: %+v", ms[0])
	}
	if ms[1].Role != "member" || ms[1].Scope["tier"] != "basic" {
		t.Fatalf("member 行不符(scope 默认 basic): %+v", ms[1])
	}

	// 软删 → 学员视角失效 → 再入团 ON CONFLICT 复活(历史连续)
	if err := team.EndMembership(ctx, pool, id, "m1"); err != nil {
		t.Fatalf("EndMembership: %v", err)
	}
	if _, err := team.TeamOfMember(ctx, pool, "m1"); !errors.Is(err, team.ErrNoLiveTeam) {
		t.Fatalf("软删后学员视角应 ErrNoLiveTeam, got %v", err)
	}
	if err := team.EndMembership(ctx, pool, id, "m1"); !errors.Is(err, team.ErrNotMember) {
		t.Fatalf("重复软删应 ErrNotMember, got %v", err)
	}
	if err := team.AddMember(ctx, pool, id, "m1"); err != nil {
		t.Fatalf("软删后返团应 ON CONFLICT 复活: %v", err)
	}
	if !team.IsLiveMember(ctx, pool, id, "m1") {
		t.Fatal("复活后应为活成员")
	}

	// 团名与确认卡(团长昵称脱敏面)
	if err := team.RenameTeam(ctx, pool, id, "沐禾二班"); err != nil {
		t.Fatalf("RenameTeam: %v", err)
	}
	name, ownerDN, err := team.TeamWithOwner(ctx, pool, id)
	if err != nil || name != "沐禾二班" || ownerDN == "" {
		t.Fatalf("TeamWithOwner: name=%s ownerDN=%s err=%v", name, ownerDN, err)
	}
	if _, err := team.TeamOfMember(ctx, pool, "m1"); err != nil {
		t.Fatalf("复活后学员视角应可见: %v", err)
	}
}

// TestRealTransfer 转让原子三 UPDATE:正常互换 / 竞态前置漂移回滚 / owner 行唯一。
func TestRealTransfer(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Clean(t, pool)
	ctx := t.Context()
	seedUsers(t, pool, "coach", "m1", "m2")
	id, err := team.CreateTeam(ctx, pool, "coach", "团")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	team.AddMember(ctx, pool, id, "m1")
	team.AddMember(ctx, pool, id, "m2")

	// 正常转让:owner 互换,原团长留团为 member(W12)
	if err := team.TransferTeam(ctx, pool, id, "coach", "m1"); err != nil {
		t.Fatalf("TransferTeam: %v", err)
	}
	ms, _ := team.Members(ctx, pool, id)
	roles := map[string]string{}
	for _, m := range ms {
		roles[m.UID] = m.Role
	}
	if roles["m1"] != "owner" || roles["coach"] != "member" {
		t.Fatalf("转让后角色不符: %v", roles)
	}
	if tr, _ := team.TeamOfOwner(ctx, pool, "m1"); tr.ID != id {
		t.Fatalf("新团长视角应见团: %+v", tr)
	}

	// 竞态前置漂移:发起人已非 owner 时再次转让 → ErrStaleOwner 整体回滚,
	// 不得留下双 owner 行(role='owner' 无唯一约束,静默 0 行即数据损坏)
	if err := team.TransferTeam(ctx, pool, id, "coach", "m2"); !errors.Is(err, team.ErrStaleOwner) {
		t.Fatalf("非 owner 发起转让应 ErrStaleOwner, got %v", err)
	}
	ms, _ = team.Members(ctx, pool, id)
	owners := 0
	for _, m := range ms {
		if m.Role == "owner" {
			owners++
		}
	}
	if owners != 1 {
		t.Fatalf("owner 行必须唯一, got %d: %v", owners, ms)
	}
	if _, err := team.TeamOfOwner(ctx, pool, "m1"); err != nil {
		t.Fatalf("回滚后 m1 仍应为 owner: %v", err)
	}

	// 目标非活成员 → ErrNotMember
	team.EndMembership(ctx, pool, id, "m2")
	if err := team.TransferTeam(ctx, pool, id, "m1", "m2"); !errors.Is(err, team.ErrNotMember) {
		t.Fatalf("目标非活成员应 ErrNotMember, got %v", err)
	}
}

// TestRealPlans 分发状态机:distributed→loaded/declined,重分发版本链,撤回边界。
func TestRealPlans(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Clean(t, pool)
	ctx := t.Context()
	seedUsers(t, pool, "coach", "m1")
	id, _ := team.CreateTeam(ctx, pool, "coach", "团")
	team.AddMember(ctx, pool, id, "m1")

	snap := []byte(`{"name":"PPL首周","routines":[{"id":"r1"}]}`)
	aid, err := team.CreateAssignment(ctx, pool, id, "coach", "m1", "PPL", snap)
	if err != nil || aid == "" {
		t.Fatalf("CreateAssignment: %v", err)
	}

	// 教练/成员双视角(err 显形——行扫描错被吞只见空表)
	rows, err := team.AssignmentsOfTeam(ctx, pool, id)
	if err != nil {
		t.Fatalf("AssignmentsOfTeam err: %v", err)
	}
	if len(rows) != 1 || rows[0].Status != "distributed" {
		t.Fatalf("AssignmentsOfTeam: %+v", rows)
	}
	if rows, err := team.AssignmentsOfMember(ctx, pool, "m1"); err != nil || len(rows) != 1 {
		t.Fatalf("AssignmentsOfMember: %v (n=%d)", err, len(rows))
	}

	// 重分发:新行 + 旧行挂版本链
	aid2, err := team.CreateAssignment(ctx, pool, id, "coach", "m1", "PPL", snap)
	if err != nil {
		t.Fatalf("重分发: %v", err)
	}
	team.SupersedePending(ctx, pool, id, "m1", "PPL", aid2)
	rows, err = team.AssignmentsOfTeam(ctx, pool, id)
	if err != nil {
		t.Fatalf("重分发查询 err: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("重分发应两行, got %d", len(rows))
	}
	for _, r := range rows {
		if r.ID == aid && (r.SupersededBy == nil || *r.SupersededBy != aid2) {
			t.Fatalf("旧行应挂 superseded_by=aid2: %+v", r)
		}
	}

	// 结算 loaded;重复结算 ErrSettled
	if err := team.AckAssignment(ctx, pool, aid2, "m1", "loaded"); err != nil {
		t.Fatalf("AckAssignment: %v", err)
	}
	if err := team.AckAssignment(ctx, pool, aid2, "m1", "declined"); !errors.Is(err, team.ErrSettled) {
		t.Fatalf("重复结算应 ErrSettled, got %v", err)
	}
	// 越链结算旧行(已 superseded)——Ack 只看 status='distributed',旧行已挂链仍是
	// distributed → 可结算;契约上成员只拉 pending 首行,此处锁 SQL 语义
	if err := team.AckAssignment(ctx, pool, aid, "m1", "loaded"); err != nil {
		t.Fatalf("旧行(distributed)可结算应成功: %v", err)
	}

	// 撤回:仅 distributed 可删;已结算 ErrNotFound
	aid3, _ := team.CreateAssignment(ctx, pool, id, "coach", "m1", "FB", snap)
	if err := team.RevokeDistributed(ctx, pool, aid3, id); err != nil {
		t.Fatalf("撤回 distributed: %v", err)
	}
	if err := team.RevokeDistributed(ctx, pool, aid2, id); !errors.Is(err, team.ErrNotFound) {
		t.Fatalf("撤回已结算应 ErrNotFound, got %v", err)
	}

	// 归档团后成员视角不可见(活团过滤)
	if _, err := pool.Exec(ctx, `UPDATE teams SET archived_at=now() WHERE id=$1`, id); err != nil {
		t.Fatalf("归档: %v", err)
	}
	if rows, _ := team.AssignmentsOfMember(ctx, pool, "m1"); len(rows) != 0 {
		t.Fatalf("归档团分发行不应可见, got %d", len(rows))
	}
}

// TestRealPlansChained 事务版分发(审查批 1.0):CreateAssignmentChained 单事务出行
// + 挂链——同员同名重分发后恰一条 pending(旧行必已挂 superseded_by)。
func TestRealPlansChained(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Clean(t, pool)
	ctx := t.Context()
	seedUsers(t, pool, "coach", "m1")
	id, _ := team.CreateTeam(ctx, pool, "coach", "团")

	snap := []byte(`{"name":"PPL首周","routines":[{"id":"r1"}]}`)
	aid, err := team.CreateAssignmentChained(ctx, pool, id, "coach", "m1", "PPL", snap)
	if err != nil || aid == "" {
		t.Fatalf("CreateAssignmentChained: %v", err)
	}
	aid2, err := team.CreateAssignmentChained(ctx, pool, id, "coach", "m1", "PPL", snap)
	if err != nil {
		t.Fatalf("重分发: %v", err)
	}

	rows, err := team.AssignmentsOfTeam(ctx, pool, id)
	if err != nil {
		t.Fatalf("AssignmentsOfTeam err: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应两行, got %d", len(rows))
	}
	pending := 0
	for _, r := range rows {
		if r.ID == aid && (r.SupersededBy == nil || *r.SupersededBy != aid2) {
			t.Fatalf("旧行应在同事务挂 superseded_by=aid2: %+v", r)
		}
		if r.Status == "distributed" && r.SupersededBy == nil {
			pending++
		}
	}
	if pending != 1 {
		t.Fatalf("事务挂链后 pending 应恰一条, got %d", pending)
	}
}

// TestRealInviteAndCap 团码(内存后端):形状/清洗/一次性/一团一活码/Revoke +
// W8 软上限 50(活成员计数)。
func TestRealInviteAndCap(t *testing.T) {
	pool := dbtest.Open(t)
	dbtest.Clean(t, pool)
	ctx := t.Context()
	seedUsers(t, pool, "coach", "m1")
	id, _ := team.CreateTeam(ctx, pool, "coach", "团")

	// normalizeCode 清洗(§13.3 手输容错):去空格横线+大写+8 位补横线
	for _, c := range []struct{ in, want string }{
		{"abcd-ef23", "ABCD-EF23"},
		{" ab cd ef 23 ", "ABCD-EF23"}, // 清理后恰 8 位 → 补横线
		{"abcdef23", "ABCD-EF23"},
		{"ab-cd-ef-23", "ABCD-EF23"},
	} {
		if got := team.NormalizeCode(c.in); got != c.want {
			t.Fatalf("NormalizeCode(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	inv := team.NewInviteStore() // REDIS_ADDR 未设 → 内存后端
	code := inv.NewCode(id)
	if len(code) != 9 || code[4] != '-' {
		t.Fatalf("码形状应 XXXX-XXXX, got %q", code)
	}

	// 一团一活码:新码发旧码死
	code2 := inv.NewCode(id)
	if got := inv.Take(code); got != "" {
		t.Fatalf("旧码应作废, got teamID=%q", got)
	}
	if got := inv.Take(code2); got != id {
		t.Fatalf("新码应兑出 teamID, got %q", got)
	}
	// 一次性
	if got := inv.Take(code2); got != "" {
		t.Fatalf("码应一次性, got %q", got)
	}
	// 解散全灭
	code3 := inv.NewCode(id)
	inv.RevokeTeam(id)
	if got := inv.Take(code3); got != "" {
		t.Fatalf("RevokeTeam 后应全灭, got %q", got)
	}

	// W8 软上限:活成员 50 后 AddMember → ErrTeamFull(cap 用户先建档过 uid FK)
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, name) SELECT 'cap'||g, 'cap'||g FROM generate_series(1,50) g`); err != nil {
		t.Fatalf("造 cap 用户: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO team_members (team_id, uid, role)
		 SELECT $1, 'cap'||g, 'member' FROM generate_series(1,50) g`, id); err != nil {
		t.Fatalf("造 50 成员: %v", err)
	}
	if err := team.AddMember(ctx, pool, id, "m1"); !errors.Is(err, team.ErrTeamFull) {
		t.Fatalf("满员应 ErrTeamFull, got %v", err)
	}
	// 软删一个腾位 → 可入(复活语义不受上限误伤)
	team.EndMembership(ctx, pool, id, "cap1")
	team.EndMembership(ctx, pool, id, "cap2")
	if err := team.AddMember(ctx, pool, id, "m1"); err != nil {
		t.Fatalf("腾位后应可入: %v", err)
	}
}
