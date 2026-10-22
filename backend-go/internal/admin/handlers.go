// admin 端点(P5):overview/users/user/disable/delete/quota/roles/audit-log/step-up。
package admin

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
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

// Roles GET /api/admin/roles — 活跃角色列表(super_admin)。
func (s *Service) Roles(c *gin.Context) {
	rows, err := s.DB.Query(c.Request.Context(), `
		SELECT r.user_id, u.name, r.role::text, COALESCE(r.granted_by::text,''), r.granted_at
		FROM admin_roles r JOIN users u ON u.id = r.user_id
		WHERE r.revoked_at IS NULL ORDER BY r.granted_at`)
	if err != nil {
		c.JSON(500, gin.H{"error": "list"})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var uid, name, role, by string
		var at time.Time
		rows.Scan(&uid, &name, &role, &by, &at)
		out = append(out, gin.H{"user_id": uid, "name": name, "role": role, "granted_by": by, "granted_at": at.UnixMilli()})
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
		SELECT id, admin_user_id, action, COALESCE(target_type,''), COALESCE(target_id,''), detail, created_at
		FROM admin_audit_log ORDER BY id DESC LIMIT 50`)
	if err != nil {
		c.JSON(500, gin.H{"error": "list"})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id int64
		var admin, action, tt, tid string
		var detail []byte
		var at time.Time
		rows.Scan(&id, &admin, &action, &tt, &tid, &detail, &at)
		var d any
		json.Unmarshal(detail, &d)
		out = append(out, gin.H{"id": id, "admin": admin, "action": action, "targetType": tt,
			"targetId": tid, "detail": d, "createdAt": at.UnixMilli()})
	}
	c.JSON(200, gin.H{"events": out})
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
