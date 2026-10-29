// Package auth — 认证(手机号+短信验证码 / 微信小程序码扫码;身份表模型)。
//
// 认证重构(2026-10-29,决议见 .zcf/plan):passkey/密码/设备配对退役
// (契约标 deprecated;Go 生产栈无对应路由)。会话层 session.go 与
// Node 字节级兼容,灰度期 cookie 互通 —— 这一层不动。
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/changzhi777/openGym/backend-go/internal/sms"
	"github.com/changzhi777/openGym/backend-go/internal/captcha"
	"github.com/changzhi777/openGym/backend-go/internal/store"
	"github.com/changzhi777/openGym/backend-go/internal/wechat"
)

// Service 认证服务(手机/微信;admin step-up 短信确认也走这里的能力)。
type Service struct {
	DB           *pgxpool.Pool
	Sessions     *Sessions
	Rdb          *redis.Client        // 验证码/频控/微信票据
	Sms          sms.Sender           // 腾讯云 SMS(dev 模式=未配凭据)
	SmsStore     sms.Store
	Wx           *wechat.MiniProgram  // 小程序扫码;nil/未配 = 503
	InviteEnvDef bool                 // INVITE_ONLY env 兜底(instance_settings.invite_only 覆盖)
}

type authBody struct {
	Phone       string `json:"phone"`
	Code        string `json:"code"`
	Invite      string `json:"invite"`
	Ticket      string `json:"ticket"`
	WxCode      string `json:"wxCode"`
	CaptchaID   string `json:"captchaId"`
	CaptchaCode string `json:"captchaCode"`
}

func readBody(c *gin.Context) *authBody {
	b := &authBody{}
	if raw, ok := c.Get("rawBody"); ok {
		if bs, ok2 := raw.([]byte); ok2 {
			json.Unmarshal(bs, b)
		}
	}
	return b
}

func jsonFail(c *gin.Context, status int, msg, code string) {
	c.JSON(status, gin.H{"error": msg, "code": code})
}

func (s *Service) setCookie(c *gin.Context, uid string, sv int64) {
	for _, v := range s.Sessions.SetCookieValue(uid, sv) {
		c.Writer.Header().Add("Set-Cookie", v)
	}
}

// UserOf 从 cookie/Bearer 解出有效用户(Node sessionOf 同义:disabled/sv 校验)。
// 包级导出:admin/presence 等包复用同一会话判定。
func UserOf(c *gin.Context, db *pgxpool.Pool, ss *Sessions) (store.User, bool) {
	s := &Service{DB: db, Sessions: ss}
	tok := ""
	for _, raw := range strings.Split(c.GetHeader("Cookie"), ";") {
		if i := strings.IndexByte(raw, '='); i >= 0 && strings.TrimSpace(raw[:i]) == s.Sessions.CookieName() {
			tok = strings.TrimSpace(raw[i+1:])
		}
	}
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		tok = strings.TrimPrefix(h, "Bearer ")
	}
	uid, _, sv, ok := s.Sessions.Verify(tok)
	if !ok {
		return store.User{}, false
	}
	u, err := store.UserByID(c.Request.Context(), db, uid)
	if err != nil || u.Disabled || (u.SV != sv && sv != 0) {
		return store.User{}, false
	}
	return u, true
}

// Me GET /api/me — 当前账号 + 身份列表。
func (s *Service) Me(c *gin.Context) {
	u, ok := UserOf(c, s.DB, s.Sessions)
	if !ok {
		jsonFail(c, 401, "not signed in", "")
		return
	}
	ids, _ := store.IdentitiesOf(c.Request.Context(), s.DB, u.ID)
	c.JSON(200, gin.H{"user": gin.H{"id": u.ID, "name": u.Name, "admin": s.isAdmin(c, u.ID)}, "identities": ids})
}

func (s *Service) isAdmin(ctx context.Context, uid string) bool {
	var one int
	s.DB.QueryRow(ctx, `SELECT 1 FROM admin_roles WHERE user_id=$1 AND role='super_admin' AND revoked_at IS NULL`, uid).Scan(&one)
	return one == 1
}

// audit 认证事件审计(best-effort,写失败不阻断;见 store.AuditAuth)。
func (s *Service) audit(c *gin.Context, actor, action string, detail map[string]any) {
	store.AuditAuth(c.Request.Context(), s.DB, actor, action, detail, c.ClientIP(), c.GetHeader("User-Agent"))
}

