// 认证端点(P2a):register/login 的 options+verify、me、logout。
// 契约与 api/server.js 逐字段对齐(前端 lib/api.js 是消费方)。
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/changzhi777/openGym/backend-go/internal/store"
)

type Service struct {
	DB         *pgxpool.Pool
	WA         *WebAuthn
	Sessions   *Sessions
	Challenges *ChallengeStore
}

type authBody struct {
	Name       string          `json:"name"`
	Code       string          `json:"code"`
	CID        string          `json:"cid"`
	Credential json.RawMessage `json:"credential"`
}

func readBody(c *gin.Context) *authBody {
	b := &authBody{}
	raw, _ := io.ReadAll(c.Request.Body)
	json.Unmarshal(raw, b)
	return b
}

func jsonFail(c *gin.Context, status int, msg, code string) {
	if code == "" {
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(status, gin.H{"error": msg, "code": code})
}

// RegisterOptions POST /api/register/options
func (s *Service) RegisterOptions(c *gin.Context) {
	b := readBody(c)
	name := strings.TrimSpace(b.Name)
	if len(name) > 40 {
		name = name[:40]
	}
	if name == "" {
		jsonFail(c, 400, "name required", "")
		return
	}
	raw := make([]byte, 12)
	rand.Read(raw)
	uid := base64.RawURLEncoding.EncodeToString(raw)
	options, challenge, err := s.WA.CreationOptions([]byte(uid), name)
	if err != nil {
		jsonFail(c, 500, "options", "")
		return
	}
	cid := s.Challenges.Put(&Challenge{Kind: "register", Challenge: challenge, UID: uid, Name: name, Code: strings.ToUpper(strings.TrimSpace(b.Code))})
	c.JSON(200, gin.H{"cid": cid, "options": options})
}

// RegisterVerify POST /api/register/verify
func (s *Service) RegisterVerify(c *gin.Context) {
	b := readBody(c)
	ch := s.Challenges.Take(b.CID)
	if ch == nil || ch.Kind != "register" || ch.UID == "" {
		jsonFail(c, 400, "challenge expired, try again", "challenge-expired")
		return
	}
	key, err := s.WA.VerifyRegistration([]byte(ch.UID), ch.Name, b.Credential, ch.Challenge)
	if err != nil {
		jsonFail(c, 400, "verification failed", "")
		return
	}
	if err := store.InsertUser(c.Request.Context(), s.DB, ch.UID, ch.Name); err != nil {
		jsonFail(c, 500, "save", "")
		return
	}
	if err := store.InsertKey(c.Request.Context(), s.DB, ch.UID, store.KeyRow{ID: key.ID, PublicKey: key.PublicKey}); err != nil {
		jsonFail(c, 409, "credential already registered", "")
		return
	}
	s.setCookie(c, ch.UID, 0)
	c.JSON(200, gin.H{"user": gin.H{"id": ch.UID, "name": ch.Name, "admin": false}})
}

// LoginOptions POST /api/login/options
func (s *Service) LoginOptions(c *gin.Context) {
	options, challenge, err := s.WA.RequestOptions()
	if err != nil {
		jsonFail(c, 500, "options", "")
		return
	}
	cid := s.Challenges.Put(&Challenge{Kind: "login", Challenge: challenge})
	c.JSON(200, gin.H{"cid": cid, "options": options})
}

// LoginVerify POST /api/login/verify
func (s *Service) LoginVerify(c *gin.Context) {
	b := readBody(c)
	ch := s.Challenges.Take(b.CID)
	if ch == nil || ch.Kind != "login" {
		jsonFail(c, 400, "challenge expired, try again", "challenge-expired")
		return
	}
	var cred struct {
		ID string `json:"id"`
	}
	json.Unmarshal(b.Credential, &cred)
	key, uid, err := store.KeyByID(c.Request.Context(), s.DB, cred.ID)
	if err != nil {
		jsonFail(c, 404, "unknown passkey, create a profile first", "unknown-credential")
		return
	}
	u, err := store.UserByID(c.Request.Context(), s.DB, uid)
	if err != nil || u.Disabled {
		jsonFail(c, 404, "unknown passkey, create a profile first", "unknown-credential")
		return
	}
	keys, err := store.KeysOf(c.Request.Context(), s.DB, uid)
	if err != nil {
		jsonFail(c, 500, "keys", "")
		return
	}
	handles := make([]KeyHandle, len(keys))
	for i, k := range keys {
		handles[i] = KeyHandle{ID: k.ID, PublicKey: k.PublicKey, Counter: k.Counter}
	}
	counter, err := s.WA.VerifyAuthentication([]byte(uid), u.Name, handles, b.Credential, ch.Challenge)
	if err != nil {
		jsonFail(c, 400, "verification failed", "")
		return
	}
	store.UpdateKeyCounter(c.Request.Context(), s.DB, key.ID, counter)
	s.setCookie(c, uid, u.SV)
	c.JSON(200, gin.H{"user": gin.H{"id": u.ID, "name": u.Name, "admin": false}})
}

// Me GET /api/me
func (s *Service) Me(c *gin.Context) {
	u, ok := s.userOf(c)
	if !ok {
		jsonFail(c, 401, "not signed in", "")
		return
	}
	c.JSON(200, gin.H{"user": gin.H{"id": u.ID, "name": u.Name, "admin": false}})
}

// Logout POST /api/logout
func (s *Service) Logout(c *gin.Context) {
	for _, v := range s.Sessions.ClearValue() {
		c.Writer.Header().Add("Set-Cookie", v)
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Service) setCookie(c *gin.Context, uid string, sv int64) {
	for _, v := range s.Sessions.SetCookieValue(uid, sv) {
		c.Writer.Header().Add("Set-Cookie", v)
	}
}

// userOf 从 cookie/Bearer 解出有效用户(Node sessionOf 同义:disabled/sv 校验)。
func (s *Service) userOf(c *gin.Context) (store.User, bool) {
	tok := ""
	for _, raw := range strings.Split(c.GetHeader("Cookie"), ";") {
		if i := strings.IndexByte(raw, '='); i >= 0 && strings.TrimSpace(raw[:i]) == s.Sessions.CookieName() {
			tok = strings.TrimSpace(raw[i+1:])
		}
	}
	if tok == "" {
		if a := c.GetHeader("Authorization"); strings.HasPrefix(a, "Bearer ") {
			tok = strings.TrimSpace(a[7:])
		}
	}
	if tok == "" {
		return store.User{}, false
	}
	uid, _, sv, ok := s.Sessions.Verify(tok)
	if !ok {
		return store.User{}, false
	}
	u, err := store.UserByID(c.Request.Context(), s.DB, uid)
	if err != nil || u.Disabled || u.SV != sv {
		return store.User{}, false
	}
	return u, true
}
