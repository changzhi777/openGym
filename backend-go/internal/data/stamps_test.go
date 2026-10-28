// stamps_test.go — Node test/sync-stamps.test.js 的核心用例翻译(Go 侧保护网)。
package data

import (
	"encoding/json"
	"testing"
)

func j(s string) map[string]any {
	var m map[string]any
	json.Unmarshal([]byte(s), &m)
	return m
}

func TestMergeDeletionsLaterWins(t *testing.T) {
	a := j(`{"workouts":{"w1":100,"w2":50}}`)
	b := j(`{"workouts":{"w1":200,"w3":10}}`)
	out := MergeDeletions(a, b)
	w := asMap(out["workouts"])
	if num(w["w1"]) != 200 {
		t.Fatalf("w1 应取更晚的 200,得 %v", w["w1"])
	}
	if _, ok := w["w2"]; !ok {
		t.Fatal("记录只增不减:w2 被丢了")
	}
	if _, ok := w["w3"]; !ok {
		t.Fatal("w3 应并入")
	}
}

func TestMergeDeletionsTieAddbackWins(t *testing.T) {
	// 平局:负值(加回)胜
	out := MergeDeletions(j(`{"routines":{"r1":100}}`), j(`{"routines":{"r1":-100}}`))
	if num(asMap(out["routines"])["r1"]) != -100 {
		t.Fatalf("平局应负值胜,得 %v", asMap(out["routines"])["r1"])
	}
}

func TestMergeEdits(t *testing.T) {
	out := MergeEdits(j(`{"unit":5,"theme":9}`), j(`{"unit":8}`))
	if num(out["unit"]) != 8 {
		t.Fatal("unit 应取 8")
	}
	if num(out["theme"]) != 9 {
		t.Fatal("theme 保留 9")
	}
}

func TestStampPutDropsWidFromEdits(t *testing.T) {
	// 写 id 是服务端账目:即使写方误把它们当设置打戳,合并结果也不带(Node QA 2026-10-07)
	cur := j(`{"edited":{}}`)
	next := j(`{"edited":{"unit":5,"_wid":"abc","_wids":["z"]}}`)
	StampPut(cur, next, true, 1000)
	ed := asMap(next["edited"])
	if _, ok := ed["_wid"]; ok {
		t.Fatal("_wid 不应出现在合并后的 edited")
	}
	if num(ed["unit"]) != 5 {
		t.Fatal("unit 应保留")
	}
}

func TestHighestStamp(t *testing.T) {
	s := j(`{"_ts":100,"edited":{"a":300},"workouts":[{"_ts":250,"_f":{"note":400}}]}`)
	if HighestStamp(s) != 400 {
		t.Fatalf("应取嵌套 _f 的 400,得 %v", HighestStamp(s))
	}
}

func TestKeepHeld(t *testing.T) {
	next := j(`{"deleted":{"routines":{"r1":500}},"routines":[{"id":"r1","_ts":100}]}`)
	KeepHeld(next, 1000)
	if num(asMap(asMap(next["deleted"])["routines"])["r1"]) != -1000 {
		t.Fatalf("加回的条目应标 -max(now,at+1),得 %v", asMap(asMap(next["deleted"])["routines"])["r1"])
	}
}

func TestStampPutStampedPath(t *testing.T) {
	cur := j(`{"deleted":{"workouts":{"w1":10}},"edited":{"unit":5}}`)
	next := j(`{"workouts":[],"edited":{"theme":9},"resetAt":0}`)
	StampPut(cur, next, true, 1000)
	if asMap(next["deleted"]) == nil {
		t.Fatal("records 只增不减:deleted 被删了")
	}
	if num(asMap(asMap(next["deleted"])["workouts"])["w1"]) != 10 {
		t.Fatal("w1 记录应保留")
	}
	if num(asMap(next["edited"])["unit"]) != 5 {
		t.Fatal("unit 的编辑记录应保留")
	}
}
