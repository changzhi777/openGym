// Package dbtest — G2 真库批帮手（裁定 C9：真库 SQL 行为面达标才切 151 生产）。
//
// 门控：TEST_PG_DSN 未设 → t.Skip——零 PG 环境（本地裸跑/CI 无服务容器）套件
// 保持全绿，护栏面不依赖本批；CI（api-ci.yml 服务容器已预埋 env）或本地
// compose 设上即点亮。
//
// 隔离纪律：
//   - TEST_PG_DSN 必须指向专用测试库（muhe_test / muhe_ci），禁指生产；
//   - go test 带 -p 1（包间串行）：各测试包共用一库，Clean 的 TRUNCATE
//     不能与并行包互踩；
//   - 每用例开头 Clean(t, pool) 清表（动态枚举，迁移加表零维护）。
//
// 迁移走 store.Migrate（内嵌 0001-0016，幂等）：首个真库用例跑齐 schema，
// 后续直通 ErrNoChange——迁移面顺带被测。
package dbtest

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/changzhi777/openGym/backend-go/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Open 返回迁移到最新 schema 的真库 pool（t.Cleanup 接管关闭，无需手动 Close）。
func Open(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN 未设——真库批跳过（护栏面不依赖本批）")
	}
	// 迁移 driver 需 pgx5://（main.go pgx5Scheme 惯例;sessions_test 同款测试侧拷贝）,
	// pgx pool 仍用 postgres:// —— 同一 DSN 双格式各喂各的。
	migrateURL := strings.Replace(dsn, "postgres://", "pgx5://", 1)
	if err := store.Migrate(migrateURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := store.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Clean 用例间清库：动态枚举 public 全表（schema_migrations 例外）后
// TRUNCATE ... RESTART IDENTITY CASCADE。
func Clean(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx, `
		SELECT string_agg(quote_ident(tablename), ', ' ORDER BY tablename)
		FROM pg_tables
		WHERE schemaname = 'public' AND tablename <> 'schema_migrations'`)
	if err != nil {
		t.Fatalf("enumerate tables: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("enumerate tables: %v", rows.Err())
	}
	var tables string
	if err := rows.Scan(&tables); err != nil {
		t.Fatalf("scan tables: %v", err)
	}
	if tables == "" {
		return
	}
	if _, err := pool.Exec(ctx, "TRUNCATE "+tables+" RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}
