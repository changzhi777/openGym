// 身份凭证存取(契约 authSmsVerify/authWechat*/authBindPhone;身份表模型)。
// kind: phone(11 位大陆手机号)| wx(微信 openid,小程序域内唯一)。
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	IdPhone = "phone"
	IdWx    = "wx"
)

type IdentityRow struct {
	Kind       string `json:"kind"`
	Identifier string `json:"identifier"`
	Created    string `json:"created"`
}

// UserByIdentity 身份 → 账号;无绑定 ok=false。
func UserByIdentity(ctx context.Context, db *pgxpool.Pool, kind, identifier string) (uid string, ok bool, err error) {
	err = db.QueryRow(ctx,
		`SELECT user_id FROM auth_identities WHERE kind=$1 AND identifier=$2`, kind, identifier).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return uid, err == nil, err
}

// CreateIdentity 挂新身份。(kind,identifier) 唯一约束兜底;已存在返回 ErrConflict。
func CreateIdentity(ctx context.Context, db *pgxpool.Pool, uid, kind, identifier string) error {
	_, err := db.Exec(ctx,
		`INSERT INTO auth_identities (user_id, kind, identifier) VALUES ($1,$2,$3)
		 ON CONFLICT (kind, identifier) DO NOTHING`, uid, kind, identifier)
	if err != nil {
		return err
	}
	var owner string
	err = db.QueryRow(ctx,
		`SELECT user_id FROM auth_identities WHERE kind=$1 AND identifier=$2`, kind, identifier).Scan(&owner)
	if err != nil {
		return err
	}
	if owner != uid {
		return ErrConflict
	}
	return nil
}

// IdentitiesOf 账号的全部身份(created 升序)。
func IdentitiesOf(ctx context.Context, db *pgxpool.Pool, uid string) ([]IdentityRow, error) {
	rows, err := db.Query(ctx,
		`SELECT kind, identifier, to_char(created_at,'YYYY-MM-"T"HH24:MI:SS"Z"')
		 FROM auth_identities WHERE user_id=$1 ORDER BY created_at`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IdentityRow{}
	for rows.Next() {
		var r IdentityRow
		if err := rows.Scan(&r.Kind, &r.Identifier, &r.Created); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

var _ = time.Now
