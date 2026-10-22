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
		Created   string `json:"created"`
		Disabled  bool   `json:"disabled"`
		InvitedBy string `json:"invitedBy"`
		SV        int64  `json:"sv"`
	} `json:"users"`
	Creds []struct {
		ID        string `json:"id"`
		UserID    string `json:"userId"`
		PublicKey string `json:"publicKey"` // base64url
		Counter   uint32 `json:"counter"`
		Created   string `json:"created"`
	} `json:"creds"`
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
		_, err := db.Exec(ctx, `INSERT INTO users (id, name, disabled, sv, created_at)
			VALUES ($1,$2,COALESCE($3,false),$4,COALESCE($5,now()))
			ON CONFLICT (id) DO UPDATE SET name=$2, disabled=$3, sv=$4`,
			u.ID, u.Name, u.Disabled, u.SV, nilIfZero(u.Created))
		die(err)
	}
	for _, c := range d.Creds {
		pk, _ := base64.RawURLEncoding.DecodeString(c.PublicKey)
		_, err := db.Exec(ctx, `INSERT INTO passkeys (id, user_id, public_key, counter, created_at)
			VALUES ($1,$2,$3,$4,COALESCE($5,now())) ON CONFLICT (id) DO NOTHING`,
			c.ID, c.UserID, pk, c.Counter, nilIfZero(c.Created))
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
	rows, err := db.Query(ctx, `SELECT id, name, to_char(created_at,'YYYY-MM-\"T\"HH24:MI:SS\"Z\"'), disabled FROM users`)
	die(err)
	type uRow struct {
		ID       string  `json:"id"`
		Name     string  `json:"name"`
		Created  *string `json:"created"`
		Disabled bool    `json:"disabled"`
	}
	var users []uRow
	for rows.Next() {
		var r uRow
		var cr *string
		die(rows.Scan(&r.ID, &r.Name, &cr, &r.Disabled))
		r.Created = cr
		users = append(users, r)
	}
	rows.Close()
	creds, err := db.Query(ctx, `SELECT id, user_id, public_key, counter, to_char(created_at,'YYYY-MM-\"T\"HH24:MI:SS\"Z\"') FROM passkeys ORDER BY created_at`)
	die(err)
	type cRow struct {
		ID        string `json:"id"`
		UserID    string `json:"userId"`
		PublicKey string `json:"publicKey"`
		Counter   uint32 `json:"counter"`
		Created   *string `json:"created"`
	}
	var cs []cRow
	for creds.Next() {
		var r cRow
		var pk []byte
		die(creds.Scan(&r.ID, &r.UserID, &pk, &r.Counter, &r.Created))
		r.PublicKey = base64.RawURLEncoding.EncodeToString(pk) // 与 Node 存储同编码
		cs = append(cs, r)
	}
	creds.Close()
	out := map[string]any{"users": users, "creds": cs, "subs": []any{}, "invites": []any{}}
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
