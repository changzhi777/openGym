// challenge store — 与 Node 版同构:cid(base64url 16B)→ 一次性领取,5 分钟 TTL,
// 上限逐出最旧。kind 区分四种仪式(register/login/passkey-add/device-link)。
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

type Challenge struct {
	Kind      string
	Challenge string // base64url,WebAuthn challenge
	UID       string
	Name      string
	Code      string // invite code(register 用)
	Exp       time.Time
}

type ChallengeStore struct {
	mu   sync.Mutex
	m    map[string]*Challenge
	max  int
	ttl  time.Duration
}

func NewChallengeStore() *ChallengeStore {
	return &ChallengeStore{m: make(map[string]*Challenge), max: 500, ttl: 5 * time.Minute}
}

func (s *ChallengeStore) Put(data *Challenge) string {
	raw := make([]byte, 16)
	rand.Read(raw)
	cid := base64.RawURLEncoding.EncodeToString(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.m) >= s.max { // 与 Node 相同:超限删最旧(插入序近似)
		for k := range s.m {
			delete(s.m, k)
			break
		}
	}
	data.Exp = time.Now().Add(s.ttl)
	s.m[cid] = data
	return cid
}

// Take 一次性取出;过期/不存在返回 nil。
func (s *ChallengeStore) Take(cid string) *Challenge {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.m[cid]
	delete(s.m, cid)
	if !ok || time.Now().After(c.Exp) {
		return nil
	}
	return c
}
