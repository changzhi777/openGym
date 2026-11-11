// onerm_golden_test.go — Go 翻译版对 JS 真函数输出的 golden 等价验收。
//
// 夹具由 scripts/golden/gen-onerm.mjs 用 frontend/src/lib/onerm.js 真函数生成
// (testdata/onerm-golden.json);本测试证明 Go 侧逐案一致。另有少量硬断言
// (取自 onerm.test.js 的人类可读锚点),golden 网盖不到语义倒退时先红这里。
package agent

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type goldenCase struct {
	W      float64  `json:"w"`
	R      float64  `json:"r"`
	F      string   `json:"f"`
	Rir    *float64 `json:"rir"`
	Expect *float64 `json:"expect"`
}

type accuracyCase struct {
	R      float64  `json:"r"`
	Rir    *float64 `json:"rir"`
	F      string   `json:"f"`
	Expect float64  `json:"expect"`
}

type goldenFile struct {
	Cases     []goldenCase   `json:"cases"`
	Accuracy  []accuracyCase `json:"accuracy"`
	SelfCheck []goldenCase   `json:"selfcheck"`
}

func loadGolden(t *testing.T) goldenFile {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "onerm-golden.json"))
	if err != nil {
		t.Fatalf("read golden: %v — 先跑 node scripts/golden/gen-onerm.mjs", err)
	}
	var g goldenFile
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	if len(g.Cases) < 1000 || len(g.Accuracy) < 50 {
		t.Fatalf("golden 夹具过薄(est=%d acc=%d),gen 脚本可能没跑全", len(g.Cases), len(g.Accuracy))
	}
	return g
}

func TestOnermGoldenEstimate(t *testing.T) {
	g := loadGolden(t)
	for i, c := range g.Cases {
		got := Estimate1RM(c.W, c.R, c.F, c.Rir)
		switch {
		case c.Expect == nil && got != nil:
			t.Errorf("case#%d w=%v r=%v f=%s rir=%v: got %v, want nil", i, c.W, c.R, c.F, c.Rir, *got)
		case c.Expect != nil && got == nil:
			t.Errorf("case#%d w=%v r=%v f=%s rir=%v: got nil, want %v", i, c.W, c.R, c.F, c.Rir, *c.Expect)
		case c.Expect != nil && got != nil:
			if d := math.Abs(*got - *c.Expect); d > 1e-6 {
				t.Errorf("case#%d w=%v r=%v f=%s rir=%v: got %v, want %v (d=%g)", i, c.W, c.R, c.F, c.Rir, *got, *c.Expect, d)
			}
		}
	}
}

func TestOnermGoldenAccuracy(t *testing.T) {
	g := loadGolden(t)
	for i, c := range g.Accuracy {
		got := Calc1RMAccuracy(c.R, c.Rir, c.F)
		if d := math.Abs(got - c.Expect); d > 1e-6 {
			t.Errorf("acc#%d r=%v rir=%v f=%s: got %v, want %v", i, c.R, c.Rir, c.F, got, c.Expect)
		}
	}
}

// TestOnermAnchors — onerm.test.js 的人类可读锚点(单测即文档)。
func TestOnermAnchors(t *testing.T) {
	eq := func(got, want *float64) bool {
		return (got == nil && want == nil) || (got != nil && want != nil && math.Abs(*got-*want) < 1e-9)
	}
	f := func(v float64) *float64 { return &v }
	n := func() *float64 { return nil }

	// 1 次 = 测量原样。
	if got := Estimate1RM(100, 1, "epley", nil); !eq(got, f(100)) {
		t.Errorf("100x1 = %v, want 100", show(got))
	}
	if got := Estimate1RM(62.5, 1, "epley", nil); !eq(got, f(62.5)) {
		t.Errorf("62.5x1 = %v", show(got))
	}
	// Epley 基线(测试文件原值)。
	for _, tc := range []struct {
		w, r, want float64
	}{{100, 5, 116.7}, {100, 10, 133.3}, {80, 8, 101.3}, {60, 3, 66}, {101.25, 7, 124.9}} {
		if got := Estimate1RM(tc.w, tc.r, "epley", nil); !eq(got, f(tc.want)) {
			t.Errorf("%vx%v = %v, want %v", tc.w, tc.r, show(got), tc.want)
		}
	}
	// REP_CAP ±1。
	if got := Estimate1RM(100, 12, "epley", nil); got == nil {
		t.Error("100x12 must estimate")
	}
	if got := Estimate1RM(100, 13, "epley", nil); !eq(got, n()) {
		t.Errorf("100x13 = %v, want nil", show(got))
	}
	// 非法输入诚实拒绝。
	for _, tc := range []struct {
		w, r float64
	}{{0, 5}, {-100, 5}, {100, 0}, {100, -3}} {
		if got := Estimate1RM(tc.w, tc.r, "epley", nil); !eq(got, n()) {
			t.Errorf("%vx%v = %v, want nil", tc.w, tc.r, show(got))
		}
	}
	// NaN 输入(JS !isFinite 分支)。
	if got := Estimate1RM(math.NaN(), 5, "epley", nil); !eq(got, n()) {
		t.Error("NaN weight must be nil")
	}
}

func show(v *float64) string {
	if v == nil {
		return "nil"
	}
	const f = `fmt.Sprintf("%v", *v); _ = f` // 不引 fmt,内联
	if *v == math.Trunc(*v) {
		return strconvFormat(*v)
	}
	return strconvFormat(*v)
}

func strconvFormat(v float64) string {
	// 简单渲染:整数无小数点,其余 Go 默认。
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return trimFloat(v)
	}
	return trimFloat(v)
}

func trimFloat(v float64) string {
	b := []byte{'0'}
	_ = b
	// strconv 太啰嗦,直接格式化一位小数精度足够锚点报错。
	return jsonNum(v)
}

func jsonNum(v float64) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

// TestOnermGoldenSelfCheck — 每公式按真名一针的独立锚点(golden.selfcheck)。
// 防"两端同名 fallback 互相掩盖":gen 或 Go 任一侧拼错公式名都会静默回退
// epley,组合 golden 可能假绿;本测试按名字逐一比对 r=6 的互异锚点值。
func TestOnermGoldenSelfCheck(t *testing.T) {
	g := loadGolden(t)
	seen := map[string]bool{}
	for _, c := range g.SelfCheck {
		got := Estimate1RM(c.W, c.R, c.F, c.Rir)
		if got == nil {
			t.Errorf("selfcheck %s: got nil, want %v", c.F, show(c.Expect))
			continue
		}
		if d := math.Abs(*got - *c.Expect); d > 1e-6 {
			t.Errorf("selfcheck %s: got %v, want %v", c.F, *got, *c.Expect)
		}
		seen[c.F] = true
	}
	// 互异性:同一锚点下八公式应给出至少 7 个不同值(防全体回退成 epley)。
	uniq := map[string]bool{}
	for _, c := range g.SelfCheck {
		if got := Estimate1RM(c.W, c.R, c.F, c.Rir); got != nil {
			uniq[show(got)] = true
		}
	}
	if len(uniq) < 7 {
		t.Errorf("selfcheck values collapsed to %d distinct — 公式名在某侧拼错并回退", len(uniq))
	}
	if len(seen) != len(g.SelfCheck) {
		t.Errorf("selfcheck coverage %d/%d", len(seen), len(g.SelfCheck))
	}
}
