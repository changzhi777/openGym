// handlers.go — 团队域端点(§13.7 核心链 10 操作;summary/board 属批 3,plans 属批 3.5)。
// 鉴权统一 auth.UserOf;owner 闸统一 ownerTeam;展示名恒昵称(裁定 #8——uid 仅作
// 管理句柄,displayName 空时回退掩码)。body 一律读 BodyGuard 预读的 rawBody(仓内惯例)。
package team

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/changzhi777/openGym/backend-go/internal/auth"
	"github.com/changzhi777/openGym/backend-go/internal/qr"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Handlers struct {
	DB       *pgxpool.Pool
	Sessions *auth.Sessions
	Invites  *InviteStore
	Origin   string
	Rdb      *redis.Client // 看板 60s 缓存;nil = 直解析(开发态)
}

const nameMax = 80

func clip(s string, n int) string {
	rs := []rune(s)
	if len(rs) > n {
		return string(rs[:n])
	}
	return s
}

func bindBody(c *gin.Context, v any) {
	if raw, ok := c.Get("rawBody"); ok {
		if b, ok2 := raw.([]byte); ok2 && len(b) > 0 {
			json.Unmarshal(b, v)
		}
	}
}

// me 会话 uid;未登录 401(已写响应)。
func (h *Handlers) me(c *gin.Context) (string, bool) {
	u, ok := auth.UserOf(c, h.DB, h.Sessions)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not signed in"})
		return "", false
	}
	return u.ID, true
}

// ownerTeam 取调用者的活团(须为 owner);member-only → 403,无团 → 404(已写响应)。
func (h *Handlers) ownerTeam(c *gin.Context) (TeamRow, string, bool) {
	uid, ok := h.me(c)
	if !ok {
		return TeamRow{}, "", false
	}
	t, err := TeamOfOwner(c.Request.Context(), h.DB, uid)
	if err == nil {
		return t, uid, true
	}
	if _, e2 := TeamOfMember(c.Request.Context(), h.DB, uid); e2 == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "owner only", "code": "team-owner-only"})
		return TeamRow{}, uid, false
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "no live team", "code": "team-none"})
	return TeamRow{}, uid, false
}

