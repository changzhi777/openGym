// insights.go — 教练分析端点数据面(§13.4 看板深化:双窗环比 + 规则化解读文案)。
// 复用 summary.go 周件/1RM 移植函数族与 board.go memberCard 的缓存骨架;
// verdict = 纯规则生成,零 AI 依赖(审查批裁定:LLM 解读留二版接 coach 域)。
package team

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/changzhi777/openGym/backend-go/internal/store"
)

const insightCacheKey = "team:insight:"

// InsightRow 分析卡(契约 TeamInsight 同形;DisplayName 由端点层并库补入,不进缓存)。
type InsightRow struct {
	UID          string         `json:"uid"`
	DisplayName  string         `json:"displayName,omitempty"`
	WeekDone     int            `json:"weekDone"`
	WeekPlanned  int            `json:"weekPlanned"`
	Rate         int            `json:"rate"` // 完成率 0-100(未排计划 = 0,verdict 特判)
	StreakWeeks  int            `json:"streakWeeks"`
	Vol4w        float64        `json:"vol4w"`
	VolPrev4w    float64        `json:"volPrev4w"`
	VolDelta     int            `json:"volDelta"` // 体量环比 %(-100..;前窗为零时本期有量记 +100)
	Est1rm       float64        `json:"est1rm"`
	Est1rmPrev4w float64        `json:"est1rmPrev4w"`
	Est1rmDelta  int            `json:"est1rmDelta"`
	LastWorkout  map[string]any `json:"lastWorkout"` // nil = 无训练记录
	Weekly       []int          `json:"weekly"`      // 12 周打卡列(oldest-first,当周最后)
	Verdict      string         `json:"verdict"`     // 规则化中文解读
	Flags        []string       `json:"flags"`       // hot/miss/gain/drop
}

// deltaOf 环比百分比:前窗为零时本期有量记 +100(新起练),全零记 0。
func deltaOf(cur, prev float64) int {
	if prev <= 0 {
		if cur > 0 {
			return 100
		}
		return 0
	}
	return int((cur - prev) / prev * 100)
}

// verdictOf 规则解读(审查批 A4+1.2):主句完成态 → 连续周 → 环比句 → 亮点/告警句;
// flags 与文案同源生成,单遍历零分叉。
func verdictOf(r *InsightRow) (string, []string) {
	var flags []string
	if r.LastWorkout == nil {
		return "还没有训练记录，从第一堂课开始吧", []string{"miss"}
	}
	if r.WeekDone == 0 {
		flags = append(flags, "miss")
	} else if r.WeekPlanned > 0 && r.WeekDone >= r.WeekPlanned {
		flags = append(flags, "hot")
	}
	gain := r.Est1rmDelta >= 5 && r.Est1rm > 0 && r.Est1rmPrev4w > 0
	drop := r.VolDelta <= -30 && r.VolPrev4w > 0
	if gain {
		flags = append(flags, "gain")
	}
	if drop {
		flags = append(flags, "drop")
	}

	var b strings.Builder
	if r.WeekPlanned > 0 {
		fmt.Fprintf(&b, "本周 %d/%d", r.WeekDone, r.WeekPlanned)
		if r.WeekDone >= r.WeekPlanned {
			b.WriteString("，计划完成")
		} else if r.WeekDone > 0 {
			fmt.Fprintf(&b, "，还差 %d 场", r.WeekPlanned-r.WeekDone)
		} else {
			b.WriteString("，本周还未开始")
		}
	} else {
		fmt.Fprintf(&b, "本周已练 %d 场（未排计划）", r.WeekDone)
	}
	if r.StreakWeeks > 0 {
		fmt.Fprintf(&b, "，连续 %d 周", r.StreakWeeks)
	}
	if r.VolPrev4w > 0 && r.VolDelta != 0 {
		fmt.Fprintf(&b, "；四周体量较上期 %+d%%", r.VolDelta)
	}
	if gain {
		fmt.Fprintf(&b, "；最佳估算 1RM 较上期 %+d%%", r.Est1rmDelta)
	}
	if drop {
		b.WriteString("；训练量明显回落，值得聊一聊")
	}
	return b.String(), flags
}

// memberInsight 单成员分析卡(双窗一次循环;缓存骨架与 memberCard 同款,TTL 60s)。
func (h *Handlers) memberInsight(ctx context.Context, uid string, now time.Time) (*InsightRow, error) {
	key := insightCacheKey + uid
	if h.Rdb != nil {
		if b, err := h.Rdb.Get(ctx, key).Bytes(); err == nil {
			var r InsightRow
			if json.Unmarshal(b, &r) == nil && len(r.Weekly) == boardWeeks {
				r.UID = uid // 键即身份,防串档
				return &r, nil
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
	cut4w := isoOf(now.AddDate(0, 0, -28))
	cut8w := isoOf(now.AddDate(0, 0, -56))
	weekCols := make([]string, boardWeeks)
	for i := 0; i < boardWeeks; i++ {
		weekCols[boardWeeks-1-i] = weekKey(isoOf(now.AddDate(0, 0, -7*i)), ws)
	}
	r := &InsightRow{
		WeekPlanned: ownPlannedDays(doc),
		StreakWeeks: StreakWeeks(workouts, ws, now),
		Weekly:      make([]int, boardWeeks),
	}
	for _, wm := range workouts {
		d := wm["d"].(string)
		k := weekKey(d, ws)
		if k == todayKey {
			r.WeekDone++
		}
		for i, wk := range weekCols {
			if wk == k {
				r.Weekly[i]++
			}
		}
		vol := fOr2(wm["vol"])
		est := bestEntryEstimate(wm)
		if d >= cut4w {
			r.Vol4w += vol
			if est > r.Est1rm {
				r.Est1rm = est
			}
		} else if d >= cut8w {
			r.VolPrev4w += vol
			if est > r.Est1rmPrev4w {
				r.Est1rmPrev4w = est
			}
		}
		if r.LastWorkout == nil || d > r.LastWorkout["d"].(string) {
			name, _ := wm["name"].(string)
			r.LastWorkout = map[string]any{"d": d, "name": name, "vol": vol}
		}
	}
	if r.WeekPlanned > 0 {
		r.Rate = r.WeekDone * 100 / r.WeekPlanned
	}
	r.VolDelta = deltaOf(r.Vol4w, r.VolPrev4w)
	r.Est1rmDelta = deltaOf(r.Est1rm, r.Est1rmPrev4w)
	r.Verdict, r.Flags = verdictOf(r)
	if h.Rdb != nil {
		if b, err := json.Marshal(r); err == nil {
			h.Rdb.Set(ctx, key, b, sumCacheTTL)
		}
	}
	return r, nil
}
