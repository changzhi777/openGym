// admin 端点(P5):overview/users/user/disable/delete/quota/roles/audit-log/step-up。
package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/changzhi777/openGym/backend-go/internal/auth"
	"github.com/changzhi777/openGym/backend-go/internal/presence"
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

// Overview GET /api/admin/overview — 聚合 KPI(契约 adminOverview 扩形,§3.3):
// 基础计数 + 今日训练/活跃 + 14 天趋势(active=当日有训练的用户数,历史活跃无
// 独立数据源以此近似)+ 需要关注(停用;quota_near 待 0008 usage_counters 落地)
// + 在线数(presence)。一次请求服务端聚合,前端不再自拉 users 客户端求和。
func (s *Service) Overview(c *gin.Context) {
	ct := c.Request.Context()
	var users, disabled, workouts, workoutsToday, activeToday int64
	s.DB.QueryRow(ct, `SELECT count(*) FROM users`).Scan(&users)
	s.DB.QueryRow(ct, `SELECT count(*) FROM users WHERE disabled`).Scan(&disabled)
	s.DB.QueryRow(ct, `SELECT count(*) FROM workouts`).Scan(&workouts)
	s.DB.QueryRow(ct, `SELECT count(*) FROM workouts WHERE date = to_char(now(),'YYYY-MM-DD')`).Scan(&workoutsToday)
	s.DB.QueryRow(ct, `SELECT count(DISTINCT user_id) FROM workouts WHERE date = to_char(now(),'YYYY-MM-DD')`).Scan(&activeToday)
	var aiMonth any // usage_counters 随 0008(批3收编)可用;缺表 → null(契约:落地后出现)
	s.DB.QueryRow(ct, `SELECT COALESCE(sum(used),0) FROM usage_counters WHERE resource='coach' AND period = to_char(now(),'YYYY-MM')`).Scan(&aiMonth)

	trend := []gin.H{}
	tr, err := s.DB.Query(ct, `
		SELECT to_char(g.d,'YYYY-MM-DD'), COALESCE(w.cnt,0), COALESCE(w.u_cnt,0)
		FROM generate_series(current_date - interval '13 days', current_date, interval '1 day') AS g(d)
		LEFT JOIN (SELECT date, count(*) cnt, count(DISTINCT user_id) u_cnt FROM workouts
			WHERE date >= to_char(current_date - interval '13 days','YYYY-MM-DD') GROUP BY date) w
		ON w.date = to_char(g.d,'YYYY-MM-DD') ORDER BY g.d`)
	if err == nil {
		for tr.Next() {
			var d string
			var cnt, uc int64
			tr.Scan(&d, &cnt, &uc)
			trend = append(trend, gin.H{"date": d, "active": uc, "workouts": cnt})
		}
		tr.Close()
	}

	attention := []gin.H{}
	if disabled > 0 {
		attention = append(attention, gin.H{"code": "disabled", "count": disabled})
	}

	live := 0
	if s.Rdb != nil {
		live = len(presence.Index(ct, s.Rdb))
	}
	c.JSON(200, gin.H{"users": users, "active_today": activeToday, "workouts": workouts,
		"workouts_today": workoutsToday, "live": live, "ai_calls_month": aiMonth,
		"trend_14d": trend, "attention": attention, "now": time.Now().UnixMilli()})
}

