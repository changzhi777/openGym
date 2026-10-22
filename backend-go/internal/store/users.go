// users/passkeys 数据访问(P2 手写 SQL;P3 引入 sqlc 后逐步替换)。
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type User struct {
	ID       string
	Name     string
	Disabled bool
	SV       int64
}

type KeyRow struct {
	ID        string // credential id(base64url)
	PublicKey []byte
	Counter   uint32
}

func InsertUser(ctx context.Context, db *pgxpool.Pool, id, name string) error {
	_, err := db.Exec(ctx, `INSERT INTO users (id, name) VALUES ($1, $2)`, id, name)
	return err
}

func UserByID(ctx context.Context, db *pgxpool.Pool, id string) (User, error) {
	var u User
	err := db.QueryRow(ctx, `SELECT id, name, disabled, sv FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Name, &u.Disabled, &u.SV)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// KeyByID 全局找一把 passkey(登录时由 credential id 定位用户;与 Node db.creds.find 同义)。
func KeyByID(ctx context.Context, db *pgxpool.Pool, credID string) (KeyRow, string, error) {
	var k KeyRow
	var uid string
	err := db.QueryRow(ctx, `SELECT id, public_key, counter, user_id FROM passkeys WHERE id = $1`, credID).
		Scan(&k.ID, &k.PublicKey, &k.Counter, &uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return k, "", ErrNotFound
	}
	return k, uid, err
}

func KeysOf(ctx context.Context, db *pgxpool.Pool, uid string) ([]KeyRow, error) {
	rows, err := db.Query(ctx, `SELECT id, public_key, counter FROM passkeys WHERE user_id = $1`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KeyRow
	for rows.Next() {
		var k KeyRow
		if err := rows.Scan(&k.ID, &k.PublicKey, &k.Counter); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func InsertKey(ctx context.Context, db *pgxpool.Pool, uid string, k KeyRow) error {
	_, err := db.Exec(ctx,
		`INSERT INTO passkeys (id, user_id, public_key, counter) VALUES ($1, $2, $3, $4)`,
		k.ID, uid, k.PublicKey, k.Counter)
	return err
}

func UpdateKeyCounter(ctx context.Context, db *pgxpool.Pool, credID string, counter uint32) error {
	_, err := db.Exec(ctx, `UPDATE passkeys SET counter = $2 WHERE id = $1`, credID, counter)
	return err
}

// PasskeyRow 列表条目(GET /api/account/passkeys 的形状)。
type PasskeyRow struct {
	ID        string  `json:"id"`
	Name      *string `json:"name"`
	CreatedAt string  `json:"createdAt"`
	LastUsed  *string `json:"lastUsed"`
}

func ListPasskeys(ctx context.Context, db *pgxpool.Pool, uid string) ([]PasskeyRow, error) {
	rows, err := db.Query(ctx, `SELECT id, name, created_at, last_used FROM passkeys WHERE user_id = $1 ORDER BY created_at`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PasskeyRow{}
	for rows.Next() {
		var r PasskeyRow
		var lu *time.Time
		if err := rows.Scan(&r.ID, &r.Name, &r.CreatedAt, &lu); err != nil {
			return nil, err
		}
		if lu != nil {
			s := lu.Format(time.RFC3339); r.LastUsed = &s
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func DeletePasskey(ctx context.Context, db *pgxpool.Pool, uid, credID string) error {
	ct, err := db.Exec(ctx, `DELETE FROM passkeys WHERE id = $1 AND user_id = $2`, credID, uid)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func CountPasskeys(ctx context.Context, db *pgxpool.Pool, uid string) (int, error) {
	var n int
	err := db.QueryRow(ctx, `SELECT count(*) FROM passkeys WHERE user_id = $1`, uid).Scan(&n)
	return n, err
}
