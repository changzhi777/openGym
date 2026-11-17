// loop.go — 受限 tool-loop 主循环(方案 v2 五.5.2,AI-Native L2 内核心脏)。
//
// 形状:for turn in 1..MAX_TURNS { invoke → tool_calls? → 执行/提议 → 回填 → continue
//       : 文本回复 → 审核(T1 先审后流) → token/done 事件 → 结束 }。
//
// 不变量(方案 v2 红线):
//   - 提议工具(Proposal)永不执行,调用即转确认队列——未经确认写入为零;
//   - MAX_TURNS 硬顶、工具结果截断——对话循环失控防线;
//   - 审核在流出之前——拦截 = 不发送,无撤回机制(T1 同步预审)。
//
// v1 非流式:回复整段过审后才 emit 单个 token 事件——SSE 逐 token 由
// provider.Stream(子步③后段)接入时保持同一事件契约。
package agent

import (
	"context"
	"fmt"

	"github.com/changzhi777/openGym/backend-go/internal/llm"
)

// SSE 事件类型(契约 GET /api/coach/chat/{id}/events 六事件)。
const (
	EventToken         = "token"
	EventToolCall      = "tool_call"
	EventProposal      = "proposal"
	EventConfirmResult = "confirm_result" // Confirm 端点决断后发(批次 B 接通)
	EventBlocked       = "blocked"
	EventDone          = "done"
)

// Event 是发给 SSE 层的一个事件。Token 携带增量文本;Proposal 携带提议;
// Blocked 携带拦截原因(sensitive/llm);Done 是最后一帧。
type Event struct {
	Kind       string
	Text       string
	ToolName   string
	Refs       []string      // tool_call:结果携带的引用 id(动作卡渲染源)
	ProposalID string
	Input      map[string]any // proposal:提议将写入的内容(确认卡片正文——用户须先见后确认)
	Usage      llm.Usage     // Done 帧携带全程累计
}

// Proposal 是模型的一次写提议(记忆/改计划),等用户 confirm。
type Proposal struct {
	ID    string
	Tool  string
	Input map[string]any
}

// LoopConfig 循环护栏(admin 可配,方案 D1/治理台)。
type LoopConfig struct {
	MaxTurns     int // 硬顶,v2 方案默认 4
	MaxToolBytes int // 单个工具结果进上下文的字节上界,默认 2048
	// ProposalPrefix 提议 id 前缀。coach_proposals.id 是全库主键而 seq 每回合
	// 归零——裸 "p1" 从全库第二条起必撞键(2026-11-18 审查轮);生产侧传
	// "<sessionID>:p",默认 "p" 仅为兼容既有测试断言。
	ProposalPrefix string
}

func (c LoopConfig) withDefaults() LoopConfig {
	if c.MaxTurns <= 0 {
		c.MaxTurns = 4
	}
	if c.MaxToolBytes <= 0 {
		c.MaxToolBytes = 2048
	}
	return c
}

// Reviewer 是审核挂点(T1):回复全文过审后才允许流出。返回 false 时
// reason 作为 blocked 事件与 RunResult.BlockReason。nil = 直通(测试用)。
// 接口形态(函数类型的教训:实现者无法入集合/链表;适配器见 ReviewerFunc)。
type Reviewer interface {
	Review(ctx context.Context, reply string) (ok bool, reason string)
}

// ReviewerFunc 把普通函数适配成 Reviewer(测试闭包用)。
type ReviewerFunc func(ctx context.Context, reply string) (ok bool, reason string)

// Review 实现 Reviewer。
func (f ReviewerFunc) Review(ctx context.Context, reply string) (bool, string) {
	return f(ctx, reply)
}

// Loop 是一次会话回合的执行器(无状态,可并发;每个回合一个 Run)。
type Loop struct {
	Provider llm.Provider
	Tools    Registry
	Config   LoopConfig
	Review   Reviewer
}

// RunResult 是一个回合的账本。
type RunResult struct {
	Text        string     // 最终文本回复(超限/拦截时可能为空)
	Turns       int        // 实际消耗轮数
	ToolRuns    int        // 只读工具执行次数
	Proposals   []Proposal // 本回合全部提议(调用方落确认队列)
	StopReason  string     // end_turn | max_turns | blocked
	BlockReason string     // 审核拦截原因(blocked 时)
	Usage       llm.Usage  // 全轮累计
}

