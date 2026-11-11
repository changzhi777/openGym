// client_test.go — WeKnora 客户端单测(httptest 假端点,零外部依赖)。
// 覆盖:检索(线格式/空 ids 拒绝/未配置拒绝)、建库、导入、错误透传。
package weknora

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchWireFormat(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"success": true, "data": [
			{"content": "深蹲要点:核心收紧", "score": 0.92, "knowledge_id": "k1", "filename": "action.md"},
			{"content": "第二段", "score": 0.71, "knowledge_id": "k2"}
		], "has_more": false}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, APIKey: "wk"}
	chunks, err := c.Search(context.Background(), SearchRequest{
		Query: "深蹲", KnowledgeBaseIDs: []string{"kb-global"}, MatchCount: 5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotPath != "/knowledge-search" || gotAuth != "Bearer wk" {
		t.Errorf("path=%q auth=%q", gotPath, gotAuth)
	}
	if gotBody["query"] != "深蹲" {
		t.Errorf("query = %v", gotBody["query"])
	}
	ids := gotBody["knowledge_base_ids"].([]any)
	if len(ids) != 1 || ids[0] != "kb-global" {
		t.Errorf("kb ids = %v", ids)
	}
	if len(chunks) != 2 || chunks[0].Content != "深蹲要点:核心收紧" || chunks[0].Score != 0.92 || chunks[1].Filename != "" {
		t.Errorf("chunks = %+v", chunks)
	}
}

func TestSearchGuards(t *testing.T) {
	c := &Client{BaseURL: "http://x", APIKey: "k"}
	if _, err := c.Search(context.Background(), SearchRequest{Query: "q"}); err == nil || !strings.Contains(err.Error(), "knowledge_base_ids") {
		t.Errorf("empty kb ids must be refused (D14): %v", err)
	}
	if _, err := c.Search(context.Background(), SearchRequest{KnowledgeBaseIDs: []string{"kb"}}); err == nil {
		t.Error("empty query must be refused")
	}
	var nilC *Client
	if _, err := nilC.Search(context.Background(), SearchRequest{Query: "q", KnowledgeBaseIDs: []string{"kb"}}); err == nil {
		t.Error("nil client must error")
	}
}

func TestCreateKBAndImport(t *testing.T) {
	var bodies []map[string]any
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		bodies = append(bodies, m)
		paths = append(paths, r.URL.Path)
		w.Header().Set("content-type", "application/json")
		if r.URL.Path == "/knowledge-bases" {
			_, _ = w.Write([]byte(`{"data": {"id": "kb-9", "name": "动作知识"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data": {"id": "doc-1"}}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, APIKey: "k"}
	kb, err := c.CreateKnowledgeBase(context.Background(), "动作知识", "catalogue 转入")
	if err != nil || kb.ID != "kb-9" {
		t.Fatalf("kb = %+v %v", kb, err)
	}
	docID, err := c.ImportDocument(context.Background(), kb.ID, "深蹲", "深蹲要点…")
	if err != nil || docID != "doc-1" {
		t.Fatalf("doc = %q %v", docID, err)
	}
	if paths[1] != "/knowledge-bases/kb-9/documents" {
		t.Errorf("import path = %q", paths[1])
	}
	if bodies[1]["knowledge_base_id"] != "kb-9" || bodies[1]["title"] != "深蹲" {
		t.Errorf("import body = %v", bodies[1])
	}
	// 空文本拒绝(防 ETL 空文档刷库)。
	if _, err := c.ImportDocument(context.Background(), kb.ID, "空", "  "); err == nil {
		t.Error("blank text must be refused")
	}
}

func TestErrorPassthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error": "overloaded"}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, APIKey: "k"}
	_, err := c.Search(context.Background(), SearchRequest{Query: "q", KnowledgeBaseIDs: []string{"kb"}})
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("err = %v, want status 503", err)
	}
}

func TestFromEnvDiscipline(t *testing.T) {
	t.Setenv("WEKNORA_BASE_URL", "")
	t.Setenv("WEKNORA_API_KEY", "")
	if c := FromEnv(); c != nil {
		t.Fatal("未配置必须返回 nil(D15 降级由调用方接)")
	}
	t.Setenv("WEKNORA_BASE_URL", "http://127.0.0.1:9")
	t.Setenv("WEKNORA_API_KEY", "wk")
	if c := FromEnv(); c == nil || c.BaseURL != "http://127.0.0.1:9" {
		t.Fatalf("FromEnv = %+v", c)
	}
}
