// summary.go — 团员看板聚合(设计 §13.4,W9 案②:实时解析 states.doc,零写入改造)。
// 算法单源三实现教义:JS 真源 = ck/frontend/src/lib{format.js 周件, history.js
// streakWeeks, onerm.js estimate1RM 封套};Go 独立实现 + golden 对拍
// (summary_golden_test.go 锚点 = TZ=UTC + 定格 Date 的 node 实算)。
// 服务端简化口径两处(注释面明示):① 周计划数不含队列感知半径(客户端
// weekTally 的 countsFor/queueDone 全语义属 AR1/AR4 金矿);② 体量一律读已存
// 正典 vol 字段,不重算 completedVolumeOf 深链(payloadWorkoutMeta 先例);
// ③ est1RM 只走 epley 默认封套('weighted' 混合式挂疲劳模型权重表,属展示层)。
package team

import (
	"math"
	"strings"
	"time"
)

/* ---------- 周件(fmt.js 移植;JS 的隐式 now 在 Go 侧注入为参) ---------- */

// isoOf 本地日期(TZ=UTC 部署下即 UTC 日期,与 node 同形)。
func isoOf(t time.Time) string {
	return t.Format("2006-01-02")
}

// weekDayOffset 该 weekday 坐过本周起点几天(0..6)。
func weekDayOffset(day, ws int) int { return (day - ws + 7) % 7 }

// startOfWeek iso 所在周首日(noon 防 DST 挪日;JS new Date(iso+'T12:00:00') 同义)。
func startOfWeek(iso string, ws int) time.Time {
	d, err := time.ParseInLocation("2006-01-02", iso, time.UTC)
	if err != nil {
		return time.Time{}
	}
	d = d.Add(12 * time.Hour)
	return d.AddDate(0, 0, -weekDayOffset(int(d.Weekday()), ws))
}

// weekKey 同周等键 = 周首日 ISO(仅作 map 键,非 ISO 周号——JS 注释同款裁定)。
func weekKey(iso string, ws int) string { return isoOf(startOfWeek(iso, ws)) }

// weekStartOf profile 首日:0=Sunday 否则 Monday(缺失缺省 Monday,JS ===SUNDAY 判等)。
// doc 根与 settings 子对象都认(settings 载体随批 3 契约定型时收口)。
func weekStartOf(doc map[string]any) int {
	if v, ok := doc["weekStart"].(float64); ok && v == 0 {
		return 0
	}
	if s, ok := doc["settings"].(map[string]any); ok {
		if v, ok := s["weekStart"].(float64); ok && v == 0 {
			return 0
		}
	}
	return 1
}

// docWorkouts workouts 规整(非对象剔除;空 d 在 JS 侧永不入周键,等效剔除)。
func docWorkouts(v any) []map[string]any {
	a, _ := v.([]any)
	var out []map[string]any
	for _, w := range a {
		if wm, ok := w.(map[string]any); ok {
			if d, _ := wm["d"].(string); d != "" {
				out = append(out, wm)
			}
		}
	}
	return out
}

// StreakWeeks history.js:882 逐行移植:520 周回扫;当周(i==0)无训练不破连,
// 中间断周即断;now 注入(JS 为隐式 new Date(),服务端可测性)。
func StreakWeeks(workouts []map[string]any, ws int, now time.Time) int {
	if len(workouts) == 0 {
		return 0
	}
	weeks := map[string]bool{}
	for _, wm := range workouts {
		weeks[weekKey(wm["d"].(string), ws)] = true
	}
	streak := 0
	cur := now
	for i := 0; i < 520; i++ {
		if weeks[weekKey(isoOf(cur), ws)] {
			streak++
		} else if i > 0 {
			break
		}
		cur = cur.AddDate(0, 0, -7)
	}
	return streak
}

// ownPlannedDays 周计划自排天数:「指针指向现存 routine」才算一天(删档指针
// = 休息日,与 JS ownDays 半径一致);队列覆盖差异见文件头注记①。
func ownPlannedDays(doc map[string]any) int {
	live := map[string]bool{}
	if a, ok := doc["routines"].([]any); ok {
		for _, r := range a {
			if rm, ok := r.(map[string]any); ok {
				if id, _ := rm["id"].(string); id != "" {
					live[id] = true
				}
			}
		}
	}
	week, _ := doc["week"].(map[string]any)
	n := 0
	for _, v := range week {
		ids, ok := v.([]any)
		if !ok {
			continue
		}
		for _, x := range ids {
			if id, _ := x.(string); live[id] {
				n++
				break
			}
		}
	}
	return n
}

/* ---------- est1RM(onerm.js 移植;'weighted' 分支不进,见文件头注记③) ---------- */

const (
	repCap         = 12
	monthWindowDay = 1 // 自然月由 now 的年月锚定,无窗长常量
)

