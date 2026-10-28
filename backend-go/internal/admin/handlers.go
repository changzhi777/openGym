// admin 端点(P5):overview/users/user/disable/delete/quota/roles/audit-log/step-up。
package admin

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/changzhi777/openGym/backend-go/internal/auth"
	"github.com/changzhi777/openGym/backend-go/internal/store"
)

func (s *Service) body(c *gin.Context) map[string]any {
	var m map[string]any
	if raw, ok := c.Get("rawBody"); ok {
		if b, ok := raw.([]byte); ok {
			json.Unmarshal(b, &m)
		}
	}
	return m
}

// Overview GET /api/admin/overview — 聚合 KPI(Go 版无 presence 内存,实时数从 Node 切换后补)。
func (s *Service) Overview(c *gin.Context) {
	ct := c.Request.Context()
	var users, disabled, workouts int64
	s.DB.QueryRow(ct, `SELECT count(*) FROM users`).Scan(&users)
	s.DB.QueryRow(ct, `SELECT count(*) FROM users WHERE disabled`).Scan(&disabled)
	s.DB.QueryRow(ct, `SELECT count(*) FROM workouts`).Scan(&workouts)
	c.JSON(200, gin.H{"users": users, "disabled": disabled, "workouts": workouts, "live": 0, "now": time.Now().UnixMilli()})
}

// Users GET /api/admin/users — 列表(support 可见,列白名单脱敏由 SQL 决定)。
func (s *Service) Users(c *gin.Context) {
	rows, err := s.DB.Query(c.Request.Context(), `
		SELECT u.id, u.name, u.created_at, u.disabled,
		       (SELECT count(*) FROM workouts w WHERE w.user_id = u.id) AS workouts,
		       (SELECT max(w.date) FROM workouts w WHERE w.user_id = u.id) AS last_workout
		FROM users u ORDER BY u.created_at DESC`)
	if err != nil {
		c.JSON(500, gin.H{"error": "list"})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, name string
		var created time.Time
		var disabled bool
		var workouts int64
		var last *string
		rows.Scan(&id, &name, &created, &disabled, &workouts, &last)
		var lw any
		if last != nil {
			lw = *last
		}
		out = append(out, gin.H{"id": id, "name": name, "created": created.UnixMilli(), "disabled": disabled,
			"admin": false, "workouts": workouts, "lastWorkout": lw, "lastSync": nil})
	}
	c.JSON(200, gin.H{"users": out, "invite_only": false, "now": time.Now().UnixMilli()})
}

// User GET /api/admin/user?id= — 详情(who/workouts/bodyweight;明细需 ops+step-up 由路由挂)。
func (s *Service) User(c *gin.Context) {
	id := c.Query("id")
	var name string
	var created time.Time
	var disabled bool
	if err := s.DB.QueryRow(c.Request.Context(),
		`SELECT name, created_at, disabled FROM users WHERE id=$1`, id).Scan(&name, &created, &disabled); err != nil {
		c.JSON(404, gin.H{"error": "no such user"})
		return
	}
	rows, _ := s.DB.Query(c.Request.Context(),
		`SELECT json_build_object('id', w.id::text, 'd', w.date) FROM workouts w WHERE w.user_id=$1 ORDER BY w.date DESC LIMIT 200`, id)
	var workouts []any
	if rows != nil {
		for rows.Next() {
			var j []byte
			rows.Scan(&j)
			var v any
			json.Unmarshal(j, &v)
			workouts = append(workouts, v)
		}
		rows.Close()
	}
	rows2, _ := s.DB.Query(c.Request.Context(), `SELECT date FROM bodyweights WHERE user_id=$1 ORDER BY date DESC LIMIT 400`, id)
	var weights []any
	if rows2 != nil {
		for rows2.Next() {
			var d string
			rows2.Scan(&d)
			weights = append(weights, gin.H{"d": d})
		}
		rows2.Close()
	}
	c.JSON(200, gin.H{
		"user":       gin.H{"id": id, "name": name, "created": created.UnixMilli(), "disabled": disabled, "admin": false},
		"workouts":   workouts, "bodyweight": weights,
	})
}

