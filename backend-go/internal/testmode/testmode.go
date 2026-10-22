// Package testmode — Node 测试套件双跑桥(Go 侧,P3.5)。
//
// MUHE_TEST=1 + DATA_DIR=<dir> 时:
//  1. 以 PG 管理连接为每个测试实例建独立库 muhe_t_<rand>(隔离并行 spawn;同时清理
//     一小时前的残留库 —— 测试用 SIGKILL 退出,无法自行 DROP);
//  2. 迁移 + 导入 DATA_DIR 的 db.json/state-*.json(与 muhemigrate 同规则);
//  3. secret 文件作为会话 HMAC 密钥(与 Node 测试同密钥体系);
//  4. PORT=0 随机端口,boot line 打 Node 格式 "gym-api on :<N>"(test/helpers.mjs
//     的 boundPort 正则直接命中)。
package testmode

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
)

// Setup 返回 (该实例的 DATABASE_URL, secret)。dataDir 由测试写入。
func Setup(adminURL, dataDir string) (string, []byte, string) {
	ctx := context.Background()
	// 清理一小时前的残留测试库(测试进程被 SIGKILL,不能自清)
	cleanup(ctx, adminURL)
	name := fmt.Sprintf("muhe_t_%d", rand.Int63())
	conn, err := pgx.Connect(ctx, adminURL)
	fatal(err)
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, `CREATE DATABASE ` + name)
	fatal(err)
	// schema:复用 migrations(internal/store 的 embed 由 api 进程启动迁移,这里只建库)
	secret, _ := os.ReadFile(filepath.Join(dataDir, "secret"))
	return adminURLTo(adminURL, name), secret, dataDir
}

func cleanup(ctx context.Context, adminURL string) {
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return // 清理失败不阻塞
	}
	defer conn.Close(ctx)
	rows, _ := conn.Query(ctx, `SELECT datname FROM pg_database
		WHERE datname LIKE 'muhe_t_%' AND pg_postmaster_start_time() < now()`)
	var names []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		names = append(names, n)
	}
	rows.Close()
	for _, n := range names {
		conn.Exec(ctx, `DROP DATABASE IF EXISTS ` + n + ` WITH (FORCE)`)
	}
}

func adminURLTo(adminURL, db string) string {
	// postgres://user:pass@host:port/admin → .../db
	for i := len(adminURL) - 1; i >= 0; i-- {
		if adminURL[i] == '/' {
			return adminURL[:i+1] + db + "?sslmode=disable"
		}
	}
	return adminURL
}

// FreePort 随机可用端口(PORT=0 的显式等价,便于打印 Node 风格 boot line)。
func FreePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	fatal(err)
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}

// BootLine Node test/helpers.mjs boundPort 的正则格式。
func BootLine(port int) string { return fmt.Sprintf("gym-api on :%d", port) }

func fatal(err error) {
	if err != nil {
		log.Fatal("testmode: ", err)
	}
}

// Seed 把 DATA_DIR 的 db.json/state-*.json 灌进实例库(与 muhemigrate 同规则;
// 迁移已由 api 进程的启动迁移完成,这里只搬数据)。
func Seed(dbURL, dataDir string) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	fatal(err)
	defer conn.Close(ctx)
	raw, err := os.ReadFile(filepath.Join(dataDir, "db.json"))
	if err == nil {
		var d struct {
			Users []struct {
				ID, Name string
				Created  string `json:"created"`
				Disabled bool   `json:"disabled"`
				SV       int64  `json:"sv"`
			} `json:"users"`
			Creds []struct {
				ID, UserID, PublicKey string `json:"publicKey"`
				Counter               uint32
			} `json:"creds"`
		}
		if json.Unmarshal(raw, &d) == nil {
			for _, u := range d.Users {
				_, e := conn.Exec(ctx, `INSERT INTO users (id,name,disabled,sv) VALUES ($1,$2,$3,$4)
					ON CONFLICT (id) DO UPDATE SET name=$2,disabled=$3,sv=$4`, u.ID, u.Name, u.Disabled, u.SV)
				fatal(e)
			}
			for _, c := range d.Creds {
				pk, _ := base64.RawURLEncoding.DecodeString(c.PublicKey)
				_, e := conn.Exec(ctx, `INSERT INTO passkeys (id,user_id,public_key,counter)
					VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING`, c.ID, c.UserID, pk, c.Counter)
				fatal(e)
			}
		}
	}
	files, _ := filepath.Glob(filepath.Join(dataDir, "state-*.json"))
	for _, f := range files {
		uid := filepath.Base(f)
		uid = uid[len("state-") : len(uid)-len(".json")]
		doc, e := os.ReadFile(f)
		fatal(e)
		var m map[string]any
		if json.Unmarshal(doc, &m) != nil {
			continue
		}
		rev := int64(0)
		if r, ok := m["_rev"].(float64); ok {
			rev = int64(r)
		}
		_, e = conn.Exec(ctx, `INSERT INTO states (user_id,doc,rev) VALUES ($1,$2,$3)
			ON CONFLICT (user_id) DO UPDATE SET doc=$2,rev=$3`, uid, doc, rev)
		fatal(e)
	}
}
