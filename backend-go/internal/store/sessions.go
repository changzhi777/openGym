// 会话与设备管理(store 层):登录签发的会话行,支撑账号页「设备与会话」列表/踢出。
// 每次登录 INSERT 一行;UserOf 校验链查 revoked;logout-all 全撤销(sv++ 依旧,双保险)。
package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SessionRow struct {
	ID          string     `json:"id"`
	DeviceName  string     `json:"deviceName"`
	UserAgent   string     `json:"userAgent"`
	IP          string     `json:"ip"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastSeenAt  time.Time  `json:"lastSeenAt"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
	Current     bool       `json:"current,omitempty"` // 响应组装时由 handler 标记
}

// CreateSession 登录成功时落一行。id 由调用方生成(crypto/rand)。
func CreateSession(ctx context.Context, db *pgxpool.Pool, id, userID, device, ua, ip string) error {
	_, err := db.Exec(ctx,
		`INSERT INTO sessions (id, user_id, device_name, user_agent, ip) VALUES ($1,$2,$3,$4,$5)`,
		id, userID, device, ua, ip)
	return err
}

// ListSessions 某用户的未撤销会话,新者在前。
func ListSessions(ctx context.Context, db *pgxpool.Pool, userID string) ([]SessionRow, error) {
	rows, err := db.Query(ctx,
		`SELECT id, device_name, user_agent, ip, created_at, last_seen_at, revoked_at
		   FROM sessions WHERE user_id=$1 AND revoked_at IS NULL
		  ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionRow
	for rows.Next() {
		var r SessionRow
		if err := rows.Scan(&r.ID, &r.DeviceName, &r.UserAgent, &r.IP, &r.CreatedAt, &r.LastSeenAt, &r.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RevokeSession 踢出一条会话(校验属主,防越权)。返回是否真的撤销了某行。
func RevokeSession(ctx context.Context, db *pgxpool.Pool, sid, userID string) bool {
	tag, err := db.Exec(ctx,
		`UPDATE sessions SET revoked_at=now() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`,
		sid, userID)
	if err != nil {
		return false
	}
	return tag.RowsAffected() > 0
}

// RevokeAllFor logout-all:该用户全部会话撤销。
func RevokeAllFor(ctx context.Context, db *pgxpool.Pool, userID string) error {
	_, err := db.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, userID)
	return err
}

// SessionCheckAndTouch 校验链每请求调用:刷新最近活跃并返回该会话是否已被踢出。
// 单往返完成(替代分离的 SELECT+UPDATE);sid 不存在(旧会话 cookie)返回 false 不撤销。
func SessionCheckAndTouch(ctx context.Context, db *pgxpool.Pool, sid string) (revoked bool) {
	var revokedAt *time.Time
	err := db.QueryRow(ctx,
		`UPDATE sessions SET last_seen_at=now() WHERE id=$1 RETURNING revoked_at`, sid).Scan(&revokedAt)
	if err != nil {
		return false
	}
	return revokedAt != nil
}
