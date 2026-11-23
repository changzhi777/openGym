// Package sms — 腾讯云 SMS 验证码发送与频控(契约 authSmsSend)。
//
// TC3-HMAC-SHA256 签名用标准库实现,不引官方 SDK(backend-go 依赖克制纪律;
// 官方 SDK 体积/模块校验在 CI 不可用)。未配 SMS_SECRET_ID 时为 dev 模式:
// 不发短信,验证码打进日志(本地/联调用)。
package sms

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Sender 腾讯云 SMS 配置(env: SMS_SECRET_ID/KEY/APP_ID/SIGN/TEMPLATE_ID/REGION)。
type Sender struct {
	SecretID   string
	SecretKey  string
	AppID      string // SmsSdkAppId,如 1400xxxxxx
	Sign       string // 签名内容,如 沐禾健身
	TemplateID string // 验证码模板 ID
	Region     string // 默认 ap-guangzhou
	Enabled    bool   // 全部凭据就绪;false = dev 模式(码打日志不发送)
}

// FromEnv 读取配置;Enabled=false = dev 模式(不真正发送)。
func FromEnv() Sender {
	s := Sender{
		SecretID:   os.Getenv("SMS_SECRET_ID"),
		SecretKey:  os.Getenv("SMS_SECRET_KEY"),
		AppID:      os.Getenv("SMS_APP_ID"),
		Sign:       os.Getenv("SMS_SIGN"),
		TemplateID: os.Getenv("SMS_TEMPLATE_ID"),
		Region:     orDefault(os.Getenv("SMS_REGION"), "ap-guangzhou"),
	}
	s.Enabled = s.SecretID != "" && s.SecretKey != "" && s.AppID != "" &&
		s.Sign != "" && s.TemplateID != ""
	return s
}

const apiHost = "sms.tencentcloudapi.com"

