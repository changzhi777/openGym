// handlers.go — AI-Native 对话面七端点(契约 P0 五组,批次 1 已冻结):
//
//	POST   /api/coach/chat              发起/续聊,回会话句柄,轮次异步执行
//	GET    /api/coach/chat/{id}/events  SSE 六事件(回放+实时跟随至 done)
//	POST   /api/coach/chat/{id}/confirm 提议决断(Agent 唯一写入口)
//	GET    /api/coach/memory            双类记忆(叙事 n*/约束 c*)
//	PUT    /api/coach/memory/{id}       改记忆文本
//	DELETE /api/coach/memory/{id}       删记忆(D17:约束即刻失效)
//	GET    /api/coach/usage             三池日/月聚合
//
// 轮次执行模型:POST 创建 turnStream(hub)→ goroutine 跑 Loop → 事件进流;
// GET SSE 回放 buf + 订阅跟随。单实例内存态,重启即丢(会话史持久于 pg,
// 中断会话可续聊;流本身一次性)。D15:工具失败回填错误,回合仍产出回复。
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/changzhi777/openGym/backend-go/internal/auth"
	"github.com/changzhi777/openGym/backend-go/internal/httpx"
	"github.com/changzhi777/openGym/backend-go/internal/llm"
)

// Service 装配七端点。Provider 为 nil = Coach 未连接(503 off,与既有 CoachOff 同语义)。
type Service struct {
	DB       *pgxpool.Pool
	Sessions *auth.Sessions
	Provider llm.Provider
	Tools    Registry
	System   SystemPrompt // nil → 默认装配
	EnvWords string       // MUHE_REVIEW_WORDS(env 兜底词表,与配置词表并集)

	hub *turnHub // InitHub 装配(main.go 构造后调一次;非并发安全,仅启动期)
}

// InitHub 装配事件流注册表(main.go 启动期一次性调用)。
func (s *Service) InitHub() { s.hub = newTurnHub() }

func (s *Service) hubOf() *turnHub {
	if s.hub == nil {
		s.InitHub()
	}
	return s.hub
}

// SystemPrompt 装配系统提示(角色人设+约束硬注入+叙事摘要)。返回 (system, ok):
// false = 不执行(记忆装配失败时宁可拒绝,不裸跑)。
type SystemPrompt func(ctx context.Context, userID string) (string, bool)

// DefaultSystem 默认装配:人设 + 结构化约束硬注入 + 叙事记忆前 N 条。
func DefaultSystem(db *pgxpool.Pool) SystemPrompt {
	return func(ctx context.Context, userID string) (string, bool) {
		var b strings.Builder
		b.WriteString("你是沐禾健身的 AI 教练,语气专业而克制,建议可执行,健康红线(疼痛/眩晕/心悸)一律建议就医。\n")
		cons, err := ActiveConstraints(ctx, db, userID)
		if err != nil {
			return "", false
		}
		if len(cons) > 0 {
			b.WriteString("\n【硬约束(必须遵守,任何计划与建议不得违反)】\n")
			for _, c := range cons {
				fmt.Fprintf(&b, "- %s\n", c)
			}
		}
		return b.String(), true
	}
}

// ── turn hub:进程内事件流(回放 + 实时) ─────────────────────────────────────

type turnStream struct {
	mu   sync.Mutex
	buf  []Event        // 已发射事件(SSE 重连回放)
	done bool           // 回合结束
	subs []chan Event   // 实时订阅者
}

// publish 追加事件并投递订阅者。绝不持锁阻塞:慢/死订阅者直接剔除并关闭,
// 断线客户端靠 buf 回放补齐——一个掉队的读端不能挂死整个回合(2026-11-18
// 审查轮:此前 ch<-e 在锁内阻塞,断开后缓冲填满即 runTurn 永久卡死、会话
// 409 busy 到进程重启,外加 goroutine 泄漏)。
func (ts *turnStream) publish(e Event) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.done {
		return
	}
	ts.buf = append(ts.buf, e)
	for i := 0; i < len(ts.subs); {
		select {
		case ts.subs[i] <- e:
			i++
		default:
			close(ts.subs[i])
			ts.subs = append(ts.subs[:i], ts.subs[i+1:]...)
		}
	}
	if e.Kind == EventDone {
		ts.done = true
		for _, ch := range ts.subs {
			close(ch)
		}
		ts.subs = nil
	}
}

func (ts *turnStream) subscribe() (<-chan Event, []Event) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	replay := make([]Event, len(ts.buf))
	copy(replay, ts.buf)
	if ts.done {
		return nil, replay
	}
	ch := make(chan Event, len(ts.buf)+8)
	ts.subs = append(ts.subs, ch)
	return ch, replay
}

