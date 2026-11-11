// client_test.go — Anthropic 兼容客户端的协议契约单测(httptest 假端点,零外部依赖)。
//
// 四剧本:① 纯文本回复(校验请求头/体结构) ② 工具调用回合(校验 tools 与
// tool_use 解析) ③ 工具结果回填(校验 tool_result 线格式) ④ 错误透传。
// 另有 FromEnv 的密钥纪律断言(key 不入仓,只从 env 来)。
package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// textReply 是最小合法响应:一段文本,自然结束。
func textReply(t *testing.T, text string) map[string]any {
	t.Helper()
	return map[string]any{
		"content":     []map[string]any{{"type": "text", "text": text}},
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, code int, v any) {
	t.Helper()
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode: %v", err)
	}
}

func TestInvokeTextRoundTrip(t *testing.T) {
	var gotPath, gotKey, gotVer string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotVer = r.Header.Get("anthropic-version")
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &gotBody); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		writeJSON(t, w, 200, textReply(t, "今天安排胸+三头,4 组。"))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, APIKey: "test-key", Model: "glm-5.3-flash"}
	resp, err := c.Invoke(context.Background(), &Request{
		System:   "你是沐禾健身教练",
		Messages: []Message{{Role: "user", Text: "给我今天的计划"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if gotPath != "/v1/messages" {
		t.Errorf("path = %q, want /v1/messages", gotPath)
	}
	if gotKey != "test-key" || gotVer != AnthropicVersion {
		t.Errorf("headers: key=%q ver=%q", gotKey, gotVer)
	}
	if gotBody["model"] != "glm-5.3-flash" {
		t.Errorf("model = %v", gotBody["model"])
	}
	if gotBody["system"] != "你是沐禾健身教练" {
		t.Errorf("system = %v", gotBody["system"])
	}
	if resp.Text != "今天安排胸+三头,4 组。" {
		t.Errorf("Text = %q", resp.Text)
	}
	if resp.StopReason != "end_turn" || len(resp.ToolCalls) != 0 {
		t.Errorf("stop=%q calls=%d", resp.StopReason, len(resp.ToolCalls))
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestInvokeToolUseRoundTrip(t *testing.T) {
	var gotTools []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		gotTools = body["tools"].([]any)
		writeJSON(t, w, 200, map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "我先查一下你的周计划。"},
				{"type": "tool_use", "id": "tc_01", "name": "get_week_plan", "input": map[string]any{"week": 0}},
			},
			"stop_reason": "tool_use",
			"usage":       map[string]any{"input_tokens": 42, "output_tokens": 18},
		})
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, APIKey: "k", Model: "m"}
	resp, err := c.Invoke(context.Background(), &Request{
		Messages: []Message{{Role: "user", Text: "下周怎么练"}},
		Tools: []ToolDef{{
			Name: "get_week_plan", Description: "读取周计划",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{
				"week": map[string]any{"type": "integer"},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(gotTools) != 1 || gotTools[0].(map[string]any)["name"] != "get_week_plan" {
		t.Errorf("tools wire = %v", gotTools)
	}
	if resp.StopReason != "tool_use" {
		t.Errorf("stop = %q, want tool_use", resp.StopReason)
	}
	if resp.Text != "我先查一下你的周计划。" {
		t.Errorf("Text = %q", resp.Text)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "tc_01" ||
		resp.ToolCalls[0].Name != "get_week_plan" || resp.ToolCalls[0].Input["week"] != float64(0) {
		t.Errorf("tool calls = %+v", resp.ToolCalls)
	}
}

func TestInvokeToolResultWire(t *testing.T) {
	var gotMessages []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		gotMessages = body["messages"].([]any)
		writeJSON(t, w, 200, textReply(t, "收到,计划已核对。"))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, APIKey: "k", Model: "m"}
	_, err := c.Invoke(context.Background(), &Request{
		Messages: []Message{
			{Role: "user", Text: "下周怎么练"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "tc_01", Name: "get_week_plan", Input: map[string]any{"week": 0}}}},
			{Role: "tool", ToolCallID: "tc_01", Text: `{"days":3}`},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(gotMessages) != 3 {
		t.Fatalf("messages len = %d", len(gotMessages))
	}
	// assistant 轮:content 数组应含 tool_use 块。
	assist := gotMessages[1].(map[string]any)
	if assist["role"] != "assistant" {
		t.Errorf("assistant role = %v", assist["role"])
	}
	// tool 轮:转为 user 轮的 tool_result 块。
	tool := gotMessages[2].(map[string]any)
	if tool["role"] != "user" {
		t.Errorf("tool-result role = %v, want user", tool["role"])
	}
	blocks := tool["content"].([]any)
	first := blocks[0].(map[string]any)
	if first["type"] != "tool_result" || first["tool_use_id"] != "tc_01" || first["content"] != `{"days":3}` {
		t.Errorf("tool_result block = %v", first)
	}
}

func TestInvokeErrorPassthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, 429, map[string]any{"error": map[string]any{"type": "rate_limit_error", "message": "请求过快"}})
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, APIKey: "k", Model: "m"}
	_, err := c.Invoke(context.Background(), &Request{Messages: []Message{{Role: "user", Text: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("err = %v, want status 429", err)
	}
}

func TestInvokeUnconfigured(t *testing.T) {
	var c *Client
	if _, err := c.Invoke(context.Background(), &Request{}); err == nil {
		t.Fatal("nil client must error")
	}
	if _, err := (&Client{BaseURL: "http://x"}).Invoke(context.Background(), &Request{}); err == nil {
		t.Fatal("empty key must error")
	}
}

func TestFromEnvKeyDiscipline(t *testing.T) {
	t.Setenv("BIGMODEL_API_KEY", "")
	if c := FromEnv(); c != nil {
		t.Fatal("no key in env → FromEnv must return nil (CoachOff 语义由调用方接)")
	}
	t.Setenv("BIGMODEL_API_KEY", "env-only-key")
	t.Setenv("BIGMODEL_BASE_URL", "http://127.0.0.1:9/")
	t.Setenv("BIGMODEL_MODEL", "glm-5.3-flash")
	c, ok := FromEnv().(*Client)
	if !ok || c == nil || c.APIKey != "env-only-key" || c.BaseURL != "http://127.0.0.1:9" || c.Model != "glm-5.3-flash" {
		t.Fatalf("FromEnv = %T %+v", FromEnv(), FromEnv())
	}
}
