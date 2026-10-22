// 配对码:浏览器登录态签发 → 手机 App 兑换成 Bearer token(与 Node pairings Map 同义)。
// 每 profile 一个活码;5 分钟;一次性。
package auth

import (
	"crypto/rand"
	"math/big"
	"sync"
	"time"
)

type PairStore struct {
	mu sync.Mutex
	m  map[string]pairEntry // code → entry
}

type pairEntry struct {
	UID string
	Exp time.Time
}

func NewPairStore() *PairStore {
	return &PairStore{m: make(map[string]pairEntry)}
}

const pairAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// NewCode 每用户一码(旧的作废),形如 XXXX-XXXX(Node makePairCode 同风格)。
func (p *PairStore) NewCode(uid string) string {
	raw := make([]byte, 8)
	for i := range raw {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(pairAlphabet))))
		raw[i] = pairAlphabet[n.Int64()]
	}
	code := string(raw[0:4]) + "-" + string(raw[4:8])
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range p.m { // 一人一活码
		if v.UID == uid {
			delete(p.m, k)
		}
	}
	p.m[code] = pairEntry{UID: uid, Exp: time.Now().Add(5 * time.Minute)}
	return code
}

// Take 一次性取出;过期/不存在返回空 uid。
func (p *PairStore) Take(code string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.m[code]
	delete(p.m, code)
	if !ok || time.Now().After(e.Exp) {
		return ""
	}
	return e.UID
}
