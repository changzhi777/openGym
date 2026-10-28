// Package data — /api/data 三端点(P3a):整文档读 + rev 轻查 + PUT 全协议。
// 协议与 Node 逐字段对齐;merge 在前端(sync-merge.js),服务端只做 CAS 与文档维护。
package data

import (
	"crypto/rand"
	"log"
	"time"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/changzhi777/openGym/backend-go/internal/auth"
	"github.com/changzhi777/openGym/backend-go/internal/httpx"
	"github.com/changzhi777/openGym/backend-go/internal/store"
)

const widKeep = 20 // _wids 祖先链上限(Node WID_KEEP)

type Service struct {
	DB       *pgxpool.Pool
	Sessions *auth.Sessions
}

type putBody struct {
	State   json.RawMessage `json:"state"`
	BaseRev *int64          `json:"baseRev"`
	BaseWid *string         `json:"baseWid"`
	Stamped bool            `json:"stamped"`
}

// Get GET /api/data
func (s *Service) Get(c *gin.Context) {
	u, ok := auth.UserOf(c, s.DB, s.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	d, err := store.GetStateDoc(c.Request.Context(), s.DB, u.ID)
	if err == store.ErrNotFound {
		c.JSON(200, gin.H{"state": nil, "rev": 0})
		return
	}
	if err != nil {
		c.JSON(503, gin.H{"error": "state unreadable"})
		return
	}
	var doc json.RawMessage = d.Doc
	c.JSON(200, gin.H{"state": doc, "rev": d.Rev})
}

// GetRev GET /api/data/rev — 客户端每 30s 轮询,只回两个数。
func (s *Service) GetRev(c *gin.Context) {
	u, ok := auth.UserOf(c, s.DB, s.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	d, err := store.GetStateDoc(c.Request.Context(), s.DB, u.ID)
	if err == store.ErrNotFound {
		c.JSON(200, gin.H{"rev": 0})
		return
	}
	if err != nil {
		c.JSON(503, gin.H{"error": "state unreadable"})
		return
	}
	var doc map[string]any
	json.Unmarshal(d.Doc, &doc)
	out := gin.H{"rev": d.Rev}
	if w, ok := doc["_wid"].(string); ok && w != "" {
		out["wid"] = w
	}
	c.JSON(200, out)
}

// Put PUT /api/data — 条件写:baseRev/baseWid 不匹配 → 409 带当前文档(前端 merge 重试)。
func (s *Service) Put(c *gin.Context) {
	u, ok := auth.UserOf(c, s.DB, s.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	raw := httpx.RawBody(c)
	if raw == nil {
		raw, _ = io.ReadAll(c.Request.Body)
	}
	var b putBody
	if err := json.Unmarshal(raw, &b); err != nil || len(b.State) == 0 {
		c.JSON(400, gin.H{"error": "state required"})
		return
	}
	var st map[string]any
	if err := json.Unmarshal(b.State, &st); err != nil || st == nil {
		c.JSON(400, gin.H{"error": "invalid state"})
		return
	}
	var probe any
	json.Unmarshal(b.State, &probe)
	if _, isArr := probe.([]any); isArr { // 数组不是文档(Node 同)
		c.JSON(400, gin.H{"error": "invalid state"})
		return
	}
	// 「无一是用户数据」的空文档拒绝(_rev/_ts 是本路由自己写的账目)。
	has := false
	for k := range st {
		if k != "_rev" && k != "_ts" {
			has = true
			break
		}
	}
	if !has {
		c.JSON(400, gin.H{"error": "state required"})
		return
	}
	listOK := func(v any) bool { _, isArr := v.([]any); return v == nil || isArr }
	if !listOK(st["workouts"]) || !listOK(st["routines"]) {
		c.JSON(400, gin.H{"error": "invalid state"})
		return
	}
	recordsFilter(st, "workouts")
	recordsFilter(st, "routines")

	cur, err := store.GetStateDoc(c.Request.Context(), s.DB, u.ID)
	if err != nil && err != store.ErrNotFound {
		c.JSON(503, gin.H{"error": "state unreadable"})
		return
	}
	var curDoc map[string]any
	if err == nil {
		json.Unmarshal(cur.Doc, &curDoc)
	}
	// 冲突:数字 rev 或内容身份 wid 任一不匹配(Node 双保险:数据回滚后同名 rev 靠 wid 区分)。
	curRev := i64(curDoc["_rev"])
	curWid, _ := curDoc["_wid"].(string)
	if b.BaseRev != nil && (*b.BaseRev != curRev || (b.BaseWid != nil && curWid != "" && *b.BaseWid != curWid)) {
		c.JSON(409, gin.H{"error": "conflict", "rev": curRev, "state": curDoc})
		return
	}
	delete(st, "active") // 进行中的训练留在设备本地

	// resetAt 只进不退(每台见过重置的设备才不会再抹一次)。
	if r := i64(curDoc["resetAt"]); r > i64(st["resetAt"]) {
		st["resetAt"] = curDoc["resetAt"]
		if ids, ok := curDoc["resetIds"].(map[string]any); ok {
			st["resetIds"] = ids
		} else {
			delete(st, "resetIds")
		}
	}
	// P3a 已知限制:未打戳(stamped≠true)写入的完整 stampPut 保护(keepUnknown/records 连接/
	// 服务端打戳)记 P3b;现代客户端全走 stamped 路径,行为与 Node 等价。
	// sync stamps(Node stampPut 的 stamped 路径):records 只增不减 + keepHeld + 时间基线。
	StampPut(curDoc, st, b.Stamped, time.Now().UnixMilli())
	st["_rev"] = curRev + 1
	wids := []any{}
	if arr, ok := curDoc["_wids"].([]any); ok {
		wids = arr
	}
	if curWid != "" {
		wids = append(wids, curWid)
	}
	if len(wids) > widKeep {
		wids = wids[len(wids)-widKeep:]
	}
	st["_wids"] = wids
	newWid := randHex(8)
	st["_wid"] = newWid

	out, err := json.Marshal(st)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid state"})
		return
	}
	expect := curRev
	if err2 := store.PutStateDoc(c.Request.Context(), s.DB, u.ID, out, expect); err2 == store.ErrConflict {
		// 竞态:并发写先进来 → 重读再按 409 处理。
		d2, e2 := store.GetStateDoc(c.Request.Context(), s.DB, u.ID)
		if e2 == nil {
			var m map[string]any
			json.Unmarshal(d2.Doc, &m)
			c.JSON(409, gin.H{"error": "conflict", "rev": i64(m["_rev"]), "state": m})
			return
		}
		c.JSON(503, gin.H{"error": "state unreadable"})
		return
	} else if err2 != nil {
		log.Printf("PUT /api/data store error: %v", err2)
		c.JSON(503, gin.H{"error": "state unreadable"})
		return
	}
	ts, _ := st["_ts"]
	c.JSON(200, gin.H{"ok": true, "ts": ts, "rev": curRev + 1, "wid": newWid})
}

// recordsFilter:workouts/routines 里非对象条目剔除(Node records();数组承载的删除语义)。
func recordsFilter(st map[string]any, key string) {
	arr, ok := st[key].([]any)
	if !ok {
		return
	}
	out := make([]any, 0, len(arr))
	for _, e := range arr {
		if _, isObj := e.(map[string]any); isObj {
			out = append(out, e)
		}
	}
	st[key] = out
}

func i64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	}
	return 0
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

var _ = http.StatusOK
var _ = strings.TrimSpace
