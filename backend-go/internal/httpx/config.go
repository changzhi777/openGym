// /api/config — 登录前引导(公开标志) + 会话内 Coach 块(Node GET /api/config 同义)。
package httpx

import (
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/changzhi777/openGym/backend-go/internal/auth"
)

type ConfigSvc struct {
	DB       *pgxpool.Pool
	Sessions *auth.Sessions
	Coach    CoachConfig // sidecar 桥;nil = 教练不在部署里(coach 键保持 null)
}

// CoachConfig 教练公开配置的最小接口(httpx 不 import coach,避免环)。
type CoachConfig interface {
	PublicConfig() []byte // sidecar 的 public-config;nil = 无教练
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
		// 有会话必给 key(可区分"无 Coach"与"未登录");配置经 sidecar(10s 缓存),nil = 无教练。
		coach := gin.H(nil)
		if s.Coach != nil {
			if raw := s.Coach.PublicConfig(); raw != nil {
				coach = gin.H{}
				json.Unmarshal(raw, &coach)
			}
		}
		body["coach"] = coach
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
