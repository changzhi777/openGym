// store/audit.go — 认证事件审计(auth.*;与 admin fail-closed 相反,best-effort:
// 审计写失败只 log,不阻断登录/发码)。
package store

import (
	"context"
	"encoding/json"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditAuth 写一条认证审计。actor:已登录用 uid,未登录用脱敏手机号(138****8884)。
// ip/ua 由 handler 从 gin 上下文取好传入。
func AuditAuth(ctx context.Context, db *pgxpool.Pool, actor, action string, detail map[string]any, ip, ua string) {
	raw, err := json.Marshal(detail)
	if err != nil {
		raw = []byte(`{}`)
	}
	if _, err := db.Exec(ctx,
		`INSERT INTO admin_audit_log (admin_user_id, action, detail, ip, user_agent)
		 VALUES ($1,$2,$3,$4,$5)`,
		actor, action, raw, nilIfEmpty(ip), nilIfEmpty(ua)); err != nil {
		log.Printf("audit %s: %v", action, err)
	}
}

// MaskPhone 手机号脱敏:138****8884(短输入原样返回)。
func MaskPhone(p string) string {
	if len(p) < 8 {
		return p
	}
	return p[:3] + "****" + p[len(p)-4:]
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
