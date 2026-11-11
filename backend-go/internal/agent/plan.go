// plan.go — 计划读取工具的决策链(批次 6 后段;算法单源=frontend/src/lib/queue.js
// 全文 + history.js 的 effectiveRoutineIds + mcp tools.js 的 get_week_plan 组装层)。
//
// 翻译对照(逐函数):
//   queueOf/countsFor/isDone/queueDone/queueRemaining/queueNext/activePins/nextOn/
//   queueLiveOn/pinState/queueView  ← frontend/src/lib/queue.js(L29-151)
//   effectiveRoutineIds             ← frontend/src/lib/history.js(L477-494)
//   weekPlan 输出组装               ← mcp/src/tools.js get_week_plan.handler
//
// 等价验收:testdata/plan-golden.json(gen-plan.mjs 用 MCP 真 handler 产,时钟钉死
// 2026-11-11 周三;八场景覆盖 planned_by 六分支 + queue 容错)。日期一律 ISO 字符串
// 比较(与 JS 同);getDay 以正午锚定(±8h 时区不跨界)。
//
// today 是参数而非读时钟——调用方(工具)传会话"今天",测试传固定值,与 JS 同纪律。
package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ── 状态形状(宽容解析) ────────────────────────────────────────────────────────

type planRoutine struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Emoji string `json:"emoji"`
}

type planWorkout struct {
	D          string   `json:"d"`
	Start      int64    `json:"start"`
	Name       string   `json:"name"`
	RoutineID  string   `json:"routineId"`
	RoutineIDs []string `json:"routineIds"`
}

type planQueue struct {
	IDs        []string `json:"ids"`
	Since      int64    `json:"since"`
	StartsOn   string   `json:"startsOn"`
	Label      string   `json:"label"`
	Strict     bool     `json:"strict"`
	RotationID string   `json:"rotationId"`
}

// weekValue 容忍 S.week[wd] 的两种历史形态:单 id 字符串(旧)或 id 数组(现)。
type weekValue []string

func (w *weekValue) UnmarshalJSON(b []byte) error {
	var single string
	if err := json.Unmarshal(b, &single); err == nil {
		*w = []string{single}
		return nil
	}
	return json.Unmarshal(b, (*[]string)(w))
}

type planS struct {
	Routines  []planRoutine       `json:"routines"`
	Week      map[string]weekValue `json:"week"`
	DayPlan   map[string]string   `json:"dayPlan"`
	Queue     json.RawMessage     `json:"queue"`
	Workouts  []planWorkout       `json:"workouts"`
}

// routineName 按 id 查名(不存在回 id 自身——queueView 的"退役 routine 保留 id
// 为名"语义;plannedBy/nameOf 处区分用 exists)。
func (s *planS) routineName(id string) string {
	for _, r := range s.Routines {
		if r.ID == id {
			return r.Name
		}
	}
	return id
}

func (s *planS) routineExists(id string) bool {
	for _, r := range s.Routines {
		if r.ID == id {
			return true
		}
	}
	return false
}

// ── queue.js 翻译 ────────────────────────────────────────────────────────────

func routineIDsOf(w planWorkout) []string {
	if w.RoutineIDs != nil {
		return w.RoutineIDs
	}
	if w.RoutineID != "" {
		return []string{w.RoutineID}
	}
	return nil
}

func nameParts(w planWorkout) []string {
	return strings.Split(w.Name, " + ")
}