// unsubscribe 摘除订阅(write 失败/客户端断开时的退出路径,不残留无人排空的
// channel)。已不在 subs 中(publish 剔除或 done 关闭)时是安全空操作。
func (ts *turnStream) unsubscribe(ch <-chan Event) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for i, c := range ts.subs {
		if c == ch {
			ts.subs = append(ts.subs[:i], ts.subs[i+1:]...)
			return
		}
	}
}

type turnHub struct {
	mu      sync.Mutex
	streams map[string]*turnStream
	busy    map[string]bool // 同会话单飞(并发 POST → 409 busy)
}

func newTurnHub() *turnHub {
	return &turnHub{streams: map[string]*turnStream{}, busy: map[string]bool{}}
}

func (h *turnHub) start(sessionID string) (*turnStream, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.busy[sessionID] {
		return nil, errBusy
	}
	ts := &turnStream{}
	h.streams[sessionID] = ts
	h.busy[sessionID] = true
	return ts, nil
}

// get 取活跃流(回合结束后的流保留可回放;进程重启即无 → 404)。
func (h *turnHub) get(sessionID string) *turnStream {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.streams[sessionID]
}

func (h *turnHub) finish(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.busy, sessionID)
}

var errBusy = errors.New("agent: turn already running")

// ── 端点 ─────────────────────────────────────────────────────────────────────

type chatBody struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Lang    string `json:"lang"`
}

// Chat POST /api/coach/chat
func (s *Service) Chat(c *gin.Context) {
	u, ok := auth.UserOf(c, s.DB, s.Sessions)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not signed in"})
		return
	}
	if s.Provider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "the Coach is not set up on this instance", "code": "off"})
		return
	}
	var body chatBody
	if err := json.Unmarshal(httpx.RawBody(c), &body); err != nil || strings.TrimSpace(body.Message) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message required"})
		return
	}
	msg := strings.TrimSpace(body.Message)
	if len([]rune(msg)) > 4000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message too long"})
		return
	}

	// 硬限额(D1):per-user 日回合/月 token(admin 可配,instance_settings)。
	limits, err := GetLimits(c.Request.Context(), s.DB)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "limits unreadable"})
		return
	}
	usage, err := usageAggregate(c.Request.Context(), s.DB, u.ID, limits)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "usage unreadable"})
		return
	}
	if usage.Day.Messages >= limits.MessagesPerDay {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "今日对话额度已用完,明天再来", "code": "cap"})
		return
	}
	if usage.Month.Chat+usage.Month.Review >= limits.TokensPerMonth {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "本月额度已用完", "code": "cap"})
		return
	}

	// 会话:有 id 续聊(校验归属),无 id 开新。
	sess := (*Session)(nil)
	if body.ID != "" {
		sess, err = GetSession(c.Request.Context(), s.DB, body.ID, u.ID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "no such chat"})
				return
			}
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "session unreadable"})
			return
		}
	} else {
		sess, err = CreateSession(c.Request.Context(), s.DB, u.ID)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "session create failed"})
			return
		}
	}

	ts, err := s.hubOf().start(sess.ID)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "the Coach is already thinking about your training", "code": "busy"})
		return
	}

	// 异步执行:契约承诺 POST 立即回句柄。
	go s.runTurn(sess, msg, ts)

	state := sess.State
	if state == StateWaitingConfirm {
		// 续聊会回 active 的场景不做虚假等待;新提议会再置 waiting。
		state = StateActive
	}
	c.JSON(http.StatusOK, gin.H{"id": sess.ID, "state": state})
}

