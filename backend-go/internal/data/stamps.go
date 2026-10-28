// Package data — sync stamps 的服务端半(api/sync-stamps.js 移植,P3b)。
// stamped(现代 App)路径完整:t 时间基线/records 只增不减(mergeDeletions/mergeEdits)/
// keepHeld(加回标记)。unstamped 写入者(v1.3.9 老端/API 脚本)的 keepUnknown/
// stampUnstamped/_prior 记 P3c —— fork 无存量老端;AI 教练 sidecar 期间直连 Node 主服务。
package data

import (
	"encoding/json"
	"math"
	"sort"
)

func isMap(v any) bool { _, ok := v.(map[string]any); return ok }
func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}
func list(v any) []any { a, _ := v.([]any); return a }

// same:JSON 规范化比较(encoding/json 对 map 按 key 排序,天然 canonical)。
func same(x, y any) bool {
	if x == nil || y == nil {
		return x == nil && y == nil
	}
	a, e1 := json.Marshal(x)
	b, e2 := json.Marshal(y)
	return e1 == nil && e2 == nil && string(a) == string(b)
}

func clone(v any) any {
	if v == nil {
		return nil
	}
	b, _ := json.Marshal(v)
	var out any
	json.Unmarshal(b, &out)
	return out
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

// ---- DEL_LISTS / DEL_TIME:七类条目的键与时间(workoutKey 兼容 id 缺失) ----
func workoutKey(w map[string]any) string {
	if id, ok := w["id"]; ok && id != nil {
		return jsonStr(id)
	}
	return jsonStr(w["d"]) + "|" + jsonStr(w["start"])
}
func workoutTime(w map[string]any) float64 {
	return maxf(num(w["_ts"]), num(w["end"]), num(w["start"]))
}
func entryTime(w map[string]any) float64 { return num(w["_ts"]) }
func dayTime(e map[string]any) float64   { return num(e["t"]) }

var delLists = []struct{ field string; key func(map[string]any) string }{
	{"workouts", workoutKey}, {"routines", idKey}, {"customEx", idKey},
	{"bodyweight", dayKeyOfD}, {"measurements", dayKeyOfD}, {"gymCards", idKey},
	{"equipProfiles", idKey}, {"favEx", func(m map[string]any) string { return jsonStr(m["x"]) }},
}
var delTime = map[string]func(map[string]any) float64{
	"workouts": workoutTime, "routines": entryTime, "customEx": entryTime,
	"bodyweight": dayTime, "measurements": dayTime, "gymCards": entryTime, "equipProfiles": entryTime,
}
var delListFields = func() map[string]func(map[string]any) string {
	m := map[string]func(map[string]any) string{}
	for _, d := range delLists {
		m[d.field] = d.key
	}
	return m
}()

func idKey(m map[string]any) string    { return jsonStr(m["id"]) }
func dayKeyOfD(m map[string]any) string { return jsonStr(m["d"]) }
func jsonStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

const deletedMax = 5000

// capStamps:每字段的删除记录上限,留最新的。
func capStamps(m map[string]any) {
	if len(m) <= deletedMax {
		return
	}
	type kv struct {
		k string
		v float64
	}
	ks := make([]kv, 0, len(m))
	for k, v := range m {
		ks = append(ks, kv{k, math.Abs(num(v))})
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i].v < ks[j].v })
	for _, k := range ks[:len(ks)-deletedMax] {
		delete(m, k.k)
	}
}

// laterDel:更晚的时间戳胜;平局时负值(加回)胜。
func laterDel(v, cur any) bool {
	x, y := num(v), num(cur)
	ax, ay := math.Abs(x), math.Abs(y)
	return ax > ay || (ax == ay && x < y)
}

