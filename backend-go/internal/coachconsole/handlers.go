// coachconsole — 教练工作台(fork;绑定模式):绑定学员/白名单数据/计划下发。
// 角色闸门:admin_roles 的 coach 或 super_admin;不走 admin 的 step-up 体系。
// 健康数据最小化:学员端点一律白名单投影(routines/week/workouts/bodyweight),
// 围度、进度照片、settings 等敏感字段永不透传。
package coachconsole

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/changzhi777/openGym/backend-go/internal/auth"
	"github.com/changzhi777/openGym/backend-go/internal/presence"
	"github.com/changzhi777/openGym/backend-go/internal/push"
	"github.com/changzhi777/openGym/backend-go/internal/store"
)

type Service struct {
	DB       *pgxpool.Pool
	Rdb      *redis.Client
	Sessions *auth.Sessions
	Push     *push.Handlers
}

func New(db *pgxpool.Pool, rdb *redis.Client, ss *auth.Sessions, ph *push.Handlers) *Service {
	return &Service{DB: db, Rdb: rdb, Sessions: ss, Push: ph}
}

// RequireCoach coach 或 super_admin 放行并注入 coachUID;否则 401/403。
func (s *Service) RequireCoach() gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := auth.UserOf(c, s.DB, s.Sessions)
		if !ok {
			c.AbortWithStatusJSON(401, gin.H{"error": "not signed in"})
			return
		}
		if !store.HasAnyRole(c.Request.Context(), s.DB, u.ID, "coach", "super_admin") {
			c.AbortWithStatusJSON(403, gin.H{"error": "coach role required", "code": "coach"})
			return
		}
		c.Set("coachUID", u.ID)
		c.Next()
	}
}

func fail(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}

