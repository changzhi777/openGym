// review_test.go — 审核挂点单测:词表命中/直通、LLM 复检三态(放行/拦截/
// fail-closed)、链式短路。假 provider 零外部依赖。
package agent

import (
	"context"
	"testing"

	"github.com/changzhi777/openGym/backend-go/internal/llm"
)

func TestWordlistReviewer(t *testing.T) {
	r := NewWordlistReviewer([]string{" 违禁词 ", "", "赌球"})
	if ok, reason := r.Review(context.Background(), "今天练胸,正常内容"); !ok || reason != "" {
		t.Errorf("clean reply blocked: %v %v", ok, reason)
	}
	if ok, reason := r.Review(context.Background(), "这里有违禁词内容"); ok || reason != "sensitive" {
		t.Errorf("hit missed: %v %v", ok, reason)
	}
	// 大小写不敏感。
	r2 := NewWordlistReviewer([]string{"SPAM"})
	if ok, _ := r2.Review(context.Background(), "this is spam content"); ok {
		t.Error("case-insensitive match failed")
	}
	// 空表直通。
	if ok, _ := NewWordlistReviewer(nil).Review(context.Background(), "任意内容"); !ok {
		t.Error("empty wordlist must pass")
	}
}

func TestLLMReviewerVerdicts(t *testing.T) {
	// 放行。
	ok := &fakeProvider{script: []llm.Response{textResp(`{"ok": true}`)}}
	if pass, _ := NewLLMReviewer(ok).Review(context.Background(), "正常内容"); !pass {
		t.Error("clean reply must pass")
	}
	// 拦截。
	bad := &fakeProvider{script: []llm.Response{textResp(`{"ok": false, "reason": "medical"}`)}}
	if pass, reason := NewLLMReviewer(bad).Review(context.Background(), "疑似内容"); pass || reason != "llm" {
		t.Errorf("blocked = %v %v, want false/llm", pass, reason)
	}
	// 输出不可解析 → fail-closed。
	garbage := &fakeProvider{script: []llm.Response{textResp("我不会输出 JSON")}}
	if pass, reason := NewLLMReviewer(garbage).Review(context.Background(), "x"); pass || reason != "llm_error" {
		t.Errorf("garbage = %v %v, want false/llm_error", pass, reason)
	}
	// 调用失败 → fail-closed。
	dead := &fakeProvider{} // script 空即 exhausted
	if pass, reason := NewLLMReviewer(dead).Review(context.Background(), "x"); pass || reason != "llm_error" {
		t.Errorf("dead = %v %v, want false/llm_error", pass, reason)
	}
	// nil provider 直通(装配期已判空,防御兜底)。
	var nilP *LLMReviewer
	if pass, _ := nilP.Review(context.Background(), "x"); !pass {
		t.Error("nil reviewer must pass")
	}
}

func TestLLMReviewerSendsReviewPrompt(t *testing.T) {
	fp := &fakeProvider{script: []llm.Response{textResp(`{"ok": true}`)}}
	_, _ = NewLLMReviewer(fp).Review(context.Background(), "待审文本")
	if len(fp.calls) != 1 {
		t.Fatalf("calls = %d", len(fp.calls))
	}
	req := fp.calls[0]
	if req.System == "" || len(req.Messages) != 1 || req.Messages[0].Text != "待审文本" {
		t.Errorf("review request shape = %+v", req)
	}
	if req.MaxTokens != 200 {
		t.Errorf("maxTokens = %d, want 200", req.MaxTokens)
	}
}

func TestChainedShortCircuit(t *testing.T) {
	pass := ReviewerFunc(func(context.Context, string) (bool, string) { return true, "" })
	var calls int
	block := ReviewerFunc(func(context.Context, string) (bool, string) { calls++; return false, "sensitive" })
	after := ReviewerFunc(func(context.Context, string) (bool, string) {
		t.Error("chained must short-circuit after first block")
		return true, ""
	})
	if ok, reason := (Chained{pass, block, after}).Review(context.Background(), "x"); ok || reason != "sensitive" {
		t.Errorf("chained = %v %v", ok, reason)
	}
	if calls != 1 {
		t.Errorf("block called %d times", calls)
	}
	// 全过。
	if ok, _ := (Chained{pass, pass}).Review(context.Background(), "x"); !ok {
		t.Error("all-pass chain must pass")
	}
}

// 编译期确认 Reviewer 契约粘合(Reviewer 是函数类型,用 method value 断言)。
var (
	_ Reviewer = (*WordlistReviewer)(nil)
	_ Reviewer = (*LLMReviewer)(nil)
	_ Reviewer = Chained{}
)