// planQueueOf 容错读取(L46-63):坏 shape 读作"无 queue";id 去重且必须存在;
// 无会话留存 = 无 queue;缺 startsOn = apply 当天。
func planQueueOf(s *planS) *planQueue {
	var q planQueue
	if err := json.Unmarshal(s.Queue, &q); err != nil || len(q.IDs) == 0 {
		return nil
	}
	ids := []string{}
	for i, id := range q.IDs {
		first := true
		for j := 0; j < i; j++ {
			if q.IDs[j] == id {
				first = false
				break
			}
		}
		if first && s.routineExists(id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	q.IDs = ids
	if q.StartsOn == "" {
		q.StartsOn = time.UnixMilli(q.Since).UTC().Format("2006-01-02")
	}
	return &q
}

// countsFor DONE RULE(L65-68):该训练在 apply 之后开始,或(非 strict)日期在周内
// 且名字含该 session 名(合并 session 读 'US W1 D1 + Core')。
func (s *planS) countsFor(q *planQueue, w planWorkout, id string) bool {
	if !contains(routineIDsOf(w), id) {
		return false
	}
	if w.Start >= q.Since {
		return true
	}
	if q.Strict {
		return false
	}
	if w.D < q.StartsOn {
		return false
	}
	name := s.routineName(id)
	for _, p := range nameParts(w) {
		if p == name {
			return true
		}
	}
	return false
}

func (s *planS) isDone(q *planQueue, id string) bool {
	for _, w := range s.Workouts {
		if s.countsFor(q, w, id) {
			return true
		}
	}
	return false
}

func (s *planS) queueDone(q *planQueue) []string {
	out := []string{}
	for _, id := range q.IDs {
		if s.isDone(q, id) {
			out = append(out, id)
		}
	}
	return out
}

func (s *planS) queueRemaining(q *planQueue) []string {
	out := []string{}
	for _, id := range q.IDs {
		if !s.isDone(q, id) {
			out = append(out, id)
		}
	}
	return out
}

// queueNext(L95-105):queue 只回答一天——今天,或 startsOn 尚在未来时的那天。
func (s *planS) queueNext(iso, today string) string {
	q := planQueueOf(s)
	if q == nil {
		return ""
	}
	on := today
	if q.StartsOn > on {
		on = q.StartsOn
	}
	if iso != on {
		return ""
	}
	return s.nextOn(s.queueRemaining(q), on, today)
}

// activePins(L108-111):今日及未来、指向未完成 session 的 dayPlan 条目。
func (s *planS) activePins(remaining []string, today string) []struct {
	Iso string
	ID  string
} {
	out := []struct {
		Iso string
		ID  string
	}{}
	for iso, id := range s.DayPlan {
		if iso >= today && contains(remaining, id) {
			out = append(out, struct {
				Iso string
				ID  string
			}{iso, id})
		}
	}
	return out
}

// nextOn(L113-118):当日被钉的 session,否则第一个没被钉去别日的未完成 session。
func (s *planS) nextOn(remaining []string, on, today string) string {
	pins := s.activePins(remaining, today)
	for _, p := range pins {
		if p.Iso == on {
			return p.ID
		}
	}
	for _, id := range remaining {
		pinned := false
		for _, p := range pins {
			if p.ID == id {
				pinned = true
				break
			}
		}
		if !pinned {
			return id
		}
	}
	return ""
}

// queueLiveOn(L121-125):该日是 queue 发言日且有未完成 session——即便 queueNext
// 在此日为空(全被钉去别日),计划者的 weekday 指针仍被 queue 遮蔽。
func (s *planS) queueLiveOn(iso, today string) bool {
	q := planQueueOf(s)
	if q == nil {
		return false
	}
	on := today
	if q.StartsOn > on {
		on = q.StartsOn
	}
	return iso == on && len(s.queueRemaining(q)) > 0
}

// pinState(L128-134):'open'(未完成 session 的 pin)/'done'(已满足,读作无
// override)/null(非 queue session 或无 queue)。
func (s *planS) pinState(id string) string {
	q := planQueueOf(s)
	if q == nil || id == "" || !contains(q.IDs, id) {
		return ""
	}
	if s.isDone(q, id) {
		return "done"
	}
	return "open"
}

// planQueueView 对应 queueView(L137-168):进度行全量派生。
type planQueueView struct {
	Label    string
	Items    []planQueueItem
	Remaining []string
	Complete bool
	StartsOn string
	Waiting  bool
}

type planQueueItem struct {
	ID    string
	Name  string
	State string // done | next | pinned | later
	On    string // pinned 时的钉日
}

func (s *planS) planQueueView(today string) *planQueueView {
	q := planQueueOf(s)
	if q == nil {
		return nil
	}
	remaining := s.queueRemaining(q)
	on := today
	if q.StartsOn > on {
		on = q.StartsOn
	}
	next := s.nextOn(remaining, on, today)
	pins := s.activePins(remaining, today)
	sort.Slice(pins, func(i, j int) bool { return pins[i].Iso < pins[j].Iso })
	view := &planQueueView{Label: q.Label, Remaining: remaining, Complete: len(remaining) == 0, StartsOn: q.StartsOn, Waiting: today < q.StartsOn}
	for _, id := range q.IDs {
		item := planQueueItem{ID: id, Name: s.routineName(id)}
		switch {
		case !contains(remaining, id):
			item.State = "done"
		case id == next:
			item.State = "next"
		default:
			var pin *struct {
				Iso string
				ID  string
			}
			for i := range pins {
				if pins[i].ID == id && pins[i].Iso != on {
					pin = &pins[i]
					break
				}
			}
			if pin != nil {
				item.State = "pinned"
				item.On = pin.Iso
			} else {
				item.State = "later"
			}
		}
		view.Items = append(view.Items, item)
	}
	return view
}

// ── history.js:effectiveRoutineIds ──────────────────────────────────────────

// effectiveRoutineIds(L477-494):某日的计划 routine(合并序)。
// rest override → [];pin 非 open 且指向存在的 routine → [ov](override);
// queue 日 → [queue session, ...own(自己的 weekday 顺延)];否则 weekday。
func (s *planS) effectiveRoutineIds(iso, today string) []string {
	ov := s.DayPlan[iso]
	if ov == "rest" {
		return []string{}
	}
	pin := s.pinState(ov)
	if pin == "" && ov != "" && s.routineExists(ov) {
		return []string{ov}
	}
	wd, _ := weekdayOf(iso)
	weekday := []string{}
	for _, id := range s.Week[wd] {
		if s.routineExists(id) {
			weekday = append(weekday, id)
		}
	}
	q := ""
	if pin == "open" {
		q = ov
	} else {
		q = s.queueNext(iso, today)
	}
	own := weekday
	if q != "" || s.queueLiveOn(iso, today) {
		filtered := []string{}
		for _, id := range weekday {
			if !contains(s.queueIDs(), id) {
				filtered = append(filtered, id)
			}
		}
		own = filtered
	}
	if q != "" {
		return append([]string{q}, own...)
	}
	return own
}

// queueIDs queue 原始 ids(pin 遮蔽过滤用;queue 为 nil 时空)。
func (s *planS) queueIDs() []string {
	q := planQueueOf(s)
	if q == nil {
		return nil
	}
	return q.IDs
}

// ── get_week_plan 组装(mcp tools.js handler) ────────────────────────────────

var dayNames = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// weekdayOf ISO 日期 → (JS getDay 数字, 正午锚定)。
func weekdayOf(iso string) (string, int) {
	t, err := time.Parse("2006-01-02T15:04:05", iso+"T12:00:00")
	if err != nil {
		return "", 0
	}
	return fmtInt(int(t.Weekday())), int(t.Weekday())
}

func fmtInt(n int) string {
	return fmt.Sprintf("%d", n)
}

// planDayOutput 一天的视图(days[i])。
type planDayOutput struct {
	Date         string   `json:"date"`
	Weekday      int      `json:"weekday"`
	WeekdayName  string   `json:"weekday_name"`
	RoutineIDs   []string `json:"routine_ids"`
	RoutineNames []string `json:"routine_names"`
	PlannedBy    string   `json:"planned_by"`
}

// WeekPlanOutput 对应契约 get_week_plan 的输出。
type WeekPlanOutput struct {
	Today           string   `json:"today"`
	TodayRoutineID  *string  `json:"today_routine_id"`
	TodayRoutineName *string `json:"today_routine_name"`
	TodayRoutineIDs []string `json:"today_routine_ids"`
	TodayRoutineNames []string `json:"today_routine_names"`
	CoachWeek       any      `json:"coach_week"`
	Days            []planDayOutput `json:"days"`
	Weekdays        []planWeekdayOutput `json:"weekdays"`
}

// planWeekdayOutput 周模板行(weekdays[i]):routine_ids/routine_name/routine_emoji
// + 仅今天行的 override 值(整个 dayPlan 字典不外露)。
type planWeekdayOutput struct {
	Weekday             int     `json:"weekday"`
	WeekdayName         string  `json:"weekday_name"`
	RoutineIDs          []string `json:"routine_ids"`
	RoutineNames        []string `json:"routine_names"`
	RoutineID           *string  `json:"routine_id"`
	RoutineName         *string  `json:"routine_name"`
	RoutineEmoji        *string  `json:"routine_emoji"`
	OverrideForTodayOrNull *string `json:"override_for_today_or_null"`
}

// WeekPlan 组装(MCP handler 翻译)。
func (s *planS) WeekPlan(today string) (*WeekPlanOutput, error) {
	_, todayWd := weekdayOf(today)
	weekdayIDs := func(wd int) []string {
		out := []string{}
		for _, id := range s.Week[fmtInt(wd)] {
			if s.routineExists(id) {
				out = append(out, id)
			}
		}
		return out
	}
	nameOf := func(id string) *string {
		for _, r := range s.Routines {
			if r.ID == id {
				n := r.Name
				return &n
			}
		}
		return nil
	}
	todayIDs := s.effectiveRoutineIds(today, today)

	// plannedBy(L207-213):与 effectiveRoutineIds 同一优先级,命名。
	plannedBy := func(iso string, ids []string) string {
		ov := s.DayPlan[iso]
		if ov == "rest" {
			return "rest_override"
		}
		pin := s.pinState(ov)
		if pin == "open" {
			return "pinned"
		}
		if pin == "" && ov != "" && s.routineExists(ov) {
			return "override"
		}
		if len(ids) > 0 && s.queueNext(iso, today) == ids[0] {
			return "coach"
		}
		if len(ids) > 0 {
			return "weekday"
		}
		return "rest"
	}

	out := &WeekPlanOutput{Today: today, TodayRoutineIDs: todayIDs, Days: []planDayOutput{}}
	if len(todayIDs) > 0 {
		id := todayIDs[0]
		out.TodayRoutineID = &id
		if n := nameOf(id); n != nil {
			out.TodayRoutineName = n
		}
	}
	names := make([]string, 0, len(todayIDs))
	for _, id := range todayIDs {
		if n := nameOf(id); n != nil {
			names = append(names, *n)
		}
	}
	out.TodayRoutineNames = names

	if view := s.planQueueView(today); view != nil {
		items := make([]map[string]any, 0, len(view.Items))
		for _, it := range view.Items {
			m := map[string]any{"routine_id": it.ID, "routine_name": it.Name, "state": it.State}
			if it.On != "" {
				m["pinned_to"] = it.On
			} else {
				m["pinned_to"] = nil
			}
			items = append(items, m)
		}
		out.CoachWeek = map[string]any{
			"label":    view.Label,
			"starts_on": view.StartsOn,
			"waiting":  view.Waiting,
			"complete": view.Complete,
			"sessions": items,
		}
	}

	for i := 0; i < 7; i++ {
		dt := addDays(today, i)
		iso := dt.Format("2006-01-02")
		wd := int(dt.Weekday())
		ids := s.effectiveRoutineIds(iso, today)
		day := planDayOutput{
			Date: iso, Weekday: wd, WeekdayName: dayNames[wd],
			RoutineIDs: ids, PlannedBy: plannedBy(iso, ids),
		}
		for _, id := range ids {
			if n := nameOf(id); n != nil {
				day.RoutineNames = append(day.RoutineNames, *n)
			}
		}
		if day.RoutineNames == nil {
			day.RoutineNames = []string{}
		}
		if day.RoutineIDs == nil {
			day.RoutineIDs = []string{}
		}
		out.Days = append(out.Days, day)
	}

	// weekdays 周模板行(L48-65):override 只在今天的行上外露单值。
	out.Weekdays = []planWeekdayOutput{}
	for d := 0; d <= 6; d++ {
		ids := weekdayIDs(d)
		row := planWeekdayOutput{
			Weekday: d, WeekdayName: dayNames[d],
			RoutineIDs: ids,
		}
		for _, id := range ids {
			if n := nameOf(id); n != nil {
				row.RoutineNames = append(row.RoutineNames, *n)
			}
		}
		if row.RoutineNames == nil {
			row.RoutineNames = []string{}
		}
		if len(ids) > 0 {
			id := ids[0]
			row.RoutineID = &id
			if n := nameOf(id); n != nil {
				row.RoutineName = n
			}
			for _, r := range s.Routines {
				if r.ID == id {
					if e := r.Emoji; e != "" {
						row.RoutineEmoji = &e
					} // 无 emoji:保持 nil(JS 的 undefined||null 语义)
					break
				}
			}
		}
		if d == todayWd {
			if ov, exists := s.DayPlan[today]; exists {
				row.OverrideForTodayOrNull = &ov
			}
		}
		out.Weekdays = append(out.Weekdays, row)
	}
	return out, nil
}

// addDays ISO 日期 + n 天(正午锚定)。
func addDays(iso string, n int) time.Time {
	t, err := time.Parse("2006-01-02T15:04:05", iso+"T12:00:00")
	if err != nil {
		return time.Time{}
	}
	return t.AddDate(0, 0, n)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