// Logout POST /api/logout。
func (s *Service) Logout(c *gin.Context) {
	for _, v := range s.Sessions.ClearValue() {
		c.Writer.Header().Add("Set-Cookie", v)
	}
	c.JSON(200, gin.H{"ok": true})
}

// LogoutAll POST /api/logout/all — sv+1 全端失效。
func (s *Service) LogoutAll(c *gin.Context) {
	u, ok := UserOf(c, s.DB, s.Sessions)
	if !ok {
		jsonFail(c, 401, "not signed in", "")
		return
	}
	s.DB.Exec(c.Request.Context(), `UPDATE users SET sv = sv + 1 WHERE id=$1`, u.ID)
	for _, v := range s.Sessions.ClearValue() {
		c.Writer.Header().Add("Set-Cookie", v)
	}
	c.JSON(200, gin.H{"ok": true})
}

// inviteOnly 实例邀请制生效值(settings 覆盖 env)。
func (s *Service) inviteOnly(c *gin.Context) bool {
	on, err := store.InviteOnlyEffective(c.Request.Context(), s.DB, s.InviteEnvDef)
	return err == nil && on
}

func validPhone(p string) bool {
	if len(p) != 11 {
		return false
	}
	for _, r := range p {
		if r < '0' || r > '9' {
			return false
		}
	}
	return p[0] == '1'
}

// Captcha GET /api/auth/captcha — 发短信前置的数字图形验证码(契约 authCaptcha;决议 Q3)。
func (s *Service) Captcha(c *gin.Context) {
	id, svg, err := captcha.New(c.Request.Context(), s.Rdb)
	if err != nil {
		jsonFail(c, 500, "captcha", "")
		return
	}
	c.JSON(200, gin.H{"captchaId": id, "svg": svg})
}

// SmsSend POST /api/auth/sms/send — 发验证码(契约 authSmsSend;频控在 sms.Store)。
// 防短信轰炸前置:必须先过图形验证码(决议 Q3),错/过期一律 400 code=captcha。
func (s *Service) SmsSend(c *gin.Context) {
	b := readBody(c)
	b.Phone = strings.TrimSpace(b.Phone)
	if !validPhone(b.Phone) {
		jsonFail(c, 400, "invalid phone number", "phone")
		return
	}
	if !captcha.Check(c.Request.Context(), s.Rdb, strings.TrimSpace(b.CaptchaID), strings.TrimSpace(b.CaptchaCode)) {
		jsonFail(c, 400, "image captcha wrong or expired, refresh and retry", "captcha")
		return
	}
	if s.SmsStore.Locked(c.Request.Context(), b.Phone) {
		c.JSON(429, gin.H{"error": "too many wrong codes, wait a minute", "code": "locked", "retryAfter": 60})
		return
	}
	if ra := s.SmsStore.AllowSend(c.Request.Context(), b.Phone, c.ClientIP()); ra > 0 {
		c.JSON(429, gin.H{"error": "too many codes sent", "code": "rate", "retryAfter": ra})
		return
	}
	code := fmt.Sprintf("%06d", fastrand()%1000000)
	dev, err := s.Sms.SendCode(c.Request.Context(), b.Phone, code)
	if err != nil {
		log.Printf("sms send %s***: %v", b.Phone[:3], err)
		s.audit(c, "phone:"+store.MaskPhone(b.Phone), "auth.sms.send", gin.H{"ok": false})
		jsonFail(c, 500, "sms send failed", "")
		return
	}
	s.SmsStore.PutCode(c.Request.Context(), b.Phone, code)
	s.audit(c, "phone:"+store.MaskPhone(b.Phone), "auth.sms.send", gin.H{"ok": true, "dev": dev})
	out := gin.H{"ok": true}
	if dev {
		out["dev"] = true
	}
	c.JSON(202, out)
}

