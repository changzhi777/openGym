// loop_test.go — tool-loop 引擎的八剧本单测(假 provider + 假工具,零外部依赖)。
//
// 覆盖:① 直答 ② 工具调用→回填→直答(校验回填线格式) ③ 提议工具永不执行
// ④ 未知工具错误回填不中断 ⑤ MAX_TURNS 硬顶 ⑥ 审核拦截=不流出(T1)
// ⑦ 工具结果截断 ⑧ ctx 取消传播。
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/changzhi777/openGym/backend-go/internal/llm"
)

// fakeProvider 按 script 依序回放,并记录收到的请求供回填断言。
type fakeProvider struct {
	script []llm.Response
	calls  []*llm.Request
}

func (f *fakeProvider) Invoke(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err() // 模拟真实 provider:ctx 取消立即生效
	default:
	}
	f.calls = append(f.calls, req)
	if len(f.script) == 0 {
		return nil, errors.New("fake: script exhausted")
	}
	r := f.script[0]
	f.script = f.script[1:]
	return &r, nil
}

func textResp(text string) llm.Response {
	return llm.Response{Text: text, StopReason: "end_turn"}
}

func toolResp(text string, calls ...llm.ToolCall) llm.Response {
	return llm.Response{Text: text, ToolCalls: calls, StopReason: "tool_use"}
}

// countingTool 是只读工具 spy:执行即计数,返回 echo 文本。
type countingTool struct {
	Tool
	runs int
}

func newCountingTool(name, echo string) *countingTool {
	t := &countingTool{}
	t.Tool = Tool{Name: name, Description: "spy", Exec: func(_ context.Context, _ map[string]any) (string, error) {
		t.runs++
		return echo, nil
	}}
	return t
}

func testRegistry(t *testing.T, lookup *countingTool) Registry {
	t.Helper()
	reg, err := NewRegistry(
		lookup.Tool,
		Tool{Name: "propose_memory", Description: "提议记忆", Proposal: true},
	)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return reg
}

func collectEvents(t *testing.T) (func(Event), *[]Event) {
	t.Helper()
	var evs []Event
	return func(e Event) { evs = append(evs, e) }, &evs
}

func TestLoopDirectAnswer(t *testing.T) {
	fp := &fakeProvider{script: []llm.Response{textResp("今天练胸,4 组卧推。")}}
	reg, _ := NewRegistry(Tool{Name: "get_week_plan", Description: "d", Exec: func(_ context.Context, _ map[string]any) (string, error) { return "{}", nil }})
	emit, evs := collectEvents(t)
	l := &Loop{Provider: fp, Tools: reg}

	res, err := l.Run(context.Background(), "sys", nil, "今天练什么", emit)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Text != "今天练胸,4 组卧推。" || res.Turns != 1 || res.StopReason != "end_turn" {
		t.Errorf("res = %+v", res)
	}
	if len(res.Proposals) != 0 || res.ToolRuns != 0 {
		t.Errorf("res = %+v", res)
	}
	kinds := []string{(*evs)[0].Kind, (*evs)[len(*evs)-1].Kind}
	if kinds[0] != EventToken || kinds[1] != EventDone {
		t.Errorf("events: first=%s last=%s", kinds[0], kinds[1])
	}
}

func TestLoopToolThenAnswer(t *testing.T) {
	spy := newCountingTool("get_week_plan", `{"days":3,"week":[["胸","背","腿"]]}`)
	fp := &fakeProvider{script: []llm.Response{
		toolResp("我先查你的周计划。", llm.ToolCall{ID: "t1", Name: "get_week_plan", Input: map[string]any{"week": float64(0)}}),
		textResp("本周 3 练:胸/背/腿。"),
	}}
	emit, evs := collectEvents(t)
	l := &Loop{Provider: fp, Tools: Registry{spy.Tool}}

	res, err := l.Run(context.Background(), "sys", nil, "下周怎么练", emit)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if spy.runs != 1 {
		t.Errorf("tool ran %d times, want 1", spy.runs)
	}
	if res.Text != "本周 3 练:胸/背/腿。" || res.Turns != 2 || res.ToolRuns != 1 {
		t.Errorf("res = %+v", res)
	}
	// 回填线格式:第二次请求应含 assistant(tool_use) + user(tool_result)。
	if len(fp.calls) != 2 {
		t.Fatalf("provider calls = %d", len(fp.calls))
	}
	msgs := fp.calls[1].Messages
	n := len(msgs)
	if n < 3 || msgs[n-2].Role != "assistant" || len(msgs[n-2].ToolCalls) != 1 {
		t.Fatalf("backfill assistant msg bad: %+v", msgs)
	}
	last := msgs[n-1]
	if last.Role != "tool" || last.ToolCallID != "t1" || !strings.Contains(last.Text, "胸") {
		t.Errorf("tool result msg = %+v", last)
	}
	// 事件里有 tool_call。
	found := false
	for _, e := range *evs {
		if e.Kind == EventToolCall && e.ToolName == "get_week_plan" {
			found = true
		}
	}
	if !found {
		t.Error("no tool_call event")
	}
}

