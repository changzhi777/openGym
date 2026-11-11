// Package weknora — WeKnora(腾讯 LLM 知识平台)REST 客户端(方案 v2 六,D14/D15/D16)。
//
// 集成形态:旁路独立服务,沐禾侧零依赖进主栈(net/http + encoding/json)。
//   - 全局知识区(共享只读)与用户记忆区(metadata/独立 KB)经本客户端访问;
//   - 客户端强制按调用方传入的 knowledgeBaseIDs 检索——业务代码无法漏传范围,
//     隔离边界在封装层(D14);
//   - 所有方法失败返回 error,由调用方决定降级(D15:工具失败回填,Coach 不拒服务)。
//
// API 形状依据 WeKnora docs/docs.go(swag 生成,v0.9):POST /knowledge-search
// (SearchKnowledgeRequest),响应 {success, data[], has_more};basePath 是部署期
// 模板(默认 /api/v1,可经 WEKNORA_BASE_URL 覆盖,实施时按实际部署校准——D16)。
// 认证 Bearer 或 ApiKey(服务间用 API key)。
package weknora

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

// Client 是 WeKnora REST 客户端。可并发使用。
type Client struct {
	BaseURL string // 含 API 前缀,不带尾斜杠;调用拼 "/knowledge-search" 等
	APIKey  string
	HTTP    *http.Client // nil → 默认(30s;检索是旁路,超时收敛)
}

// FromEnv 按环境变量装配:WEKNORA_BASE_URL/WEKNORA_API_KEY。
// 任一未配置返回 nil——调用方据此走 D15 降级(工具不注册/记忆落本地镜像)。
func FromEnv() *Client {
	base := os.Getenv("WEKNORA_BASE_URL")
	key := os.Getenv("WEKNORA_API_KEY")
	if base == "" || key == "" {
		return nil
	}
	return &Client{BaseURL: strings.TrimRight(base, "/"), APIKey: key, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Chunk 是一条检索命中(宽容解析:未知字段忽略,缺失字段零值)。
type Chunk struct {
	Content     string  `json:"content"`
	Score       float64 `json:"score"`
	KnowledgeID string  `json:"knowledge_id"`
	Filename    string  `json:"filename"`
}

// SearchRequest 是全局知识检索请求(D14:kbIDs 必须显式给出——全局知识区
// 传共享 KB,用户记忆区传用户 KB;空 ids 直接报错,防裸检索越区)。
type SearchRequest struct {
	Query            string   `json:"query"`
	KnowledgeBaseIDs []string `json:"knowledge_base_ids,omitempty"`
	MatchCount       int      `json:"match_count,omitempty"` // topK,0 → 服务端默认
}

// Search 跨指定知识库检索。返回命中切片(按相关度,服务端序)。
func (c *Client) Search(ctx context.Context, req SearchRequest) ([]Chunk, error) {
	if c == nil || c.APIKey == "" {
		return nil, fmt.Errorf("weknora: client not configured")
	}
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("weknora: empty query")
	}
	if len(req.KnowledgeBaseIDs) == 0 {
		return nil, fmt.Errorf("weknora: knowledge_base_ids required (D14 隔离:不允许裸检索)")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	raw, err := c.do(ctx, http.MethodPost, "/knowledge-search", body)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Success bool `json:"success"`
		Data    []struct {
			Content     string          `json:"content"`
			Score       float64         `json:"score"`
			KnowledgeID string          `json:"knowledge_id"`
			Filename    string          `json:"filename"`
			Extra       json.RawMessage `json:"-"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("weknora: decode search: %w", err)
	}
	out := make([]Chunk, 0, len(resp.Data))
	for _, d := range resp.Data {
		out = append(out, Chunk{Content: d.Content, Score: d.Score, KnowledgeID: d.KnowledgeID, Filename: d.Filename})
	}
	return out, nil
}

// KnowledgeBase 是一个知识库句柄。
type KnowledgeBase struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateKnowledgeBase 建库(ETL/用户记忆区初始化用)。已同名时返回既有句柄的
// 责任在调用方(先 List 再 Create 的幂等由上层保证)。
func (c *Client) CreateKnowledgeBase(ctx context.Context, name, description string) (*KnowledgeBase, error) {
	if c == nil || c.APIKey == "" {
		return nil, fmt.Errorf("weknora: client not configured")
	}
	body, err := json.Marshal(map[string]any{"name": name, "description": description})
	if err != nil {
		return nil, err
	}
	raw, err := c.do(ctx, http.MethodPost, "/knowledge-bases", body)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data KnowledgeBase `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("weknora: decode kb: %w", err)
	}
	return &resp.Data, nil
}

// ImportDocument 向知识库导入一份纯文本文档(catalogue ETL 与记忆条目写入共用;
// D16:媒体永不进管道)。返回文档 ID。服务端异步解析,导入成功≠立即可检索
// (ETL 侧以轮询/时间窗处理,v1 不等待)。
func (c *Client) ImportDocument(ctx context.Context, kbID, title, text string) (string, error) {
	if c == nil || c.APIKey == "" {
		return "", fmt.Errorf("weknora: client not configured")
	}
	if kbID == "" || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("weknora: kbID and text required")
	}
	body, err := json.Marshal(map[string]any{
		"knowledge_base_id": kbID,
		"title":             title,
		"content":           text,
	})
	if err != nil {
		return "", err
	}
	raw, err := c.do(ctx, http.MethodPost, "/knowledge-bases/"+kbID+"/documents", body)
	if err != nil {
		return "", err
	}
	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("weknora: decode doc: %w", err)
	}
	return resp.Data.ID, nil
}

// do 统一请求(Bearer 认证 + 状态码收敛 + 响应体上界)。
func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	httpc := c.HTTP
	if httpc == nil {
		httpc = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("weknora: request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+c.APIKey)
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weknora: transport: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("weknora: read: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(raw)
		if len(snippet) > 512 {
			snippet = snippet[:512]
		}
		return nil, fmt.Errorf("weknora: status %d: %s", resp.StatusCode, snippet)
	}
	return raw, nil
}