// Users GET /api/admin/users — 列表(契约 adminUsers):搜索/角色/状态筛选 + 排序 +
// 经典分页 + 统计头;邮箱恒脱敏(§3.4:完整地址是 step-up 能力,列表永不返回)。
func (s *Service) Users(c *gin.Context) {
	ct := c.Request.Context()
	q := strings.TrimSpace(c.Query("q"))
	roleF, statusF, sort := c.Query("role"), c.Query("status"), c.Query("sort")
	page, per := 1, 25
	if v, err := strconv.Atoi(c.Query("page")); err == nil && v >= 1 {
		page = v
	}
	if v, err := strconv.Atoi(c.Query("per_page")); err == nil && v >= 1 && v <= 100 {
		per = v
	}

	where, args := []string{"1=1"}, []any{}
	n := 0
	add := func(v any) string { args = append(args, v); n++; return fmt.Sprintf("$%d", n) }
	if q != "" {
		p := add("%" + q + "%")
		where = append(where, fmt.Sprintf("(u.name ILIKE %s OR u.email ILIKE %s)", p, p))
	}
	if statusF == "active" {
		where = append(where, "NOT u.disabled")
	} else if statusF == "disabled" {
		where = append(where, "u.disabled")
	}
	switch roleF {
	case "none":
		where = append(where, "NOT EXISTS (SELECT 1 FROM admin_roles ar WHERE ar.user_id=u.id AND ar.revoked_at IS NULL)")
	case RoleSuperAdmin, RoleOps, RoleSupport:
		p := add(roleF)
		where = append(where, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM admin_roles ar WHERE ar.user_id=u.id AND ar.revoked_at IS NULL AND ar.role::text=%s)", p))
	}
	w := strings.Join(where, " AND ")
	order := map[string]string{
		"workouts": "workouts DESC, created DESC",
		"created":  "created DESC",
	}[sort]
	if order == "" {
		order = "last_sync DESC NULLS LAST, created DESC" // 默认:最近同步(§3.4)
	}

	var total, todayNew, disabledCount int
	s.DB.QueryRow(ct, `SELECT count(*) FROM users u WHERE `+w, args...).Scan(&total)
	s.DB.QueryRow(ct, `SELECT count(*) FROM users WHERE created_at::date = now()::date`).Scan(&todayNew)
	s.DB.QueryRow(ct, `SELECT count(*) FROM users WHERE disabled`).Scan(&disabledCount)

	rows, err := s.DB.Query(ct, `
		SELECT u.id, u.name, COALESCE(u.email,''), to_char(u.created_at,'YYYY-MM-"T"HH24:MI:SS"Z"') AS created, u.disabled,
		       (SELECT count(*) FROM workouts wk WHERE wk.user_id=u.id) AS workouts,
		       to_char((SELECT max(wk.date) FROM workouts wk WHERE wk.user_id=u.id),'YYYY-MM-DD') AS last_workout,
		       (SELECT EXTRACT(epoch FROM st.updated_at)*1000 FROM states st WHERE st.user_id=u.id) AS last_sync,
		       (SELECT count(*) > 0 FROM push_subs ps WHERE ps.user_id=u.id) AS has_push,
		       (SELECT ar.role::text FROM admin_roles ar WHERE ar.user_id=u.id AND ar.revoked_at IS NULL
		        ORDER BY CASE ar.role::text WHEN 'super_admin' THEN 3 WHEN 'ops' THEN 2 ELSE 1 END DESC LIMIT 1) AS role
		FROM users u WHERE `+w+` ORDER BY `+order+` LIMIT `+fmt.Sprint(per)+` OFFSET `+fmt.Sprint((page-1)*per), args...)
	if err != nil {
		c.JSON(500, gin.H{"error": "list"})
		return
	}
	defer rows.Close()
	live := map[string]map[string]any{}
	if s.Rdb != nil {
		live = presence.Index(ct, s.Rdb)
	}
	out := []gin.H{}
	for rows.Next() {
		var id, name, created, role string
		var email *string
		var disabled bool
		var workouts int64
		var lastWorkout, lastSync *string
		var hasPush bool
		if rows.Scan(&id, &name, &email, &created, &disabled, &workouts, &lastWorkout, &lastSync, &hasPush, &role) != nil {
			continue
		}
		var ls any
		if lastSync != nil {
			ls, _ = strconv.ParseFloat(*lastSync, 64)
		}
		out = append(out, gin.H{
			"id": id, "name": name, "email": maskEmail(email), "created": created,
			"disabled": disabled, "admin": role != "", "role": nilIfEmpty(role),
			"workouts": workouts, "lastWorkout": lastWorkout, "lastSync": ls,
			"hasPush": hasPush, "live": live[id],
		})
	}
	on, _ := store.InviteOnlyEffective(ct, s.DB, s.InviteEnvDef)
	c.JSON(200, gin.H{"users": out, "total": total, "today_new": todayNew,
		"disabled_count": disabledCount, "page": page, "per_page": per,
		"invite_only": on, "now": time.Now().UnixMilli()})
}

