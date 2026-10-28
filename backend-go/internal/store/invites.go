// 邀请码与实例设置(契约 adminInvites*/adminSettings*;与 Node db.invites 同语义)。
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InviteRow GET /api/admin/invites 的行形状(UsedName 由 JOIN 解析,契约 usedByName)。
type InviteRow struct {
	Code       string     `json:"code"`
	Note       string     `json:"note"`
	CreatedBy  string     `json:"createdBy"`
	Created    string     `json:"created"` // ISO(与 Node new Date().toISOString() 同形)
	UsedBy     *string    `json:"usedBy"`
	UsedAt     *string    `json:"usedAt"`
	UsedName   *string    `json:"usedByName"`
	Revoked    bool       `json:"revoked"`
}

func CreateInvite(ctx context.Context, db *pgxpool.Pool, code, note, createdBy string) error {
	_, err := db.Exec(ctx,
		`INSERT INTO admin_invites (code, note, created_by) VALUES ($1,$2,$3)
		 ON CONFLICT (code) DO NOTHING`, code, note, createdBy)
	return err
}

func ListInvites(ctx context.Context, db *pgxpool.Pool) ([]InviteRow, error) {
	rows, err := db.Query(ctx, `
		SELECT i.code, i.note, i.created_by, to_char(i.created_at,'YYYY-MM-"T"HH24:MI:SS"Z"'),
		       i.used_by, to_char(i.used_at,'YYYY-MM-"T"HH24:MI:SS"Z"'), u.name, i.revoked
		FROM admin_invites i LEFT JOIN users u ON u.id = i.used_by
		ORDER BY i.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InviteRow{}
	for rows.Next() {
		var r InviteRow
		var ua *string
		if err := rows.Scan(&r.Code, &r.Note, &r.CreatedBy, &r.Created, &r.UsedBy, &ua, &r.UsedName, &r.Revoked); err != nil {
			return nil, err
		}
		if ua != nil {
			r.UsedAt = ua
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ConsumeInvite 用后即毁式核销:只有未用未撤销的码可消费;0 行更新 = 码无效(ErrNotFound)。
func ConsumeInvite(ctx context.Context, db *pgxpool.Pool, code, uid string) error {
	ct, err := db.Exec(ctx,
		`UPDATE admin_invites SET used_by=$2, used_at=now()
		 WHERE code=$1 AND used_by IS NULL AND NOT revoked`, code, uid)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeInvite 物理删除(Node 同语义);已用码不可撤(ErrConflict),未知码 ErrNotFound。
func RevokeInvite(ctx context.Context, db *pgxpool.Pool, code string) error {
	var used *string
	err := db.QueryRow(ctx, `SELECT used_by FROM admin_invites WHERE code=$1`, code).Scan(&used)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if used != nil {
		return ErrConflict
	}
	_, err = db.Exec(ctx, `DELETE FROM admin_invites WHERE code=$1`, code)
	return err
}

// SettingsGet 读取实例开关;未设置的键由调用方用 env 兜底(此处只答库里持久化的)。
func SettingsGet(ctx context.Context, db *pgxpool.Pool, key string) (val any, ok bool, err error) {
	err = db.QueryRow(ctx, `SELECT value FROM instance_settings WHERE key=$1`, key).Scan(&val)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	return val, err == nil, err
}

// SettingsPut 持久化实例开关;keys 白名单由调用方校验。
func SettingsPut(ctx context.Context, db *pgxpool.Pool, key string, val any) error {
	_, err := db.Exec(ctx, `INSERT INTO instance_settings (key, value) VALUES ($1,$2)
		ON CONFLICT (key) DO UPDATE SET value=$2, updated_at=now()`, key, val)
	return err
}

// InviteOnlyEffective invite_only 生效值:settings 覆盖 env 默认。
func InviteOnlyEffective(ctx context.Context, db *pgxpool.Pool, envDefault bool) (bool, error) {
	v, ok, err := SettingsGet(ctx, db, "invite_only")
	if err != nil || !ok {
		return envDefault, err
	}
	b, _ := v.(bool)
	return b, nil
}

var _ = time.Now