// Create POST /api/team — 建团(W1 单角色;W7 一教练一活团)。
func (h *Handlers) Create(c *gin.Context) {
	uid, ok := h.me(c)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	bindBody(c, &body)
	name := clip(strings.TrimSpace(body.Name), nameMax)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	id, err := CreateTeam(c.Request.Context(), h.DB, uid, name)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // teams_one_live_per_owner (W7)
			c.JSON(http.StatusConflict, gin.H{"error": "already leading a team", "code": "team-live-exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"team": gin.H{"id": id, "name": name}, "role": "owner"})
}

// Get GET /api/team — 我的团(owner 或 member 视角)。
func (h *Handlers) Get(c *gin.Context) {
	uid, ok := h.me(c)
	if !ok {
		return
	}
	if t, err := TeamOfOwner(c.Request.Context(), h.DB, uid); err == nil {
		c.JSON(http.StatusOK, gin.H{"team": gin.H{"id": t.ID, "name": t.Name}, "role": "owner"})
		return
	}
	if t, err := TeamOfMember(c.Request.Context(), h.DB, uid); err == nil {
		c.JSON(http.StatusOK, gin.H{"team": gin.H{"id": t.ID, "name": t.Name}, "role": "member"})
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "no live team", "code": "team-none"})
}

// Rename PUT /api/team — 团名修改(owner)。
func (h *Handlers) Rename(c *gin.Context) {
	t, _, ok := h.ownerTeam(c)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	bindBody(c, &body)
	name := clip(strings.TrimSpace(body.Name), nameMax)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	if err := RenameTeam(c.Request.Context(), h.DB, t.ID, name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Invite POST /api/team/invite — 出码(一团一活码,旧的作废)。
func (h *Handlers) Invite(c *gin.Context) {
	t, _, ok := h.ownerTeam(c)
	if !ok {
		return
	}
	code := h.Invites.NewCode(t.ID)
	c.JSON(http.StatusOK, gin.H{"code": code, "expires": time.Now().Add(inviteTTL).UnixMilli()})
}

// InviteDelete DELETE /api/team/invite — 烧码。
func (h *Handlers) InviteDelete(c *gin.Context) {
	t, _, ok := h.ownerTeam(c)
	if !ok {
		return
	}
	h.Invites.RevokeTeam(t.ID)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// InviteQR GET /api/team/invite/qr — 团码二维码 PNG(每次渲染重出一枚活码,
// device-link 同语义)。muhe-fit 正式版 APP 的 MuheTeamApi 声明此端点(ImageView 直显),
// 测试版须契约对齐;PWA 侧用前端 QrCanvas 不走它。
func (h *Handlers) InviteQR(c *gin.Context) {
	t, _, ok := h.ownerTeam(c)
	if !ok {
		return
	}
	code := h.Invites.NewCode(t.ID)
	size := 512
	if v, err := strconv.Atoi(c.Query("size")); err == nil && v >= 64 && v <= 1024 {
		size = v
	}
	logo := c.Query("logo") != "0"
	png, err := qr.Render(h.Origin+"/?team="+url.QueryEscape(code), size, logo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "render"})
		return
	}
	c.Header("Cache-Control", "no-store") // 码即一次性凭据
	c.Data(http.StatusOK, "image/png", png)
}

// Join POST /api/team/join — 凭码入团(哈希查→TTL→烧码→插入/复活)。
func (h *Handlers) Join(c *gin.Context) {
	uid, ok := h.me(c)
	if !ok {
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	bindBody(c, &body)
	teamID := h.Invites.Take(NormalizeCode(body.Code))
	if teamID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired code", "code": "team-code-invalid"})
		return
	}
	if IsLiveMember(c.Request.Context(), h.DB, teamID, uid) {
		c.JSON(http.StatusConflict, gin.H{"error": "already a member", "code": "team-joined"})
		return
	}
	if err := AddMember(c.Request.Context(), h.DB, teamID, uid); err != nil {
		if errors.Is(err, ErrTeamFull) {
			c.JSON(http.StatusConflict, gin.H{"error": "team is full", "code": "team-full"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save"})
		return
	}
	name, ownerDN, err := TeamWithOwner(c.Request.Context(), h.DB, teamID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"team":  gin.H{"id": teamID, "name": name},
		"owner": gin.H{"displayName": displayNameOf(ownerDN, "")},
	})
}

// Members GET /api/team/members — 花名册(scope/joinedAt 仅 owner 视角,W3)。
func (h *Handlers) Members(c *gin.Context) {
	uid, ok := h.me(c)
	if !ok {
		return
	}
	var t TeamRow
	role := ""
	if tt, err := TeamOfOwner(c.Request.Context(), h.DB, uid); err == nil {
		t, role = tt, "owner"
	} else if tt, err := TeamOfMember(c.Request.Context(), h.DB, uid); err == nil {
		t, role = tt, "member"
	} else {
		c.JSON(http.StatusNotFound, gin.H{"error": "no live team", "code": "team-none"})
		return
	}
	rows, err := Members(c.Request.Context(), h.DB, t.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load"})
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, m := range rows {
		e := gin.H{"uid": m.UID, "role": m.Role, "displayName": displayNameOf(m.DisplayName, m.UID)}
		if role == "owner" {
			e["scope"] = m.Scope
			e["joinedAt"] = m.JoinedAt
		}
		out = append(out, e)
	}
	c.JSON(http.StatusOK, gin.H{"role": role, "members": out})
}

// Kick DELETE /api/team/members/:uid — 移除成员(软删 W4;团长不可踢)。
func (h *Handlers) Kick(c *gin.Context) {
	t, uid, ok := h.ownerTeam(c)
	if !ok {
		return
	}
	target := c.Param("uid")
	if target == "" || target == uid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot kick the owner", "code": "team-owner-target"})
		return
	}
	if err := EndMembership(c.Request.Context(), h.DB, t.ID, target); err != nil {
		if errors.Is(err, ErrNotMember) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no such member", "code": "member-gone"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Leave POST /api/team/leave — 退团(member;团长须先转让)。
func (h *Handlers) Leave(c *gin.Context) {
	uid, ok := h.me(c)
	if !ok {
		return
	}
	if _, err := TeamOfOwner(c.Request.Context(), h.DB, uid); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "owner cannot leave; transfer first", "code": "team-owner-leave"})
		return
	}
	t, err := TeamOfMember(c.Request.Context(), h.DB, uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no live team", "code": "team-none"})
		return
	}
	EndMembership(c.Request.Context(), h.DB, t.ID, uid)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Summary GET /api/team/members/:uid/summary — 单成员看板卡(owner;60s 缓存)。
func (h *Handlers) Summary(c *gin.Context) {
	t, _, ok := h.ownerTeam(c)
	if !ok {
		return
	}
	target := c.Param("uid")
	if target == "" || !IsLiveMember(c.Request.Context(), h.DB, t.ID, target) {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such member", "code": "member-gone"})
		return
	}
	card, err := h.memberCard(c.Request.Context(), target, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load"})
		return
	}
	card.DisplayName = displayNameOf(UserDisplayName(c.Request.Context(), h.DB, target), target)
	c.JSON(http.StatusOK, card)
}

// Board GET /api/team/board — 团看板(排行 = weekDone 降序,streak 破平;W5 昵称+数值)。
// 周列头用周一缺省(成员列按各自 ws,混设周首日的团列头近似,契约已注明)。
func (h *Handlers) Board(c *gin.Context) {
	t, _, ok := h.ownerTeam(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	now := time.Now()
	roster, err := Members(ctx, h.DB, t.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load"})
		return
	}
	cards := make([]*SummaryCard, 0, len(roster))
	live := 0
	for _, m := range roster {
		card, err := h.memberCard(ctx, m.UID, now)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "load"})
			return
		}
		card.DisplayName = displayNameOf(m.DisplayName, m.UID)
		if card.WeekDone > 0 {
			live++
		}
		cards = append(cards, card)
	}
	sort.SliceStable(cards, func(i, j int) bool {
		if cards[i].WeekDone != cards[j].WeekDone {
			return cards[i].WeekDone > cards[j].WeekDone
		}
		return cards[i].StreakWeeks > cards[j].StreakWeeks
	})
	c.JSON(http.StatusOK, gin.H{"live": live, "weeks": boardWeeksOf(nil, now), "rows": cards})
}

// Insights GET /api/team/insights — 教练分析卡(rate 降序 weekDone 破平;双窗环比 + 规则解读)。
// 与 Board 同源 states.doc 案② 聚合;60s 缓存同款语义。
func (h *Handlers) Insights(c *gin.Context) {
	t, _, ok := h.ownerTeam(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	now := time.Now()
	roster, err := Members(ctx, h.DB, t.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load"})
		return
	}
	rows := make([]InsightRow, 0, len(roster))
	for _, m := range roster {
		r, err := h.memberInsight(ctx, m.UID, now)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "load"})
			return
		}
		r.UID = m.UID
		r.DisplayName = displayNameOf(m.DisplayName, m.UID)
		rows = append(rows, *r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Rate != rows[j].Rate {
			return rows[i].Rate > rows[j].Rate
		}
		return rows[i].WeekDone > rows[j].WeekDone
	})
	c.JSON(http.StatusOK, gin.H{"rows": rows})
}

