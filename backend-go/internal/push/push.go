// Package push — Web Push(VAPID + 发送 + 休息计时调度)。
// VAPID 键与 Node 共用 data/vapid.json(灰度期两栈可互发);文件缺失时生成并 0600 落盘。
package push

import (
	"encoding/json"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type VAPID struct {
	PublicKey  string `json:"publicKey"`
	PrivateKey string `json:"privateKey"`
}

type Service struct {
	V      VAPID
	mu     sync.Mutex
	timers map[string]*time.Timer
}

// New 读/生成 data/vapid.json(Node 同文件格式)。
func New(dataDir string) *Service {
	path := filepath.Join(dataDir, "vapid.json")
	s := &Service{timers: map[string]*time.Timer{}}
	if raw, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(raw, &s.V) == nil && s.V.PublicKey != "" {
			return s
		}
	}
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Printf("vapid 生成失败: %v", err)
		return s
	}
	s.V = VAPID{PublicKey: pub, PrivateKey: priv}
	raw, _ := json.Marshal(s.V)
	os.WriteFile(path, raw, 0o600)
	return s
}

// Send 发一条推送;401/410 由调用方删订阅(Node sendPush 同义)。
func (s *Service) Send(sub Endpoint, payload []byte) (int, error) {
	resp, err := webpush.SendNotification(payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		VAPIDPublicKey:  s.V.PublicKey,
		VAPIDPrivateKey: s.V.PrivateKey,
		TTL:             3600,
		Urgency:         webpush.Urgency(defaultUrgency(payload)),
	})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func defaultUrgency(p []byte) string {
	var m map[string]any
	if json.Unmarshal(p, &m) == nil {
		if u, ok := m["urgency"].(string); ok {
			return u
		}
	}
	return "normal"
}

type Endpoint struct{ Endpoint, P256dh, Auth string }

// ScheduleRest 休息计时推送(同 device 新计时覆盖旧的)。
func (s *Service) ScheduleRest(key string, d time.Duration, fire func()) {
	s.mu.Lock()
	if t, ok := s.timers[key]; ok {
		t.Stop()
	}
	s.timers[key] = time.AfterFunc(d, func() {
		s.mu.Lock()
		delete(s.timers, key)
		s.mu.Unlock()
		fire()
	})
	s.mu.Unlock()
}

func (s *Service) CancelRest(key string) {
	s.mu.Lock()
	if t, ok := s.timers[key]; ok {
		t.Stop()
		delete(s.timers, key)
	}
	s.mu.Unlock()
}

// EndpointError Node pushEndpointError 同义:https + 无凭据 + 非私网字面地址。
func EndpointError(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "endpoint is not a valid URL"
	}
	if u.Scheme != "https" {
		return "endpoint must be an https:// URL"
	}
	if u.User != nil {
		return "endpoint must not carry credentials"
	}
	h := strings.Trim(u.Hostname(), "[]")
	if strings.Contains(h, ":") || isNumericHost(h) {
		if isPrivate(h) {
			return "endpoint must not point at a private address"
		}
	}
	return ""
}

func isNumericHost(h string) bool {
	for _, c := range h {
		if !(c == '.' || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return h != ""
}

func isPrivate(h string) bool {
	return strings.HasPrefix(h, "10.") || strings.HasPrefix(h, "127.") ||
		strings.HasPrefix(h, "192.168.") || strings.HasPrefix(h, "169.254.") ||
		strings.HasPrefix(h, "172.16.") || strings.HasPrefix(h, "172.17.") ||
		strings.HasPrefix(h, "172.18.") || strings.HasPrefix(h, "172.19.") ||
		strings.HasPrefix(h, "172.2") || strings.HasPrefix(h, "172.30.") ||
		strings.HasPrefix(h, "172.31.") || h == "0.0.0.0" || strings.HasPrefix(h, "::1") ||
		strings.HasPrefix(h, "fe80") || strings.HasPrefix(h, "fc") || strings.HasPrefix(h, "fd")
}