// SmsVerify POST /api/auth/sms/verify — 注册登录一体(契约 authSmsVerify)。
func (s *Service) SmsVerify(c *gin.Context) {
	b := readBody(c)
	b.Phone = strings.TrimSpace(b.Phone)
	if !validPhone(b.Phone) || b.Code == "" {
		jsonFail(c, 400, "phone and code required", "phone")
		return
	}
	ct := c.Request.Context()
	uid, bound, err := store.UserByIdentity(ct, s.DB, store.IdPhone, b.Phone)
	if err != nil {
		jsonFail(c, 500, "lookup", "")
		return
	}
	isNew := false
	if !bound {
		if s.inviteOnly(c) {
			if b.Invite == "" {
				jsonFail(c, 403, "a valid invite code is required", "invite")
				return
			}
			if err := store.ConsumeInvite(ct, s.DB, strings.ToUpper(strings.TrimSpace(b.Invite)), ""); err != nil {
				jsonFail(c, 403, "a valid invite code is required", "invite")
				return
			}
		}
		uid = newUID()
		if err := store.InsertUser(ct, s.DB, uid, defaultName(b.Phone)); err != nil {
			jsonFail(c, 500, "save", "")
			return
		}
		isNew = true
	}
	if !s.SmsStore.Verify(ct, b.Phone, b.Code) {
		s.audit(c, "phone:"+store.MaskPhone(b.Phone), "auth.sms.verify", gin.H{"ok": false})
		jsonFail(c, 401, "wrong or expired code", "sms-code")
		return
	}
	if err := store.CreateIdentity(ct, s.DB, uid, store.IdPhone, b.Phone); err != nil {
		jsonFail(c, 500, "identity", "")
		return
	}
	s.setCookie(c, uid, 0)
	// 响应名取库中真名(老用户回登显示既有名,勿用手机号派生名覆盖)——新建号刚插入,同一路径可查
	name := defaultName(b.Phone)
	if u, err := store.UserByID(ct, s.DB, uid); err == nil && u.Name != "" {
		name = u.Name
	}
	s.audit(c, uid, "auth.sms.verify", gin.H{"ok": true, "isNew": isNew})
	c.JSON(200, gin.H{"user": gin.H{"id": uid, "name": name}, "isNew": isNew})
}

// BindPhone POST /api/auth/bind-phone — 已登录账号绑手机号(契约 authBindPhone)。
func (s *Service) BindPhone(c *gin.Context) {
	u, ok := UserOf(c, s.DB, s.Sessions)
	if !ok {
		jsonFail(c, 401, "not signed in", "")
		return
	}
	b := readBody(c)
	b.Phone = strings.TrimSpace(b.Phone)
	if !validPhone(b.Phone) || b.Code == "" {
		jsonFail(c, 400, "phone and code required", "phone")
		return
	}
	ct := c.Request.Context()
	if owner, bound, _ := store.UserByIdentity(ct, s.DB, store.IdPhone, b.Phone); bound && owner != u.ID {
		c.JSON(409, gin.H{"error": "the number already belongs to another account"})
		return
	}
	if !s.SmsStore.Verify(ct, b.Phone, b.Code) {
		jsonFail(c, 401, "wrong or expired code", "sms-code")
		return
	}
	if err := store.CreateIdentity(ct, s.DB, u.ID, store.IdPhone, b.Phone); err != nil {
		if errors.Is(err, store.ErrConflict) {
			c.JSON(409, gin.H{"error": "the number already belongs to another account"})
			return
		}
		jsonFail(c, 500, "bind", "")
		return
	}
	c.JSON(200, gin.H{"ok": true, "phone": b.Phone})
	s.audit(c, u.ID, "auth.bind.phone", gin.H{"phone": store.MaskPhone(b.Phone)})
}

// WechatQrcode GET /api/auth/wechat/qrcode — 出小程序码(契约 authWechatQrcode)。
func (s *Service) WechatQrcode(c *gin.Context) {
	if s.Wx == nil || !s.Wx.Enabled() {
		jsonFail(c, 503, "wechat login is not configured", "wechat-off")
		return
	}
	ticket, err := s.Wx.MintTicket(c.Request.Context())
	if err != nil {
		jsonFail(c, 500, "ticket", "")
		return
	}
	png, err := s.Wx.QRCodePNG(c.Request.Context(), ticket)
	if err != nil {
		jsonFail(c, 502, "qrcode render failed", "")
		return
	}
	c.JSON(200, gin.H{"ticket": ticket, "image": base64.StdEncoding.EncodeToString(png)})
}

