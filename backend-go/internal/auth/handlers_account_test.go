// handlers_account_test.go — 自助注销与导出的真库集成测试。
//
// 前置:TEST_PG_ADMIN_URL(独立测试库,同 store 包测法);
// TEST_REDIS_URL 可选(注销 proof 的短信码走 Redis,缺省跳过 proof 相关断言)。
package auth_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/changzhi777/openGym/backend-go/internal/auth"
	"github.com/changzhi777/openGym/backend-go/internal/sms"
	"github.com/changzhi777/openGym/backend-go/internal/store"
)

// newTestEnv 每测试独立库 + gin 引擎挂两路由。返回 (service, engine)。
func newTestEnv(t *testing.T) (*auth.Service, *gin.Engine) {
	t.Helper()
	admin := os.Getenv("TEST_PG_ADMIN_URL")
	if admin == "" {
		t.Skip("无 TEST_PG_ADMIN_URL — 起依赖: docker compose -f compose.go.yml up -d pg")
	}
	pool := newAuthTestDB(t, admin)
	t.Cleanup(pool.Close)

	svc := &auth.Service{
		DB:       pool,
		Sessions: auth.NewSessions([]byte("test-secret-32bytes-aaaaaaaaaaaaaa"), 90, false),
		DataDir:  t.TempDir(),
	}
	if ru := os.Getenv("TEST_REDIS_URL"); ru != "" {
		rdb := redis.NewClient(&redis.Options{Addr: strings.TrimPrefix(ru, "redis://")})
		if err := rdb.Ping(context.Background()).Err(); err == nil {
			svc.Rdb = rdb
			svc.SmsStore = sms.Store{Rdb: rdb}
			t.Cleanup(func() { rdb.Close() })
		}
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 生产里 BodyGuard(httpx)读 body 塞 rawBody,handler 的 readBody 从那儿取;测试补同款。
	r.Use(func(c *gin.Context) {
		if c.Request.Body != nil {
			bs, _ := io.ReadAll(c.Request.Body)
			c.Set("rawBody", bs)
		}
		c.Next()
	})
	r.POST("/api/account/delete", svc.DeleteAccount)
	r.GET("/api/account/export", svc.ExportAccount)
	return svc, r
}

// newAuthTestDB 每测试独立库(建→迁移→cleanup 删)。store 包 newTestDB 的本地拷贝
// (跨包不导出,测试自足)。库名由测试名派生,重跑先 DROP IF EXISTS。
func newAuthTestDB(t *testing.T, admin string) *pgxpool.Pool {
	t.Helper()
	i := strings.LastIndex(admin, "/")
	pgURL := admin[:i+1] + "acct_test_" + sanitize(t.Name())
	pgx5URL := strings.Replace(pgURL, "postgres://", "pgx5://", 1)
	name := pgURL[strings.LastIndex(pgURL, "/")+1:]

	adm, err := pgxpool.New(context.Background(), admin)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	defer adm.Close()
	if _, err := adm.Exec(context.Background(), `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`); err != nil {
		t.Fatalf("drop stale: %v", err)
	}
	if _, err := adm.Exec(context.Background(), `CREATE DATABASE `+name); err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { adm.Exec(context.Background(), `DROP DATABASE `+name+` WITH (FORCE)`) })

	pool, err := pgxpool.New(context.Background(), pgURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if err := store.Migrate(pgx5URL); err != nil {
		pool.Close()
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

func sanitize(name string) string {
	// 全小写:PG 把不加引号的标识符折叠为小写存储,而连接串里的库名精确匹配,
	// 大写会 "database does not exist"。
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return '_'
	}, name)
}

func seedAcct(t *testing.T, db *pgxpool.Pool, id, name string, withState bool, phone string) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Exec(ctx, `INSERT INTO users (id, name) VALUES ($1,$2) ON CONFLICT (id) DO NOTHING`, id, name); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if withState {
		if _, err := db.Exec(ctx, `INSERT INTO states (user_id, doc) VALUES ($1,'{"key":"val"}') ON CONFLICT (user_id) DO NOTHING`, id); err != nil {
			t.Fatalf("seed state: %v", err)
		}
	}
	if phone != "" {
		if err := store.CreateIdentity(ctx, db, id, store.IdPhone, phone); err != nil {
			t.Fatalf("seed identity: %v", err)
		}
	}
}

