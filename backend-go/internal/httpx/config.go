// /api/config — 登录前引导(公开标志) + 会话内 Coach 块(Node GET /api/config 同义)。
package httpx

import (
	"net/http"
	"regexp"
	"strings"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/changzhi777/openGym/backend-go/internal/auth"
)

type ConfigSvc struct {
	DB       *pgxpool.Pool
	Sessions *auth.Sessions
}

// Get GET /api/config
func (s *ConfigSvc) Get(c *gin.Context) {
	body := gin.H{
		"invite_only": envBool("INVITE_ONLY", false),
		"allow_guest": envBool("ALLOW_GUEST", true),
	}
	if envBool("PASSWORD_LOGIN", false) {
		body["password_login"] = true // 仅开启时出现,与 Node 一致
	}
	if v := strings.TrimSpace(os.Getenv("DEFAULT_LANG")); v != "" && langTagRe.MatchString(v) {
		body["default_lang"] = v // 登录页第一读者;畸形码忽略(Node 77-83 同义)
	}
	if os.Getenv("MEDIA_UPLOADS") != "0" {
		body["media"] = gin.H{"enabled": true, "max_mb": 20} // 上限 P4 对齐 media.js
	}
	if _, ok := auth.UserOf(c, s.DB, s.Sessions); ok {
		body["coach"] = nil // 有会话必给 key(可区分“无 Coach”与“未登录”);sidecar 就位后填 provider
	}
	c.JSON(http.StatusOK, body)
}

var langTagRe = regexp.MustCompile(`^[A-Za-z]{2,3}([-_][A-Za-z0-9]{2,8})?$`)

func envBool(k string, def bool) bool {
	switch os.Getenv(k) {
	case "":
		return def
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}