/* ---------- W批3.5:计划分发(§13.5 快照中转) + 团长转让(§13.10) ---------- */

// PlansCreate POST /api/team/plans — 分发(owner;同员同名 pending 挂版本链)。
func (h *Handlers) PlansCreate(c *gin.Context) {
	t, uid, ok := h.ownerTeam(c)
	if !ok {
		return
	}
	var body struct {
		UID  string         `json:"uid"`
		Plan map[string]any `json:"plan"`
	}
	bindBody(c, &body)
	planName := ""
	if body.Plan != nil {
		if n, _ := body.Plan["name"].(string); n != "" {
			planName = clip(strings.TrimSpace(n), 120)
		}
	}
	if body.UID == "" || planName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plan name required"})
		return
	}
	if !IsLiveMember(c.Request.Context(), h.DB, t.ID, body.UID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such member", "code": "member-gone"})
		return
	}
	snap, err := json.Marshal(body.Plan)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan"})
		return
	}
	id, err := CreateAssignmentChained(c.Request.Context(), h.DB, t.ID, uid, body.UID, planName, snap)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save"})
		return
	}
	row, err := assignmentsQuery(c.Request.Context(), h.DB,
		`SELECT `+assignmentCols+` FROM plan_assignments WHERE id=$1`, id)
	if err != nil || len(row) != 1 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load"})
		return
	}
	row[0].MemberDisplayName = displayNameOf(UserDisplayName(c.Request.Context(), h.DB, body.UID), body.UID)
	c.JSON(http.StatusOK, gin.H{"plan": row[0]})
}

