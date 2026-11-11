// session.go — 对话会话状态机与 pg 存取(方案 v2 五/六,0013_agent_chat_memory)。
//
// 状态机:active ⇄ waiting_confirm → closed。
//   POST /chat:        active 会话追加轮次(状态不变);
//   轮内产生提议:       → waiting_confirm;
//   POST /confirm:     全部待决提议有决定 → 回 active。
// 红线落点:Proposals 表是 Agent 唯一写入口;DecideProposal 的事务里同时落
// 结构化约束/叙事记忆(accept 时)——写入只经由用户确认发生。
package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/changzhi777/openGym/backend-go/internal/llm"
)

// 会话状态(契约 CoachChatSession.state 枚举)。
const (
	StateActive         = "active"
	StateWaitingConfirm = "waiting_confirm"
	StateClosed         = "closed"
)

// ErrNotFound 统一 404 语义(handler 转 404)。
var ErrNotFound = errors.New("agent: not found")

// Session 是一行 coach_chat_sessions 的投影。
type Session struct {
	ID     string
	UserID string
	State  string
	History []llm.Message
	UsageIn  int64
	UsageOut int64
}

// randID 16 hex(与 coach job id 同形)。
func randID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 失败是系统性故障,向上抛比返回可猜测 id 安全。
		panic(fmt.Errorf("agent: rand: %w", err))
	}
	return hex.EncodeToString(b)
}

// historyCodec:llm.Message ↔ jsonb。数组形状与契约描述一致(零输入容忍)。
func marshalHistory(msgs []llm.Message) ([]byte, error) {
	return json.Marshal(msgs)
}

func unmarshalHistory(raw []byte) []llm.Message {
	var msgs []llm.Message
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return nil // 坏行当空史:会话仍可续聊,旧轮丢但不炸(审计留 in/out 计数)
	}
	return msgs
}

// CreateSession 开新会话(active)。
func CreateSession(ctx context.Context, db *pgxpool.Pool, userID string) (*Session, error) {
	id := randID()
	_, err := db.Exec(ctx, `INSERT INTO coach_chat_sessions (id, user_id) VALUES ($1,$2)`, id, userID)
	if err != nil {
		return nil, fmt.Errorf("agent: create session: %w", err)
	}
	return &Session{ID: id, UserID: userID, State: StateActive}, nil
}

// GetSession 取会话;ErrNotFound 转 404。
func GetSession(ctx context.Context, db *pgxpool.Pool, id, userID string) (*Session, error) {
	row := db.QueryRow(ctx, `SELECT id, user_id, state, history, usage_in, usage_out
		FROM coach_chat_sessions WHERE id=$1 AND user_id=$2`, id, userID)
	var s Session
	var raw []byte
	if err := row.Scan(&s.ID, &s.UserID, &s.State, &raw, &s.UsageIn, &s.UsageOut); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("agent: get session: %w", err)
	}
	s.History = unmarshalHistory(raw)
	return &s, nil
}

// saveTurn 会话级事务:追加双方消息 + 累计 usage + 推进状态。
// waitingConfirm 只升不降(提议产生即 waiting;confirm 回 active 由 DecideProposal 负责)。
func saveTurn(ctx context.Context, db *pgxpool.Pool, s *Session) error {
	raw, err := marshalHistory(s.History)
	if err != nil {
		return fmt.Errorf("agent: marshal history: %w", err)
	}
	_, err = db.Exec(ctx, `UPDATE coach_chat_sessions
		SET history=$2, usage_in=$3, usage_out=$4,
		    state=CASE WHEN state='closed' THEN state
		              WHEN $5 THEN 'waiting_confirm' ELSE state END,
		    updated_at=now()
		WHERE id=$1 AND user_id=$6`,
		s.ID, raw, s.UsageIn, s.UsageOut, s.State == StateWaitingConfirm, s.UserID)
	if err != nil {
		return fmt.Errorf("agent: save turn: %w", err)
	}
	return nil
}

// AddProposal 落一条待决提议(提议队列写入口;Agent 侧只经此函数写)。
func AddProposal(ctx context.Context, db *pgxpool.Pool, sessionID string, p Proposal) error {
	input, err := json.Marshal(p.Input)
	if err != nil {
		return fmt.Errorf("agent: marshal proposal: %w", err)
	}
	_, err = db.Exec(ctx, `INSERT INTO coach_proposals (id, session_id, tool, input) VALUES ($1,$2,$3,$4)`,
		p.ID, sessionID, p.Tool, input)
	if err != nil {
		return fmt.Errorf("agent: add proposal: %w", err)
	}
	return nil
}

