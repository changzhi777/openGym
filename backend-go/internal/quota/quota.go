// Package quota — 计费资源的周期硬限(fork P5 瘦尾)。
// 方案 §配额:Redis 周期键(月,TTL 兜底) + 60s 异步落账 usage_counters;AI 教练为首个资源。
// Redis 不可用时退化内存计数(自部署可用性优先,宁多记不挡路);落账失败静默重试下一轮。
package quota

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	// CoachMonthly AI 教练任务的默认月配额(宽松起步,admin 可按用户调 user_quotas)。
	CoachMonthly int64 = 100

	flushEvery = 60 * time.Second
	keyTTL     = 35 * 24 * time.Hour // 覆盖整月 + 时差余量;跨月自然换键
)

type Service struct {
	DB  *pgxpool.Pool
	RDB *redis.Client

	mu    sync.Mutex
	pend  map[string]int64 // "uid|resource|period" → 本轮增量(落账用)
	limit func(uid string) int64
}

func New(db *pgxpool.Pool, rdb *redis.Client) *Service {
	s := &Service{DB: db, RDB: rdb, pend: map[string]int64{}, limit: defaultLimit}
	return s
}

func defaultLimit(string) int64 { return CoachMonthly }

// SetLimitSource 限额来源注入(admin 的 user_quotas 读取;测试可替换)。
func (s *Service) SetLimitSource(f func(uid string) int64) {
	if f != nil {
		s.limit = f
	}
}

func period(t time.Time) string { return t.Format("2006-01") }

// Allow 计一次并判定是否放行。resource 如 "coach"。
// 计数发生在判定前:被拒的尝试也记账(它们同样是资源消耗的信号)。
// Redis 故障时退化为本进程计数(pend 近似,宁少不挡;恢复后自然接续 Redis 真值)。
func (s *Service) Allow(ctx context.Context, uid, resource string) (ok bool, used, limit int64) {
	limit = s.limit(uid)
	key := "quota:" + uid + ":" + resource + ":" + period(time.Now())
	var n int64
	v, err := s.RDB.Incr(ctx, key).Result()
	if err == nil {
		n = v
		if v == 1 {
			s.RDB.Expire(ctx, key, keyTTL)
		}
	}
	s.mu.Lock()
	s.pend[key]++ // 落账增量(Redis 成败都记;flush 进 PG)
	if err != nil {
		n = s.pend[key] // 故障兜底:本进程视角的计数
	}
	s.mu.Unlock()
	return n <= limit, n, limit
}

// Flush 把 pend 增量 upsert 进 usage_counters(resource/period 从键解析);main 的 goroutine 周期调。
func (s *Service) Flush(ctx context.Context) {
	s.mu.Lock()
	batch := s.pend
	s.pend = map[string]int64{}
	s.mu.Unlock()
	for k, n := range batch {
		uid, resource, per, ok := splitKey(k)
		if !ok {
			continue
		}
		_, err := s.DB.Exec(ctx, `
			INSERT INTO usage_counters (user_id, resource, period, used) VALUES ($1,$2,$3,$4)
			ON CONFLICT (user_id, resource, period) DO UPDATE SET used = usage_counters.used + $4`,
			uid, resource, per, n)
		if err != nil {
			log.Printf("quota: flush %s: %v", k, err)
			s.mu.Lock()
			s.pend[k] += n // 下轮重试
			s.mu.Unlock()
		}
	}
}

// Run 周期落账(每 60s;进程退出即停,增量已在 Redis 兜住)。
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(flushEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			s.Flush(context.Background())
			return
		case <-t.C:
			s.Flush(ctx)
		}
	}
}

func splitKey(k string) (uid, resource, per string, ok bool) {
	parts := strings.Split(k, ":") // quota:<uid>:<resource>:<period>
	if len(parts) != 4 || parts[0] != "quota" {
		return "", "", "", false
	}
	return parts[1], parts[2], parts[3], true
}

// ParseLimit helper:user_quotas.limits(jsonb)里的 ai_calls_month。
func ParseLimit(raw []byte, fallback int64) int64 {
	if len(raw) == 0 {
		return fallback
	}
	// 最小 json 提取({"ai_calls_month":N});畸形输入一律回默认。
	s := string(raw)
	i := indexOf(s, `"ai_calls_month"`)
	if i < 0 {
		return fallback
	}
	rest := s[i+len(`"ai_calls_month"`):]
	for len(rest) > 0 && (rest[0] == ' ' || rest[0] == ':' || rest[0] == '"') {
		rest = rest[1:]
	}
	j := 0
	for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
		j++
	}
	if j == 0 {
		return fallback
	}
	if n, err := strconv.ParseInt(rest[:j], 10, 64); err == nil && n >= 0 {
		return n
	}
	return fallback
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