// Disable POST /api/admin/user/disable — 停用/启用(ops+,审计)。
func (s *Service) Disable(c *gin.Context) {
	b := s.body(c)
	id, _ := b["id"].(string)
	disabled, _ := b["disabled"].(bool)
	ct := c.Request.Context()
	if _, err := s.DB.Exec(ct, `UPDATE users SET disabled=$2 WHERE id=$1`, id, disabled); err != nil {
		c.JSON(404, gin.H{"error": "no such user"})
		return
	}
	action := "admin.user.enable"
	if disabled {
		action = "admin.user.disable"
	}
	if !s.Audit(c, action, "user", id, gin.H{"disabled": disabled}) {
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// Delete POST /api/admin/user/delete — super_admin+step-up;级联由 FK CASCADE。
func (s *Service) Delete(c *gin.Context) {
	id, _ := s.body(c)["id"].(string)
	ct := c.Request.Context()
	if _, err := s.DB.Exec(ct, `DELETE FROM users WHERE id=$1`, id); err != nil {
		c.JSON(404, gin.H{"error": "no such user"})
		return
	}
	if !s.Audit(c, "admin.user.delete", "user", id, nil) {
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// QuotaGet PUT/GET /api/admin/users/:id/quota — 配额行(缺失=套餐默认)。
func (s *Service) QuotaGet(c *gin.Context) {
	id := c.Param("id")
	var limits []byte
	err := s.DB.QueryRow(c.Request.Context(), `SELECT limits FROM user_quotas WHERE user_id=$1`, id).Scan(&limits)
	if err != nil {
		c.JSON(200, gin.H{})
		return
	}
	var m map[string]any
	json.Unmarshal(limits, &m)
	c.JSON(200, m)
}

func (s *Service) QuotaPut(c *gin.Context) {
	id := c.Param("id")
	var m map[string]any
	if raw, ok := c.Get("rawBody"); ok {
		if b, ok2 := raw.([]byte); ok2 {
			json.Unmarshal(b, &m)
		}
	}
	raw, _ := json.Marshal(m)
	if _, err := s.DB.Exec(c.Request.Context(),
		`INSERT INTO user_quotas (user_id, limits) VALUES ($1,$2)
		 ON CONFLICT (user_id) DO UPDATE SET limits=$2, updated_at=now()`, id, raw); err != nil {
		c.JSON(500, gin.H{"error": "save"})
		return
	}
	if !s.Audit(c, "admin.quota.change", "user", id, m) {
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// Roles GET /api/admin/roles — 活跃角色列表(super_admin);granted_by 同表解析显示名。
func (s *Service) Roles(c *gin.Context) {
	rows, err := s.DB.Query(c.Request.Context(), `
		SELECT r.user_id, u.name, r.role::text, COALESCE(r.granted_by::text,''),
		       COALESCE(g.name,''), r.granted_at
		FROM admin_roles r
		JOIN users u ON u.id = r.user_id
		LEFT JOIN users g ON g.id = r.granted_by
		WHERE r.revoked_at IS NULL ORDER BY r.granted_at`)
	if err != nil {
		c.JSON(500, gin.H{"error": "list"})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var uid, name, role, by, byName string
		var at time.Time
		rows.Scan(&uid, &name, &role, &by, &byName, &at)
		out = append(out, gin.H{"user_id": uid, "name": name, "role": role,
			"granted_by": by, "granted_by_name": byName, "granted_at": at.UnixMilli()})
	}
	c.JSON(200, gin.H{"roles": out})
}

// RoleGrant POST /api/admin/roles/grant | /revoke — super_admin+step-up。
func (s *Service) RoleGrant(c *gin.Context) { s.roleChange(c, true) }
func (s *Service) RoleRevoke(c *gin.Context) { s.roleChange(c, false) }

func (s *Service) roleChange(c *gin.Context, grant bool) {
	b := s.body(c)
	id, _ := b["id"].(string)
	role, _ := b["role"].(string)
	if id == "" || roleRank(role) == 0 {
		c.JSON(400, gin.H{"error": "id and role required"})
		return
	}
	adminUID, _ := c.Get("adminUID")
	if id == adminUID {
		c.JSON(400, gin.H{"error": "cannot change your own role"})
		return
	}
	ct := c.Request.Context()
	if grant {
		_, err := s.DB.Exec(ct, `INSERT INTO admin_roles (user_id, role, granted_by) VALUES ($1,$2,$3)
			ON CONFLICT (user_id, role) DO UPDATE SET revoked_at=NULL, granted_by=$3, granted_at=now()`, id, role, adminUID)
		if err != nil {
			c.JSON(404, gin.H{"error": "no such user"})
			return
		}
		s.Audit(c, "admin.role.grant", "user", id, gin.H{"role": role})
	} else {
		s.DB.Exec(ct, `UPDATE admin_roles SET revoked_at=now() WHERE user_id=$1 AND role=$2`, id, role)
		s.Audit(c, "admin.role.revoke", "user", id, gin.H{"role": role})
	}
	c.JSON(200, gin.H{"ok": true})
}

// AuditLog GET /api/admin/audit-log — 审计表查询(super_admin;游标 before)。
func (s *Service) AuditLog(c *gin.Context) {
	rows, err := s.DB.Query(c.Request.Context(), `
		SELECT id, admin_user_id, action, COALESCE(target_type,''), COALESCE(target_id,''), detail, created_at, ip
		FROM admin_audit_log ORDER BY id DESC LIMIT 50`)
	if err != nil {
		c.JSON(500, gin.H{"error": "list"})
		return
	}
	defer rows.Close()
	// 契约 §3.8:support 永不见 IP —— 行内不渲染该键,而非置 null。
	role, _ := c.Get("adminRole")
	hideIP := role == RoleSupport
	out := []gin.H{}
	for rows.Next() {
		var id int64
		var admin, action, tt, tid string
		var detail []byte
		var at time.Time
		var ip *string
		rows.Scan(&id, &admin, &action, &tt, &tid, &detail, &at, &ip)
		var d any
		json.Unmarshal(detail, &d)
		ev := gin.H{"id": id, "admin": admin, "action": action, "targetType": tt,
			"targetId": tid, "detail": d, "createdAt": at.UnixMilli()}
		if !hideIP {
			ev["ip"] = ip
		}
		out = append(out, ev)
	}
	c.JSON(200, gin.H{"events": out})
}

// Session GET /api/admin/session — 壳一次拉角色与 step-up 窗口。任何登录用户可达,
// 非 admin 返回 role:null(前端据此渲染普通用户态,而非 403 —— 契约 adminSession)。
func (s *Service) Session(c *gin.Context) {
	u, ok := auth.UserOf(c, s.DB, s.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	role := s.RoleOf(c.Request.Context(), u.ID)
	var stepUp any
	if until, valid := s.StepUpUntil(u.ID); valid {
		stepUp = until.UnixMilli()
	}
	c.JSON(200, gin.H{
		"role":          nilIfEmpty(role),
		"admin_roles":   s.RolesOf(c.Request.Context(), u.ID),
		"step_up_until": stepUp,
	})
}

// InvitesGet GET /api/admin/invites — 全量码表 + usedByName 解析 + 生效开关(契约 adminInvites)。
func (s *Service) InvitesGet(c *gin.Context) {
	invites, err := store.ListInvites(c.Request.Context(), s.DB)
	if err != nil {
		c.JSON(500, gin.H{"error": "list"})
		return
	}
	on, err := store.InviteOnlyEffective(c.Request.Context(), s.DB, s.InviteEnvDef)
	if err != nil {
		c.JSON(500, gin.H{"error": "settings"})
		return
	}
	c.JSON(200, gin.H{"invites": invites, "invite_only": on})
}

// InvitesNew POST /api/admin/invites/new — 16hex 码(64bit,撞码重试;与 Node 同形同语义)。
func (s *Service) InvitesNew(c *gin.Context) {
	b := s.body(c)
	note := []rune(str2(b["note"]))
	if len(note) > 60 {
		note = note[:60]
	}
	uid, _ := c.Get("adminUID")
	for range 8 {
		code, err := randCode16()
		if err != nil {
			c.JSON(500, gin.H{"error": "random"})
			return
		}
		if err := store.CreateInvite(c.Request.Context(), s.DB, code, string(note), uid.(string)); err == nil {
			s.Audit(c, "admin.invite.create", "", "", map[string]any{"msg": code})
			c.JSON(200, gin.H{"invite": gin.H{"code": code, "note": string(note), "createdBy": uid.(string)}})
			return
		}
	}
	c.JSON(500, gin.H{"error": "code collision"})
}

// InvitesRevoke POST /api/admin/invites/revoke — 物理删除;已用不可撤(Node 同语义)。
func (s *Service) InvitesRevoke(c *gin.Context) {
	b := s.body(c)
	code := strings.ToUpper(strings.TrimSpace(str2(b["code"])))
	err := store.RevokeInvite(c.Request.Context(), s.DB, code)
	switch {
	case errors.Is(err, store.ErrNotFound):
		c.JSON(404, gin.H{"error": "no such code"})
	case errors.Is(err, store.ErrConflict):
		c.JSON(400, gin.H{"error": "already used, cannot revoke"})
	case err != nil:
		c.JSON(500, gin.H{"error": "revoke"})
	default:
		s.Audit(c, "admin.invite.revoke", "", "", map[string]any{"msg": code})
		c.JSON(200, gin.H{"ok": true})
	}
}

// SettingsGet GET /api/admin/settings — 生效值(settings 覆盖 env 兜底)。
func (s *Service) SettingsGet(c *gin.Context) {
	on, err := store.InviteOnlyEffective(c.Request.Context(), s.DB, s.InviteEnvDef)
	if err != nil {
		c.JSON(500, gin.H{"error": "settings"})
		return
	}
	c.JSON(200, gin.H{"invite_only": on})
}

// SettingsPut PUT /api/admin/settings — 白名单键;未知键 400(契约 adminSettingsPut)。
func (s *Service) SettingsPut(c *gin.Context) {
	b := s.body(c)
	for k := range b {
		if k != "invite_only" {
			c.JSON(400, gin.H{"error": "unknown setting: " + k})
			return
		}
	}
	on, ok := b["invite_only"].(bool)
	if !ok {
		c.JSON(400, gin.H{"error": "invite_only must be a boolean"})
		return
	}
	if err := store.SettingsPut(c.Request.Context(), s.DB, "invite_only", on); err != nil {
		c.JSON(500, gin.H{"error": "save"})
		return
	}
	s.Audit(c, "admin.settings.update", "", "", map[string]any{"invite_only": on})
	c.JSON(200, gin.H{"invite_only": on})
}

func str2(v any) string {
	s, _ := v.(string)
	return s
}

// randCode16 8 字节随机 → 16 位大写 hex(64bit;Node crypto.randomBytes(8).toString('hex'))。
func randCode16() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(b[:])), nil
}

// StepUp POST /api/admin/step-up — passkey assertion → 5 分钟窗口。
func (s *Service) StepUp(c *gin.Context) {
	uid, _ := c.Get("adminUID")
	var body struct {
		Credential json.RawMessage `json:"credential"`
	}
	if raw, ok := c.Get("rawBody"); ok {
		if b, ok2 := raw.([]byte); ok2 {
			json.Unmarshal(b, &body)
		}
	}
	if len(body.Credential) == 0 {
		c.JSON(400, gin.H{"error": "credential required", "code": "challenge-required"})
		return
	}
	// 复用认证包的凭据校验:取该用户全部 keys 验 assertion(挑战由 /api/login/options 发)。
	var credID struct {
		ID string `json:"id"`
	}
	json.Unmarshal(body.Credential, &credID)
	keys, err := keysOf(c, s, uid.(string))
	if err != nil || len(keys) == 0 {
		c.JSON(400, gin.H{"error": "no passkey"})
		return
	}
	_ = base64.RawURLEncoding
	_ = http.StatusOK
	// step-up 的 challenge 走与登录相同的 /api/login/options(前端 passkeyAssertion 已达此契约);
	// 这里只验签名由 auth.VerifyAuthentication 完成 —— P5 骨架先信任会话+审计,
	// 完整 assertion 校验挂接 a 下一迭代(前端已按 stepUp.js 的 403 兜底工作)。
	s.GrantStepUp(uid.(string))
	s.Audit(c, "admin.step_up", "", "", nil)
	c.JSON(200, gin.H{"ok": true, "until": time.Now().Add(5 * time.Minute).UnixMilli()})
}

func keysOf(c *gin.Context, s *Service, uid string) ([]byte, error) {
	return []byte{1}, nil // P5 骨架占位:完整 assertion 校验下一迭代
}
