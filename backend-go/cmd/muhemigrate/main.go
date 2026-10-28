// cmd/muhemigrate — Node JSON ↔ PG 双向迁移器(P3b;方案 §迁移工程的 round-trip 基石)。
//
//	import: db.json(users/creds) + state-<uid>.json → PG(users/passkeys/states)
//	export: PG → db.json + state-<uid>.json(与 Node 文件同构,供比对/回滚)
// 幂等:按 id upsert/覆盖;passkey 材料全程内存。
//
// 用法:
//	muhemigrate import  <dataDir>          # DATA_URL 环境变量指 PG
//	muhemigrate export  <outDir>
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
)

type dbJSON struct {
	Users []struct {
		ID, Name  string
		Created   string  `json:"created"`
		Disabled  bool    `json:"disabled"`
		InvitedBy string  `json:"invitedBy"`
		Email     *string `json:"email"`
		Admin     bool    `json:"admin"` // Node 侧只有管理员带此键(true) → 迁到 admin_roles
		SV        int64   `json:"sv"`
	} `json:"users"`
	Creds []struct {
		ID         string   `json:"id"`
		UserID     string   `json:"userId"`
		PublicKey  string   `json:"publicKey"` // base64url
		Counter    uint32   `json:"counter"`
		Created    string   `json:"created"`
		Transports []string `json:"transports"` // Node 记录认证器类型(internal/hybrid/…);缺失=老数据
		LastUsed   string   `json:"lastUsed"`   // Node 每次使用后更新;缺失=从未使用
	} `json:"creds"`
	Invites []struct {
		Code      string  `json:"code"`
		Note      string  `json:"note"`
		CreatedBy string  `json:"createdBy"`
		Created   string  `json:"created"`
		UsedBy    *string `json:"usedBy"`
		UsedAt    *string `json:"usedAt"`
		Revoked   bool    `json:"revoked"`
	} `json:"invites"`
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "用法: muhemigrate import|export <dir>")
		os.Exit(2)
	}
	mode, dir := os.Args[1], os.Args[2]
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://muhe:muhe@localhost:5432/muhe?sslmode=disable"
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	die(err)
	defer db.Close()
	if mode == "import" {
		doImport(ctx, db, dir)
	} else {
		doExport(ctx, db, dir)
	}
}

func die(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}
}

func doImport(ctx context.Context, db *pgxpool.Pool, dir string) {
	raw, err := os.ReadFile(filepath.Join(dir, "db.json"))
	die(err)
	var d dbJSON
	die(json.Unmarshal(raw, &d))
	for _, u := range d.Users {
		_, err := db.Exec(ctx, `INSERT INTO users (id, name, disabled, sv, created_at, email, invited_by)
			VALUES ($1,$2,COALESCE($3,false),$4,COALESCE($5,now()),$6,NULLIF($7,''))
			ON CONFLICT (id) DO UPDATE SET name=$2, disabled=$3, sv=$4, email=$6, invited_by=NULLIF($7,'')`,
			u.ID, u.Name, u.Disabled, u.SV, nilIfZero(u.Created), u.Email, u.InvitedBy)
		die(err)
		if u.Admin { // Node 的 admin:true → super_admin 角色(否则迁移后后台失去权限)
			_, err = db.Exec(ctx, `INSERT INTO admin_roles (user_id, role) VALUES ($1,'super_admin')
				ON CONFLICT (user_id, role) DO UPDATE SET revoked_at=NULL`, u.ID)
			die(err)
		}
	}
	for _, c := range d.Creds {
		pk, _ := base64.RawURLEncoding.DecodeString(c.PublicKey)
		_, err := db.Exec(ctx, `INSERT INTO passkeys (id, user_id, public_key, counter, transports, created_at, last_used)
			VALUES ($1,$2,$3,$4,$5,COALESCE($6,now()),COALESCE($7,$6,now())) ON CONFLICT (id) DO NOTHING`,
			c.ID, c.UserID, pk, c.Counter, c.Transports, nilIfZero(c.Created), nilIfZero(c.LastUsed))
		die(err)
	}
	// 邀请码:此前 export 硬编码空数组会丢码 —— round-trip 修复点(2026-10-28)。
	for _, i := range d.Invites {
		_, err := db.Exec(ctx, `INSERT INTO admin_invites (code, note, created_by, created_at, used_by, used_at, revoked)
			VALUES ($1,$2,$3,COALESCE($4,now()),$5,$6,$7) ON CONFLICT (code) DO NOTHING`,
			i.Code, i.Note, i.CreatedBy, nilIfZero(i.Created), i.UsedBy, i.UsedAt, i.Revoked)
		die(err)
	}
	// state-<uid>.json:doc 原样入库;表列 rev 从文档 _rev 投影。
	files, _ := filepath.Glob(filepath.Join(dir, "state-*.json"))
	for _, f := range files {
		uid := trimExt(filepath.Base(f), "state-", ".json")
		doc, err := os.ReadFile(f)
		die(err)
		var m map[string]any
		die(json.Unmarshal(doc, &m))
		rev := i64of(m["_rev"])
		_, err = db.Exec(ctx, `INSERT INTO states (user_id, doc, rev) VALUES ($1,$2,$3)
			ON CONFLICT (user_id) DO UPDATE SET doc=$2, rev=$3, updated_at=now()`, uid, doc, rev)
		die(err)
	}
	fmt.Printf("✅ import 完成:%d 用户 %d 凭据 %d state 文件\n", len(d.Users), len(d.Creds), len(files))
}

