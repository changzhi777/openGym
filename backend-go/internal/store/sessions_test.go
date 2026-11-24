// sessions_test.go — 会话与设备管理(store 层)的 PG 集成测试。
//
// 前置:TEST_PG_ADMIN_URL 指向管理连接(compose.go.yml 的 pg 起;缺省 skip,
// 与 agent 包 PG 测试同模式)。每测试独立库,迁移跑到最新(含 0015_sessions)。
package store_test

import (
	"context"
		"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/changzhi777/openGym/backend-go/internal/store"
)

// pgx5Scheme 迁移 driver 需 pgx5://(main.go pgx5URL 的测试侧拷贝)。
func pgx5Scheme(u string) string {
	return strings.Replace(u, "postgres://", "pgx5://", 1)
}

var dbSeq atomic.Int64

// newTestDB 每测试独立库(建→迁移→测试→删),互不污染。
func newTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	admin := os.Getenv("TEST_PG_ADMIN_URL")
	if admin == "" {
		t.Skip("无 TEST_PG_ADMIN_URL — 起依赖: docker compose -f compose.go.yml up -d pg")
	}
	name := "sess_test_" + strconv.FormatInt(dbSeq.Add(1), 10)
	adm, err := pgxpool.New(context.Background(), admin)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	if _, err := adm.Exec(context.Background(), `CREATE DATABASE `+name); err != nil {
		adm.Close()
		t.Fatalf("create db: %v", err)
	}
	url := swapDBName(admin, name)
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		adm.Close()
		t.Fatalf("pool: %v", err)
	}
	if err := store.Migrate(pgx5Scheme(url)); err != nil {
		adm.Close()
		pool.Close()
		t.Fatalf("migrate: %v", err)
	}
	cleanup := func() {
		pool.Close()
		adm.Exec(context.Background(), `DROP DATABASE `+name+` WITH (FORCE)`)
		adm.Close()
	}
	t.Cleanup(cleanup)
	return pool
}

// swapDBName 把管理连接串里的库名段替换为测试库名。
func swapDBName(url, name string) string {
	i := strings.LastIndex(url, "/")
	return url[:i+1] + name
}

func seedUser(t *testing.T, db *pgxpool.Pool, id, name string) {
	t.Helper()
	if _, err := db.Exec(context.Background(),
		`INSERT INTO users (id, name, email) VALUES ($1,$2,$3) ON CONFLICT (id) DO NOTHING`,
		id, name, name+"@muhe.fit"); err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	seedUser(t, db, "u-1", "用户一")
	seedUser(t, db, "u-2", "用户二")

	// 两条会话入同一账号:列表新者在前。
	for _, sid := range []string{"sess-a", "sess-b"} {
		if err := store.CreateSession(ctx, db, sid, "u-1", "Android 浏览器", "UA/1.0", "10.0.0.1"); err != nil {
			t.Fatalf("create %s: %v", sid, err)
		}
	}
	// 他账号一条:不得串进 u-1 的列表。
	if err := store.CreateSession(ctx, db, "sess-x", "u-2", "iOS 浏览器", "UA/2.0", "10.0.0.2"); err != nil {
		t.Fatalf("create x: %v", err)
	}

	rows, err := store.ListSessions(ctx, db, "u-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(rows))
	}
	if rows[0].ID != "sess-b" || rows[1].ID != "sess-a" {
		t.Fatalf("order want [b a], got [%s %s]", rows[0].ID, rows[1].ID)
	}

	// CheckAndTouch:已知 sid 刷新且未撤销;未知 sid 不撤销。
	if store.SessionCheckAndTouch(ctx, db, "sess-a") {
		t.Fatal("sess-a should not be revoked")
	}
	if store.SessionCheckAndTouch(ctx, db, "sess-ghost") {
		t.Fatal("ghost sid should not report revoked")
	}

	// 越权防护:u-2 踢 u-1 的会话 → 无行受影响。
	if store.RevokeSession(ctx, db, "sess-a", "u-2") {
		t.Fatal("cross-user revoke must not report success")
	}

	// 属主踢出 sess-b:列表剩一条,CheckAndTouch 报已撤销。
	if !store.RevokeSession(ctx, db, "sess-b", "u-1") {
		t.Fatal("revoke sess-b failed")
	}
	rows, err = store.ListSessions(ctx, db, "u-1")
	if err != nil || len(rows) != 1 || rows[0].ID != "sess-a" {
		t.Fatalf("after revoke want [sess-a], got %+v (%v)", rows, err)
	}
	if !store.SessionCheckAndTouch(ctx, db, "sess-b") {
		t.Fatal("revoked sess-b must report revoked")
	}

	// RevokeAllFor 只撤自己:u-1 全撤,u-2 的 sess-x 不受影响。
	if err := store.RevokeAllFor(ctx, db, "u-1"); err != nil {
		t.Fatalf("revoke all: %v", err)
	}
	if rows, _ = store.ListSessions(ctx, db, "u-1"); len(rows) != 0 {
		t.Fatalf("after revoke-all want 0, got %d", len(rows))
	}
	if rows, _ = store.ListSessions(ctx, db, "u-2"); len(rows) != 1 {
		t.Fatalf("u-2 sessions must be untouched, got %d", len(rows))
	}
}

func TestCascadeOnUserDelete(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	seedUser(t, db, "u-del", "将删除")
	if err := store.CreateSession(ctx, db, "sess-del", "u-del", "浏览器", "UA", "10.0.0.3"); err != nil {
		t.Fatalf("create: %v", err)
	}
	// users 行 ON DELETE CASCADE:删号连带会话行(注销功能的级联底座)。
	if _, err := db.Exec(ctx, `DELETE FROM users WHERE id='u-del'`); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if rows, _ := store.ListSessions(ctx, db, "u-del"); len(rows) != 0 {
		t.Fatalf("sessions must cascade away, got %d", len(rows))
	}
}
