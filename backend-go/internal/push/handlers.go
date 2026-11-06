// push 七端点(P4):public-key / subscribe / status / unsubscribe / test / rest-timer(+/cancel)。
package push

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/changzhi777/openGym/backend-go/internal/auth"
	"github.com/changzhi777/openGym/backend-go/internal/store"
)

type Handlers struct {
	DB       *pgxpool.Pool
	Sessions *auth.Sessions
	Svc      *Service
	// 限频(安全对齐,上游 febb2b2b3):rest-timer 60 次/分、测试推送 10 次/10 分,按账号。
	RestSets  *rateWindow
	PushTests *rateWindow
}

// rateWindow 固定窗口限频(按账号;内存态,重启清零——与限流器同口径)。
type rateWindow struct {
	mu   sync.Mutex
	hits map[string][]time.Time
	max  int
	win  time.Duration
}

func newRateWindow(max int, win time.Duration) *rateWindow {
	return &rateWindow{hits: map[string][]time.Time{}, max: max, win: win}
}

func (w *rateWindow) allow(user string) bool {
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	fresh := w.hits[user][:0]
	for _, t := range w.hits[user] {
		if now.Sub(t) < w.win {
			fresh = append(fresh, t)
		}
	}
	if len(fresh) >= w.max {
		w.hits[user] = fresh
		return false
	}
	w.hits[user] = append(fresh, now)
	return true
}

func (h *Handlers) body(c *gin.Context) map[string]any {
	var m map[string]any
	if raw, ok := c.Get("rawBody"); ok {
		if b, ok := raw.([]byte); ok {
			json.Unmarshal(b, &m)
		}
	}
	return m
}

// PublicKey GET /api/push/public-key
func (h *Handlers) PublicKey(c *gin.Context) { c.JSON(200, gin.H{"key": h.Svc.V.PublicKey}) }

// Subscribe POST /api/push/subscribe
func (h *Handlers) Subscribe(c *gin.Context) {
	u, ok := auth.UserOf(c, h.DB, h.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	sub, _ := h.body(c)["subscription"].(map[string]any)
	keys, _ := sub["keys"].(map[string]any)
	endpoint, _ := sub["endpoint"].(string)
	p256dh, _ := keys["p256dh"].(string)
	authKey, _ := keys["auth"].(string)
	if endpoint == "" || p256dh == "" || authKey == "" {
		c.JSON(400, gin.H{"error": "invalid subscription"})
		return
	}
	if bad := EndpointError(endpoint); bad != "" {
		c.JSON(400, gin.H{"error": bad})
		return
	}
	dev, _ := h.body(c)["deviceId"].(string)
	if err := store.UpsertSub(c.Request.Context(), h.DB, store.PushSub{
		Endpoint: endpoint, UserID: u.ID, DeviceID: dev, P256dh: p256dh, Auth: authKey,
	}); err != nil {
		c.JSON(500, gin.H{"error": "save"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// Status GET /api/push/status?endpoint=
func (h *Handlers) Status(c *gin.Context) {
	u, ok := auth.UserOf(c, h.DB, h.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	ep := c.Query("endpoint")
	if dev, subscribed := store.SubStatus(c.Request.Context(), h.DB, u.ID, ep); subscribed {
		c.JSON(200, gin.H{"subscribed": true, "deviceId": nilIfEmpty(dev)})
		return
	}
	c.JSON(200, gin.H{"subscribed": false})
}

// Unsubscribe POST /api/push/unsubscribe
func (h *Handlers) Unsubscribe(c *gin.Context) {
	u, ok := auth.UserOf(c, h.DB, h.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	ep, _ := h.body(c)["endpoint"].(string)
	store.DeleteSub(c.Request.Context(), h.DB, u.ID, ep)
	c.JSON(200, gin.H{"ok": true})
}

// Test POST /api/push/test — 给该用户全部订阅发一条。
func (h *Handlers) Test(c *gin.Context) {
	u, ok := auth.UserOf(c, h.DB, h.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	if h.PushTests != nil && !h.PushTests.allow(u.ID) {
		c.JSON(429, gin.H{"error": "too many test pushes", "code": "rate"})
		return
	}
	payload, _ := json.Marshal(map[string]any{"title": "沐禾健身", "body": "推送测试成功", "tag": "test"})
	h.fanout(c, u.ID, payload)
	c.JSON(200, gin.H{"ok": true})
}

// RestTimer POST /api/push/rest-timer — seconds 缺失/非法 → 400(先校验后夹取,Node 同序)。
func (h *Handlers) RestTimer(c *gin.Context) {
	u, ok := auth.UserOf(c, h.DB, h.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	raw := h.body(c)["seconds"]
	var n float64
	switch v := raw.(type) {
	case float64:
		n = v
	case string:
		fmt.Sscanf(v, "%f", &n)
	default:
		n = -1
	}
	if !(n >= 1) {
		c.JSON(400, gin.H{"error": "seconds required"})
		return
	}
	if h.RestSets != nil && !h.RestSets.allow(u.ID) {
		c.JSON(429, gin.H{"error": "too many rest timers", "code": "rate"})
		return
	}
	sec := int(min(3600, n+0.5))
	dev, _ := h.body(c)["deviceId"].(string)
	payload, _ := json.Marshal(map[string]any{"title": "休息结束", "body": "下一组", "tag": "rest", "urgency": "high"})
	h.Svc.ScheduleRest(u.ID+"|"+dev, time.Duration(sec)*time.Second, func() { h.fanout(nil, u.ID, payload) })
	c.JSON(200, gin.H{"ok": true})
}

// RestTimerCancel POST /api/push/rest-timer/cancel
func (h *Handlers) RestTimerCancel(c *gin.Context) {
	u, ok := auth.UserOf(c, h.DB, h.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	dev, _ := h.body(c)["deviceId"].(string)
	h.Svc.CancelRest(u.ID + "|" + dev)
	c.JSON(200, gin.H{"ok": true})
}

func (h *Handlers) fanout(c *gin.Context, uid string, payload []byte) {
	h.Notify(nilCtx(c), uid, payload)
}

// Notify 按 uid 向全部端点广播(rest-timer 与内部桥共用;尽力而为,410/404 死订阅即删)。
// 停用账号不投递(安全对齐,上游 febb2b2b3:停用账号收不到提醒/推送)。
func (h *Handlers) Notify(ctx context.Context, uid string, payload []byte) {
	var disabled bool
	h.DB.QueryRow(ctx, `SELECT disabled FROM users WHERE id=$1`, uid).Scan(&disabled)
	if disabled {
		return
	}
	subs, err := store.SubsOf(ctx, h.DB, uid)
	if err != nil {
		return
	}
	for _, s := range subs {
		code, err := h.Svc.Send(Endpoint{Endpoint: s.Endpoint, P256dh: s.P256dh, Auth: s.Auth}, payload)
		if err == nil && (code == http.StatusGone || code == http.StatusNotFound) {
			store.DeleteSub(ctx, h.DB, uid, s.Endpoint) // 410 死订阅即删(Node 同义)
		}
	}
}

func nilCtx(c *gin.Context) context.Context {
	if c != nil {
		return c.Request.Context()
	}
	return context.Background()
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
