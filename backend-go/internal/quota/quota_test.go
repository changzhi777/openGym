// quota 测试:键解析/限额解析为纯函数;Allow/Flush 走真 Redis/PG(测试环境缺失则跳过)。
package quota

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func TestSplitKey(t *testing.T) {
	uid, res, per, ok := splitKey("quota:u_1:coach:2026-10")
	if !ok || uid != "u_1" || res != "coach" || per != "2026-10" {
		t.Fatalf("got %q %q %q %v", uid, res, per, ok)
	}
	if _, _, _, ok := splitKey("quota:x"); ok {
		t.Fatal("short key must not parse")
	}
}

func TestParseLimit(t *testing.T) {
	cases := []struct {
		raw  string
		want int64
	}{
		{`{"ai_calls_month": 5}`, 5},
		{`{"ai_calls_month":0}`, 0}, // 0 = 明确禁用,不是默认
		{`{"other":1}`, 100},
		{``, 100},
		{`{"ai_calls_month": "x"}`, 100},
		{`{"ai_calls_month":99999999}`, 99999999},
	}
	for _, c := range cases {
		if got := ParseLimit([]byte(c.raw), 100); got != c.want {
			t.Errorf("ParseLimit(%s) = %d, want %d", c.raw, got, c.want)
		}
	}
}

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("QUOTA_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:56379" // 本地测试容器
	}
	c := redis.NewClient(&redis.Options{Addr: addr, DialTimeout: 2 * time.Second})
	if err := c.Ping(context.Background()).Err(); err != nil {
		c.Close()
		t.Skipf("redis unavailable: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestAllowCountsAndRejectsAtLimit(t *testing.T) {
	rdb := testRedis(t)
	ctx := context.Background()
	s := New(nil, rdb)
	s.SetLimitSource(func(string) int64 { return 3 })
	uid := "t_quota_" + time.Now().Format("150405.000000000")
	rdb.Del(ctx, "quota:"+uid+":coach:"+period(time.Now()))
	defer rdb.Del(ctx, "quota:"+uid+":coach:"+period(time.Now()))

	for i := 1; i <= 3; i++ {
		ok, used, limit := s.Allow(ctx, uid, "coach")
		if !ok || used != int64(i) || limit != 3 {
			t.Fatalf("call %d: ok=%v used=%d limit=%d", i, ok, used, limit)
		}
	}
	ok, used, _ := s.Allow(ctx, uid, "coach") // 第 4 次:拒,但记账
	if ok || used != 4 {
		t.Fatalf("over-limit: ok=%v used=%d", ok, used)
	}
}

func TestFlushUpsertsUsageCounters(t *testing.T) {
	rdb := testRedis(t)
	url := os.Getenv("QUOTA_DATABASE_URL")
	if url == "" {
		url = "postgres://muhe:muhe@127.0.0.1:55432/debug_rt?sslmode=disable"
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Skip("bad db url")
	}
	cfg.MaxConns = 2
	db, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Skipf("pg unavailable: %v", err)
	}
	defer db.Close()
	if db.Ping(context.Background()) != nil {
		t.Skip("pg unavailable")
	}

	ctx := context.Background()
	s := New(db, rdb)
	uid := "u_quota_t"
	per := period(time.Now())
	db.Exec(ctx, `INSERT INTO users (id,name) VALUES ($1,'quota t') ON CONFLICT DO NOTHING`, uid)
	// 幂等起点:清掉本周期行;测完连用户一起清。
	db.Exec(ctx, `DELETE FROM usage_counters WHERE user_id=$1 AND resource='coach' AND period=$2`, uid, per)
	t.Cleanup(func() {
		db.Exec(ctx, `DELETE FROM usage_counters WHERE user_id=$1 AND resource='coach' AND period=$2`, uid, per)
		db.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid)
	})

	s.Allow(ctx, uid, "coach")
	s.Allow(ctx, uid, "coach")
	s.Flush(ctx)

	var used int64
	if err := db.QueryRow(ctx,
		`SELECT used FROM usage_counters WHERE user_id=$1 AND resource='coach' AND period=$2`,
		uid, per).Scan(&used); err != nil || used != 2 {
		t.Fatalf("after flush: used=%d err=%v (want 2)", used, err)
	}
	// 再计一次再 flush:增量叠加,不是覆盖。
	s.Allow(ctx, uid, "coach")
	s.Flush(ctx)
	if err := db.QueryRow(ctx,
		`SELECT used FROM usage_counters WHERE user_id=$1 AND resource='coach' AND period=$2`,
		uid, per).Scan(&used); err != nil || used != 3 {
		t.Fatalf("after second flush: used=%d err=%v (want 3)", used, err)
	}
	db.Exec(ctx, `DELETE FROM usage_counters WHERE user_id=$1 AND resource='coach' AND period=$2`, uid, per)
}
