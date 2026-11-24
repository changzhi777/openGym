// 教练-学员绑定存取(契约 coach-console;绑定模式:教练只看绑定学员)。
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HasRole 账号是否持有某活跃角色(coach/super_admin/...)。
func HasRole(ctx context.Context, db *pgxpool.Pool, uid, role string) bool {
	var one int
	db.QueryRow(ctx,
		`SELECT 1 FROM admin_roles WHERE user_id=$1 AND role::text=$2 AND revoked_at IS NULL`,
		uid, role).Scan(&one)
	return one == 1
}

// HasAnyRole 账号是否持有任一活跃角色。
func HasAnyRole(ctx context.Context, db *pgxpool.Pool, uid string, roles ...string) bool {
	var one int
	db.QueryRow(ctx,
		`SELECT 1 FROM admin_roles WHERE user_id=$1 AND role::text = ANY($2) AND revoked_at IS NULL`,
		uid, roles).Scan(&one)
	return one == 1
}

// RolesOf 账号的全部活跃角色名(/api/me 的 roles 字段;含 admin 三角色与 coach)。
func RolesOf(ctx context.Context, db *pgxpool.Pool, uid string) ([]string, error) {
	rows, err := db.Query(ctx,
		`SELECT role::text FROM admin_roles WHERE user_id=$1 AND revoked_at IS NULL ORDER BY role::text`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// BindStudentByPhone 按手机号身份绑定学员;幂等(重复绑定返回现有绑定)。
// 未注册手机号 → ErrNotFound(契约:学生须先注册)。
func BindStudentByPhone(ctx context.Context, db *pgxpool.Pool, coachID, phone string) (studentID, name string, err error) {
	uid, ok, err := UserByIdentity(ctx, db, IdPhone, phone)
	if err != nil || !ok {
		return "", "", ErrNotFound
	}
	if uid == coachID {
		return "", "", ErrConflict // 自己不能绑定自己
	}
	if _, err = db.Exec(ctx,
		`INSERT INTO coach_assignments (coach_id, student_id) VALUES ($1,$2)
		 ON CONFLICT (coach_id, student_id) DO NOTHING`, coachID, uid); err != nil {
		return "", "", err
	}
	err = db.QueryRow(ctx, `SELECT id, name FROM users WHERE id=$1`, uid).Scan(&studentID, &name)
	return studentID, name, err
}

// UnbindStudent 解绑(仅解自己的绑定,他人绑定 404 语义)。
func UnbindStudent(ctx context.Context, db *pgxpool.Pool, coachID, studentID string) (bool, error) {
	ct, err := db.Exec(ctx,
		`DELETE FROM coach_assignments WHERE coach_id=$1 AND student_id=$2`, coachID, studentID)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// StudentIDsOf 教练的全部绑定学员 id。
func StudentIDsOf(ctx context.Context, db *pgxpool.Pool, coachID string) ([]string, error) {
	rows, err := db.Query(ctx,
		`SELECT student_id FROM coach_assignments WHERE coach_id=$1 ORDER BY created_at`, coachID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// IsStudentOf student 是否绑定于该教练。
func IsStudentOf(ctx context.Context, db *pgxpool.Pool, coachID, studentID string) (bool, error) {
	var one int
	err := db.QueryRow(ctx,
		`SELECT 1 FROM coach_assignments WHERE coach_id=$1 AND student_id=$2`, coachID, studentID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return one == 1, err
}

var _ = time.Now
