// 认证端点(P2a):register/login 的 options+verify、me、logout。
// 契约与 api/server.js 逐字段对齐(前端 lib/api.js 是消费方)。
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/changzhi777/openGym/backend-go/internal/store"
)

type Service struct {
	DB         *pgxpool.Pool
	WA         *WebAuthn
	Sessions   *Sessions
	Challenges *ChallengeStore
	Pairs      *PairStore
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

/* ---- P2b:pair / passkey 管理 ---- */

// PairCreate POST /api/pair/create — 浏览器登录态签发配对码。
func (s *Service) PairCreate(c *gin.Context) {
	u, ok := s.userOf(c)
	if !ok {
		jsonFail(c, 401, "not signed in", "")
		return
	}
	c.JSON(200, gin.H{"code": s.Pairs.NewCode(u.ID)})
}

// PairRedeem POST /api/pair/redeem — 手机兑换;token 与 cookie 同格式(Bearer 用)。
func (s *Service) PairRedeem(c *gin.Context) {
	b := readBody(c)
	code := strings.ToUpper(strings.TrimSpace(b.Code))
	uid := s.Pairs.Take(code)
	if uid == "" {
		jsonFail(c, 400, "invalid or expired code", "pair-invalid")
		return
	}
	u, err := store.UserByID(c.Request.Context(), s.DB, uid)
	if err != nil || u.Disabled {
		jsonFail(c, 400, "invalid or expired code", "pair-invalid")
		return
	}
	c.JSON(200, gin.H{"token": s.Sessions.Token(u.ID, u.SV), "user": gin.H{"id": u.ID, "name": u.Name, "admin": false}})
}

// PasskeysGet GET /api/account/passkeys
func (s *Service) PasskeysGet(c *gin.Context) {
	u, ok := s.userOf(c)
	if !ok {
		jsonFail(c, 401, "not signed in", "")
		return
	}
	list, err := store.ListPasskeys(c.Request.Context(), s.DB, u.ID)
	if err != nil {
		jsonFail(c, 500, "list", "")
		return
	}
	c.JSON(200, gin.H{"passkeys": list, "password": false,
		"lastWayIn": len(list) <= 1}) // 无密码通道:仅剩最后一把时拒绝删除
}

// PasskeysOptions POST /api/account/passkeys/options — 加第二把(exclude 现有)。
func (s *Service) PasskeysOptions(c *gin.Context) {
	u, ok := s.userOf(c)
	if !ok {
		jsonFail(c, 401, "not signed in", "")
		return
	}
	n, _ := store.CountPasskeys(c.Request.Context(), s.DB, u.ID)
	if n >= 20 {
		jsonFail(c, 409, "a profile can have at most 20 passkeys", "passkey-limit")
		return
	}
	options, challenge, err := s.WA.CreationOptionsExclude([]byte(u.ID), u.Name, s.exclusionList(c, u.ID))
	if err != nil {
		jsonFail(c, 500, "options", "")
		return
	}
	cid := s.Challenges.Put(&Challenge{Kind: "add", Challenge: challenge, UID: u.ID})
	c.JSON(200, gin.H{"cid": cid, "options": options})
}

// PasskeysVerify POST /api/account/passkeys/verify
func (s *Service) PasskeysVerify(c *gin.Context) {
	u, ok := s.userOf(c)
	if !ok {
		jsonFail(c, 401, "not signed in", "")
		return
	}
	b := readBody(c)
	ch := s.Challenges.Take(b.CID)
	if ch == nil || ch.Kind != "add" || ch.UID != u.ID {
		jsonFail(c, 400, "challenge expired, try again", "challenge-expired")
		return
	}
	key, err := s.WA.VerifyRegistration([]byte(u.ID), u.Name, b.Credential, ch.Challenge)
	if err != nil {
		jsonFail(c, 400, "verification failed", "")
		return
	}
	if err := store.InsertKey(c.Request.Context(), s.DB, u.ID, store.KeyRow{ID: key.ID, PublicKey: key.PublicKey}); err != nil {
		jsonFail(c, 409, "credential already registered", "credential-exists")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// PasskeysDelete DELETE /api/account/passkeys — 最后一把不可删(否则锁死)。
func (s *Service) PasskeysDelete(c *gin.Context) {
	u, ok := s.userOf(c)
	if !ok {
		jsonFail(c, 401, "not signed in", "")
		return
	}
	if n, _ := store.CountPasskeys(c.Request.Context(), s.DB, u.ID); n <= 1 {
		jsonFail(c, 409, "this is the last way in", "last-way-in")
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	json.NewDecoder(c.Request.Body).Decode(&body)
	if body.ID == "" {
		jsonFail(c, 400, "id required", "")
		return
	}
	if err := store.DeletePasskey(c.Request.Context(), s.DB, u.ID, body.ID); err != nil {
		jsonFail(c, 404, "no such passkey", "not-found")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Service) exclusionList(c *gin.Context, uid string) []KeyHandle {
	keys, err := store.KeysOf(c.Request.Context(), s.DB, uid)
	if err != nil {
		return nil
	}
	out := make([]KeyHandle, len(keys))
	for i, k := range keys {
		out[i] = KeyHandle{ID: k.ID}
	}
	return out
}

/* ---- P2c:device-link ×3 / rename / logout-all ---- */

func b64rand(n int) string {
	raw := make([]byte, n)
	rand.Read(raw)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// DeviceLinkCreate POST /api/account/device-link — 登录态,码 15 分钟(Node 同 TTL)。
func (s *Service) DeviceLinkCreate(c *gin.Context) {
	u, ok := s.userOf(c)
	if !ok {
		jsonFail(c, 401, "not signed in", "")
		return
	}
	if n, _ := store.CountPasskeys(c.Request.Context(), s.DB, u.ID); n >= 20 {
		jsonFail(c, 409, "a profile can have at most 20 passkeys", "passkey-limit")
		return
	}
	code := b64rand(9) // 明码只出现一次,库里只存哈希
	h := b64rand(12)
	exp := time.Now().Add(15 * time.Minute)
	store.TouchLink(c.Request.Context(), s.DB, sha256hex(code), u.ID, h, exp)
	c.JSON(200, gin.H{"code": code, "expires": exp.UnixMilli()})
}

// DeviceLinkOptions POST /api/device-link/options — 无会话,码即凭据;不消费(可重试)。
func (s *Service) DeviceLinkOptions(c *gin.Context) {
	b := readBody(c)
	code := strings.TrimSpace(b.Code)
	uid, h, ok := store.FindLink(c.Request.Context(), s.DB, sha256hex(code))
	if !ok {
		jsonFail(c, 400, "that code is wrong, used or expired", "link-invalid")
		return
	}
	u, err := store.UserByID(c.Request.Context(), s.DB, uid)
	if err != nil || u.Disabled {
		jsonFail(c, 400, "that code is wrong, used or expired", "link-invalid")
		return
	}
	keys, _ := store.KeysOf(c.Request.Context(), s.DB, uid)
	excl := make([]KeyHandle, len(keys))
	for i, k := range keys {
		excl[i] = KeyHandle{ID: k.ID}
	}
	options, challenge, err := s.WA.CreationOptionsExclude([]byte(u.ID), u.Name, excl)
	if err != nil {
		jsonFail(c, 500, "options", "")
		return
	}
	cid := s.Challenges.Put(&Challenge{Kind: "link", Challenge: challenge, UID: u.ID, LH: h})
	c.JSON(200, gin.H{"cid": cid, "options": options, "id": u.ID, "name": u.Name})
}

// DeviceLinkVerify POST /api/device-link/verify — 新凭据入库 + 烧码 + 签 cookie 登录。
func (s *Service) DeviceLinkVerify(c *gin.Context) {
	b := readBody(c)
	code := strings.TrimSpace(b.Code)
	ch := s.Challenges.Take(b.CID)
	uid, h, ok := store.FindLink(c.Request.Context(), s.DB, sha256hex(code))
	if !ok {
		jsonFail(c, 400, "that code is wrong, used or expired", "link-invalid")
		return
	}
	if ch == nil || ch.Kind != "link" || ch.LH != h || ch.UID != uid {
		jsonFail(c, 400, "challenge expired, try again", "challenge-expired")
		return
	}
	u, err := store.UserByID(c.Request.Context(), s.DB, uid)
	if err != nil || u.Disabled {
		jsonFail(c, 400, "that code is wrong, used or expired", "link-invalid")
		return
	}
	key, err := s.WA.VerifyRegistration([]byte(u.ID), u.Name, b.Credential, ch.Challenge)
	if err != nil {
		jsonFail(c, 400, "verification failed", "")
		return
	}
	if err := store.InsertKey(c.Request.Context(), s.DB, u.ID, store.KeyRow{ID: key.ID, PublicKey: key.PublicKey}); err != nil {
		jsonFail(c, 409, "credential already registered", "credential-exists")
		return
	}
	store.BurnLink(c.Request.Context(), s.DB, sha256hex(code))
	s.setCookie(c, u.ID, u.SV)
	c.JSON(200, gin.H{"user": gin.H{"id": u.ID, "name": u.Name, "admin": false}})
}

// PasskeysRename POST /api/account/passkeys/rename — {id, name}。
func (s *Service) PasskeysRename(c *gin.Context) {
	u, ok := s.userOf(c)
	if !ok {
		jsonFail(c, 401, "not signed in", "")
		return
	}
	var body struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	json.NewDecoder(c.Request.Body).Decode(&body)
	if body.ID == "" {
		jsonFail(c, 400, "id required", "")
		return
	}
	name := strings.TrimSpace(body.Name)
	if len(name) > 40 {
		name = name[:40]
	}
	if err := store.RenamePasskey(c.Request.Context(), s.DB, u.ID, body.ID, name); err != nil {
		jsonFail(c, 404, "no such passkey", "not-found")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// LogoutAll POST /api/logout/all — sv+1,全部旧 cookie/Bearer 失效。
func (s *Service) LogoutAll(c *gin.Context) {
	u, ok := s.userOf(c)
	if !ok {
		jsonFail(c, 401, "not signed in", "")
		return
	}
	if err := store.BumpSV(c.Request.Context(), s.DB, u.ID); err != nil {
		jsonFail(c, 500, "bump", "")
		return
	}
	for _, v := range s.Sessions.ClearValue() {
		c.Writer.Header().Add("Set-Cookie", v)
	}
	c.JSON(200, gin.H{"ok": true})
}