func call(r *gin.Engine, method, path, cookie, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func cookieOf(svc *auth.Service, uid string) string {
	return "gymsid=" + svc.Sessions.Token(uid, 0)
}

func TestAccountExport(t *testing.T) {
	svc, r := newTestEnv(t)
	seedAcct(t, svc.DB, "u-ex", "导出者", true, "13800138000")

	w := call(r, "GET", "/api/account/export", cookieOf(svc, "u-ex"), "")
	if w.Code != 200 {
		t.Fatalf("export: %d %s", w.Code, w.Body.String())
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, "u-ex") {
		t.Fatalf("disposition: %q", cd)
	}
	var out struct {
		Account   struct{ ID, Name string }
		Identities []struct{ Kind, Identifier string }
		State     struct {
			Doc map[string]any `json:"doc"`
			Rev int64          `json:"rev"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("json: %v", err)
	}
	if out.Account.ID != "u-ex" || out.Account.Name != "导出者" {
		t.Fatalf("account: %+v", out.Account)
	}
	if len(out.Identities) != 1 || out.Identities[0].Kind != "phone" || out.Identities[0].Identifier != "13800138000" {
		t.Fatalf("identities: %+v", out.Identities)
	}
	if out.State.Doc["key"] != "val" || out.State.Rev != 0 { // seed 直插,schema 默认 rev=0
		t.Fatalf("state: %+v", out.State)
	}

	// 无 state 的账号:空 doc 而非 500。
	seedAcct(t, svc.DB, "u-empty", "空账号", false, "")
	w = call(r, "GET", "/api/account/export", cookieOf(svc, "u-empty"), "")
	if w.Code != 200 {
		t.Fatalf("empty export: %d", w.Code)
	}

	// 未登录:401。
	if w = call(r, "GET", "/api/account/export", "", ""); w.Code != 401 {
		t.Fatalf("no cookie: %d", w.Code)
	}
}

func TestAccountDeleteFlow(t *testing.T) {
	svc, r := newTestEnv(t)
	ctx := context.Background()
	seedAcct(t, svc.DB, "u-del", "删除者", true, "")
	if err := store.CreateSession(ctx, svc.DB, "sess-1", "u-del", "浏览器", "UA", "127.0.0.1"); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	mediaFile := filepath.Join(svc.DataDir, "media", "u-del", "abc.jpg")
	if err := os.MkdirAll(filepath.Dir(mediaFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	ck := cookieOf(svc, "u-del")

	// 错名:400 confirm-name,账号仍在。
	if w := call(r, "POST", "/api/account/delete", ck, `{"name":"别人"}`); w.Code != 400 {
		t.Fatalf("wrong name: %d", w.Code)
	}
	var n int
	svc.DB.QueryRow(ctx, `SELECT count(*) FROM users WHERE id='u-del'`).Scan(&n)
	if n != 1 {
		t.Fatal("account must survive wrong-name delete")
	}

	// 正确名(无绑定手机 = 会话即证明):200 → 全链级联消失 + 媒体目录清 + cookie 双清。
	w := call(r, "POST", "/api/account/delete", ck, `{"name":"删除者"}`)
	if w.Code != 200 {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	for _, q := range []string{
		`SELECT count(*) FROM users WHERE id='u-del'`,
		`SELECT count(*) FROM states WHERE user_id='u-del'`,
		`SELECT count(*) FROM sessions WHERE user_id='u-del'`,
	} {
		svc.DB.QueryRow(ctx, q).Scan(&n)
		if n != 0 {
			t.Fatalf("cascade left rows: %s = %d", q, n)
		}
	}
	if _, err := os.Stat(mediaFile); !os.IsNotExist(err) {
		t.Fatal("media dir must be removed")
	}
	clears := 0
	for _, v := range w.Header().Values("Set-Cookie") {
		if strings.Contains(v, "Max-Age=0") {
			clears++
		}
	}
	if clears < 2 {
		t.Fatalf("want both cookies cleared, got %d: %v", clears, w.Header().Values("Set-Cookie"))
	}

	// 旧 cookie 已死:401。
	if w = call(r, "GET", "/api/account/export", ck, ""); w.Code != 401 {
		t.Fatalf("old cookie after delete: %d", w.Code)
	}
}

func TestAccountDeleteLastAdmin(t *testing.T) {
	svc, r := newTestEnv(t)
	ctx := context.Background()
	seedAcct(t, svc.DB, "u-admin", "管理员", false, "")
	seedAcct(t, svc.DB, "u-other", "别人", false, "")
	svc.DB.Exec(ctx, `INSERT INTO admin_roles (user_id, role) VALUES ('u-admin','super_admin') ON CONFLICT DO NOTHING`)

	// 唯一管理员 + 还有别人:409。
	w := call(r, "POST", "/api/account/delete", cookieOf(svc, "u-admin"), `{"name":"管理员"}`)
	if w.Code != 409 {
		t.Fatalf("last-admin: %d %s", w.Code, w.Body.String())
	}
	var n int
	svc.DB.QueryRow(ctx, `SELECT count(*) FROM users WHERE id='u-admin'`).Scan(&n)
	if n != 1 {
		t.Fatal("admin must survive")
	}

	// 别人删自己(非管理员):放行 → 剩管理员一人 → 现在唯一账号可自删。
	if w = call(r, "POST", "/api/account/delete", cookieOf(svc, "u-other"), `{"name":"别人"}`); w.Code != 200 {
		t.Fatalf("other delete: %d %s", w.Code, w.Body.String())
	}
	if w = call(r, "POST", "/api/account/delete", cookieOf(svc, "u-admin"), `{"name":"管理员"}`); w.Code != 200 {
		t.Fatalf("sole admin self-delete: %d %s", w.Code, w.Body.String())
	}
}

func TestAccountDeleteSmsProof(t *testing.T) {
	svc, r := newTestEnv(t)
	if svc.SmsStore.Rdb == nil {
		t.Skip("无 TEST_REDIS_URL — 短信 proof 子案跳过")
	}
	ctx := context.Background()
	seedAcct(t, svc.DB, "u-sms", "验证码用户", false, "13800138000")
	ck := cookieOf(svc, "u-sms")

	// name-only:绑了手机但没发码 → 放行(proof=name;前端 UI 的现行为)。
	if w := call(r, "POST", "/api/account/delete", ck, `{"name":"验证码用户"}`); w.Code != 200 {
		t.Fatalf("name-only: %d %s", w.Code, w.Body.String())
	}
	// 可选增强通道:带码才校验 —— 错码拒。
	seedAcct(t, svc.DB, "u-sms2", "验证码用户二", false, "13900139000")
	ck2 := cookieOf(svc, "u-sms2")
	if w := call(r, "POST", "/api/account/delete", ck2, `{"name":"验证码用户二","smsCode":"000000"}`); w.Code != 400 {
		t.Fatalf("bad code: %d", w.Code)
	}
	// 好码 → proof=sms → 200。
	svc.SmsStore.PutCode(ctx, "13900139000", "654321")
	if w := call(r, "POST", "/api/account/delete", ck2, `{"name":"验证码用户二","smsCode":"654321"}`); w.Code != 200 {
		t.Fatalf("good code: %d %s", w.Code, w.Body.String())
	}
	var n int
	svc.DB.QueryRow(ctx, `SELECT count(*) FROM users WHERE id='u-sms'`).Scan(&n)
	if n != 0 {
		t.Fatal("first account must be gone")
	}
	svc.DB.QueryRow(ctx, `SELECT count(*) FROM users WHERE id='u-sms2'`).Scan(&n)
	if n != 0 {
		t.Fatal("second account must be gone")
	}
}
