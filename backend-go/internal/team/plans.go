// plans.go — 训练计划分发数据面(§13.5;快照中转,教练不写学员档,W10):
// distributed→received→loaded/declined 状态机(M1 拉模型不用 received),
// 重分发 = 新行 + superseded_by 版本链;撤回仅限 distributed(已结算即历史)。
package team

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrSettled  = errors.New("assignment already settled or superseded") // 不可再改
	ErrNotFound = errors.New("no such row")                              // 与 store.ErrNotFound 同义别名面
)

// AssignmentRow 分发行(契约 PlanAssignment 同形;Plan 为原样快照)。
type AssignmentRow struct {
	ID                string         `json:"id"`
	MemberUID         string         `json:"memberUid"`
	MemberDisplayName string         `json:"memberDisplayName,omitempty"`
	PlanName          string         `json:"planName"`
	Status            string         `json:"status"`
	SupersededBy      *string        `json:"supersededBy"`
	Plan              map[string]any `json:"plan"`
	DistributedAt     string         `json:"distributedAt"`
	SettledAt         *string        `json:"settledAt"`
}

const assignmentCols = `id, member_uid, plan_name, status, superseded_by, plan_snapshot,
	to_char(distributed_at,'YYYY-MM-"T"HH24:MI:SS"Z"'),
	to_char(settled_at,'YYYY-MM-"T"HH24:MI:SS"Z"')`

func scanAssignment(row pgx.Row) (AssignmentRow, error) {
	var a AssignmentRow
	var plan []byte
	var sup, settled, dist *string // to_char 直出 ISO 串(timestamptz 二进制不能扫 *string/*time.Time——批①同款)
	err := row.Scan(&a.ID, &a.MemberUID, &a.PlanName, &a.Status, &sup, &plan, &dist, &settled)
	if err != nil {
		return a, err
	}
	a.SupersededBy = sup
	a.Plan = map[string]any{}
	if len(plan) > 0 {
		if json.Unmarshal(plan, &a.Plan) != nil {
			a.Plan = map[string]any{} // 坏快照视为空对象,行仍返回
		}
	}
	if dist != nil {
		a.DistributedAt = *dist
	}
	a.SettledAt = settled
	return a, nil
}

// CreateAssignment 出一条分发行(快照原样入 jsonb;身份+scope+确认三重闸在端点层)。
func CreateAssignment(ctx context.Context, db *pgxpool.Pool, teamID, coachUID, memberUID, planName string, planSnap []byte) (string, error) {
	var id string
	err := db.QueryRow(ctx,
		`INSERT INTO plan_assignments (team_id, coach_uid, member_uid, plan_name, plan_snapshot)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id`, teamID, coachUID, memberUID, planName, planSnap).Scan(&id)
	return id, err
}

// SupersedePending 同员同名旧 pending 行挂上版本链(重分发语义)。
func SupersedePending(ctx context.Context, db *pgxpool.Pool, teamID, memberUID, planName, newID string) {
	db.Exec(ctx,
		`UPDATE plan_assignments SET superseded_by=$4
		 WHERE team_id=$1 AND member_uid=$2 AND plan_name=$3 AND status='distributed' AND superseded_by IS NULL AND id<>$4`,
		teamID, memberUID, planName, newID)
}

// supersedePendingTx SupersedePending 的事务内形态(单库往返;同语义)。
func supersedePendingTx(ctx context.Context, tx pgx.Tx, teamID, memberUID, planName, newID string) error {
	_, err := tx.Exec(ctx,
		`UPDATE plan_assignments SET superseded_by=$4
		 WHERE team_id=$1 AND member_uid=$2 AND plan_name=$3 AND status='distributed' AND superseded_by IS NULL AND id<>$4`,
		teamID, memberUID, planName, newID)
	return err
}

// CreateAssignmentChained 出分发行 + 同事务挂版本链(审查批 1.0:原端点两步独立
// 调用,supersede 失败时同员同名旧新两行同时 distributed 且未挂链——学员端
// isPending 会看到两条待接收。原子化后要么整行成链要么整体回滚)。
func CreateAssignmentChained(ctx context.Context, db *pgxpool.Pool, teamID, coachUID, memberUID, planName string, planSnap []byte) (string, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var id string
	if err := tx.QueryRow(ctx,
		`INSERT INTO plan_assignments (team_id, coach_uid, member_uid, plan_name, plan_snapshot)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id`, teamID, coachUID, memberUID, planName, planSnap).Scan(&id); err != nil {
		return "", err
	}
	if err := supersedePendingTx(ctx, tx, teamID, memberUID, planName, id); err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

func assignmentsQuery(ctx context.Context, db *pgxpool.Pool, sql string, args ...any) ([]AssignmentRow, error) {
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AssignmentRow
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AssignmentsOfTeam 教练视角全团分发行(新→旧)。
func AssignmentsOfTeam(ctx context.Context, db *pgxpool.Pool, teamID string) ([]AssignmentRow, error) {
	return assignmentsQuery(ctx, db,
		`SELECT `+assignmentCols+` FROM plan_assignments WHERE team_id=$1 ORDER BY distributed_at DESC`, teamID)
}

// AssignmentsOfMember 成员视角自己的行(限活团;新→旧)。
func AssignmentsOfMember(ctx context.Context, db *pgxpool.Pool, uid string) ([]AssignmentRow, error) {
	return assignmentsQuery(ctx, db,
		`SELECT a.`+assignmentCols+` FROM plan_assignments a
		 JOIN teams t ON t.id = a.team_id
		 WHERE a.member_uid=$1 AND t.archived_at IS NULL ORDER BY a.distributed_at DESC`, uid)
}

// AckAssignment 成员结算(loaded/declined;仅 distributed 行,received 留 M2)。
// 0 行 = 不存在/已结算/已越链 → ErrSettled(端点层统一 404 plan-gone)。
func AckAssignment(ctx context.Context, db *pgxpool.Pool, id, memberUID, status string) error {
	ct, err := db.Exec(ctx,
		`UPDATE plan_assignments SET status=$3, settled_at=now()
		 WHERE id=$1 AND member_uid=$2 AND status='distributed'`, id, memberUID, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrSettled
	}
	return nil
}

// RevokeDistributed 教练撤回(仅 distributed;已结算不可改写)。0 行 → ErrNotFound。
func RevokeDistributed(ctx context.Context, db *pgxpool.Pool, id, teamID string) error {
	ct, err := db.Exec(ctx,
		`DELETE FROM plan_assignments WHERE id=$1 AND team_id=$2 AND status='distributed'`, id, teamID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
