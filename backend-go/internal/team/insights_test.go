// insights_test.go — 分析卡规则面保护网(零 Redis 零端口):
// 环比(deltaOf 边界) + 规则解读(verdictOf 文案与 flags 同源)。
// memberInsight 的真库面挂 TEST_PG_DSN 批(见 team_real_test.go)。
package team

import (
	"strings"
	"testing"
)

func TestDeltaOf(t *testing.T) {
	cases := []struct {
		name     string
		cur, prev float64
		want     int
	}{
		{"增长", 120, 100, 20},
		{"回落", 70, 100, -30},
		{"前窗为零有量记满", 50, 0, 100},
		{"双零不动", 0, 0, 0},
		{"本期归零", 0, 100, -100},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := deltaOf(c.cur, c.prev); got != c.want {
				t.Fatalf("deltaOf(%v,%v)=%d want %d", c.cur, c.prev, got, c.want)
			}
		})
	}
}

func insightRow(mut func(r *InsightRow)) InsightRow {
	r := InsightRow{
		WeekDone: 3, WeekPlanned: 4, StreakWeeks: 5,
		Vol4w: 120, VolPrev4w: 100, VolDelta: 20,
		Est1rm: 100, Est1rmPrev4w: 100, Est1rmDelta: 0,
		LastWorkout: map[string]any{"d": "2026-11-24", "name": "push", "vol": 40},
	}
	if mut != nil {
		mut(&r)
	}
	return r
}

func TestVerdictOf(t *testing.T) {
	cases := []struct {
		name      string
		row       InsightRow
		wantSubs  []string // 文案必须包含
		wantNoSub string   // 文案不得包含
		wantFlags []string
	}{
		{
			name:      "空档引导第一课",
			row:       InsightRow{},
			wantSubs:  []string{"还没有训练记录"},
			wantFlags: []string{"miss"},
		},
		{
			name: "计划完成 → hot + 连续周",
			row: insightRow(func(r *InsightRow) {
				r.WeekDone, r.WeekPlanned = 4, 4
			}),
			wantSubs:  []string{"本周 4/4", "计划完成", "连续 5 周"},
			wantFlags: []string{"hot"},
		},
		{
			name: "部分完成报差额",
			row: insightRow(nil),
			wantSubs:  []string{"本周 3/4", "还差 1 场", "连续 5 周", "体量较上期 +20%"},
			wantNoSub: "本周还未开始",
		},
		{
			name: "本周零次 → miss",
			row: insightRow(func(r *InsightRow) {
				r.WeekDone = 0
				r.LastWorkout = map[string]any{"d": "2026-11-10", "name": "push", "vol": 40}
			}),
			wantSubs:  []string{"本周 0/4", "本周还未开始"},
			wantFlags: []string{"miss"},
		},
		{
			name: "未排计划按已练场数",
			row: insightRow(func(r *InsightRow) {
				r.WeekPlanned = 0
			}),
			wantSubs:  []string{"本周已练 3 场（未排计划）"},
			wantNoSub: "本周 3/4",
		},
		{
			name: "1RM 上升 → gain",
			row: insightRow(func(r *InsightRow) {
				r.Est1rm, r.Est1rmPrev4w, r.Est1rmDelta = 108, 100, 8
			}),
			wantSubs:  []string{"最佳估算 1RM 较上期 +8%"},
			wantFlags: []string{"gain"},
		},
		{
			name: "前窗为零的 gain 不成立(无基线)",
			row: insightRow(func(r *InsightRow) {
				r.Est1rm, r.Est1rmPrev4w, r.Est1rmDelta = 108, 0, 100
			}),
			wantNoSub: "最佳估算 1RM",
			wantFlags: []string{},
		},
		{
			name: "体量回落三成 → drop 告警",
			row: insightRow(func(r *InsightRow) {
				r.Vol4w, r.VolPrev4w, r.VolDelta = 60, 100, -40
			}),
			wantSubs:  []string{"体量较上期 -40%", "训练量明显回落"},
			wantFlags: []string{"drop"},
		},
		{
			name: "hot + gain 组合按序拼句",
			row: insightRow(func(r *InsightRow) {
				r.WeekDone, r.WeekPlanned = 5, 4
				r.Est1rm, r.Est1rmPrev4w, r.Est1rmDelta = 110, 100, 10
			}),
			wantSubs:  []string{"计划完成", "+10%"},
			wantFlags: []string{"hot", "gain"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, flags := verdictOf(&c.row)
			for _, s := range c.wantSubs {
				if !strings.Contains(v, s) {
					t.Fatalf("verdict 缺 %q: %s", s, v)
				}
			}
			if c.wantNoSub != "" && strings.Contains(v, c.wantNoSub) {
				t.Fatalf("verdict 不应含 %q: %s", c.wantNoSub, v)
			}
			if len(flags) != len(c.wantFlags) {
				t.Fatalf("flags=%v want %v", flags, c.wantFlags)
			}
			for i, f := range c.wantFlags {
				if flags[i] != f {
					t.Fatalf("flags=%v want %v", flags, c.wantFlags)
				}
			}
		})
	}
}
