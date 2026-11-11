// review.go — 审核挂点实现(T1 同步预审的 Reviewer 侧,D12)。
//
// 组合形态(main.go 装配):词表 → LLM 复检 串联,任一拦截即拦截。
// 失败策略 **fail-closed**:LLM 复检调用失败/输出不可解析一律拦截并给
// reason="llm_error"——合规优先于可用性;误伤由 admin 治理台观察词表调优(批次 5)。
//
// 注意分工:本审查保护 **chat 自由文本**;coach 批任务产物的结构安全边界仍是
// Node 侧 validate.js(方案红线①"不动",批任务迁 Go 时才翻译,见 v2 方案八)。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/changzhi777/openGym/backend-go/internal/llm"
)

// Reviewer 契约同 loop.go 的定义(此处为实现体)。

// WordlistReviewer 敏感词表审查(env MUHE_REVIEW_WORDS 逗号分隔;admin 词库管理
// 在批次 5 治理台,当前 env 即真相)。子串包含匹配,大小写不敏感。
type WordlistReviewer struct {
	words []string
}

// NewWordlistReviewer 构造;空表 = 直通(Review 恒 true)。
func NewWordlistReviewer(words []string) *WordlistReviewer {
	clean := make([]string, 0, len(words))
	for _, w := range words {
		if s := strings.TrimSpace(w); s != "" {
			clean = append(clean, strings.ToLower(s))
		}
	}
	return &WordlistReviewer{words: clean}
}

// Review 实现 Reviewer。命中返回 (false, "sensitive")。
func (w *WordlistReviewer) Review(_ context.Context, reply string) (bool, string) {
	if len(w.words) == 0 {
		return true, ""
	}
	low := strings.ToLower(reply)
	for _, word := range w.words {
		if strings.Contains(low, word) {
			return false, "sensitive"
		}
	}
	return true, ""
}

// llmReviewerSystem 复检模型的人设与输出契约(强 JSON,拒绝发挥)。
const llmReviewerSystem = `你是内容安全审核员。对给定文本只做合规判断,不复述、不评点。
只输出一个 JSON 对象,格式: {"ok": true} 或 {"ok": false, "reason": "简短类别"}。
拦截标准:违法信息、医疗诊断与处方(建议就医除外)、自伤/暴力助长、歧视辱骂。
健身指导、训练激励、一般营养常识一律放行。`

// LLMReviewer 用 provider 做第二道复检(D12:复用同一 GLM 端点,独立审核
// prompt;ledger 单列 review 池)。fail-closed。
type LLMReviewer struct {
	Provider llm.Provider
	MaxReply int // 审核送检的回复截断(长回复只影响成本,上界 8000 字符默认)
}

// NewLLMReviewer 构造(nil provider 时 Review 直通——调用方应在装配期判空)。
func NewLLMReviewer(p llm.Provider) *LLMReviewer {
	return &LLMReviewer{Provider: p, MaxReply: 8000}
}

type llmReviewVerdict struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason"`
}

// Review 实现 Reviewer。拦截返回 (false, "llm");调用失败/解析失败返回
// (false, "llm_error")——fail-closed。
func (l *LLMReviewer) Review(ctx context.Context, reply string) (bool, string) {
	if l == nil || l.Provider == nil {
		return true, ""
	}
	text := reply
	if l.MaxReply > 0 && len(text) > l.MaxReply {
		text = text[:l.MaxReply]
	}
	resp, err := l.Provider.Invoke(ctx, &llm.Request{
		System:    llmReviewerSystem,
		Messages:  []llm.Message{{Role: "user", Text: text}},
		MaxTokens: 200,
	})
	if err != nil {
		return false, "llm_error"
	}
	var v llmReviewVerdict
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Text)), &v); err != nil {
		return false, "llm_error"
	}
	if v.OK {
		return true, ""
	}
	return false, "llm"
}

// Chained 串联多个 Reviewer:短路于第一个拦截。
type Chained []Reviewer

// Review 实现 Reviewer。
func (c Chained) Review(ctx context.Context, reply string) (bool, string) {
	for _, r := range c {
		if ok, reason := r.Review(ctx, reply); !ok {
			return false, reason
		}
	}
	return true, ""
}

// String 调试/治理台展示用(链长;Reviewer 是函数类型,无类型身份可枚举——
// 链的构成以 main.go 装配代码为准)。
func (c Chained) String() string {
	return fmt.Sprintf("review-chain(%d)", len(c))
}
