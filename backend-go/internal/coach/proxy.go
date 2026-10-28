// Package coach — AI 教练 sidecar 桥(fork 过渡层)。
// /api/coach/* 与 /api/admin/coach/* 反代到 Node sidecar(coach-sidecar/server.mjs,
// 复用 api/coach 全部逻辑);鉴权在 Go 完成后注入 X-Coach-UID/X-Coach-Admin。
// sidecar 反向依赖的数据(/internal/coach/*:state、群体、推送、公开配置)也挂在这里,
// 以 INTERNAL_TOKEN 相认(未设 token 时内部端点不注册,fail-closed)。
package coach

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/changzhi777/openGym/backend-go/internal/auth"
	"github.com/changzhi777/openGym/backend-go/internal/httpx"
	"github.com/changzhi777/openGym/backend-go/internal/push"
)

type Proxy struct {
	Addr     string // sidecar 基址(http://127.0.0.1:3111);空 = 教练不在部署里
	Token    string // 与 sidecar 相同的 INTERNAL_TOKEN
	DB       *pgxpool.Pool
	Sessions *auth.Sessions
	Push     *push.Handlers
	Quota    QuotaGate // 周期硬限;nil = 不设限(如未部署 Redis)
	StepUpOK func(uid string) bool // 敏感管理操作(config/connect/disconnect)的 step-up 窗口;nil = 不强制
	client   *http.Client

	cfgMu sync.Mutex
	cfgAt time.Time
	cfg   []byte // sidecar 的 public-config(10s 缓存,/api/config 的 Coach 块数据源)
}

// QuotaGate 计费闸门的最小接口(coach 不 import quota,测试可替换)。
type QuotaGate interface {
	Allow(ctx context.Context, uid, resource string) (ok bool, used, limit int64)
}

func New(addr, token string, db *pgxpool.Pool, ss *auth.Sessions, ph *push.Handlers) *Proxy {
	return &Proxy{
		Addr: addr, Token: token, DB: db, Sessions: ss, Push: ph,
		client: &http.Client{Timeout: 120 * time.Second}, // 一个教练任务要跑分钟级
	}
}

// billable 用户主动触发的付费动作(定时巡检在 sidecar 内,不经此门——过渡期已知漏计)。
var billable = map[string]bool{
	"/api/coach/plan":    true,
	"/api/coach/debrief": true,
	"/api/coach/review":  true,
}

// Enabled 教练是否在部署里(sidecar 地址已配)。路由挂载时决定 503 还是转发。
func (p *Proxy) Enabled() bool { return p != nil && p.Addr != "" }

