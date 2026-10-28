// Package presence — 训练中在线表(契约 /api/activity + Presence;Node 同语义移植)。
// Redis TTL 承载易失态:20s 客户端心跳,TTL 70s(~3.5×);hidden(页面进后台)后
// 45s 宽限才从面板消失。管理端消费:users 行 live 字段 + /api/admin/live SSE。
package presence

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/changzhi777/openGym/backend-go/internal/auth"
)

const (
	keyPrefix = "alive:"
	ttl       = 70 * time.Second       // Node PRESENCE_TTL
	hiddenFor = 45 * time.Second       // Node PRESENCE_HIDDEN_GRACE
)

// Row 与契约 Presence 同形(camelCase,前端直接消费)。
type Row struct {
	Name      string `json:"name"`
	ExIdx     int    `json:"exIdx"`
	ExTotal   int    `json:"exTotal"`
	SetsDone  int    `json:"setsDone"`
	SetsTotal int    `json:"setsTotal"`
	StartedAt int64  `json:"startedAt"`
	UpdatedAt int64  `json:"updatedAt"`
	HiddenAt  int64  `json:"hiddenAt,omitempty"`
}

func key(uid string) string { return keyPrefix + uid }

// Set 心跳落地(active=true);每次重写清除 hidden 标记(与 Node 同义)。
func Set(ctx context.Context, rdb redis.UniversalClient, uid string, r Row) {
	r.UpdatedAt = time.Now().UnixMilli()
	if b, err := json.Marshal(r); err == nil {
		rdb.Set(ctx, key(uid), b, ttl)
	}
}

// MarkHidden 页面进后台:只记首个 hiddenAt(重复标不延长宽限,Node 同义)。
func MarkHidden(ctx context.Context, rdb redis.UniversalClient, uid string) {
	var r Row
	if err := json.Unmarshal([]byte(rdb.Get(ctx, key(uid)).Val()), &r); err != nil || r.HiddenAt != 0 {
		return
	}
	r.HiddenAt = time.Now().UnixMilli()
	if b, err := json.Marshal(r); err == nil {
		rdb.Set(ctx, key(uid), b, ttl)
	}
}

func Drop(ctx context.Context, rdb redis.UniversalClient, uid string) {
	rdb.Del(ctx, key(uid))
}

// Live 单用户在线行;过期/hidden 超宽限 → false(Node livePresence 同义)。
func Live(ctx context.Context, rdb redis.UniversalClient, uid string) (map[string]any, bool) {
	var r Row
	if json.Unmarshal([]byte(rdb.Get(ctx, key(uid)).Val()), &r) != nil {
		return nil, false
	}
	now := time.Now().UnixMilli()
	if now-r.UpdatedAt > ttl.Milliseconds() || (r.HiddenAt != 0 && now-r.HiddenAt > hiddenFor.Milliseconds()) {
		return nil, false
	}
	b, _ := json.Marshal(r)
	var m map[string]any
	json.Unmarshal(b, &m)
	delete(m, "hiddenAt")
	return m, true
}

// Index 全量在线表(uid → 行),供 users 列表与概览一次性取用。
func Index(ctx context.Context, rdb redis.UniversalClient) map[string]map[string]any {
	out := map[string]map[string]any{}
	keys, err := rdb.Keys(ctx, keyPrefix+"*").Result()
	if err != nil || len(keys) == 0 {
		return out
	}
	vals, err := rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return out
	}
	now := time.Now().UnixMilli()
	for i, v := range vals {
		s, _ := v.(string)
		if s == "" {
			continue
		}
		var r Row
		if json.Unmarshal([]byte(s), &r) != nil {
			continue
		}
		if now-r.UpdatedAt > ttl.Milliseconds() || (r.HiddenAt != 0 && now-r.HiddenAt > hiddenFor.Milliseconds()) {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(s), &m) != nil {
			continue
		}
		delete(m, "hiddenAt")
		out[strings.TrimPrefix(keys[i], keyPrefix)] = m
	}
	return out
}

// Handlers — /api/activity 心跳与 /api/admin/live SSE。
type Handlers struct {
	Rdb      *redis.Client
	Sessions *auth.Sessions
	DB       *pgxpool.Pool
}

// Report POST /api/activity — 训练心跳(Node 同语义):active 落地 / hidden 标记 /
// 其余(navigated/closed/旧客户端)即删。
func (h *Handlers) Report(c *gin.Context) {
	u, ok := auth.UserOf(c, h.DB, h.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	uid := u.ID
	var body struct {
		Active    bool   `json:"active"`
		Reason    string `json:"reason"`
		Name      string `json:"name"`
		ExIdx     int    `json:"exIdx"`
		ExTotal   int    `json:"exTotal"`
		SetsDone  int    `json:"setsDone"`
		SetsTotal int    `json:"setsTotal"`
		StartedAt int64  `json:"startedAt"`
	}
	if raw, ok := c.Get("rawBody"); ok {
		if b, ok2 := raw.([]byte); ok2 {
			json.Unmarshal(b, &body)
		}
	}
	switch {
	case body.Active:
		Set(c.Request.Context(), h.Rdb, uid, Row{
			Name: clip(body.Name, 60), ExIdx: body.ExIdx, ExTotal: body.ExTotal,
			SetsDone: body.SetsDone, SetsTotal: body.SetsTotal,
			StartedAt: orNow(body.StartedAt),
		})
	case body.Reason == "hidden":
		MarkHidden(c.Request.Context(), h.Rdb, uid)
	default:
		Drop(c.Request.Context(), h.Rdb, uid)
	}
	c.JSON(200, gin.H{"ok": true})
}

// LiveSSE GET /api/admin/live — 每 15s 推在线快照,10 分钟后断开(客户端重连;契约 adminLive)。
func (h *Handlers) LiveSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	fl, _ := c.Writer.(http.Flusher)
	deadline := time.After(10 * time.Minute)
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	push := func() bool {
		b, err := json.Marshal(gin.H{"live": rowsOf(Index(c.Request.Context(), h.Rdb)), "now": time.Now().UnixMilli()})
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(c.Writer, "event: live\ndata: %s\n\n", b); err != nil {
			return false
		}
		if fl != nil {
			fl.Flush()
		}
		return true
	}
	push()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-deadline:
			return
		case <-tick.C:
			if !push() {
				return
			}
		}
	}
}

func rowsOf(idx map[string]map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(idx))
	for uid, m := range idx {
		m["id"] = uid
		out = append(out, m)
	}
	return out
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func orNow(v int64) int64 {
	if v <= 0 {
		return time.Now().UnixMilli()
	}
	return v
}