var formulas = map[string]func(w, r float64) float64{
	"epley":    func(w, r float64) float64 { return w * (1 + r/30) },
	"brzycki":  func(w, r float64) float64 { return w * 36 / (37 - r) },
	"lombardi": func(w, r float64) float64 { return w * math.Pow(r, 0.1) },
	"oconner":  func(w, r float64) float64 { return w * (1 + r/40) },
	"mayhew":   func(w, r float64) float64 { return w * 100 / (52.2 + 41.9*math.Exp(-0.055*r)) },
	"wathan":   func(w, r float64) float64 { return w * 100 / (48.8 + 53.8*math.Exp(-0.075*r)) },
	"lander":   func(w, r float64) float64 { return w * 100 / (101.3 - 2.67123*r) },
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// Estimate1RM onerm.js:108 封套:非有限/零负载荷/无次数/超 cap → nil;
// 单次无 RIR(或 RIR0)= 测量而非估算,原值 0.1 精度返回。
func Estimate1RM(w, r float64, formula string, rir *float64) *float64 {
	if math.IsNaN(w) || math.IsInf(w, 0) || math.IsNaN(r) || math.IsInf(r, 0) {
		return nil
	}
	if w <= 0 || r < 1 {
		return nil
	}
	validRir := rir != nil && *rir >= 0
	if r == 1 && (!validRir || *rir == 0) {
		return round1p(w)
	}
	if r > repCap {
		return nil
	}
	fn := formulas[formula]
	if fn == nil {
		fn = formulas["epley"]
	}
	if r == 1 {
		return round1p(w) // JS: reps===1 时公式不施
	}
	return round1p(fn(w, math.Round(r)))
}

func round1p(f float64) *float64 { v := round1(f); return &v }

// isWarmupSet workout-model.js:21 镜像(phase 归一胜 legacy 布尔;与 coach
// payloadIsWarmupSet 同语义,包间独立实现)。
func isWarmupSet(s map[string]any) bool {
	if p, _ := s["phase"].(string); strings.TrimSpace(p) != "" {
		p = strings.ToLower(strings.TrimSpace(p))
		return p == "warmup" || p == "warm-up" || p == "warm_up"
	}
	b, _ := s["warmup"].(bool)
	return b
}

// bestEntryEstimate 单 entry 完成组里的最佳估值(topW 不计——无次数绑定,JS 同注)。
func bestEntryEstimate(entry map[string]any) float64 {
	best := 0.0
	a, _ := entry["sets"].([]any)
	for _, x := range a {
		sm, ok := x.(map[string]any)
		if !ok || isWarmupSet(sm) {
			continue
		}
		if d, _ := sm["done"].(bool); !d {
			continue
		}
		wv, _ := sm["w"].(float64)
		rv, _ := sm["r"].(float64)
		var rir *float64
		if rv, has := sm["rir"].(float64); has {
			rir = &rv
		}
		if e := Estimate1RM(wv, rv, "epley", rir); e != nil && *e > best {
			best = *e
		}
	}
	return best
}

/* ---------- 汇总(§13.4 summary 形状;display_name 由端点层并库补入) ---------- */

// MemberSummary 看板卡聚合。周计数 done=本周训练场数;月=自然月打卡与体量;
// trend=28 天窗体量与最佳估值(全部读已存正典字段,零重算)。
func MemberSummary(doc map[string]any, now time.Time) map[string]any {
	workouts := docWorkouts(doc["workouts"])
	ws := weekStartOf(doc)
	todayKey := weekKey(isoOf(now), ws)

	done := 0
	monthVol, monthCheckins := 0.0, 0
	vol4w := 0.0
	bestEst := 0.0
	cutMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	cut4w := isoOf(now.AddDate(0, 0, -28))
	var last map[string]any
	for _, wm := range workouts {
		d := wm["d"].(string)
		if weekKey(d, ws) == todayKey {
			done++
		}
		if d >= cutMonth {
			monthCheckins++
			monthVol += fOr2(wm["vol"])
		}
		if d >= cut4w {
			vol4w += fOr2(wm["vol"])
			if e := bestEntryEstimate(wm); e > bestEst {
				bestEst = e
			}
		}
		if last == nil || d > last["d"].(string) {
			name, _ := wm["name"].(string)
			last = map[string]any{"d": d, "name": name, "vol": fOr2(wm["vol"])}
		}
	}
	return map[string]any{
		"week":        map[string]any{"done": done, "planned": ownPlannedDays(doc)},
		"streakWeeks": StreakWeeks(workouts, ws, now),
		"month":       map[string]any{"volume": monthVol, "checkins": monthCheckins},
		"lastWorkout": last,
		"trend":       map[string]any{"volume4w": vol4w, "est1rm": bestEst},
	}
}

func fOr2(v any) float64 {
	f, _ := v.(float64)
	return f
}
