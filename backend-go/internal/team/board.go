// board.go — 看板端点聚合(§13.4 批 3;案② 实时解析 states.doc,零写入改造)。
// 缓存 = 60s TTL 防风暴(设计明示);Redis 缺席直接解析(ms 级)。聚合下发,
// 成员原文永不出库(脱敏 #8);weekly 热图列序 oldest-first 当周最后。
package team

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/changzhi777/openGym/backend-go/internal/store"
)

const (
	boardWeeks  = 12
	sumCacheTTL = 60 * time.Second
	sumCacheKey = "team:sum:"
)

// SummaryCard 看板卡(契约 TeamMemberSummary 同形;JSON 由 tags 定,不靠 map)。
type SummaryCard struct {
	UID           string         `json:"uid"`
	DisplayName   string         `json:"displayName"`
	WeekDone      int            `json:"weekDone"`
	WeekPlanned   int            `json:"weekPlanned"`
	StreakWeeks   int            `json:"streakWeeks"`
	MonthVolume   float64        `json:"monthVolume"`
	MonthCheckins int            `json:"monthCheckins"`
	LastWorkout   map[string]any `json:"lastWorkout"` // nil = 无训练记录(渲染 null)
	TrendVolume4w float64        `json:"trendVolume4w"`
	TrendEst1rm   float64        `json:"trendEst1rm"`
	Weekly        []int          `json:"weekly"`
}

// memberCard 单成员卡:GetStateDoc → MemberSummary 语义 + 12 周列;缓存键仅 uid
// (TTL 60s 自过期;rev 进键需先读整文档,缓存即失效,故按设计走纯 TTL 起步)。
func (h *Handlers) memberCard(ctx context.Context, uid string, now time.Time) (*SummaryCard, error) {
	key := sumCacheKey + uid
	if h.Rdb != nil {
		if b, err := h.Rdb.Get(ctx, key).Bytes(); err == nil {
			var c SummaryCard
			if json.Unmarshal(b, &c) == nil && len(c.Weekly) == boardWeeks {
				c.UID = uid // 键即身份,防串档
				return &c, nil
			}
		}
	}
	var doc map[string]any
	sd, err := store.GetStateDoc(ctx, h.DB, uid)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if err == nil {
		json.Unmarshal(sd.Doc, &doc) // 坏 doc 视为空档(基线零卡)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	ws := weekStartOf(doc)
	workouts := docWorkouts(doc["workouts"])
	todayKey := weekKey(isoOf(now), ws)
	cutMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	cut4w := isoOf(now.AddDate(0, 0, -28))
	weekCols := make([]string, boardWeeks)
	for i := 0; i < boardWeeks; i++ { // oldest-first:11 周前 → 当周
		weekCols[boardWeeks-1-i] = weekKey(isoOf(now.AddDate(0, 0, -7*i)), ws)
	}
	card := &SummaryCard{
		UID:         uid,
		WeekPlanned: ownPlannedDays(doc),
		StreakWeeks: StreakWeeks(workouts, ws, now),
		Weekly:      make([]int, boardWeeks),
	}
	for _, wm := range workouts {
		d := wm["d"].(string)
		k := weekKey(d, ws)
		if k == todayKey {
			card.WeekDone++
		}
		for i, wk := range weekCols {
			if wk == k {
				card.Weekly[i]++ // 同周多场各计一次(打卡数,非去重)
			}
		}
		if d >= cutMonth {
			card.MonthCheckins++
			card.MonthVolume += fOr2(wm["vol"])
		}
		if d >= cut4w {
			card.TrendVolume4w += fOr2(wm["vol"])
			if e := bestEntryEstimate(wm); e > card.TrendEst1rm {
				card.TrendEst1rm = e
			}
		}
		if card.LastWorkout == nil || d > card.LastWorkout["d"].(string) {
			name, _ := wm["name"].(string)
			card.LastWorkout = map[string]any{"d": d, "name": name, "vol": fOr2(wm["vol"])}
		}
	}
	if h.Rdb != nil {
		if b, err := json.Marshal(card); err == nil {
			h.Rdb.Set(ctx, key, b, sumCacheTTL)
		}
	}
	return card, nil
}

// boardWeeksOf 看板周列(用调用者 doc 的 weekStart;成员各自 ws 与列的错位
// 仅发生在混设周首日的团,批 3 契约面接受此近似)。
func boardWeeksOf(doc map[string]any, now time.Time) []string {
	ws := weekStartOf(doc)
	out := make([]string, boardWeeks)
	for i := 0; i < boardWeeks; i++ {
		out[boardWeeks-1-i] = weekKey(isoOf(now.AddDate(0, 0, -7*i)), ws)
	}
	return out
}