// DecideProposal 决断一条提议(事务):落决定 + accept 时按工具落地数据 +
// 会话回 active。tool=propose_constraint → 结构化约束硬表;propose_memory →
// 叙事记忆(批次 3 起主写 WeKnora,本表为降级镜像);其他工具名原样记录不落地。
func DecideProposal(ctx context.Context, db *pgxpool.Pool, sessionID, userID, proposalID, decision string) (string, error) {
	if decision != "accept" && decision != "reject" {
		return "", fmt.Errorf("agent: bad decision %q", decision)
	}
	var applied string
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		// 行锁 + 归属校验(经会话归属确认提案归属,杜绝跨用户确认)。
		var tool string
		var inputRaw []byte
		var pending bool
		err := tx.QueryRow(ctx, `UPDATE coach_proposals p SET decision=$1, decided_at=now()
			FROM coach_chat_sessions s
			WHERE p.id=$2 AND p.session_id=$3 AND p.decision IS NULL
			  AND s.id=$3 AND s.user_id=$4
			RETURNING p.tool, p.input, (s.state='waiting_confirm')`,
			decision, proposalID, sessionID, userID).Scan(&tool, &inputRaw, &pending)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("agent: decide: %w", err)
		}
		if decision == "accept" {
			switch tool {
			case "propose_constraint":
				var text string
				if err := json.Unmarshal(inputRaw, &struct {
					Text *string `json:"text"`
				}{&text}); err == nil && text != "" {
					_, err = tx.Exec(ctx, `INSERT INTO coach_constraints (user_id, text) VALUES ($1,$2)`, userID, text)
					if err != nil {
						return fmt.Errorf("agent: add constraint: %w", err)
					}
					applied = "constraint"
				}
			case "propose_memory":
				var text string
				if err := json.Unmarshal(inputRaw, &struct {
					Text *string `json:"text"`
				}{&text}); err == nil && text != "" {
					_, err = tx.Exec(ctx, `INSERT INTO coach_memory_narrative (user_id, text) VALUES ($1,$2)`, userID, text)
					if err != nil {
						return fmt.Errorf("agent: add memory: %w", err)
					}
					applied = "memory"
				}
			default:
				applied = "recorded" // 如 propose_change:先记录,change-list 应用在批次 4 前端卡片链路
			}
		}
		// 会话回 active(仍有其他待决提议则保持 waiting——本表决定后查余量)。
		var waiting int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM coach_proposals
			WHERE session_id=$1 AND decision IS NULL`, sessionID).Scan(&waiting); err != nil {
			return err
		}
		if waiting == 0 {
			_, err = tx.Exec(ctx, `UPDATE coach_chat_sessions SET state=$1, updated_at=now()
				WHERE id=$2 AND state='waiting_confirm'`, StateActive, sessionID)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return applied, err
}

// MemoryEntry 对应契约 CoachMemoryEntry(id 是 string:n{serial}=叙事 / c{serial}=约束,
// 两表 serial 空间独立,前缀即 kind 且互不冲突——契约 id:string 的落法)。
type MemoryEntry struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Text string `json:"text"`
	At   string `json:"at"`
}

// ListMemory 合并两类记忆(叙事在前,约束在后;均按新→旧)。
func ListMemory(ctx context.Context, db *pgxpool.Pool, userID string) ([]MemoryEntry, []MemoryEntry, error) {
	var narrative, constraints []MemoryEntry
	rows, err := db.Query(ctx, `SELECT id, text, created_at FROM coach_memory_narrative
		WHERE user_id=$1 AND active ORDER BY id DESC`, userID)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var e MemoryEntry
		var at time.Time
		var serial int64
		if err := rows.Scan(&serial, &e.Text, &at); err != nil {
			return nil, nil, err
		}
		e.ID, e.Kind, e.At = fmt.Sprintf("n%d", serial), "narrative", at.UTC().Format(time.RFC3339)
		narrative = append(narrative, e)
	}
	rows.Close()
	rows, err = db.Query(ctx, `SELECT id, text, created_at FROM coach_constraints
		WHERE user_id=$1 AND active ORDER BY id DESC`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e MemoryEntry
		var at time.Time
		var serial int64
		if err := rows.Scan(&serial, &e.Text, &at); err != nil {
			return nil, nil, err
		}
		e.ID, e.Kind, e.At = fmt.Sprintf("c%d", serial), "constraint", at.UTC().Format(time.RFC3339)
		constraints = append(constraints, e)
	}
	return narrative, constraints, nil
}

// parseMemoryID 拆 n/c 前缀 → (kind, serial)。
func parseMemoryID(id string) (string, int64, bool) {
	if len(id) < 2 {
		return "", 0, false
	}
	var n int64
	if _, err := fmt.Sscanf(id[1:], "%d", &n); err != nil {
		return "", 0, false
	}
	switch id[0] {
	case 'n':
		return "narrative", n, true
	case 'c':
		return "constraint", n, true
	}
	return "", 0, false
}

// DeactivateMemory 删除一条记忆(契约 DELETE /api/coach/memory/{id})。
// D17 一致性:结构化约束即时失效(active=false 即刻停止硬读取);批次 3 的
// WeKnora 镜像删除在同函数扩展。
func DeactivateMemory(ctx context.Context, db *pgxpool.Pool, userID, id string) error {
	kind, serial, ok := parseMemoryID(id)
	if !ok {
		return ErrNotFound
	}
	table := "coach_memory_narrative"
	if kind == "constraint" {
		table = "coach_constraints"
	}
	ct, err := db.Exec(ctx, `UPDATE `+table+` SET active=false
		WHERE id=$1 AND user_id=$2 AND active`, serial, userID)
	if err != nil {
		return fmt.Errorf("agent: delete memory: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateMemoryText 编辑一条记忆文本(契约 PUT /api/coach/memory/{id};用户的
// 措辞优先——kind 与创建来源不变,结构化约束改后即刻生效)。
func UpdateMemoryText(ctx context.Context, db *pgxpool.Pool, userID, id, text string) error {
	kind, serial, ok := parseMemoryID(id)
	if !ok {
		return ErrNotFound
	}
	table := "coach_memory_narrative"
	if kind == "constraint" {
		table = "coach_constraints"
	}
	ct, err := db.Exec(ctx, `UPDATE `+table+` SET text=$3
		WHERE id=$1 AND user_id=$2 AND active`, serial, userID, text)
	if err != nil {
		return fmt.Errorf("agent: update memory: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ActiveConstraints 供计划工具硬读取(检索不到也生效)。
func ActiveConstraints(ctx context.Context, db *pgxpool.Pool, userID string) ([]string, error) {
	rows, err := db.Query(ctx, `SELECT text FROM coach_constraints
		WHERE user_id=$1 AND active ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// UsageEvent 三池记账行(ledger 原始事件,看板按日/月聚合)。