// PlansList GET /api/team/plans — 角色感知:owner 全团(uid 过滤),member 自己;
// pending = 调用者自己的 distributed 未链计数(红点,W11 M1 拉模型)。
func (h *Handlers) PlansList(c *gin.Context) {
	t, uid, ok := h.ownerTeamCall2(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	var rows []AssignmentRow
	if h.isOwnerUID(ctx, uid) {
		var err error
		rows, err = AssignmentsOfTeam(ctx, h.DB, t.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "load"})
			return
		}
		if f := c.Query("uid"); f != "" {
			kept := rows[:0]
			for _, r := range rows {
				if r.MemberUID == f {
					kept = append(kept, r)
				}
			}
			rows = kept
		}
	} else {
		own, err := AssignmentsOfMember(ctx, h.DB, uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "load"})
			return
		}
		rows = own
	}
	roster, _ := Members(ctx, h.DB, t.ID)
	dn := map[string]string{}
	for _, m := range roster {
		dn[m.UID] = m.DisplayName
	}
	pending := 0
	out := make([]AssignmentRow, 0, len(rows))
	for _, r := range rows {
		if r.Status == "distributed" && r.SupersededBy == nil && r.MemberUID == uid {
			pending++
		}
		r.MemberDisplayName = displayNameOf(dn[r.MemberUID], r.MemberUID)
		out = append(out, r)
	}
	c.JSON(http.StatusOK, gin.H{"pending": pending, "plans": out})
}

// ownerTeamCall2 anyTeam 别名(owner 或 member 进,role 由 isOwnerUID 二判)。
func (h *Handlers) ownerTeamCall2(c *gin.Context) (TeamRow, string, bool) {
	uid, ok := h.me(c)
	if !ok {
		return TeamRow{}, "", false
	}
	if t, err := TeamOfOwner(c.Request.Context(), h.DB, uid); err == nil {
		return t, uid, true
	}
	if t, err := TeamOfMember(c.Request.Context(), h.DB, uid); err == nil {
		return t, uid, true
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "no live team", "code": "team-none"})
	return TeamRow{}, uid, false
}

func (h *Handlers) isOwnerUID(ctx context.Context, uid string) bool {
	_, err := TeamOfOwner(ctx, h.DB, uid)
	return err == nil
}

// PlansAck POST /api/team/plans/:id/ack — 成员结算(loaded/declined)。
func (h *Handlers) PlansAck(c *gin.Context) {
	uid, ok := h.me(c)
	if !ok {
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	bindBody(c, &body)
	if body.Status != "loaded" && body.Status != "declined" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ack status"})
		return
	}
	if err := AckAssignment(c.Request.Context(), h.DB, c.Param("id"), uid, body.Status); err != nil {
		if errors.Is(err, ErrSettled) || errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no such plan", "code": "plan-gone"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// PlansRevoke DELETE /api/team/plans/:id — 教练撤回(仅 distributed)。
func (h *Handlers) PlansRevoke(c *gin.Context) {
	t, _, ok := h.ownerTeam(c)
	if !ok {
		return
	}
	if err := RevokeDistributed(c.Request.Context(), h.DB, c.Param("id"), t.ID); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no such plan", "code": "plan-gone"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Transfer POST /api/team/transfer — 团长转让(原子三 UPDATE;原团长留团 W12)。
func (h *Handlers) Transfer(c *gin.Context) {
	t, uid, ok := h.ownerTeam(c)
	if !ok {
		return
	}
	var body struct {
		ToUID string `json:"to_uid"`
	}
	bindBody(c, &body)
	if body.ToUID == "" || body.ToUID == uid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot transfer to self", "code": "team-owner-target"})
		return
	}
	if err := TransferTeam(c.Request.Context(), h.DB, t.ID, uid, body.ToUID); err != nil {
		if errors.Is(err, ErrNotMember) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no such member", "code": "member-gone"})
			return
		}
		if errors.Is(err, ErrStaleOwner) {
			// 转让竞态:发起人已非 owner(双开/重放)——409 让客户端重拉团态
			c.JSON(http.StatusConflict, gin.H{"error": "team ownership changed, refresh", "code": "owner-stale"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "newOwner": gin.H{
		"displayName": displayNameOf(UserDisplayName(c.Request.Context(), h.DB, body.ToUID), body.ToUID),
	}})
}

// displayNameOf 展示名兜底:users.name 空 → uid 尾 6 位掩码(muhe-fit DisplayNameOf
// 同语义;测试版无 display_name 列,移植适配点)。
func displayNameOf(name, uid string) string {
	if name != "" {
		return name
	}
	if len(uid) <= 6 {
		return uid
	}
	return uid[len(uid)-6:]
}