// Run 执行一个对话回合。history 是既有轮次(不含本次用户输入);
// emit 为 nil 时事件静默丢弃。ctx 取消立即返回。
func (l *Loop) Run(ctx context.Context, system string, history []llm.Message, userMsg string, emit func(Event)) (*RunResult, error) {
	cfg := l.Config.withDefaults()
	onEvent := func(e Event) {
		if emit != nil {
			emit(e)
		}
	}
	// 消息带用户本轮输入,工作副本从 history 复制(不改调用方切片)。
	msgs := make([]llm.Message, len(history), len(history)+2*cfg.MaxTurns+1)
	copy(msgs, history)
	msgs = append(msgs, llm.Message{Role: "user", Text: userMsg})

	res := &RunResult{}
	proposalSeq := 0

	for turn := 1; turn <= cfg.MaxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("agent: canceled before turn %d: %w", turn, err)
		}
		resp, err := l.Provider.Invoke(ctx, &llm.Request{
			System: system, Messages: msgs, Tools: l.Tools.Defs(),
		})
		if err != nil {
			return nil, fmt.Errorf("agent: turn %d: %w", turn, err)
		}
		res.Turns = turn
		res.Usage.InputTokens += resp.Usage.InputTokens
		res.Usage.OutputTokens += resp.Usage.OutputTokens

		if len(resp.ToolCalls) > 0 {
			toolMsgs, proposals, err := l.handleToolCalls(ctx, resp, cfg, &proposalSeq, onEvent, res)
			if err != nil {
				return nil, err
			}
			// assistant(带 tool_use) + 各 tool 结果,同序回填。
			msgs = append(msgs, llm.Message{Role: "assistant", Text: resp.Text, ToolCalls: resp.ToolCalls})
			msgs = append(msgs, toolMsgs...)
			res.Proposals = append(res.Proposals, proposals...)
			continue
		}

		// 文本回复:T1 先审后流。
		if l.Review != nil {
			ok, reason := l.Review.Review(ctx, resp.Text)
			if !ok {
				// 拦截 = 不发送:Text 不保留(下游字段不得携带被拦截内容)。
				res.StopReason = "blocked"
				res.BlockReason = reason
				onEvent(Event{Kind: EventBlocked, Text: reason})
				onEvent(Event{Kind: EventDone, Usage: res.Usage})
				return res, nil
			}
		}
		res.Text = resp.Text
		res.StopReason = "end_turn"
		onEvent(Event{Kind: EventToken, Text: res.Text})
		onEvent(Event{Kind: EventDone, Usage: res.Usage})
		return res, nil
	}

	// MAX_TURNS 硬顶:不猜,如实上报(模型反复要工具是异常信号,治理台可观测)。
	res.StopReason = "max_turns"
	onEvent(Event{Kind: EventDone, Usage: res.Usage})
	return res, nil
}

// handleToolCalls 处理一轮的全部工具调用,返回回填消息与新产生的提议。
func (l *Loop) handleToolCalls(ctx context.Context, resp *llm.Response, cfg LoopConfig, seq *int, onEvent func(Event), res *RunResult) ([]llm.Message, []Proposal, error) {
	toolMsgs := make([]llm.Message, 0, len(resp.ToolCalls))
	var proposals []Proposal
	for _, call := range resp.ToolCalls {
		tool := l.Tools.Get(call.Name)
		switch {
		case tool == nil:
			// 未注册:回填错误,让模型自己纠偏(不中断回合)。
			toolMsgs = append(toolMsgs, llm.Message{Role: "tool", ToolCallID: call.ID,
				Text: fmt.Sprintf(`{"error":"unknown tool %q"}`, call.Name)})
			res.ToolRuns++
		case tool.Proposal:
			// 提议型:永不执行。落确认队列(调用方持久化),回填等待话术。
			*seq++
			pfx := cfg.ProposalPrefix
			if pfx == "" {
				pfx = "p"
			}
			p := Proposal{ID: fmt.Sprintf("%s%d", pfx, *seq), Tool: call.Name, Input: call.Input}
			proposals = append(proposals, p)
			// input 随事件上线:确认卡片必须展示将写入的内容,"盲确认"不是确认。
			onEvent(Event{Kind: EventProposal, ProposalID: p.ID, ToolName: call.Name, Input: p.Input})
			toolMsgs = append(toolMsgs, llm.Message{Role: "tool", ToolCallID: call.ID,
				Text: `{"accepted":"proposal_queued","note":"已记录提议,用户确认后才生效"}`})
			res.ToolRuns++
		default:
			out, err := l.Tools.execReadonly(ctx, tool, call.Input, cfg.MaxToolBytes)
			if err != nil {
				// 工具失败也回填(模型可改道),不中断回合;降级语义见 D15。
				toolMsgs = append(toolMsgs, llm.Message{Role: "tool", ToolCallID: call.ID,
					Text: fmt.Sprintf(`{"error":%q}`, err.Error())})
			} else {
				toolMsgs = append(toolMsgs, llm.Message{Role: "tool", ToolCallID: call.ID, Text: out})
			}
			ev := Event{Kind: EventToolCall, ToolName: call.Name}
			if err == nil && tool.ExtractRefs != nil {
				ev.Refs = tool.ExtractRefs(out)
			}
			onEvent(ev)
			res.ToolRuns++
		}
	}
	return toolMsgs, proposals, nil
}
