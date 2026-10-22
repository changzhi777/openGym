// Package admin — 管理后台后端(P5):角色四件套 + 审计 + 配额 + step-up。
// 与前端 views/admin/(P0-P2 已就位)的契约对齐;.zcf/plan 方案 §Admin 落库细节。
package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/changzhi777/openGym/backend-go/internal/auth"
)

type Service struct {
	DB       *pgxpool.Pool
	Sessions *auth.Sessions
	WA       *auth.WebAuthn
	mu       sync.Mutex
	stepUps  map[string]time.Time // uid → 窗口截止(5 分钟;内存,重启失效=重新验证)
}

func New(db *pgxpool.Pool, ss *auth.Sessions, wa *auth.WebAuthn) *Service {
	return &Service{DB: db, Sessions: ss, WA: wa, stepUps: map[string]time.Time{}}
}

const (
	RoleSupport    = "support"
	RoleOps        = "ops"
	RoleSuperAdmin = "super_admin"
)

// roleRank 越权比较用。
func roleRank(r string) int {
	switch r {
	case RoleSuperAdmin:
		return 3
	case RoleOps:
		return 2
	case RoleSupport:
		return 1
	}
	return 0
}

// RoleOf 该用户当前最高活跃角色(无则空)。
func (s *Service) RoleOf(ctx context.Context, uid string) string {
	rows, err := s.DB.Query(ctx,
		`SELECT role FROM admin_roles WHERE user_id=$1 AND revoked_at IS NULL`, uid)
	if err != nil {
		return ""
	}
	defer rows.Close()
	best := ""
	for rows.Next() {
		var r string
		rows.Scan(&r)
		if roleRank(r) > roleRank(best) {
			best = r
		}
	}
	return best
}

// RequireRole 中间件:低于 min 一律 403(前端也据此渲染 403 面板)。
func (s *Service) RequireRole(min string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := auth.UserOf(c, s.DB, s.Sessions)
		if !ok {
			c.AbortWithStatusJSON(401, gin.H{"error": "not signed in"})
			return
		}
		role := s.RoleOf(c.Request.Context(), u.ID)
		if roleRank(role) < roleRank(min) {
			c.AbortWithStatusJSON(403, gin.H{"error": "higher role required", "code": "role"})
			return
		}
		c.Set("adminUID", u.ID)
		c.Set("adminRole", role)
		c.Next()
	}
}

// Audit 写审计(append-only)。失败即 abort —— fail-closed:宁可不操作,不可无审计。
func (s *Service) Audit(c *gin.Context, action, targetType, targetID string, detail map[string]any) bool {
	uid, _ := c.Get("adminUID")
	raw, _ := json.Marshal(detail)
	_, err := s.DB.Exec(c.Request.Context(),
		`INSERT INTO admin_audit_log (admin_user_id, action, target_type, target_id, detail, ip, user_agent)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		uid, action, nilIfEmpty(targetType), nilIfEmpty(targetID), raw,
		nilIfEmpty(c.ClientIP()), nilIfEmpty(c.GetHeader("User-Agent")))
	if err != nil {
		c.AbortWithStatusJSON(500, gin.H{"error": "audit write failed; nothing was done"})
		return false
	}
	return true
}

// StepUpValid 5 分钟窗口是否有效。
func (s *Service) StepUpValid(uid string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.stepUps[uid]
	return ok && time.Now().Before(until)
}

// GrantStepUp 验证通过后开窗。
func (s *Service) GrantStepUp(uid string) {
	s.mu.Lock()
	s.stepUps[uid] = time.Now().Add(5 * time.Minute)
	s.mu.Unlock()
}

// RequireStepUp 敏感操作闸门:窗口失效 → 403 code=passkey-required(前端 stepUp.js 拦截重放)。
func (s *Service) RequireStepUp() gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get("adminUID")
		if s.StepUpValid(uid.(string)) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(403, gin.H{"error": "confirm with your passkey first", "code": "passkey-required"})
	}
}

func nilIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

var _ = http.StatusOK