// maskEmail §3.4:首字符 + *** + @域;无邮箱/畸形 → nil(契约 oneOf string/null)。
func maskEmail(e *string) any {
	if e == nil || *e == "" {
		return nil
	}
	at := strings.Index(*e, "@")
	if at <= 0 {
		return nil
	}
	return (*e)[:1] + "***" + (*e)[at:]
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
		"user":     gin.H{"id": id, "name": name, "created": created.UnixMilli(), "disabled": disabled, "admin": false},
		"workouts": workouts, "bodyweight": weights,
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
// Activity GET /api/admin/users/{id}/activity — 敏感明细(契约 adminUserActivity):
// ops+step-up(路由挂),每次读取写审计;形状对齐 Node GET /api/admin/user(AdminUserDetail)。
// 数据源是 states.doc 整份文档(与 Node state-<uid>.json 同构):投影 routines、
// bodyweight、workouts(去 media refs、最新在前)。
func (s *Service) Activity(c *gin.Context) {
	id := c.Param("id")
	ct := c.Request.Context()
	var name string
	if err := s.DB.QueryRow(ct, `SELECT name FROM users WHERE id=$1`, id).Scan(&name); err != nil {
		c.JSON(404, gin.H{"error": "no such user"})
		return
	}
	var doc []byte
	err := s.DB.QueryRow(ct, `SELECT doc FROM states WHERE user_id=$1`, id).Scan(&doc)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		c.JSON(500, gin.H{"error": "state"})
		return
	}
	if !s.Audit(c, "admin.user.activity", "user", id, nil) {
		return
	}
	out := gin.H{
		"user":       gin.H{"id": id, "name": name},
		"unit":       "kg",
		"routines":   []any{},
		"bodyweight": []any{},
		"workouts":   []any{},
	}
	if doc != nil {
		var m map[string]any
		if json.Unmarshal(doc, &m) == nil {
			if v, ok := m["unit"]; ok {
				out["unit"] = v
			}
			if ts, ok := m["_ts"]; ok {
				out["lastSync"] = ts
			}
			// routines 投影 {id,name,emoji,count}(ex 条目数),Node records() 同义。
			if rs, ok := m["routines"].([]any); ok {
				proj := []any{}
				for _, r := range rs {
					rm, _ := r.(map[string]any)
					count := 0
					if ex, ok := rm["ex"].([]any); ok {
						count = len(ex)
					}
					proj = append(proj, gin.H{
						"id": rm["id"], "name": rm["name"], "emoji": rm["emoji"], "count": count,
					})
				}
				out["routines"] = proj
			}
			if bw, ok := m["bodyweight"]; ok {
				out["bodyweight"] = bw
			}
			// workouts:去 media 键,最新在前(Node records().reverse() 同义)。
			if ws, ok := m["workouts"].([]any); ok {
				clean := make([]any, 0, len(ws))
				for i := len(ws) - 1; i >= 0; i-- {
					wm, _ := ws[i].(map[string]any)
					delete(wm, "media")
					clean = append(clean, wm)
				}
				out["workouts"] = clean
			}
		}
	}
	c.JSON(200, out)
}

// Export GET /api/admin/users/{id}/export — ops+step-up(路由挂),读取写审计;
// 状态文档原样导出(Node admin 注释自认 GET /api/admin/user 即 export 的同义能力)。
func (s *Service) Export(c *gin.Context) {
	id := c.Param("id")
	ct := c.Request.Context()
	var exists bool
	if err := s.DB.QueryRow(ct, `SELECT true FROM users WHERE id=$1`, id).Scan(&exists); err != nil {
		c.JSON(404, gin.H{"error": "no such user"})
		return
	}
	var doc []byte
	s.DB.QueryRow(ct, `SELECT doc FROM states WHERE user_id=$1`, id).Scan(&doc) // 无 state = 从未同步,导出空
	if !s.Audit(c, "admin.user.export", "user", id, nil) {
		return
	}
	var export any
	if doc != nil {
		json.Unmarshal(doc, &export)
	}
	c.JSON(200, gin.H{"export": export, "exported_at": time.Now().UnixMilli()})
}

