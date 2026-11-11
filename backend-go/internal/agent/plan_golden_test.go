// plan_golden_test.go — 计划决策链对 MCP get_week_plan 真 handler 输出的 golden
// 等价验收(夹具由 scripts/golden/gen-plan.mjs 生成,时钟钉死 2026-11-11 周三;
// 八场景覆盖 planned_by 六分支 + queue 容错)。组装层一并锚定。
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type planScenario struct {
	Name   string          `json:"name"`
	State  json.RawMessage `json:"state"`
	Output json.RawMessage `json:"output"`
}

func TestWeekPlanGolden(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "plan-golden.json"))
	if err != nil {
		t.Fatalf("read golden: %v — 先跑 node scripts/golden/gen-plan.mjs", err)
	}
	var g struct {
		Scenarios []planScenario `json:"scenarios"`
	}
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	if len(g.Scenarios) < 8 {
		t.Fatalf("golden 场景过薄(%d),gen 脚本可能没跑全", len(g.Scenarios))
	}
	for _, sc := range g.Scenarios {
		var st planS
		if err := json.Unmarshal(sc.State, &st); err != nil {
			t.Fatalf("%s: decode state: %v", sc.Name, err)
		}
		out, err := st.WeekPlan("2026-11-11")
		if err != nil {
			t.Fatalf("%s: WeekPlan: %v", sc.Name, err)
		}
		got, _ := json.Marshal(out)
		var wantAny, gotAny any
		_ = json.Unmarshal(sc.Output, &wantAny)
		_ = json.Unmarshal(got, &gotAny)
		if path := firstDiffPath(gotAny, wantAny, "$"); path != "" {
			t.Errorf("%s: 首个不一致字段: %s", sc.Name, path)
		}
	}
}


// firstDiffPath 递归找第一个不一致的 JSON 路径(诊断用;空串 = 全等)。
func firstDiffPath(got, want any, path string) string {
	gm, gok := got.(map[string]any)
	wm, wok := want.(map[string]any)
	if gok && wok {
		for k, wv := range wm {
			gv, in := gm[k]
			if !in {
				return path + "." + k + " (got 缺失)"
			}
			if p := firstDiffPath(gv, wv, path+"."+k); p != "" {
				return p
			}
		}
		return ""
	}
	ga, gok2 := got.([]any)
	wa, wok2 := want.([]any)
	if gok2 && wok2 {
		if len(ga) != len(wa) {
			return path + fmt.Sprintf(" (len %d vs %d)", len(ga), len(wa))
		}
		for i := range ga {
			if p := firstDiffPath(ga[i], wa[i], path+fmt.Sprintf("[%d]", i)); p != "" {
				return p
			}
		}
		return ""
	}
	if got != want {
		return path + fmt.Sprintf(" (got=%v want=%v)", got, want)
	}
	return ""
}
