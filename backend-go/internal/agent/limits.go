// limits.go — 硬限额可配化与全站用量看板(批次 5,D1/D12)。
//
// 限额存 instance_settings(key=coach_limits,value=JSON);缺省回 DefaultLimits
// (50 回合/日 + 300 万 token/月,月度口径=chat+review 合计,含审核 ×2 预算)。
// admin 治理台改值(ops+step-up),GET /api/coach/usage 回响生效值。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// limitsKey instance_settings 的键名。
const limitsKey = "coach_limits"

// GetLimits 读生效限额;未配置/坏值回默认(admin 配错不能把全站对话打死——
// 坏 JSON 按未配置处理,治理台可见的是 PUT 成功后的规范值)。
func GetLimits(ctx context.Context, db *pgxpool.Pool) (Limits, error) {
	row := db.QueryRow(ctx, `SELECT value FROM instance_settings WHERE key=$1`, limitsKey)
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		return DefaultLimits, nil // 未配置 = 默认,不是错误
	}
	var l Limits
	if err := json.Unmarshal(raw, &l); err != nil || l.MessagesPerDay <= 0 || l.TokensPerMonth <= 0 {
		return DefaultLimits, nil
	}
	return l, nil
}

// SetLimits 写生效限额(PUT 全量替换,调用方已做范围校验)。
func SetLimits(ctx context.Context, db *pgxpool.Pool, l Limits) error {
	raw, err := json.Marshal(map[string]any{"messagesPerDay": l.MessagesPerDay, "tokensPerMonth": l.TokensPerMonth})
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `INSERT INTO instance_settings (key, value, updated_at) VALUES ($1,$2,now())
		ON CONFLICT (key) DO UPDATE SET value=$2, updated_at=now()`, limitsKey, raw)
	if err != nil {
		return fmt.Errorf("agent: set limits: %w", err)
	}
	return nil
}

// UsageSummaryFor 聚合单用户用量并回响生效限额(handler 组装契约响应用)。
func UsageSummaryFor(ctx context.Context, db *pgxpool.Pool, userID string) (*UsageSummary, error) {
	limits, err := GetLimits(ctx, db)
	if err != nil {
		return nil, err
	}
	return usageAggregate(ctx, db, userID, limits)
}

// ── 全站看板(admin) ──────────────────────────────────────────────────────────

// OverviewUsage 对应契约 GET /api/admin/coach/usage-overview。
type OverviewUsage struct {
	Day      UsageStat       `json:"day"`
	Month    UsageStat       `json:"month"`
	Users    int             `json:"users"`
	Trend14d []TrendDay      `json:"trend14d"`
	Limits   Limits          `json:"-"`
}

// TrendDay 近 14 天日曲线一行。
type TrendDay struct {
	Date     string `json:"date"`
	Messages int    `json:"messages"`
	Tokens   int64  `json:"tokens"`
}

// UsageOverview 全站聚合(运营看板数据面):三池日/月 + 今日活跃用户 + 14 天趋势。
// 不含用户级行与对话原文——运营看板不进对话内容(隐私隔离,D4)。
func UsageOverview(ctx context.Context, db *pgxpool.Pool) (*OverviewUsage, error) {
	now := time.Now().UTC()
	dayStart := now.Truncate(24 * time.Hour)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	out := &OverviewUsage{Trend14d: []TrendDay{}}

	// 今日/本月三池(全站,无 user 维度)。
	for _, spec := range []struct {
		pool  string
		dst   *UsageStat
		since time.Time
	}{
		{"chat", &out.Day, dayStart}, {"chat", &out.Month, monthStart},
		{"review", &out.Day, dayStart}, {"review", &out.Month, monthStart},
		{"embedding", &out.Day, dayStart}, {"embedding", &out.Month, monthStart},
	} {
		var msgs int
		var in, tk int64
		err := db.QueryRow(ctx, `SELECT count(*), coalesce(sum(tokens_in),0), coalesce(sum(tokens_out),0)
			FROM coach_usage_events WHERE pool=$1 AND created_at >= $2`, spec.pool, spec.since).Scan(&msgs, &in, &tk)
		if err != nil {
			return nil, fmt.Errorf("agent: overview: %w", err)
		}
		spec.dst.Messages += msgs
		tokens := in + tk
		switch spec.pool {
		case "chat":
			spec.dst.Chat += tokens
		case "review":
			spec.dst.Review += tokens
		case "embedding":
			spec.dst.Embedding += tokens
		}
	}

	// 今日活跃(有过回合的独立用户)。
	if err := db.QueryRow(ctx, `SELECT count(DISTINCT user_id) FROM coach_chat_sessions
		WHERE updated_at >= $1`, dayStart).Scan(&out.Users); err != nil {
		return nil, fmt.Errorf("agent: overview users: %w", err)
	}

	// 14 天趋势(左连补零,oldest first)。
	rows, err := db.Query(ctx, `SELECT d::date, m.msgs, coalesce(m.tk,0) FROM generate_series($1::date, $2::date, '1 day') d
		LEFT JOIN (
			SELECT date_trunc('day', created_at) AS d, count(*) AS msgs,
			       coalesce(sum(tokens_in+tokens_out),0) AS tk
			FROM coach_usage_events WHERE created_at >= $1 GROUP BY 1
		) m ON m.d = d ORDER BY d`, dayStart.Add(-13*24*time.Hour), dayStart)
	if err != nil {
		return nil, fmt.Errorf("agent: trend: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var d time.Time
		var t TrendDay
		if err := rows.Scan(&d, &t.Messages, &t.Tokens); err != nil {
			return nil, err
		}
		t.Date = d.Format("2006-01-02")
		out.Trend14d = append(out.Trend14d, t)
	}
	return out, nil
}
