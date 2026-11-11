// anthropic.go — Anthropic Messages 兼容协议客户端(v2 方案 R1:首协议)。
//
// 直连智谱 GLM 的 Anthropic 兼容端点(默认 https://open.bigmodel.cn/api/anthropic,
// 路径 /v1/messages),同样适配任何说 Anthropic 协议的 base URL。密钥一律 env
// 注入(BIGMODEL_API_KEY),不入仓不入日志。
//
// 协议要点(实现依据):
//   请求: { model, max_tokens, system, messages:[{role,content}], tools:[{name,description,input_schema}] }
//   assistant 工具调用: content 数组混排 {type:"text"} 与 {type:"tool_use",id,name,input}
//   工具结果回填:       {role:"user", content:[{type:"tool_result",tool_use_id,content}]}
//   响应: { content:[...], stop_reason:"end_turn"|"tool_use"|"max_tokens", usage:{input_tokens,output_tokens} }
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// AnthropicVersion 是协议版本头,兼容端点按此解析。
	AnthropicVersion = "2023-06-01"
	// DefaultBaseURL 是智谱 Anthropic 兼容端点(可经 BIGMODEL_BASE_URL 覆盖)。
	DefaultBaseURL = "https://open.bigmodel.cn/api/anthropic"
	// DefaultModel 是 v2 方案定的起步模型(GLM flash 档,成本可控)。
	DefaultModel = "glm-5.3-flash"
	// defaultMaxTokens 与超时:对话回合的合理上界;循环层另有 MAX_TURNS 硬顶。
	defaultMaxTokens = 4096
	invokeTimeout    = 120 * time.Second
)

// Client 是 Provider 的 Anthropic 兼容实现。零依赖:net/http + encoding/json。
// 可并发使用。
type Client struct {
	// BaseURL 不带尾斜杠,调用时拼 "/v1/messages"。
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client // nil → 默认(120s 超时)
}

// FromEnv 按环境变量装配:BIGMODEL_API_KEY(必填)/BIGMODEL_BASE_URL/BIGMODEL_MODEL。
// key 未配置返回 nil——调用方据此走"Coach 未连接"的既有语义(CoachOff)。
// 返回类型必须是接口(而非 *Client):typed nil 装进接口后 == nil 恒为 false,
// "未配置 → off"语义会整段失效(Go 经典陷阱,教训见装配处注释)。
func FromEnv() Provider {
	key := os.Getenv("BIGMODEL_API_KEY")
	if key == "" {
		return nil
	}
	base := os.Getenv("BIGMODEL_BASE_URL")
	if base == "" {
		base = DefaultBaseURL
	}
	model := os.Getenv("BIGMODEL_MODEL")
	if model == "" {
		model = DefaultModel
	}
	return &Client{BaseURL: strings.TrimRight(base, "/"), APIKey: key, Model: model}
}

// ── 协议线格式(私有) ─────────────────────────────────────────────────────────

type wireText struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

type wireToolUse struct {
	Type  string         `json:"type"` // "tool_use"
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

type wireToolResult struct {
	Type      string `json:"type"` // "tool_result"
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
}

type wireMessage struct {
	Role    string            `json:"role"`
	Content []json.RawMessage `json:"content"`
}

type wireRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system,omitempty"`
	Messages  []wireMessage `json:"messages"`
	Tools     []wireTool    `json:"tools,omitempty"`
}

type wireTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type wireUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type wireResponse struct {
	Content    []json.RawMessage `json:"content"`
	StopReason string            `json:"stop_reason"`
	Usage      wireUsage         `json:"usage"`
	// 兼容端点报错时: {type:"error", error:{type,message}}
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ── 中立模型 → 协议线格式 ────────────────────────────────────────────────────

func marshalBlock(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	return b, err
}

func toWireMessages(msgs []Message) ([]wireMessage, error) {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		wm := wireMessage{Role: m.Role}
		switch m.Role {
		case "user":
			if m.Text != "" {
				b, err := marshalBlock(wireText{Type: "text", Text: m.Text})
				if err != nil {
					return nil, err
				}
				wm.Content = append(wm.Content, b)
			}
		case "assistant":
			if m.Text != "" {
				b, err := marshalBlock(wireText{Type: "text", Text: m.Text})
				if err != nil {
					return nil, err
				}
				wm.Content = append(wm.Content, b)
			}
			for _, tc := range m.ToolCalls {
				b, err := marshalBlock(wireToolUse{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: tc.Input})
				if err != nil {
					return nil, err
				}
				wm.Content = append(wm.Content, b)
			}
		case "tool":
			b, err := marshalBlock(wireToolResult{Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Text})
			if err != nil {
				return nil, err
			}
			wm.Content = append(wm.Content, b)
			wm.Role = "user" // Anthropic 把 tool_result 放在 user 轮
		default:
			return nil, fmt.Errorf("llm: unsupported message role %q", m.Role)
		}
		if len(wm.Content) == 0 {
			return nil, fmt.Errorf("llm: %s message with no content", m.Role)
		}
		out = append(out, wm)
	}
	return out, nil
}

func toWireTools(defs []ToolDef) []wireTool {
	out := make([]wireTool, 0, len(defs))
	for _, d := range defs {
		out = append(out, wireTool{Name: d.Name, Description: d.Description, InputSchema: d.InputSchema})
	}
	return out
}

// ── 协议线格式 → 中立模型 ────────────────────────────────────────────────────

func fromWireResponse(w wireResponse) (*Response, error) {
	resp := &Response{StopReason: w.StopReason, Usage: Usage{InputTokens: w.Usage.InputTokens, OutputTokens: w.Usage.OutputTokens}}
	var texts []string
	for _, raw := range w.Content {
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			return nil, fmt.Errorf("llm: content block: %w", err)
		}
		switch probe.Type {
		case "text":
			var b wireText
			if err := json.Unmarshal(raw, &b); err != nil {
				return nil, fmt.Errorf("llm: text block: %w", err)
			}
			texts = append(texts, b.Text)
		case "tool_use":
			var b wireToolUse
			if err := json.Unmarshal(raw, &b); err != nil {
				return nil, fmt.Errorf("llm: tool_use block: %w", err)
			}
			resp.ToolCalls = append(resp.ToolCalls, ToolCall{ID: b.ID, Name: b.Name, Input: b.Input})
		default:
			// thinking 等未知块:按契约不影响回合语义,忽略。
		}
	}
	resp.Text = strings.Join(texts, "")
	return resp, nil
}

// ── Provider ─────────────────────────────────────────────────────────────────

// Invoke 发起一次补全(Provider 实现)。
func (c *Client) Invoke(ctx context.Context, req *Request) (*Response, error) {
	if c == nil {
		return nil, fmt.Errorf("llm: client not configured")
	}
	if c.APIKey == "" {
		return nil, fmt.Errorf("llm: api key empty")
	}
	maxTok := req.MaxTokens
	if maxTok <= 0 {
		maxTok = defaultMaxTokens
	}
	msgs, err := toWireMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(wireRequest{
		Model: c.Model, MaxTokens: maxTok, System: req.System,
		Messages: msgs, Tools: toWireTools(req.Tools),
	})
	if err != nil {
		return nil, fmt.Errorf("llm: encode: %w", err)
	}

	httpc := c.HTTP
	if httpc == nil {
		httpc = &http.Client{Timeout: invokeTimeout}
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llm: request: %w", err)
	}
	hreq.Header.Set("content-type", "application/json")
	hreq.Header.Set("x-api-key", c.APIKey)
	hreq.Header.Set("anthropic-version", AnthropicVersion)

	hresp, err := httpc.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("llm: transport: %w", err)
	}
	defer hresp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(hresp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("llm: read: %w", err)
	}
	if hresp.StatusCode < 200 || hresp.StatusCode >= 300 {
		snippet := string(raw)
		if len(snippet) > 512 {
			snippet = snippet[:512]
		}
		return nil, fmt.Errorf("llm: status %d: %s", hresp.StatusCode, snippet)
	}
	var w wireResponse
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, fmt.Errorf("llm: decode: %w", err)
	}
	if w.Error != nil {
		return nil, fmt.Errorf("llm: provider: %s", w.Error.Message)
	}
	return fromWireResponse(w)
}