// WechatPoll GET /api/auth/wechat/poll — 轮询扫码票据(契约 authWechatPoll)。
// confirmed 时直接落会话 cookie。
func (s *Service) WechatPoll(c *gin.Context) {
	ticket := c.Query("ticket")
	status, uid, err := s.Wx.PollTicket(c.Request.Context(), ticket)
	if err != nil {
		jsonFail(c, 500, "poll", "")
		return
	}
	out := gin.H{"status": status}
	if status == "confirmed" {
		u, err := store.UserByID(c.Request.Context(), s.DB, uid)
		if err == nil {
			out["user"] = gin.H{"id": u.ID, "name": u.Name}
		}
		s.setCookie(c, uid, 0)
	}
	c.JSON(200, out)
}

// WechatConfirm POST /api/auth/wechat/confirm — 小程序端确认(契约 authWechatConfirm)。
// 微信身份未绑定时自动建号;邀请制实例需携带 invite(小程序表单可填)。
func (s *Service) WechatConfirm(c *gin.Context) {
	if s.Wx == nil || !s.Wx.Enabled() {
		jsonFail(c, 503, "wechat login is not configured", "wechat-off")
		return
	}
	b := readBody(c)
	if b.Ticket == "" || b.WxCode == "" {
		jsonFail(c, 400, "ticket and wxCode required", "")
		return
	}
	ct := c.Request.Context()
	openid, err := s.Wx.Code2Session(ct, b.WxCode)
	if err != nil {
		jsonFail(c, 401, err.Error(), "wechat")
		return
	}
	uid, bound, err := store.UserByIdentity(ct, s.DB, store.IdWx, openid)
	if err != nil {
		jsonFail(c, 500, "lookup", "")
		return
	}
	isNew := false
	if !bound {
		if s.inviteOnly(c) {
			if b.Invite == "" {
				jsonFail(c, 403, "a valid invite code is required", "invite")
				return
			}
			if err := store.ConsumeInvite(ct, s.DB, strings.ToUpper(strings.TrimSpace(b.Invite)), ""); err != nil {
				jsonFail(c, 403, "a valid invite code is required", "invite")
				return
			}
		}
		uid = newUID()
		name := "沐禾用户" + openid[len(openid)-4:]
		if err := store.InsertUser(ct, s.DB, uid, name); err != nil {
			jsonFail(c, 500, "save", "")
			return
		}
		isNew = true
	}
	if err := store.CreateIdentity(ct, s.DB, uid, store.IdWx, openid); err != nil {
		jsonFail(c, 500, "identity", "")
		return
	}
	if err := s.Wx.ConfirmTicket(ct, b.Ticket, uid); err != nil {
		jsonFail(c, 404, err.Error(), "ticket")
		return
	}
	s.audit(c, uid, "auth.wx.confirm", gin.H{"isNew": isNew})
	c.JSON(200, gin.H{"ok": true, "user": gin.H{"id": uid, "name": name2(uid, s, ct)}, "isNew": isNew})
}

func name2(uid string, s *Service, ct context.Context) string {
	u, err := store.UserByID(ct, s.DB, uid)
	if err != nil {
		return uid
	}
	return u.Name
}

// StepUpValidate POST /api/admin/step-up 的短信码核验(admin 包调用;窗口机制在 admin.Service)。
// phone 必须已绑定到当前管理员账号。
func (s *Service) StepUpValidate(c *gin.Context, uid, phone, code string) bool {
	if !validPhone(phone) {
		return false
	}
	owner, bound, _ := store.UserByIdentity(c.Request.Context(), s.DB, store.IdPhone, phone)
	if !bound || owner != uid {
		return false
	}
	return s.SmsStore.Verify(c.Request.Context(), phone, code)
}

// SmsCodeStore / SmsSender 暴露给 admin 包(step-up 发码)。
func (s *Service) SmsDeps() (sms.Store, sms.Sender) { return s.SmsStore, s.Sms }

// newUID 16 字符 base64url 随机账号 id(与 Node uid 同长度量级)。
func newUID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// defaultName 手机号建号的默认昵称。
func defaultName(phone string) string {
	if len(phone) >= 4 {
		return "健身者" + phone[len(phone)-4:]
	}
	return "健身者"
}

func fastrand() uint64 {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
		uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])
}