// SendCode 发送验证码短信。返回 dev 标记(供端点应答 dev:true)。
func (s Sender) SendCode(ctx context.Context, phone, code string) (dev bool, err error) {
	if !s.Enabled {
		log.Printf("[sms:dev] 沐禾健身验证码 %s(未配 SMS_*,dev 模式不发送)", code)
		return true, nil
	}
	body, _ := json.Marshal(map[string]any{
		"PhoneNumberSet":    []string{"+86" + phone},
		"SmsSdkAppId":       s.AppID,
		"SignName":          s.Sign,
		"TemplateId":        s.TemplateID,
		"TemplateParamSet":  []string{code},
		"SessionContext":    "muhe",
	})
	out, err := s.tc3(ctx, "SendSms", body)
	if err != nil {
		return false, err
	}
	// SendStatusSet[0].Code == "Ok" 才算成功
	var resp struct {
		Response struct {
			SendStatusSet []struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"SendStatusSet"`
			Error *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return false, err
	}
	if resp.Response.Error != nil {
		return false, fmt.Errorf("sms: %s %s", resp.Response.Error.Code, resp.Response.Error.Message)
	}
	if len(resp.Response.SendStatusSet) == 0 || resp.Response.SendStatusSet[0].Code != "Ok" {
		msg := "unknown"
		if len(resp.Response.SendStatusSet) > 0 {
			msg = resp.Response.SendStatusSet[0].Message
		}
		return false, fmt.Errorf("sms send failed: %s", msg)
	}
	return false, nil
}

// tc3 TC3-HMAC-SHA256 签名 POST(https://cloud.tencent.com/document/api/382/52071)。
func (s Sender) tc3(ctx context.Context, action string, body []byte) ([]byte, error) {
	ts := time.Now().Unix()
	date := time.Unix(ts, 0).UTC().Format("2006-01-02")
	region := orDefault(s.Region, "ap-guangzhou")

	payloadHash := sha256.Sum256(body)
	payloadHex := hex.EncodeToString(payloadHash[:])
	canonicalHeaders := fmt.Sprintf("content-type:application/json; charset=utf-8\nhost:%s\nx-tc-action:%s\n",
		strings.ToLower(apiHost), strings.ToLower(action))
	signedHeaders := "content-type;host;x-tc-action"
	canonicalRequest := fmt.Sprintf("POST\n/\n\n%s\n%s\n%s", canonicalHeaders, signedHeaders, payloadHex)

	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, "sms")
	hash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := fmt.Sprintf("TC3-HMAC-SHA256\n%d\n%s\n%s", ts, credentialScope, hex.EncodeToString(hash[:]))

	secretDate := hmacSHA256([]byte("TC3"+s.SecretKey), []byte(date))
	secretService := hmacSHA256(secretDate, []byte("sms"))
	secretSigning := hmacSHA256(secretService, []byte("tc3_request"))
	signature := hex.EncodeToString(hmacSHA256(secretSigning, []byte(stringToSign)))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://"+apiHost+"/", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", apiHost)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", "2021-01-11")
	req.Header.Set("X-TC-Region", region)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-TC-Language", "zh-CN")
	req.Header.Set("Authorization", fmt.Sprintf(
		"TC3-HMAC-SHA256 Credential=%s, SignedHeaders=%s, Signature=%s",
		s.SecretID+"/"+credentialScope, signedHeaders, signature))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sms http %d", resp.StatusCode)
	}
	return buf.Bytes(), nil
}

func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

// ── 防刷频控(Redis;契约 §Throttling:5min/60s/单号日限10/地址日限30) ─────────

const (
	codeTTL    = 5 * time.Minute
	resendWait = 60 * time.Second
	phoneDaily = 10
	addrDaily  = 30
	failLock   = 5 // 连错 N 次锁号 1min 起倍增
)

// Store 频控与验证码的 Redis 键空间。
type Store struct{ Rdb *redis.Client }

// PutCode 落验证码(TTL 5 分钟)。
func (st Store) PutCode(ctx context.Context, phone, code string) {
	st.Rdb.Set(ctx, "sms:code:"+phone, code, codeTTL)
	st.Rdb.Del(ctx, "sms:fail:"+phone)
}

// GetCode 读当前验证码;无 ok=false。
func (st Store) GetCode(ctx context.Context, phone string) (string, bool) {
	v, err := st.Rdb.Get(ctx, "sms:code:"+phone).Result()
	if err != nil || v == "" {
		return "", false
	}
	return v, true
}

// AllowSend 发码闸门:60s 重发间隔 + 单号日限 + 地址日限。
// 返回 retryAfter 秒(0=放行)。
func (st Store) AllowSend(ctx context.Context, phone, addr string) (retryAfter int) {
	day := time.Now().Format("20060102")
	if st.Rdb.Exists(ctx, "sms:last:"+phone).Val() == 1 {
		return 60
	}
	if n := st.Rdb.Incr(ctx, "sms:day:"+phone+":"+day).Val(); n == 1 {
		st.Rdb.Expire(ctx, "sms:day:"+phone+":"+day, 26*time.Hour)
	} else if n > phoneDaily {
		return 86400
	}
	if n := st.Rdb.Incr(ctx, "sms:ipday:"+addr+":"+day).Val(); n == 1 {
		st.Rdb.Expire(ctx, "sms:ipday:"+addr+":"+day, 26*time.Hour)
	} else if n > addrDaily {
		return 86400
	}
	st.Rdb.Set(ctx, "sms:last:"+phone, "1", resendWait)
	return 0
}

// Verify 核验验证码;通过即消费。连错 failLock 次锁号(1min 起倍增)。
func (st Store) Verify(ctx context.Context, phone, code string) bool {
	want, ok := st.GetCode(ctx, phone)
	if !ok || want != code {
		n := st.Rdb.Incr(ctx, "sms:fail:"+phone).Val()
		if n == 1 {
			st.Rdb.Expire(ctx, "sms:fail:"+phone, time.Hour)
		}
		if n > failLock {
			st.Rdb.Set(ctx, "sms:lock:"+phone, "1", time.Duration(n-failLock)*time.Minute)
			// 连错超限:当前码一并作废——锁定窗口内即使穷举命中也无效(2026-11-18
			// 审查轮:verify 侧此前不受锁约束,6 位码 5 分钟 TTL 是可穷举空间)。
			st.Rdb.Del(ctx, "sms:code:"+phone)
		}
		return false
	}
	st.Rdb.Del(ctx, "sms:code:"+phone, "sms:fail:"+phone)
	return true
}

// Locked 号是否处于连错锁定中。
func (st Store) Locked(ctx context.Context, phone string) bool {
	return st.Rdb.Exists(ctx, "sms:lock:"+phone).Val() == 1
}
