// Package httpx — Gin 中间件(CSRF/P2 后各端点共用的通用件)。
package httpx

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// csrfExempt 与 api/server.js CSRF_EXEMPT 同集:登录 CSRF 面最大的五个仪式端点
// (login/password 不在内 —— 那正是攻击面)。
var csrfExempt = map[string]bool{
	"/api/register/options": true, "/api/register/verify": true,
	"/api/login/options": true, "/api/login/verify": true,
	"/api/pair/redeem": true,
}

// CSRF — Node csrfOk 移植:Sec-Fetch-Site 优先(浏览器自己盖章,不可伪造),
// 无则退回 Origin 比对;Bearer 请求无环境权限可借,放行。
func CSRF(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		m := c.Request.Method
		if m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions {
			c.Next()
			return
		}
		if csrfExempt[c.FullPath()] {
			c.Next()
			return
		}
		if strings.HasPrefix(c.GetHeader("Authorization"), "Bearer ") {
			c.Next()
			return
		}
		if site := c.GetHeader("Sec-Fetch-Site"); site != "" {
			if site == "same-origin" || site == "none" {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(403, gin.H{"error": "cross-origin request refused"})
			return
		}
		if o := c.GetHeader("Origin"); o != "" && strings.TrimRight(o, "/") != strings.TrimRight(origin, "/") {
			c.AbortWithStatusJSON(403, gin.H{"error": "cross-origin request refused"})
			return
		}
		c.Next()
	}
}

// appOrigins:Capacitor App 的 WebView origin(Node APP_ORIGINS 同集)。
var appOrigins = map[string]bool{
	"https://localhost": true, "capacitor://localhost": true, "http://localhost": true,
}

// CORS — Node 2643-2663 同义:任意 Origin 回显(无 cookie,反射无害)+ Vary;
// 绝不设 Allow-Credentials;OPTIONS 204;PNA 头仅给 App WebView 源。
func CORS(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		o := c.GetHeader("Origin")
		if o != "" {
			c.Header("Access-Control-Allow-Origin", o)
			c.Header("Vary", "Origin")
		}
		if c.Request.Method == http.MethodOptions {
			pna := strings.EqualFold(c.GetHeader("Access-Control-Request-Private-Network"), "true") && appOrigins[o]
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, CF-Access-Client-Id, CF-Access-Client-Secret")
			c.Header("Access-Control-Max-Age", "86400")
			if pna {
				c.Header("Access-Control-Allow-Private-Network", "true")
			}
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// BodyGuard — Node MAX_BODY(5MiB) + 坏 JSON 统一 400:
// 读进内存(供 handler 复用),超限 413,非空且非法 JSON 直接 400 {"error":"invalid json"}。
const maxBody = 5 * 1024 * 1024

func BodyGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil {
			c.Next()
			return
		}
		// 二进制端点(媒体上传)豁免:大小与格式由各自 handler 校验(40MB 视频 > 5MiB 全局帽)。
		if strings.HasPrefix(c.Request.URL.Path, "/api/media/") {
			c.Next()
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody+1)
		raw, err := io.ReadAll(c.Request.Body)
		if err != nil && strings.Contains(err.Error(), "request body too large") {
			c.AbortWithStatusJSON(413, gin.H{"error": "body too large"})
			return
		}
		if len(raw) > maxBody {
			c.AbortWithStatusJSON(413, gin.H{"error": "body too large"})
			return
		}
		ct := c.GetHeader("Content-Type")
		if len(raw) > 0 && (ct == "" || strings.Contains(ct, "json")) {
			var v any
			if jerr := json.Unmarshal(raw, &v); jerr != nil {
				// 深度超限是合法 JSON(只是存不下):放行,由 handler 判「invalid state」
				if !strings.Contains(jerr.Error(), "exceeded max") {
					c.AbortWithStatusJSON(400, gin.H{"error": "invalid json"})
					return
				}
			} else if _, isObj := v.(map[string]any); !isObj {
				// null/"str"/42/[] 都是合法 JSON 但不是对象(Node 同样拒)
				c.AbortWithStatusJSON(400, gin.H{"error": "invalid json"})
				return
			}
		}
		c.Set("rawBody", raw)
		c.Next()
	}
}

// RawBody 取 BodyGuard 预读的原始 body。
func RawBody(c *gin.Context) []byte {
	if v, ok := c.Get("rawBody"); ok {
		if b, ok := v.([]byte); ok {
			return b
		}
	}
	return nil
}