// QuotaGet GET /api/admin/users/{id}/quota — 限额 + 当期已用(契约 used_ai_calls_month)。
func (s *Service) QuotaGet(c *gin.Context) {
	id := c.Param("id")
	out := gin.H{}
	var limits []byte
	if err := s.DB.QueryRow(c.Request.Context(), `SELECT limits FROM user_quotas WHERE user_id=$1`, id).Scan(&limits); err == nil {
		var m map[string]any
		if json.Unmarshal(limits, &m) == nil {
			for k, v := range m {
				out[k] = v
			}
		}
	}
	// 当期已用(契约 used_ai_calls_month;usage_counters 随批3的0008迁移落地,缺表容错为 0)。
	var used int64
	s.DB.QueryRow(c.Request.Context(), `
		SELECT COALESCE((SELECT used FROM usage_counters WHERE user_id=$1 AND resource='coach' AND period=to_char(now(),'YYYY-MM')),0)`, id).Scan(&used)
	out["used_ai_calls_month"] = used
	c.JSON(200, out)
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
func (s *Service) RoleGrant(c *gin.Context)  { s.roleChange(c, true) }
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
	ct := c.Request.Context()
	limit := 50
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v >= 1 && v <= 200 {
		limit = v
	}
	where, args := []string{"1=1"}, []any{}
	n := 0
	add := func(v any) string { args = append(args, v); n++; return fmt.Sprintf("$%d", n) }
	if b, err := strconv.ParseInt(c.Query("before"), 10, 64); err == nil && b > 0 {
		where = append(where, "l.id < "+add(b))
	}
	if cat := c.Query("cat"); cat == "fail" {
		where = append(where, "false") // Go 栈无失败事件源,答空集而非报错
	} else if cat != "" {
		where = append(where, "l.action LIKE "+add(cat+".%"))
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		p := add("%" + q + "%")
		where = append(where, fmt.Sprintf(
			"(l.action ILIKE %s OR COALESCE(l.admin_user_id,'') ILIKE %s OR COALESCE(l.target_id,'') ILIKE %s OR COALESCE(u.name,'') ILIKE %s OR COALESCE(t.name,'') ILIKE %s)",
			p, p, p, p, p))
	}
	w := strings.Join(where, " AND ")
	var total int64
	s.DB.QueryRow(ct, `SELECT count(*) FROM admin_audit_log l WHERE `+w, args...).Scan(&total)

	rows, err := s.DB.Query(ct, `
		SELECT l.id, COALESCE(u.name, l.admin_user_id), l.action,
		       COALESCE(l.target_type,''), COALESCE(l.target_id,''), COALESCE(t.name,''),
		       l.detail, l.created_at, l.ip
		FROM admin_audit_log l
		LEFT JOIN users u ON u.id = l.admin_user_id
		LEFT JOIN users t ON t.id = l.target_id
		WHERE `+w+` ORDER BY l.id DESC LIMIT `+fmt.Sprint(limit), args...)
	if err != nil {
		c.JSON(500, gin.H{"error": "list"})
		return
	}
	defer rows.Close()
	// 契约 §3.8:support 永不见 IP —— 行内不渲染该键,而非置 null。
	role, _ := c.Get("adminRole")
	hideIP := role == RoleSupport
	events := []gin.H{}
	var nextBefore any
	for rows.Next() {
		var id int64
		var admin, action, tt, tid, tname string
		var detail []byte
		var at time.Time
		var ip *string
		if rows.Scan(&id, &admin, &action, &tt, &tid, &tname, &detail, &at, &ip) != nil {
			continue
		}
		var d any
		json.Unmarshal(detail, &d)
		ev := gin.H{"id": id, "admin": admin, "action": action, "ok": true,
			"targetType": tt, "targetId": tid, "targetName": nilIfEmpty(tname),
			"detail": d, "createdAt": at.UnixMilli()}
		if !hideIP {
			ev["ip"] = ip
		}
		events = append(events, ev)
		nextBefore = id
	}
	if len(events) < limit {
		nextBefore = nil
	}
	c.JSON(200, gin.H{"events": events, "total": total, "nextBefore": nextBefore,
		"enabled": true, "retention": gin.H{"max": 0, "days": AuditRetentionDays},
		"now": time.Now().UnixMilli()})
}

// AuditClear POST /api/admin/audit/clear — 清空(super 路由挂 + 输入"清空"由前端把关);
// bigserial 不回退,清空后首条审计事件天然留可见 id gap(与 Node 同语义)。
func (s *Service) AuditClear(c *gin.Context) {
	if _, err := s.DB.Exec(c.Request.Context(), `DELETE FROM admin_audit_log`); err != nil {
		c.JSON(500, gin.H{"error": "clear"})
		return
	}
	s.Audit(c, "admin.audit.clear", "", "", nil)
	c.JSON(200, gin.H{"ok": true})
}

// PurgeLoop 每日清理超保留期审计(AuditRetentionDays=90,设计 §3.8 页头口径)。
func (s *Service) PurgeLoop(ctx context.Context) {
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.DB.Exec(ctx, `DELETE FROM admin_audit_log WHERE created_at < now() - make_interval(days => $1)`, AuditRetentionDays)
		}
	}
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
// 与 Node proveOwner 的 passkey 分支同语义:cid 一次性消费、凭据必须属于当前账号、
// 验签通过才开窗;任一步失败返回 403(前端 stepUp.js 据此重新验证并重放原请求)。

// StepUp POST /api/admin/step-up — 短信码确认(passkey 退役后的一致性口径)。
// phone 必须已绑定到当前管理员;核验通过开 5 分钟窗口,审计留痕。
func (s *Service) StepUp(c *gin.Context) {
	uid, _ := c.Get("adminUID")
	var body struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	if raw, ok := c.Get("rawBody"); ok {
		if bs, ok2 := raw.([]byte); ok2 {
			json.Unmarshal(bs, &body)
		}
	}
	owner, bound, err := store.UserByIdentity(c.Request.Context(), s.DB, store.IdPhone, strings.TrimSpace(body.Phone))
	if err != nil || !bound || owner != uid.(string) {
		c.JSON(403, gin.H{"error": "this number is not bound to the signed-in admin", "code": "phone"})
		return
	}
	if !s.SmsStore.Verify(c.Request.Context(), body.Phone, strings.TrimSpace(body.Code)) {
		c.JSON(403, gin.H{"error": "wrong or expired code", "code": "sms-code"})
		return
	}
	s.GrantStepUp(uid.(string))
	s.Audit(c, "admin.step_up", "", "", gin.H{"via": "sms"})
	c.JSON(200, gin.H{"ok": true, "until": time.Now().Add(5 * time.Minute).UnixMilli()})
}