func RecordUsage(ctx context.Context, db *pgxpool.Pool, userID, pool string, in, out int64) error {
	_, err := db.Exec(ctx, `INSERT INTO coach_usage_events (user_id, pool, tokens_in, tokens_out)
		VALUES ($1,$2,$3,$4)`, userID, pool, in, out)
	return err
}

// UsageStat 对应契约 schema:messages 回合数 + 三池 token(入库+出库合计)。
type UsageStat struct {
	Messages  int   `json:"messages"`
	Chat      int64 `json:"chat"`
	Review    int64 `json:"review"`
	Embedding int64 `json:"embedding"`
}

// UsageSummary 契约 GET /api/coach/usage 的账本聚合。
type UsageSummary struct {
	Day    UsageStat `json:"day"`
	Month  UsageStat `json:"month"`
	Limits Limits    `json:"limits"`
}

// Limits 硬限额(D1;admin 可配在批次 5,现为常量)。
type Limits struct {
	MessagesPerDay int    `json:"messagesPerDay"`
	TokensPerMonth int64  `json:"tokensPerMonth"`
	ResetAt        string `json:"resetAt"`
}

// DefaultLimits 方案 v2 D1(月度口径 = chat+review 合计,含审核 ×2 预算)。
var DefaultLimits = Limits{MessagesPerDay: 50, TokensPerMonth: 3_000_000}

// GetUsage 聚合当日/当月三池(chat 计回合数;token = in+out),回响生效限额。
func GetUsage(ctx context.Context, db *pgxpool.Pool, userID string) (*UsageSummary, error) {
	limits, err := GetLimits(ctx, db)
	if err != nil {
		return nil, err
	}
	return usageAggregate(ctx, db, userID, limits)
}

// usageAggregate 聚合本体(limits 由调用方注入:GET usage 与限额检查共用一次读取)。
func usageAggregate(ctx context.Context, db *pgxpool.Pool, userID string, limits Limits) (*UsageSummary, error) {
	sum := &UsageSummary{Limits: limits}
	now := time.Now().UTC()
	dayStart := now.Truncate(24 * time.Hour)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	sum.Limits.ResetAt = time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)

	for _, spec := range []struct {
		pool  string
		dst   *UsageStat
		since time.Time
	}{
		{"chat", &sum.Day, dayStart}, {"chat", &sum.Month, monthStart},
		{"review", &sum.Day, dayStart}, {"review", &sum.Month, monthStart},
		{"embedding", &sum.Day, dayStart}, {"embedding", &sum.Month, monthStart},
	} {
		var msgs int
		var in, out int64
		err := db.QueryRow(ctx, `SELECT count(*), coalesce(sum(tokens_in),0), coalesce(sum(tokens_out),0)
			FROM coach_usage_events WHERE user_id=$1 AND pool=$2 AND created_at >= $3`,
			userID, spec.pool, spec.since).Scan(&msgs, &in, &out)
		if err != nil {
			return nil, fmt.Errorf("agent: usage: %w", err)
		}
		spec.dst.Messages += msgs
		switch spec.pool {
		case "chat":
			spec.dst.Chat += in + out
		case "review":
			spec.dst.Review += in + out
		case "embedding":
			spec.dst.Embedding += in + out
		}
	}
	return sum, nil
}


// ── ctx 上的用户身份(工具层读取;数据工具需要 uid 才能查该用户的状态) ────────

type ctxKey int

const uidKey ctxKey = 1

// WithUID 把会话用户挂进工具链上下文。
func WithUID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, uidKey, userID)
}

// UIDFrom 取会话用户;无 = 空串(数据工具据此返回"需要会话"错误)。
func UIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(uidKey).(string)
	return v
}
