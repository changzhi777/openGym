// cmd/api/main.go — 沐禾健身 SaaS 后端(Go,fork P1 骨架)
//
// 方案:.zcf/plan/current/saas-backend-go.md。P1 = 骨架:Gin + PG(pgx) + Redis +
// /api/health 探活。业务端点 P2 起(契约已由 internal/apigen 生成,零漂移)。
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	webauthn "github.com/changzhi777/openGym/backend-go/internal/auth"

	"github.com/gin-gonic/gin"

	"github.com/changzhi777/openGym/backend-go/internal/auth"
	"github.com/changzhi777/openGym/backend-go/internal/cache"
	"github.com/changzhi777/openGym/backend-go/internal/store"
)

// golang-migrate 的 pgx/v5 driver 用 pgx5:// scheme;环境变量习惯是 postgres://。
func pgx5URL(u string) string {
	const a, b = "postgres://", "pgx5://"
	if len(u) >= len(a) && u[:len(a)] == a {
		return b + u[len(a):]
	}
	return u
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3100"
	}
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	// 迁移内嵌随进程跑(幂等),部署面只剩 PG+Redis+本进程。
	if os.Getenv("MIGRATE_ON_START") != "0" {
		if err := store.Migrate(pgx5URL(dbURL)); err != nil {
			log.Fatalf("migrate: %v", err)
		}
	}
	db, err := store.Open(ctx, dbURL)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer db.Close()

	rdb, err := cache.Open(ctx, os.Getenv("REDIS_ADDR"))
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer rdb.Close()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// 与 Node 版同形:容器 healthcheck 读状态码,写不进数据目录 = 不健康。
	r.GET("/api/health", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		pgOK := db.Ping(ctx) == nil
		rdOK := rdb.Ping(ctx).Err() == nil   // Ping 返回 *StatusCmd,判断 .Err()
		status := http.StatusOK
		if !pgOK || !rdOK {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"ok": pgOK && rdOK, "pg": pgOK, "redis": rdOK, "svc": "muhe-go"})
	})

	// P2a 认证闭环(register/login ×2 + me + logout)。
	wa, err := webauthn.NewWebAuthn(rpName(), rpID(), origin())
	if err != nil {
		log.Fatalf("webauthn: %v", err)
	}
	sessions := auth.NewSessions(secret(), sessionDays(), strings.Contains(origin(), "https://"))
	svc := &auth.Service{DB: db, WA: wa, Sessions: sessions, Challenges: auth.NewChallengeStore(), Pairs: auth.NewPairStore()}
	r.POST("/api/register/options", svc.RegisterOptions)
	r.POST("/api/register/verify", svc.RegisterVerify)
	r.POST("/api/login/options", svc.LoginOptions)
	r.POST("/api/login/verify", svc.LoginVerify)
	r.GET("/api/me", svc.Me)
	r.POST("/api/logout", svc.Logout)
	r.POST("/api/pair/create", svc.PairCreate)
	r.POST("/api/pair/redeem", svc.PairRedeem)
	r.GET("/api/account/passkeys", svc.PasskeysGet)
	r.POST("/api/account/passkeys/options", svc.PasskeysOptions)
	r.POST("/api/account/passkeys/verify", svc.PasskeysVerify)
	r.DELETE("/api/account/passkeys", svc.PasskeysDelete)

	// 其余端点尚未实现 —— 与 Node 侧 501 stub 同语义。
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "planned for a later phase", "code": "not-implemented"})
	})

	srv := &http.Server{Addr: ":" + port, Handler: r, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("muhe-go api on :%s (pg=%v redis=%v)", port, os.Getenv("DATABASE_URL") != "", os.Getenv("REDIS_ADDR") != "")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// 环境读取(RP_NAME/RP_ID/ORIGIN/SESSION_DAYS/SECRET 与 Node 同名同义)。
func rpName() string { return envOr("RP_NAME", "沐禾健身") }
func rpID() string   { return envOr("RP_ID", "localhost") }
func origin() string { return envOr("ORIGIN", "http://localhost:8080") }
func sessionDays() int {
	n := 90
	if v := os.Getenv("SESSION_DAYS"); v != "" {
		fmt.Sscanf(v, "%d", &n)
	}
	return n
}
func secret() []byte {
	v := os.Getenv("SECRET")
	if v == "" {
		v = "dev-secret" // 与 Node data/secret 对接由部署注入;本地默认仅用于开发
	}
	return []byte(v)
}
func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
