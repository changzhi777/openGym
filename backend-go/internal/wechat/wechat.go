// Package wechat — 小程序扫码登录的微信侧(契约 authWechatQrcode/Poll/Confirm)。
//
// 流程:PC 拉票据(/qrcode 出小程序码) → 用户微信扫码进小程序 → 小程序 wx.login
// 拿 code 调 /api/auth/wechat/confirm → 本包 code2session 换 openid → 绑定账号
// 或自动建号 → 票据翻 confirmed → PC 轮询拿到会话。
// 凭据:WX_MINI_APPID/WX_MINI_SECRET;未配置时 QRCode/Confirm 答 503。
package wechat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"bytes"
	"crypto/rand"
	"io"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	ticketTTL   = 5 * time.Minute
	atKey       = "wx:access_token"
	atTTL       = 7000 * time.Second
	ticketPre   = "wx:ticket:"
	ErrDisabled = swerr("wechat: not configured")
	ErrTicket   = swerr("wechat: unknown or expired ticket")
)

type swerr string

func (e swerr) Error() string { return string(e) }

type MiniProgram struct {
	AppID      string
	Secret     string
	EnvVersion string // 出码目标版本:release(默认,线上正式版)|trial(体验版)|develop(开发版)
	Rdb        *redis.Client
	HTTP       *http.Client
}

func New(appID, secret, envVersion string, rdb *redis.Client) *MiniProgram {
	return &MiniProgram{AppID: appID, Secret: secret, EnvVersion: envVersion, Rdb: rdb,
		HTTP: &http.Client{Timeout: 10 * time.Second}}
}

func (m *MiniProgram) Enabled() bool { return m.AppID != "" && m.Secret != "" && m.Rdb != nil }

// MintTicket 出一张一次性扫码票据(pending,TTL 5 分钟)。
// ⚠️ 长度:scene 上限 32 字符,scene = "t=" + ticket ≤ 32 → ticket ≤ 30;
// 曾用 wx+UnixNano(19)+hex(12)=33 超限出码 40169。改纯随机 28(wx+hex14)。
func (m *MiniProgram) MintTicket(ctx context.Context) (ticket string, err error) {
	ticket = "wx" + randHex(14)
	err = m.Rdb.Set(ctx, ticketPre+ticket, "pending", ticketTTL).Err()
	return ticket, err
}

// ConfirmTicket 小程序确认:核销票据并记录账号(openid 已由调用方解析/建号)。
func (m *MiniProgram) ConfirmTicket(ctx context.Context, ticket, uid string) error {
	key := ticketPre + ticket
	v, err := m.Rdb.Get(ctx, key).Result()
	if err != nil {
		return ErrTicket
	}
	if v != "pending" {
		return ErrTicket
	}
	return m.Rdb.Set(ctx, key, "confirmed:"+uid, ticketTTL).Err()
}

// PollTicket PC 轮询:pending / confirmed(uid) / expired。
func (m *MiniProgram) PollTicket(ctx context.Context, ticket string) (status string, uid string, err error) {
	v, err := m.Rdb.Get(ctx, ticketPre + ticket).Result()
	if err == redis.Nil {
		return "expired", "", nil
	}
	if err != nil {
		return "", "", err
	}
	if v == "pending" {
		return "pending", "", nil
	}
	if len(v) > len("confirmed:") && v[:len("confirmed:")] == "confirmed:" {
		return "confirmed", v[len("confirmed:"):], nil
	}
	return "expired", "", nil
}

// Code2Session wx.login 的 code 换 openid(GET sns/jscode2session)。
func (m *MiniProgram) Code2Session(ctx context.Context, wxCode string) (openid string, err error) {
	url := fmt.Sprintf(
		"https://api.weixin.qq.com/sns/jscode2session?appid=%s&secret=%s&js_code=%s&grant_type=authorization_code",
		m.AppID, m.Secret, wxCode)
	var out struct {
		OpenID  string `json:"openid"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err = getJSON(ctx, m.HTTP, url, &out); err != nil {
		return "", err
	}
	if out.ErrCode != 0 || out.OpenID == "" {
		return "", fmt.Errorf("wechat jscode2session %d: %s", out.ErrCode, out.ErrMsg)
	}
	return out.OpenID, nil
}

// QRCodePNG getUnlimitedQRCode:scene 携带 ticket 的 小程序码 PNG(base64 由调用方包)。
// 凭据失效(40001/42001——外部刷新会作废 Redis 缓存)时强刷 token 重试一次。
func (m *MiniProgram) QRCodePNG(ctx context.Context, ticket string) (png []byte, err error) {
	at, err := m.accessToken(ctx, false)
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		png, err = m.qrCall(ctx, at, ticket)
		if err == nil {
			return png, nil
		}
		var te tokenErr
		if attempt > 0 || !errors.As(err, &te) || (te.Code != 40001 && te.Code != 42001) {
			return nil, err
		}
		if at, err = m.accessToken(ctx, true); err != nil {
			return nil, err
		}
	}
}

type tokenErr struct {
	Code int
}

func (e tokenErr) Error() string { return fmt.Sprintf("wechat qrcode %d", e.Code) }

func (m *MiniProgram) qrCall(ctx context.Context, at, ticket string) ([]byte, error) {
	body, _ := json.Marshal(map[string]any{
		"scene": "t=" + ticket,
		"page":  "pages/login-confirm/login-confirm", // 小程序确认页(对接文档 §3)
		"width": 430,
		// ⚠️ 必须布尔:曾传字符串 "false" 被微信当 true 校验路径 → 41030 page 不存在
		"check_path": m.EnvVersion == "" || m.EnvVersion == "release",
		// 联调期出体验版/开发版码(WX_MINI_ENV=trial|develop);线上发布后清空 env 走正式版
		"env_version": m.EnvVersion,
	})
	url := "https://api.weixin.qq.com/wxa/getwxacodeunlimit?access_token=" + at
	resp, err := m.HTTP.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	png, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// 接口出错时返回 JSON 而非图片:以 content-type 或 JSON 前缀甄别
	if len(png) > 0 && png[0] == '{' {
		var e struct {
			ErrCode int    `json:"errcode"`
			ErrMsg  string `json:"errmsg"`
		}
		_ = json.Unmarshal(png, &e)
		if e.ErrCode == 40001 || e.ErrCode == 42001 {
			return nil, tokenErr{Code: e.ErrCode}
		}
		return nil, fmt.Errorf("wechat qrcode %d: %s", e.ErrCode, e.ErrMsg)
	}
	return png, nil
}

// accessToken 稳定获取接口调用凭据(Redis 缓存;force=true 跳过缓存强刷)。
func (m *MiniProgram) accessToken(ctx context.Context, force bool) (string, error) {
	if !force {
		if v, err := m.Rdb.Get(ctx, atKey).Result(); err == nil && v != "" {
			return v, nil
		}
	}
	url := fmt.Sprintf(
		"https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s",
		m.AppID, m.Secret)
	var out struct {
		AccessToken string `json:"access_token"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := getJSON(ctx, m.HTTP, url, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("wechat token %d: %s", out.ErrCode, out.ErrMsg)
	}
	m.Rdb.Set(ctx, atKey, out.AccessToken, atTTL)
	return out.AccessToken, nil
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func getJSON(ctx context.Context, c *http.Client, url string, into any) error {
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, into)
}
