// states 文档存取:doc 存整份(含 _rev/_wid/_wids/_ts 嵌套,与 Node state 文件同构),
// 表列 rev 是投影(GET /api/data/rev 免解析整文档 —— Node stat cache 的 PG 等价物)。
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrConflict = errors.New("conflict")

type StateDoc struct {
	Doc []byte
	Rev int64
}

func GetStateDoc(ctx context.Context, db *pgxpool.Pool, uid string) (StateDoc, error) {
	var d StateDoc
	err := db.QueryRow(ctx, `SELECT doc, rev FROM states WHERE user_id=$1`, uid).Scan(&d.Doc, &d.Rev)
	if errors.Is(err, pgx.ErrNoRows) {
		return StateDoc{}, ErrNotFound
	}
	return d, err
}

// PutStateDoc 条件写:
//   expect=0 且行不存在 → INSERT 首建(rev=1),成功即回;
//   expect=0 但行已存在 → ErrConflict(客户端过时);
//   expect>0 → UPDATE ... WHERE rev=expect,0 行即 ErrConflict(并发写先进)。
func PutStateDoc(ctx context.Context, db *pgxpool.Pool, uid string, doc []byte, expect int64) error {
	if expect == 0 {
		ct, err := db.Exec(ctx, `INSERT INTO states (user_id, doc, rev) VALUES ($1,$2,1)
			ON CONFLICT (user_id) DO NOTHING`, uid, doc)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 1 {
			return nil // 首建成功
		}
		return ErrConflict // 行已存在而客户端以为是空 → 409 带文档
	}
	ct, err := db.Exec(ctx, `UPDATE states SET doc=$3, rev=$4, updated_at=now() WHERE user_id=$1 AND rev=$2`,
		uid, expect, doc, expect+1)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}
