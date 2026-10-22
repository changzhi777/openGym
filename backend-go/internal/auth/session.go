// Package auth — 会话与 WebAuthn(与 Node 版字节级兼容,双栈灰度期 cookie 互通)。
//
// Cookie 格式(必须与 api/server.js 完全一致):
//   gymsid=<uid>:<expms>:<sv>.<HMAC-SHA256(uid:exp:sv, SECRET) 的 base64url>
//   __Host-gymsid=<同上>  (仅 https;HttpOnly;SameSite=Lax;Path=/)
// sv = session version,logout-all 时 +1 使全部旧 cookie 失效。
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Sessions struct {
	secret []byte
	days   int
	secure bool
}

func NewSessions(secret []byte, days int, secure bool) *Sessions {
	if days <= 0 {
		days = 90
	}
	return &Sessions{secret: secret, days: days, secure: secure}
}

func (s *Sessions) sign(payload string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Sessions) verifySig(token string) (string, bool) {
	i := strings.LastIndexByte(token, '.')
	if i < 0 {
		return "", false
	}
	payload, mac := token[:i], token[i+1:]
	h := hmac.New(sha256.New, s.secret)
	h.Write([]byte(payload))
	expect := base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	if !hmac.Equal([]byte(mac), []byte(expect)) {
		return "", false
	}
	return payload, true
}

// Token 签发 uid:exp:sv。
func (s *Sessions) Token(uid string, sv int64) string {
	exp := time.Now().Add(time.Duration(s.days) * 24 * time.Hour).UnixMilli()
	return s.sign(fmt.Sprintf("%s:%d:%d", uid, exp, sv))
}

// Verify 返回 uid 与 exp(ms);缺第三段按 v0(Node 版本前 cookie 兼容)。
func (s *Sessions) Verify(token string) (uid string, exp int64, sv int64, ok bool) {
	payload, good := s.verifySig(token)
	if !good {
		return "", 0, 0, false
	}
	parts := strings.Split(payload, ":")
	if len(parts) < 2 {
		return "", 0, 0, false
	}
	e, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || e < time.Now().UnixMilli() {
		return "", 0, 0, false
	}
	v := int64(0)
	if len(parts) >= 3 {
		if v, err = strconv.ParseInt(parts[2], 10, 64); err != nil {
			return "", 0, 0, false
		}
	}
	return parts[0], e, v, true
}

func (s *Sessions) CookieName() string {
	if s.secure {
		return "__Host-gymsid"
	}
	return "gymsid"
}

func secureAttr(secure bool) string {
	if secure {
		return " Secure;"
	}
	return ""
}

func (s *Sessions) expireCookie(name string) string {
	return fmt.Sprintf("%s=; Path=/; Max-Age=0; HttpOnly;%s SameSite=Lax", name, secureAttr(s.secure))
}

// SetCookieValue 与 Node 相同的 Set-Cookie 值(https 时退役 legacy 名)。
func (s *Sessions) SetCookieValue(uid string, sv int64) []string {
	fresh := fmt.Sprintf("%s=%s; Path=/; Max-Age=%d; HttpOnly;%s SameSite=Lax",
		s.CookieName(), s.Token(uid, sv), s.days*86400, secureAttr(s.secure))
	if s.CookieName() == "gymsid" {
		return []string{fresh}
	}
	return []string{fresh, s.expireCookie("gymsid")}
}

// ClearValue 登出用的过期 Set-Cookie。
func (s *Sessions) ClearValue() []string {
	return []string{s.expireCookie(s.CookieName())}
}
