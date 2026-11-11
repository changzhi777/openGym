// session_pg_test.go — 会话存取层 + 提议确认链路的 PG 集成测试。
//
// 前置:TEST_PG_ADMIN_URL 指向管理连接(compose.go.yml 的 pg 起;缺省 skip 与
// worker e2e 同模式)。每测试独立库(testmode.Setup),迁移跑到最新含 0013。
package agent

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/changzhi777/openGym/backend-go/internal/llm"
	"github.com/changzhi777/openGym/backend-go/internal/store"
	"github.com/changzhi777/openGym/backend-go/internal/testmode"
)

// pgx5Scheme 迁移 driver 需 pgx5://(main.go pgx5URL 的测试侧拷贝,勿 import main)。
func pgx5Scheme(u string) string {
	return strings.Replace(u, "postgres://", "pgx5://", 1)
}

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	admin := os.Getenv("TEST_PG_ADMIN_URL")
	if admin == "" {
		t.Skip("无 TEST_PG_ADMIN_URL — 起依赖: docker compose -f compose.go.yml up -d pg redis")
	}
	url, _, _ := testmode.Setup(admin, t.TempDir())
	if err := store.Migrate(pgx5Scheme(url)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

const testUID = "u-test-1"

func TestSessionLifecycle(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	// 外键 users(id):测试用户落一行(seed 表结构里 users 主键 text)。
	if _, err := db.Exec(ctx, `INSERT INTO users (id, name, email) VALUES ($1,$2,$3)
		ON CONFLICT (id) DO NOTHING`, testUID, "测试用户", "agent-test@muhe.fit"); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	sess, err := CreateSession(ctx, db, testUID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(sess.ID) != 16 || sess.State != StateActive {
		t.Fatalf("session = %+v", sess)
	}

	// 回合落库(直答轮)。
	sess.History = append(sess.History,
		llm.Message{Role: "user", Text: "今天练什么"},
		llm.Message{Role: "assistant", Text: "推举 4×8。"})
	sess.UsageIn, sess.UsageOut = 100, 30
	if err := saveTurn(ctx, db, sess); err != nil {
		t.Fatalf("saveTurn: %v", err)
	}
	got, err := GetSession(ctx, db, sess.ID, testUID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.History) != 2 || got.UsageIn != 100 || got.State != StateActive {
		t.Fatalf("roundtrip = %+v", got)
	}

	// 归属隔离:别人的 id 查不到。
	if _, err := GetSession(ctx, db, sess.ID, "u-other"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user read = %v, want ErrNotFound", err)
	}

	// 提议 → waiting_confirm。
	p := Proposal{ID: "p1", Tool: "propose_constraint", Input: map[string]any{"text": "膝伤回避深蹲"}}
	if err := AddProposal(ctx, db, sess.ID, p); err != nil {
		t.Fatalf("add proposal: %v", err)
	}
	sess.State = StateWaitingConfirm
	if err := saveTurn(ctx, db, sess); err != nil {
		t.Fatalf("saveTurn: %v", err)
	}

	// 确认(accept):约束落地即时生效 + 会话回 active。
	applied, err := DecideProposal(ctx, db, sess.ID, testUID, "p1", "accept")
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if applied != "constraint" {
		t.Fatalf("applied = %q", applied)
	}
	got, _ = GetSession(ctx, db, sess.ID, testUID)
	if got.State != StateActive {
		t.Errorf("state after confirm = %s, want active", got.State)
	}
	cons, err := ActiveConstraints(ctx, db, testUID)
	if err != nil || len(cons) != 1 || cons[0] != "膝伤回避深蹲" {
		t.Fatalf("constraints = %v (%v)", cons, err)
	}

	// 跨用户确认被拒(归属校验)。
	if _, err := DecideProposal(ctx, db, sess.ID, "u-other", "p1", "accept"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user decide = %v, want ErrNotFound", err)
	}

	// 硬读取视图只看 active:删除后即空(D17 约束即刻失效)。
	narr, cons2, err := ListMemory(ctx, db, testUID)
	if err != nil || len(cons2) != 1 || len(narr) != 0 {
		t.Fatalf("memory list = %v %v (%v)", narr, cons2, err)
	}
	if err := DeactivateMemory(ctx, db, testUID, cons2[0].ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if cons3, _ := ActiveConstraints(ctx, db, testUID); len(cons3) != 0 {
		t.Fatalf("constraint still active after delete: %v", cons3)
	}

	// usage 聚合:记账一条 chat → day.messages=1。
	if err := RecordUsage(ctx, db, testUID, "chat", 100, 30); err != nil {
		t.Fatalf("usage: %v", err)
	}
	sum, err := GetUsage(ctx, db, testUID)
	if err != nil {
		t.Fatalf("getUsage: %v", err)
	}
	if sum.Day.Messages != 1 || sum.Day.Chat != 130 || sum.Month.Chat != 130 {
		t.Fatalf("usage = %+v", sum)
	}
	if sum.Limits.MessagesPerDay != 50 || sum.Limits.TokensPerMonth != 3_000_000 {
		t.Errorf("limits = %+v", sum.Limits)
	}
}

func TestLimitsAndOverview(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `INSERT INTO users (id, name) VALUES ($1,$2)
		ON CONFLICT (id) DO NOTHING`, testUID, "测试用户"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// 默认限额。
	lim, err := GetLimits(ctx, db)
	if err != nil || lim.MessagesPerDay != 50 || lim.TokensPerMonth != 3_000_000 {
		t.Fatalf("default limits = %+v (%v)", lim, err)
	}
	// 设置 + 回读。
	if err := SetLimits(ctx, db, Limits{MessagesPerDay: 80, TokensPerMonth: 5_000_000}); err != nil {
		t.Fatalf("set: %v", err)
	}
	lim, _ = GetLimits(ctx, db)
	if lim.MessagesPerDay != 80 || lim.TokensPerMonth != 5_000_000 {
		t.Fatalf("after set = %+v", lim)
	}
	// 坏 JSON 防御:手写坏值 → 回默认。
	if _, err := db.Exec(ctx, `UPDATE instance_settings SET value='{"bad":1}' WHERE key='coach_limits'`); err != nil {
		t.Fatal(err)
	}
	lim, _ = GetLimits(ctx, db)
	if lim.MessagesPerDay != 50 {
		t.Fatalf("bad json must fall back to defaults, got %+v", lim)
	}
	// 记账两条 → 全站看板聚合 + 14 天趋势含今日。
	if err := RecordUsage(ctx, db, testUID, "chat", 100, 50); err != nil {
		t.Fatal(err)
	}
	if err := RecordUsage(ctx, db, "u-other", "review", 10, 10); err != nil {
		t.Fatal(err)
	}
	ov, err := UsageOverview(ctx, db)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if ov.Day.Messages != 2 || ov.Day.Chat != 150 || ov.Day.Review != 20 {
		t.Fatalf("overview day = %+v", ov.Day)
	}
	if ov.Month.Messages != 2 || ov.Month.Chat != 150 {
		t.Fatalf("overview month = %+v", ov.Month)
	}
	if len(ov.Trend14d) != 14 {
		t.Fatalf("trend len = %d", len(ov.Trend14d))
	}
	today := ov.Trend14d[len(ov.Trend14d)-1]
	if today.Messages != 2 || today.Tokens != 170 {
		t.Fatalf("today trend = %+v", today)
	}
	// 单用户 usage 回响生效限额(80,非默认 50)。
	sum, err := UsageSummaryFor(ctx, db, testUID)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Limits.MessagesPerDay != 80 {
		t.Fatalf("summary limits = %+v", sum.Limits)
	}
}