// MergeDeletions:两份删除记录合并,逐条目取更晚的(Node mergeDeletions)。
func MergeDeletions(a, b any) map[string]any {
	x, y := asMap(a), asMap(b)
	out := map[string]any{}
	for _, d := range delLists {
		m := map[string]any{}
		for k, v := range asMap(x[d.field]) {
			m[k] = v
		}
		for k, v := range asMap(y[d.field]) {
			if _, ok := m[k]; !ok || laterDel(v, m[k]) {
				m[k] = v
			}
		}
		if len(m) > 0 {
			capStamps(m)
			out[d.field] = m
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// MergeEdits:两份编辑记录合并,逐键取更晚(Node mergeEdits)。
func MergeEdits(a, b any) map[string]any {
	x, y := asMap(a), asMap(b)
	out := map[string]any{}
	for k, v := range x {
		out[k] = v
	}
	for k, v := range y {
		if cur, ok := out[k]; !ok || !(num(cur) >= num(v)) {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// HighestStamp:一份文档携带的最晚时间戳(Node highestStamp;含嵌套 _f 与 STAMPED_MAPS)。
func HighestStamp(S any) float64 {
	m := asMap(S)
	if m == nil {
		return 0
	}
	best := 0.0
	see := func(v any) {
		if n := math.Abs(num(v)); n > best {
			best = n
		}
	}
	see(m["_ts"])
	see(asMap(m["unitSet"])["at"])
	see(m["resetAt"])
	for _, v := range asMap(m["edited"]) {
		see(v)
	}
	for _, f := range asMap(m["deleted"]) {
		for _, v := range asMap(f) {
			see(v)
		}
	}
	entryLists := []string{"routines", "customEx", "equipProfiles", "gymCards", "workouts"}
	for _, f := range entryLists {
		for _, x := range list(m[f]) {
			e := asMap(x)
			if e == nil {
				continue
			}
			see(e["_ts"])
			for _, v := range asMap(e["_f"]) {
				see(v)
			}
		}
	}
	stampedMaps := []string{"balanceOverrides", "loadKind", "plates", "dbLoad", "dumbbells", "dayNotes"}
	for _, f := range stampedMaps {
		for _, v := range asMap(m[f]) {
			see(asMap(v)["_ts"])
		}
	}
	return best
}

// KeepHeld:条目回来了而删除记录说它在其最后编辑后被删 → 标记加回(负时间戳)。
func KeepHeld(next map[string]any, now float64) {
	del := asMap(next["deleted"])
	if del == nil {
		return
	}
	for _, d := range delLists {
		m := asMap(del[d.field])
		if m == nil {
			continue
		}
		tf := delTime[d.field]
		for _, x := range list(next[d.field]) {
			e := asMap(x)
			if e == nil {
				continue
			}
			k := d.key(e)
			at := num(m[k])
			if at > 0 && at >= tf(e) {
				m[k] = -math.Max(now, at+1)
			}
		}
	}
}

// StampPut — PUT /api/data 的打戳入口(Node stampPut 的 stamped 路径移植)。
// t = max(now, highestStamp(cur)+1, highestStamp(next)+1):时钟回退也不倒退。
func StampPut(cur, next map[string]any, stamped bool, now int64) {
	if next == nil {
		return
	}
	t := maxf(float64(now), HighestStamp(cur)+1, HighestStamp(next)+1)
	reset := num(next["resetAt"]) > num(asMap(cur)["resetAt"])
	delete(next, "_unstamped")
	delete(next, "_prior")
	if cur != nil && !reset {
		if d := MergeDeletions(cur["deleted"], next["deleted"]); d != nil {
			next["deleted"] = d
		} else {
			delete(next, "deleted")
		}
		if e := MergeEdits(cur["edited"], next["edited"]); e != nil {
			// 写 id 是服务端账目,不当设置打戳(QA 2026-10-07)
			delete(e, "_wid")
			delete(e, "_wids")
			if len(e) > 0 {
				next["edited"] = e
			} else {
				delete(next, "edited")
			}
		} else {
			delete(next, "edited")
		}
		// !stamped && overRead:keepUnknown/stampUnstamped/ownRecord —— P3c(fork 无 v1.3.9 存量)
	}
	KeepHeld(next, t)
}

func maxf(xs ...float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		if x > m {
			m = x
		}
	}
	return m
}