// runTurn 跑一轮 loop 并落库(历史/提议/usage/状态)。
func (s *Service) runTurn(sess *Session, userMsg string, ts *turnStream) {
	defer s.hub.finish(sess.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	system, ok := s.systemPrompt(ctx, sess.UserID)
	if !ok {
		ts.publish(Event{Kind: EventBlocked, Text: "memory"})
		ts.publish(Event{Kind: EventDone})
		return
	}

	// 热配置(批次 6):每回合读治理配置——MAX_TURNS 与词表改完即生效,无重启。
	cfg, err := GetAgentConfig(ctx, s.DB)
	if err != nil {
		cfg = DefaultAgentConfig
	}
	loop := &Loop{
		Provider: s.Provider,
		Tools:    s.Tools,
		Config:   LoopConfig{MaxTurns: cfg.MaxTurns, ProposalPrefix: sess.ID + ":p"},
		Review:   s.reviewerFor(ctx),
	}
	res, err := loop.Run(WithUID(ctx, sess.UserID), system, sess.History, userMsg, ts.publish)
	if err != nil {
		ts.publish(Event{Kind: EventBlocked, Text: "upstream"})
		ts.publish(Event{Kind: EventDone})
		return
	}

	// 落库:史 = 旧史 + user + assistant(中间工具轮不入史,重建成本低,v1 口径)。
	sess.History = append(sess.History,
		llm.Message{Role: "user", Text: userMsg},
		llm.Message{Role: "assistant", Text: res.Text})
	sess.UsageIn += int64(res.Usage.InputTokens)
	sess.UsageOut += int64(res.Usage.OutputTokens)
	for _, p := range res.Proposals {
		if err := AddProposal(ctx, s.DB, sess.ID, p); err != nil {
			// 落库失败必须可见:回复与卡片已流出,静默吞错 = 用户确认时 404 无从排查。
			log.Printf("agent: add proposal %s/%s: %v", sess.ID, p.ID, err)
			continue
		}
		sess.State = StateWaitingConfirm
	}
	if err := saveTurn(ctx, s.DB, sess); err != nil {
		log.Printf("agent: save turn %s: %v", sess.ID, err)
	}
	if err := RecordUsage(ctx, s.DB, sess.UserID, "chat", int64(res.Usage.InputTokens), int64(res.Usage.OutputTokens)); err != nil {
		log.Printf("agent: record usage %s: %v", sess.UserID, err)
	}
}

// Events GET /api/coach/chat/{id}/events — SSE(回放 + 跟随至 done)。
func (s *Service) Events(c *gin.Context) {
	u, ok := auth.UserOf(c, s.DB, s.Sessions)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not signed in"})
		return
	}
	// 归属校验(流只给会话主人)。
	if _, err := GetSession(c.Request.Context(), s.DB, c.Param("id"), u.ID); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no such chat"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "session unreadable"})
		return
	}
	ts := s.hubOf().get(c.Param("id"))
	if ts == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such chat"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no") // nginx 反代禁缓冲,否则 SSE 事件攒成一坨
	fl, _ := c.Writer.(http.Flusher)
	write := func(e Event) bool {
		payload := map[string]any{"kind": e.Kind}
		if e.Kind == EventBlocked {
			payload["reason"] = e.Text // 契约:blocked 事件带 reason(sensitive|llm|upstream|memory)
		} else if e.Text != "" {
			payload["text"] = e.Text
		}
		if e.ToolName != "" {
			payload["tool"] = e.ToolName
		}
		if len(e.Refs) > 0 {
			payload["refs"] = e.Refs
		}
		if e.ProposalID != "" {
			payload["proposalId"] = e.ProposalID
		}
		if len(e.Input) > 0 {
			payload["input"] = e.Input
		}
		b, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", e.Kind, b); err != nil {
			return false
		}
		if fl != nil {
			fl.Flush()
		}
		return true
	}

	live, replay := ts.subscribe()
	if live != nil {
		defer ts.unsubscribe(live) // 断线即摘除,残留订阅会挂死 publish
	}
	for _, e := range replay {
		if !write(e) {
			return
		}
		if e.Kind == EventDone {
			return
		}
	}
	if live == nil {
		return // done 已在回放中
	}
	for e := range live {
		if !write(e) {
			return
		}
	}
}

// Confirm POST /api/coach/chat/{id}/confirm — 提议决断。
func (s *Service) Confirm(c *gin.Context) {
	u, ok := auth.UserOf(c, s.DB, s.Sessions)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not signed in"})
		return
	}
	var body struct {
		ProposalID string `json:"proposalId"`
		Decision   string `json:"decision"`
	}
	if err := json.Unmarshal(httpx.RawBody(c), &body); err != nil || body.ProposalID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "proposalId required"})
		return
	}
	applied, err := DecideProposal(c.Request.Context(), s.DB, c.Param("id"), u.ID, body.ProposalID, body.Decision)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no such proposal"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "confirm failed"})
		return
	}
	sess, err := GetSession(c.Request.Context(), s.DB, c.Param("id"), u.ID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "session unreadable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": sess.ID, "state": sess.State, "applied": applied})
}

// MemoryList GET /api/coach/memory
func (s *Service) MemoryList(c *gin.Context) {
	u, ok := auth.UserOf(c, s.DB, s.Sessions)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not signed in"})
		return
	}
	narrative, constraints, err := ListMemory(c.Request.Context(), s.DB, u.ID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "memory unreadable"})
		return
	}
	if narrative == nil {
		narrative = []MemoryEntry{}
	}
	if constraints == nil {
		constraints = []MemoryEntry{}
	}
	c.JSON(http.StatusOK, gin.H{"narrative": narrative, "constraints": constraints})
}