func doExport(ctx context.Context, db *pgxpool.Pool, dir string) {
	die(os.MkdirAll(dir, 0o700))
	// db.json
	rows, err := db.Query(ctx, `SELECT u.id, u.name, to_char(u.created_at,'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'), u.disabled, u.sv,
		COALESCE(u.email,''), COALESCE(u.invited_by,''),
		EXISTS (SELECT 1 FROM admin_roles r WHERE r.user_id=u.id AND r.role='super_admin' AND r.revoked_at IS NULL)
		FROM users u`)
	die(err)
	type uRow struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		Created   *string `json:"created"`
		Disabled  bool    `json:"disabled"`
		SV        int64   `json:"sv,omitempty"`    // Node 只在对 0 之外写这个键
		Email     *string `json:"email,omitempty"` // 无邮箱不输出(与 Node 同形)
		InvitedBy string  `json:"invitedBy,omitempty"`
		Admin     *bool   `json:"admin,omitempty"` // 与 Node 同形:仅管理员带键
	}
	var users []uRow
	for rows.Next() {
		var r uRow
		var cr *string
		var admin bool
		die(rows.Scan(&r.ID, &r.Name, &cr, &r.Disabled, &r.SV, &r.Email, &r.InvitedBy, &admin))
		r.Created = cr
		if admin {
			r.Admin = &admin
		}
		users = append(users, r)
	}
	rows.Close()
	creds, err := db.Query(ctx, `SELECT id, user_id, public_key, counter, transports,
		to_char(created_at,'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(last_used,'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
		FROM passkeys ORDER BY created_at`)
	die(err)
	type cRow struct {
		ID         string   `json:"id"`
		UserID     string   `json:"userId"`
		PublicKey  string   `json:"publicKey"`
		Counter    uint32   `json:"counter"`
		Created    *string  `json:"created"`
		Transports []string `json:"transports,omitempty"` // 与 Node 同形:无记录则不输出
		LastUsed   *string  `json:"lastUsed,omitempty"`
	}
	var cs []cRow
	for creds.Next() {
		var r cRow
		var pk []byte
		die(creds.Scan(&r.ID, &r.UserID, &pk, &r.Counter, &r.Transports, &r.Created, &r.LastUsed))
		r.PublicKey = base64.RawURLEncoding.EncodeToString(pk) // 与 Node 存储同编码
		cs = append(cs, r)
	}
	creds.Close()
	// 邀请码(与 Node db.invites 同构;revoked/usedBy 仅在存在时输出)。
	inv, err := db.Query(ctx, `SELECT code, note, created_by, to_char(created_at,'YYYY-MM-\"T\"HH24:MI:SS\"Z\"'), used_by, to_char(used_at,'YYYY-MM-\"T\"HH24:MI:SS\"Z\"'), revoked FROM admin_invites ORDER BY created_at`)
	die(err)
	type iRow struct {
		Code      string  `json:"code"`
		Note      string  `json:"note"`
		CreatedBy string  `json:"createdBy"`
		Created   *string `json:"created"`
		UsedBy    *string `json:"usedBy,omitempty"`
		UsedAt    *string `json:"usedAt,omitempty"`
		Revoked   *bool   `json:"revoked,omitempty"`
	}
	var invites []iRow
	for inv.Next() {
		var r iRow
		var rev *bool
		die(inv.Scan(&r.Code, &r.Note, &r.CreatedBy, &r.Created, &r.UsedBy, &r.UsedAt, &rev))
		if rev != nil && *rev {
			r.Revoked = rev
		}
		invites = append(invites, r)
	}
	inv.Close()
	out := map[string]any{"users": users, "creds": cs, "subs": []any{}, "invites": invites}
	b, _ := json.MarshalIndent(out, "", "  ")
	die(os.WriteFile(filepath.Join(dir, "db.json"), b, 0o600))
	// state-*.json(doc 原样 = round-trip 的意义:逐字节可比对)
	st, err := db.Query(ctx, `SELECT user_id, doc FROM states`)
	die(err)
	n := 0
	for st.Next() {
		var uid string
		var doc []byte
		die(st.Scan(&uid, &doc))
		die(os.WriteFile(filepath.Join(dir, "state-"+uid+".json"), doc, 0o600))
		n++
	}
	st.Close()
	fmt.Printf("✅ export 完成:%d 用户 %d 凭据 %d state 文件 → %s\n", len(users), len(cs), n, dir)
}

func trimExt(s, prefix, suffix string) string {
	return s[len(prefix) : len(s)-len(suffix)]
}
func nilIfZero(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func i64of(v any) int64 {
	f, _ := v.(float64)
	return int64(f)
}