func TestLoopProposalNeverExecutes(t *testing.T) {
	spy := newCountingTool("get_week_plan", "{}")
	fp := &fakeProvider{script: []llm.Response{
		toolResp("我建议记住你偏好哑铃。", llm.ToolCall{ID: "t2", Name: "propose_memory", Input: map[string]any{"text": "喜欢哑铃"}}),
		textResp("已记下提议,等你确认。"),
	}}
	emit, evs := collectEvents(t)
	reg := testRegistry(t, spy)
	l := &Loop{Provider: fp, Tools: reg}

	res, err := l.Run(context.Background(), "sys", nil, "记住我喜欢哑铃", emit)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Proposals) != 1 || res.Proposals[0].Tool != "propose_memory" || res.Proposals[0].ID != "p1" {
		t.Fatalf("proposals = %+v", res.Proposals)
	}
	// 红线:提议工具绝不执行(没有任何 Exec 被调)。
	if spy.runs != 0 {
		t.Errorf("exec ran %d times — 提议绝不能执行", spy.runs)
	}
	found := false
	for _, e := range *evs {
		if e.Kind == EventProposal && e.ProposalID == "p1" {
			found = true
		}
	}
	if !found {
		t.Error("no proposal event")
	}
}

func TestLoopUnknownToolBackfills(t *testing.T) {
	fp := &fakeProvider{script: []llm.Response{
		toolResp("查一下知识库。", llm.ToolCall{ID: "t3", Name: "search_knowledge", Input: nil}),
		textResp("知识服务暂时不可用。"),
	}}
	l := &Loop{Provider: fp, Tools: Registry{}}
	res, err := l.Run(context.Background(), "sys", nil, "深蹲标准动作", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Text != "知识服务暂时不可用。" || res.ToolRuns != 1 {
		t.Errorf("res = %+v", res)
	}
	// 回填含 unknown tool 错误,模型据此改道。
	msgs := fp.calls[1].Messages
	last := msgs[len(msgs)-1]
	if last.ToolCallID != "t3" || !strings.Contains(last.Text, "unknown tool") {
		t.Errorf("backfill = %+v", last)
	}
}

func TestLoopMaxTurnsCap(t *testing.T) {
	tool := llm.ToolCall{ID: "t", Name: "get_week_plan", Input: nil}
	script := make([]llm.Response, 8)
	for i := range script {
		script[i] = toolResp("再查一次。", tool)
	}
	fp := &fakeProvider{script: script}
	spy := newCountingTool("get_week_plan", "{}")
	l := &Loop{Provider: fp, Tools: Registry{spy.Tool}, Config: LoopConfig{MaxTurns: 2}}

	res, err := l.Run(context.Background(), "sys", nil, "问", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.StopReason != "max_turns" || res.Turns != 2 || res.Text != "" {
		t.Errorf("res = %+v", res)
	}
	if fp.calls[1].Messages == nil || len(fp.calls) != 2 {
		t.Errorf("calls = %d", len(fp.calls))
	}
}

func TestLoopReviewBlocksBeforeStream(t *testing.T) {
	fp := &fakeProvider{script: []llm.Response{textResp("这段不该流出。")}}
	emit, evs := collectEvents(t)
	l := &Loop{Provider: fp, Tools: Registry{}, Review: ReviewerFunc(func(_ context.Context, _ string) (bool, string) {
		return false, "sensitive"
	})}

	res, err := l.Run(context.Background(), "sys", nil, "问", emit)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.StopReason != "blocked" || res.BlockReason != "sensitive" || res.Text != "" {
		t.Fatalf("res = %+v", res)
	}
	// T1:拦截 = 不发送。事件流里不得有任何 token。
	for _, e := range *evs {
		if e.Kind == EventToken {
			t.Error("blocked reply must not emit token events")
		}
	}
	hasBlocked := false
	for _, e := range *evs {
		if e.Kind == EventBlocked && e.Text == "sensitive" {
			hasBlocked = true
		}
	}
	if !hasBlocked {
		t.Error("no blocked event")
	}
}

func TestLoopToolResultTruncated(t *testing.T) {
	big := strings.Repeat("沐禾", 10000) // 60KB,远超 2KB 上界
	spy := newCountingTool("get_week_plan", big)
	fp := &fakeProvider{script: []llm.Response{
		toolResp("查。", llm.ToolCall{ID: "t9", Name: "get_week_plan"}),
		textResp("ok"),
	}}
	l := &Loop{Provider: fp, Tools: Registry{spy.Tool}}
	if _, err := l.Run(context.Background(), "sys", nil, "问", nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	msgs := fp.calls[1].Messages
	last := msgs[len(msgs)-1]
	if len(last.Text) > 2048+64 || !strings.Contains(last.Text, "截断") {
		t.Errorf("tool result not truncated: %d bytes", len(last.Text))
	}
}

func TestLoopToolErrorBackfills(t *testing.T) {
	boom := Tool{Name: "get_bodyweight", Description: "d", Exec: func(_ context.Context, _ map[string]any) (string, error) {
		return "", errors.New("weknora unreachable")
	}}
	fp := &fakeProvider{script: []llm.Response{
		toolResp("查体重。", llm.ToolCall{ID: "t4", Name: "get_bodyweight"}),
		textResp("服务暂不可用,先凭你的记录聊。"),
	}}
	l := &Loop{Provider: fp, Tools: Registry{boom}} // D15 降级:错误回填,不中断
	res, err := l.Run(context.Background(), "sys", nil, "我最近体重", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	msgs := fp.calls[1].Messages
	last := msgs[len(msgs)-1]
	if !strings.Contains(last.Text, "weknora unreachable") {
		t.Errorf("error not backfilled: %+v", last)
	}
	if res.Text == "" {
		t.Error("degraded turn should still answer")
	}
}

func TestLoopCtxCancel(t *testing.T) {
	fp := &fakeProvider{script: []llm.Response{textResp("x")}}
	l := &Loop{Provider: fp, Tools: Registry{}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.Run(ctx, "sys", nil, "问", nil); err == nil {
		t.Fatal("canceled ctx must propagate")
	}
}

func TestRegistryValidation(t *testing.T) {
	if _, err := NewRegistry(Tool{Name: "a"}); err == nil {
		t.Error("read-only without Exec must fail")
	}
	if _, err := NewRegistry(Tool{Name: "b", Proposal: true, Exec: func(_ context.Context, _ map[string]any) (string, error) { return "", nil }}); err == nil {
		t.Error("proposal with Exec must fail")
	}
	if _, err := NewRegistry(Tool{Name: "c", Description: "d", Exec: func(_ context.Context, _ map[string]any) (string, error) { return "", nil }},
		Tool{Name: "c", Description: "d", Exec: func(_ context.Context, _ map[string]any) (string, error) { return "", nil }}); err == nil {
		t.Error("duplicate name must fail")
	}
	if _, err := fmt.Sprintf("%v", nil), error(nil); err != nil {
		t.Fatal("unreachable")
	}
}

// TestLoopRefsRideAlong — 引用 id 随 tool_call 事件流出(动作卡渲染源,批次 6 后段)。
func TestLoopRefsRideAlong(t *testing.T) {
	lookup := Tool{Name: "search_knowledge", Description: "d",
		ExtractRefs: func(result string) []string {
			var p struct {
				Results []struct {
					RefID string `json:"ref_id"`
				} `json:"results"`
			}
			_ = json.Unmarshal([]byte(result), &p)
			out := []string{}
			for _, r := range p.Results {
				out = append(out, r.RefID)
			}
			return out
		},
		Exec: func(_ context.Context, _ map[string]any) (string, error) {
			return `{"results":[{"ref_id":"1000"},{"ref_id":"1001"}]}`, nil
		}}
	fp := &fakeProvider{script: []llm.Response{
		toolResp("查知识库。", llm.ToolCall{ID: "tr1", Name: "search_knowledge"}),
		textResp("答"),
	}}
	emit, evs := collectEvents(t)
	l := &Loop{Provider: fp, Tools: Registry{lookup}}
	if _, err := l.Run(context.Background(), "sys", nil, "深蹲怎么做", emit); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var found []string
	for _, e := range *evs {
		if e.Kind == EventToolCall {
			found = e.Refs
		}
	}
	if len(found) != 2 || found[0] != "1000" || found[1] != "1001" {
		t.Errorf("refs = %v, want [1000 1001]", found)
	}
}