// MemoryDelete DELETE /api/coach/memory/{id}
func (s *Service) MemoryDelete(c *gin.Context) {
	u, ok := auth.UserOf(c, s.DB, s.Sessions)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not signed in"})
		return
	}
	if err := DeactivateMemory(c.Request.Context(), s.DB, u.ID, c.Param("id")); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no such entry"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "delete failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// MemoryPut PUT /api/coach/memory/{id}
func (s *Service) MemoryPut(c *gin.Context) {
	u, ok := auth.UserOf(c, s.DB, s.Sessions)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not signed in"})
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(httpx.RawBody(c), &body); err != nil || strings.TrimSpace(body.Text) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text required"})
		return
	}
	if len([]rune(body.Text)) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text too long"})
		return
	}
	if err := UpdateMemoryText(c.Request.Context(), s.DB, u.ID, c.Param("id"), strings.TrimSpace(body.Text)); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no such entry"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "update failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Usage GET /api/coach/usage
func (s *Service) Usage(c *gin.Context) {
	u, ok := auth.UserOf(c, s.DB, s.Sessions)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not signed in"})
		return
	}
	sum, err := UsageSummaryFor(c.Request.Context(), s.DB, u.ID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "usage unreadable"})
		return
	}
	c.JSON(http.StatusOK, sum)
}

// hub 挂 Service(delegate 保持 Service 结构面窄)。
func (s *Service) initHub() { if s.hub == nil { s.hub = newTurnHub() } }

// systemPrompt 委派(默认装配兜底)。
func (s *Service) systemPrompt(ctx context.Context, userID string) (string, bool) {
	if s.System != nil {
		return s.System(ctx, userID)
	}
	return DefaultSystem(s.DB)(ctx, userID)
}


// ── admin:计量看板与限额(批次 5;角色/step-up 由 main.go 装配挂) ─────────────

// AdminUsageOverview GET /api/admin/coach/usage-overview — 全站三池聚合 + 14 天趋势。
func (s *Service) AdminUsageOverview(c *gin.Context) {
	out, err := UsageOverview(c.Request.Context(), s.DB)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "usage unreadable"})
		return
	}
	c.JSON(http.StatusOK, out)
}

// AdminLimitsPut PUT /api/admin/coach/limits — 设置 per-user 硬限额(部分更新)。
func (s *Service) AdminLimitsPut(c *gin.Context) {
	var body struct {
		MessagesPerDay *int   `json:"messagesPerDay"`
		TokensPerMonth *int64 `json:"tokensPerMonth"`
	}
	if err := json.Unmarshal(httpx.RawBody(c), &body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	cur, err := GetLimits(c.Request.Context(), s.DB)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "limits unreadable"})
		return
	}
	if body.MessagesPerDay != nil {
		if *body.MessagesPerDay < 1 || *body.MessagesPerDay > 10000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "messagesPerDay out of range"})
			return
		}
		cur.MessagesPerDay = *body.MessagesPerDay
	}
	if body.TokensPerMonth != nil {
		if *body.TokensPerMonth < 1000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "tokensPerMonth out of range"})
			return
		}
		cur.TokensPerMonth = *body.TokensPerMonth
	}
	if err := SetLimits(c.Request.Context(), s.DB, cur); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "save failed"})
		return
	}
	now := time.Now().UTC()
	cur.ResetAt = time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	c.JSON(http.StatusOK, cur)
}


// ── admin:治理台配置(批次 6;ops+step-up 由 main.go 装配挂) ─────────────────

// AdminAgentConfigGet GET /api/admin/coach/agent-config
func (s *Service) AdminAgentConfigGet(c *gin.Context) {
	cfg, err := GetAgentConfig(c.Request.Context(), s.DB)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "config unreadable"})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// AdminAgentConfigPut PUT /api/admin/coach/agent-config — 部分更新,下一回合热生效。
func (s *Service) AdminAgentConfigPut(c *gin.Context) {
	var body struct {
		MaxTurns    *int     `json:"maxTurns"`
		ReviewWords *[]string `json:"reviewWords"`
	}
	if err := json.Unmarshal(httpx.RawBody(c), &body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	cur, err := GetAgentConfig(c.Request.Context(), s.DB)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "config unreadable"})
		return
	}
	if body.MaxTurns != nil {
		if *body.MaxTurns < 1 || *body.MaxTurns > 8 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "maxTurns out of range"})
			return
		}
		cur.MaxTurns = *body.MaxTurns
	}
	if body.ReviewWords != nil {
		clean := make([]string, 0, len(*body.ReviewWords))
		for _, w := range *body.ReviewWords {
			if t := strings.TrimSpace(w); t != "" {
				clean = append(clean, t)
			}
		}
		cur.ReviewWords = clean
	}
	if err := SetAgentConfig(c.Request.Context(), s.DB, cur); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "save failed"})
		return
	}
	c.JSON(http.StatusOK, cur)
}