func validPhone(p string) bool {
	if len(p) != 11 || p[0] != '1' {
		return false
	}
	for _, r := range p {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

type rowScanner interface {
	Scan(dest ...any) error
}

// summarySQL 一条 SQL 出白名单摘要(workouts 计数与最近训练日走 workouts 表,
// lastSync/planDays 走 states 文档投影——与 admin Users 端点同款手法)。
const summarySQL = `
	SELECT u.id, u.name,
	       COALESCE((SELECT ai.identifier FROM auth_identities ai
	                 WHERE ai.user_id=u.id AND ai.kind='phone' LIMIT 1), ''),
	       (SELECT count(*) FROM workouts wk WHERE wk.user_id=u.id),
	       (SELECT max(wk.date) FROM workouts wk WHERE wk.user_id=u.id),
	       (SELECT EXTRACT(epoch FROM st.updated_at)*1000 FROM states st WHERE st.user_id=u.id),
	       COALESCE((SELECT jsonb_array_length(st.doc->'week') FROM states st WHERE st.user_id=u.id), 0),
	       to_char(ca.created_at,'YYYY-MM-DD"T"HH24:MI:SS"Z"')
	FROM coach_assignments ca JOIN users u ON u.id=ca.student_id
	WHERE %s ORDER BY ca.created_at`

func scanSummary(row rowScanner) (map[string]any, error) {
	var id, name, phone, boundAt string
	var workouts int64
	var lastWorkout, lastSync *string
	var planDays int
	if err := row.Scan(&id, &name, &phone, &workouts, &lastWorkout, &lastSync, &planDays, &boundAt); err != nil {
		return nil, err
	}
	var ls any
	if lastSync != nil {
		if f, err := strconv.ParseFloat(*lastSync, 64); err == nil {
			ls = f
		}
	}
	return map[string]any{
		"id": id, "name": name, "phoneTail": store.MaskPhone(phone),
		"workouts": workouts, "lastWorkout": lastWorkout, "lastSync": ls,
		"planDays": planDays, "boundAt": boundAt,
	}, nil
}

// Students GET /api/coach-console/students — 绑定学员列表(presence 实时补)。
func (s *Service) Students(c *gin.Context) {
	uid, _ := c.Get("coachUID")
	rows, err := s.DB.Query(c.Request.Context(), fmt.Sprintf(summarySQL, "ca.coach_id=$1"), uid.(string))
	if err != nil {
		fail(c, 500, "list")
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		su, err := scanSummary(rows)
		if err != nil {
			continue
		}
		if live, ok := presence.Live(c.Request.Context(), s.Rdb, su["id"].(string)); ok {
			su["live"] = live
		} else {
			su["live"] = nil
		}
		out = append(out, su)
	}
	c.JSON(200, gin.H{"students": out})
}

// Bind POST /api/coach-console/bindings — 按手机号绑定;未注册 404。
func (s *Service) Bind(c *gin.Context) {
	uid, _ := c.Get("coachUID")
	// body 经全局 BodyGuard 预读缓存(rawBody),ShouldBindJSON 再读是空流 → 一律走 rawBody
	var body struct {
		Phone string `json:"phone"`
	}
	if raw, ok := c.Get("rawBody"); ok {
		if bs, ok2 := raw.([]byte); ok2 {
			_ = json.Unmarshal(bs, &body)
		}
	}
	if !validPhone(strings.TrimSpace(body.Phone)) {
		fail(c, 400, "a valid 11-digit phone number is required")
		return
	}
	sid, _, err := store.BindStudentByPhone(c.Request.Context(), s.DB, uid.(string), strings.TrimSpace(body.Phone))
	switch {
	case errors.Is(err, store.ErrNotFound):
		fail(c, 404, "no account with that phone number — the student must register first")
		return
	case errors.Is(err, store.ErrConflict):
		fail(c, 400, "a coach cannot bind themselves")
		return
	case err != nil:
		log.Printf("coach bind: %v", err)
		fail(c, 500, "bind")
		return
	}
	su, err := s.studentSummary(c.Request.Context(), uid.(string), sid, "")
	if err != nil {
		fail(c, 500, "summary")
		return
	}
	c.JSON(200, su)
}

// Unbind DELETE /api/coach-console/bindings/:studentId。
func (s *Service) Unbind(c *gin.Context) {
	uid, _ := c.Get("coachUID")
	ok, err := store.UnbindStudent(c.Request.Context(), s.DB, uid.(string), c.Param("studentId"))
	if err != nil {
		fail(c, 500, "unbind")
		return
	}
	if !ok {
		fail(c, 404, "not bound to this coach")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// studentDetailData 白名单组装(Detail 与 Export 共用;未绑定 → ErrNotFound)。
func (s *Service) studentDetailData(ctx context.Context, coachID, sid string) (gin.H, error) {
	su, err := s.studentSummary(ctx, coachID, sid, "")
	if err != nil {
		return nil, err
	}
	out := gin.H{"student": su, "workouts": []any{}, "bodyweight": []any{},
		"routines": nil, "week": nil, "unit": "kg", "lastSync": su["lastSync"]}
	if d, err := store.GetStateDoc(ctx, s.DB, sid); err == nil {
		out["rev"] = d.Rev
		var raw struct {
			Routines   json.RawMessage `json:"routines"`
			Week       json.RawMessage `json:"week"`
			Workouts   json.RawMessage `json:"workouts"`
			Bodyweight json.RawMessage `json:"bodyweight"`
			Unit       string          `json:"unit"`
		}
		if json.Unmarshal(d.Doc, &raw) == nil {
			if len(raw.Routines) > 0 {
				out["routines"] = raw.Routines
			}
			if len(raw.Week) > 0 {
				out["week"] = raw.Week
			}
			if len(raw.Workouts) > 0 {
				out["workouts"] = raw.Workouts
			}
			if len(raw.Bodyweight) > 0 {
				out["bodyweight"] = raw.Bodyweight
			}
			if raw.Unit != "" {
				out["unit"] = raw.Unit
			}
		}
	}
	return out, nil
}

// StudentDetail GET /api/coach-console/students/:id — 白名单投影。
func (s *Service) StudentDetail(c *gin.Context) {
	uid, _ := c.Get("coachUID")
	out, err := s.studentDetailData(c.Request.Context(), uid.(string), c.Param("id"))
	if err != nil {
		fail(c, 404, "not bound to this coach")
		return
	}
	c.JSON(200, out)
}

// Export GET /api/coach-console/students/:id/export — 白名单导出(附件)。
func (s *Service) Export(c *gin.Context) {
	uid, _ := c.Get("coachUID")
	out, err := s.studentDetailData(c.Request.Context(), uid.(string), c.Param("id"))
	if err != nil {
		fail(c, 404, "not bound to this coach")
		return
	}
	out["exported_at"] = time.Now().UnixMilli()
	c.Writer.Header().Set("Content-Disposition",
		"attachment; filename=student-"+c.Param("id")+".json")
	c.JSON(200, out)
}

// PutPlan PUT /api/coach-console/students/:id/plan — 下发计划(routines+week),
// rev 乐观并发(409 带当前文档),可选 Web Push 通知学员。
func (s *Service) PutPlan(c *gin.Context) {
	uid, _ := c.Get("coachUID")
	sid := c.Param("id")
	if bound, _ := store.IsStudentOf(c.Request.Context(), s.DB, uid.(string), sid); !bound {
		fail(c, 404, "not bound to this coach")
		return
	}
	var body struct {
		Routines json.RawMessage `json:"routines"`
		Week     json.RawMessage `json:"week"`
		Expect   *int64          `json:"expect"`
		Notify   *bool           `json:"notify"`
	}
	// 同 Bind:body 走 rawBody 缓存,不用 ShouldBindJSON
	if raw, ok := c.Get("rawBody"); ok {
		if bs, ok2 := raw.([]byte); ok2 {
			_ = json.Unmarshal(bs, &body)
		}
	}
	if len(body.Routines) == 0 || string(body.Routines) == "null" || len(body.Week) == 0 || string(body.Week) == "null" {
		fail(c, 400, "routines and week required")
		return
	}
	d, err := store.GetStateDoc(c.Request.Context(), s.DB, sid)
	if errors.Is(err, store.ErrNotFound) {
		// 学员从未同步过:无文档可写——让学员先在 APP 打开一次完成首建
		fail(c, 409, "the student has no synced state yet; ask them to open the app once")
		return
	}
	if err != nil {
		fail(c, 500, "read")
		return
	}
	var m map[string]any
	if err := json.Unmarshal(d.Doc, &m); err != nil {
		fail(c, 500, "parse")
		return
	}
	m["routines"] = json.RawMessage(body.Routines)
	m["week"] = json.RawMessage(body.Week)
	newDoc, _ := json.Marshal(m)
	expect := d.Rev // 不传 expect = 以读取到的当前 rev 为基(读改写窗口极短)
	if body.Expect != nil {
		expect = *body.Expect
	}
	if err := store.PutStateDoc(c.Request.Context(), s.DB, sid, newDoc, expect); err != nil {
		if errors.Is(err, store.ErrConflict) {
			c.AbortWithStatusJSON(409, gin.H{"error": "the student synced meanwhile — rebase and resend",
				"code": "conflict", "doc": json.RawMessage(d.Doc)})
			return
		}
		log.Printf("coach put plan: %v", err)
		fail(c, 500, "write")
		return
	}
	if body.Notify == nil || *body.Notify {
		payload, _ := json.Marshal(map[string]any{
			"title": "沐禾健身", "body": "教练更新了你的训练计划", "tag": "coach-plan", "urgency": "high"})
		go s.Push.Notify(context.Background(), sid, payload)
	}
	c.JSON(200, gin.H{"ok": true, "rev": d.Rev + 1})
}

// studentSummary 单学员摘要(IsStudentOf 校验后调用)。
func (s *Service) studentSummary(ctx context.Context, coachID, sid, name string) (map[string]any, error) {
	rows, err := s.DB.Query(ctx, fmt.Sprintf(summarySQL, "ca.coach_id=$2 AND ca.student_id=$1"), sid, coachID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, store.ErrNotFound
	}
	su, err := scanSummary(rows)
	if err != nil {
		return nil, err
	}
	if name != "" {
		su["name"] = name
	}
	if live, ok := presence.Live(ctx, s.Rdb, sid); ok {
		su["live"] = live
	} else {
		su["live"] = nil
	}
	return su, nil
}
