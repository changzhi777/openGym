// team.go — 团队关系数据面(设计 §13.2;orgs 不复用,关系不进 stamps)。
// 展示一律 users.name(测试版无 display_name 列);教练不写学员档——分发走 plan_assignments 快照中转。
package team

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrTeamFull   = errors.New("team member limit reached") // W8 软上限 → 409
	ErrNoLiveTeam = errors.New("no live team")
	ErrNotMember  = errors.New("target is not a live member")
	ErrStaleOwner = errors.New("caller is no longer the owner") // 转让竞态:前置状态漂移,事务回滚
)

const memberCap = 50 // W8 成员软上限

type TeamRow struct {
	ID   string
	Name string
}

// CreateTeam 建团 + 团长入团(单事务;W1 教练=团长单角色)。
func CreateTeam(ctx context.Context, db *pgxpool.Pool, ownerUID, name string) (string, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var id string
	if err := tx.QueryRow(ctx,
		`INSERT INTO teams (owner_uid, name) VALUES ($1,$2) RETURNING id`, ownerUID, name).Scan(&id); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO team_members (team_id, uid, role) VALUES ($1,$2,'owner')`, id, ownerUID); err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

// TeamOfOwner 教练的活团(W7 一教练一活团由部分唯一索引兜底)。
func TeamOfOwner(ctx context.Context, db *pgxpool.Pool, ownerUID string) (TeamRow, error) {
	var t TeamRow
	err := db.QueryRow(ctx,
		`SELECT id, name FROM teams WHERE owner_uid=$1 AND archived_at IS NULL`, ownerUID).
		Scan(&t.ID, &t.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNoLiveTeam
	}
	return t, err
}

// TeamOfMember 学员视角「我的教练」。
func TeamOfMember(ctx context.Context, db *pgxpool.Pool, uid string) (TeamRow, error) {
	var t TeamRow
	err := db.QueryRow(ctx,
		`SELECT t.id, t.name FROM teams t
		 JOIN team_members m ON m.team_id = t.id
		 WHERE m.uid=$1 AND m.ended_at IS NULL AND t.archived_at IS NULL`, uid).
		Scan(&t.ID, &t.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNoLiveTeam
	}
	return t, err
}

// AddMember 入团;W8 软上限 50(活成员计数,超限 409 由端点层翻译)。
// 软删后返团:ON CONFLICT 复活原行(ended_at 清空,历史连续)。
func AddMember(ctx context.Context, db *pgxpool.Pool, teamID, uid string) error {
	var n int
	if err := db.QueryRow(ctx,
		`SELECT count(*) FROM team_members WHERE team_id=$1 AND ended_at IS NULL`, teamID).Scan(&n); err != nil {
		return err
	}
	if n >= memberCap {
		return ErrTeamFull
	}
	_, err := db.Exec(ctx,
		`INSERT INTO team_members (team_id, uid, role) VALUES ($1,$2,'member')
		 ON CONFLICT (team_id, uid) DO UPDATE SET role='member', ended_at=NULL, joined_at=now()`, teamID, uid)
	return err
}

// IsLiveMember 该 uid 是否此团活成员(join 防重入)。
func IsLiveMember(ctx context.Context, db *pgxpool.Pool, teamID, uid string) bool {
	var n int
	return db.QueryRow(ctx,
		`SELECT count(*) FROM team_members WHERE team_id=$1 AND uid=$2 AND ended_at IS NULL`, teamID, uid).Scan(&n) == nil && n > 0
}

// EndMembership 软删(W4);无活成员行 → ErrNotMember。
func EndMembership(ctx context.Context, db *pgxpool.Pool, teamID, uid string) error {
	ct, err := db.Exec(ctx,
		`UPDATE team_members SET ended_at=now() WHERE team_id=$1 AND uid=$2 AND ended_at IS NULL`, teamID, uid)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotMember
	}
	return nil
}

// MemberRow 成员列表条目;DisplayName 恒为昵称(display_name 空 → 端点层回退掩码)。
type MemberRow struct {
	UID         string         `json:"uid"`
	Role        string         `json:"role"`
	DisplayName string         `json:"displayName"`
	Scope       map[string]any `json:"scope,omitempty"`
	JoinedAt    string         `json:"joinedAt,omitempty"`
}

// Members 活成员列表(含 scope,教练端按 W3 两档判断可见面)。
func Members(ctx context.Context, db *pgxpool.Pool, teamID string) ([]MemberRow, error) {
	rows, err := db.Query(ctx,
		`SELECT m.uid, m.role, m.scope, COALESCE(u.name,''), m.joined_at
		 FROM team_members m JOIN users u ON u.id = m.uid
		 WHERE m.team_id=$1 AND m.ended_at IS NULL ORDER BY m.joined_at`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemberRow
	for rows.Next() {
		var r MemberRow
		var scope []byte
		var joined time.Time
		if err := rows.Scan(&r.UID, &r.Role, &scope, &r.DisplayName, &joined); err != nil {
			return nil, err
		}
		if len(scope) > 0 {
			json.Unmarshal(scope, &r.Scope)
		}
		r.JoinedAt = joined.UTC().Format(time.RFC3339)
		out = append(out, r)
	}
	return out, rows.Err()
}

// RenameTeam 团名修改(owner)。
func RenameTeam(ctx context.Context, db *pgxpool.Pool, teamID, name string) error {
	_, err := db.Exec(ctx, `UPDATE teams SET name=$2 WHERE id=$1`, teamID, name)
	return err
}

// TeamWithOwner 团名 + 团长展示名(join 确认卡;团长昵称脱敏 §13.3)。
func TeamWithOwner(ctx context.Context, db *pgxpool.Pool, teamID string) (name, ownerDN string, err error) {
	err = db.QueryRow(ctx,
		`SELECT t.name, COALESCE(u.name,'') FROM teams t
		 JOIN users u ON u.id = t.owner_uid
		 WHERE t.id=$1 AND t.archived_at IS NULL`, teamID).Scan(&name, &ownerDN)
	return name, ownerDN, err
}

// UserDisplayName 单人展示名(users 表 display_name;缺档空串 → 端点层回退掩码)。
func UserDisplayName(ctx context.Context, db *pgxpool.Pool, uid string) string {
	var dn string
	db.QueryRow(ctx, `SELECT COALESCE(name,'') FROM users WHERE id=$1`, uid).Scan(&dn)
	return dn
}

// TransferTeam 团长转让(§13.10 原子事务三 UPDATE):原团长降 member / 目标升 owner /
// teams.owner_uid 更新;转让后原团长留团为 member(W12)。前置:目标是活 member 且非自己。
func TransferTeam(ctx context.Context, db *pgxpool.Pool, teamID, fromUID, toUID string) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var one int
	err = tx.QueryRow(ctx,
		`SELECT 1 FROM team_members WHERE team_id=$1 AND uid=$2 AND role='member' AND ended_at IS NULL`,
		teamID, toUID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotMember
	}
	if err != nil {
		return err
	}
	// 行数校验(真库批 G2 抓出):UPDATE1/UPDATE3 静默 0 行 = 前置状态已漂移
	// (旧 owner 已降级/owner_uid 已易主)——不回滚会留下双 owner 行(role='owner'
	// 无唯一约束兜底)。任一步 0 行即整体回滚。
	for _, up := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE team_members SET role='member' WHERE team_id=$1 AND uid=$2 AND role='owner'`, []any{teamID, fromUID}},
		{`UPDATE team_members SET role='owner' WHERE team_id=$1 AND uid=$2`, []any{teamID, toUID}},
		{`UPDATE teams SET owner_uid=$3 WHERE id=$1 AND owner_uid=$2`, []any{teamID, fromUID, toUID}},
	} {
		ct, err := tx.Exec(ctx, up.sql, up.args...)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return ErrStaleOwner
		}
	}
	return tx.Commit(ctx)
}
