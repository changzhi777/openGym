// Package llm — AI-Native 内核的 provider 抽象层(方案 v2 五.5.1,T2)。
//
// 三件套接口(invoke + 流式 + 计量)的 v1:先落 Invoke(非流式,tool-loop 主路径)。
// 流式在批次 2 子步③接入 SSE 事件流时补(Stream);Usage 随 Response 返回。
//
// 首实现 anthropic.go:Anthropic Messages 兼容协议客户端,直连 GLM
// (智谱 open.bigmodel.cn/api/anthropic 兼容端点,密钥 env 注入,零入仓)。
// 后续 openai_compat.go:Qwen/DeepSeek 入可配路由表。
//
// 依赖纪律:零新依赖(net/http + encoding/json 即够),go.mod 直接依赖不涨。
package llm

import "context"

// Message 是 tool-loop 的中立消息模型(不绑任何一家协议;协议形状由各 provider
// 适配器转换)。Role: "user" | "assistant" | "tool"。
//  - user:      Text 即用户输入。
//  - assistant: Text 是回复文本;ToolCalls 非空表示模型要调工具。
//  - tool:      ToolCallID 指向被回填的调用,Text 是结果(≤2KB,循环层截断)。
type Message struct {
	Role       string     `json:"role"`
	Text       string     `json:"text,omitempty"`
	ToolCalls  []ToolCall `json:"toolCalls,omitempty"`
	ToolCallID string     `json:"toolCallId,omitempty"`
}

// ToolCall 是模型发起的一次工具调用(propose_* 提议工具同样走此形状,
// 是否执行的裁决权在循环层与用户确认闸门,见方案 v2 五.5.2)。
type ToolCall struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input,omitempty"`
}

// ToolDef 是注册给模型的工具契约(zod schema 的 Go 侧对应,JSON Schema 形状)。
type ToolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// Usage 是一次调用的 token 计量(ledger 三池分账的原始数据,D12/D13')。
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Request 是一次补全调用。System 独立于消息(Anthropic 顶层参数;OpenAI 兼容层
// 自行降为 system 消息)。Tools 非空时模型可发起 ToolCalls。
type Request struct {
	System    string
	Messages  []Message
	Tools     []ToolDef
	MaxTokens int // 0 → provider 默认
}

// Response 是一次补全的结果。StopReason: "end_turn"(文本回复) |
// "tool_use"(要调工具) | "max_tokens"(截断,循环层按文本处理并记录)。
type Response struct {
	Text       string
	ToolCalls  []ToolCall
	StopReason string
	Usage      Usage
}

// Provider 是模型后端的抽象(T2 三件套的 v1:先 Invoke)。实现必须可并发使用。
type Provider interface {
	// Invoke 发起一次补全。实现不得修改 req;ctx 取消/超时必须生效。
	Invoke(ctx context.Context, req *Request) (*Response, error)
}