// Handle 用户端点(/api/coach/*):会话在 Go 验,uid 注入后转发;付费动作先过周期闸门。
func (p *Proxy) Handle(c *gin.Context) {
	u, ok := auth.UserOf(c, p.DB, p.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	if p.Quota != nil && billable[c.Request.URL.Path] {
		if ok, used, limit := p.Quota.Allow(c.Request.Context(), u.ID, "coach"); !ok {
			// 与 Node 的 cap 同码同义(sidecar 的每日 cap 之外的第二层:月度配额)。
			c.JSON(429, gin.H{"error": "the monthly Coach allowance is used up", "code": "cap", "used": used, "limit": limit})
			return
		}
	}
	p.forward(c, u.ID, false)
}

// HandleAdmin 管理端点(/api/admin/coach/*):ops 及以上(矩阵 §2.4) + 会话;
// config/connect/disconnect 属"保存密钥"语义,需 step-up 窗口(§2.4,403 code=passkey-required
// 由前端 apiAdmin 拦截弹验证重放)。
func (p *Proxy) HandleAdmin(c *gin.Context) {
	u, ok := auth.UserOf(c, p.DB, p.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	var one int
	p.DB.QueryRow(c.Request.Context(),
		`SELECT 1 FROM admin_roles WHERE user_id=$1 AND revoked_at IS NULL AND role IN ('ops','super_admin')`,
		u.ID).Scan(&one)
	if one != 1 {
		c.JSON(403, gin.H{"error": "higher role required", "code": "role"})
		return
	}
	switch c.Request.URL.Path {
	case "/api/admin/coach/config", "/api/admin/coach/connect", "/api/admin/coach/disconnect":
		if p.StepUpOK != nil && !p.StepUpOK(u.ID) {
			c.JSON(403, gin.H{"error": "confirm with your passkey first", "code": "passkey-required"})
			return
		}
	}
	p.forward(c, u.ID, true)
}

// forward 转发到 sidecar。body 从 BodyGuard 预读的字节重建(原 body 已被读空)。
func (p *Proxy) forward(c *gin.Context, uid string, admin bool) {
	if !p.Enabled() {
		c.JSON(503, gin.H{"error": "the Coach is not set up on this instance", "code": "off"})
		return
	}
	body := httpx.RawBody(c)
	if body == nil {
		body, _ = io.ReadAll(c.Request.Body)
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method,
		p.Addr+c.Request.URL.Path+"?"+c.Request.URL.RawQuery, bytes.NewReader(body))
	if err != nil {
		c.JSON(500, gin.H{"error": "internal"})
		return
	}
	req.ContentLength = int64(len(body))
	req.Header = c.Request.Header.Clone()
	req.Header.Set("X-Coach-UID", uid)
	if admin {
		req.Header.Set("X-Coach-Admin", "1")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		c.JSON(503, gin.H{"error": "the Coach is not set up on this instance", "code": "off"})
		return
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	for k, vs := range resp.Header {
		for _, v := range vs {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)
	c.Writer.Write(out)
}

/* ---------- /internal/coach/*:sidecar 的回读面(INTERNAL_TOKEN 相认) ---------- */

func (p *Proxy) internalOK(c *gin.Context) bool {
	if p.Token == "" || c.GetHeader("Authorization") != "Bearer "+p.Token {
		c.JSON(404, gin.H{"error": "no such route"})
		return false
	}
	return true
}

// IState GET /internal/coach/state/:uid — sidecar 的 readState 桥(PG 即真相)。
func (p *Proxy) IState(c *gin.Context) {
	if !p.internalOK(c) {
		return
	}
	var doc []byte
	err := p.DB.QueryRow(c.Request.Context(),
		`SELECT doc FROM states WHERE user_id = $1`, c.Param("uid")).Scan(&doc)
	if err != nil {
		c.String(200, "null") // Node readState 的缺省语义:读不到 = null
		return
	}
	c.Data(200, "application/json", doc)
}

// IUsers GET /internal/coach/users — 教练巡检/同侪对照的群体(cohort)。
func (p *Proxy) IUsers(c *gin.Context) {
	if !p.internalOK(c) {
		return
	}
	rows, err := p.DB.Query(c.Request.Context(), `SELECT id FROM users ORDER BY id`)
	if err != nil {
		c.JSON(500, gin.H{"error": "db"})
		return
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		rows.Scan(&id)
		out = append(out, id)
	}
	c.JSON(200, out)
}

// IPush POST /internal/coach/push — 教练提案就绪的广播(Node setProposalHook 同义)。
func (p *Proxy) IPush(c *gin.Context) {
	if !p.internalOK(c) {
		return
	}
	raw, _ := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	var body struct {
		Uid         string `json:"uid"`
		Title       string `json:"title"`
		Body        string `json:"body"`
		Tag         string `json:"tag"`
		URL         string `json:"url"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || body.Uid == "" {
		c.JSON(400, gin.H{"error": "bad body"})
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"title": body.Title, "body": body.Body,
		"tag": body.Tag, "url": body.URL, "kind": "coach",
	})
	go p.Push.Notify(context.Background(), body.Uid, payload)
	c.JSON(200, gin.H{"ok": true})
}

// IPublicConfig GET /internal/coach/public-config — /api/config 的 Coach 块(10s 缓存)。
func (p *Proxy) IPublicConfig(c *gin.Context) {
	if !p.internalOK(c) {
		return
	}
	c.Data(200, "application/json", p.publicConfig())
}

// PublicConfig 供 httpx.ConfigSvc 填 /api/config 的 coach 键(nil = 无教练)。
func (p *Proxy) PublicConfig() []byte {
	if !p.Enabled() {
		return nil
	}
	return p.publicConfig()
}

func (p *Proxy) publicConfig() []byte {
	p.cfgMu.Lock()
	defer p.cfgMu.Unlock()
	if time.Since(p.cfgAt) < 10*time.Second {
		return p.cfg
	}
	req, err := http.NewRequest("GET", p.Addr+"/internal/coach/public-config", nil)
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+p.Token)
		if resp, err := p.client.Do(req); err == nil {
			defer resp.Body.Close()
			if b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)); err == nil && resp.StatusCode == 200 {
				p.cfg, p.cfgAt = b, time.Now()
				return p.cfg
			}
		}
	}
	return p.cfg // sidecar 暂不可达:上次的答案比没有好
}

// TokenFromEnv 部署侧的统一取法:独立 env,与 sidecar 的 INTERNAL_TOKEN 同值。
func TokenFromEnv() string { return os.Getenv("INTERNAL_TOKEN") }
