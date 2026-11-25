// invite.go — 团码(G4 双后端,设计 §13.3):8 位无歧义码(auth/pair 码表同源),
// 只存 sha256 哈希(device-link 同款),一团长一活码,TTL 15min,一次性兑换即烧。
package team

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const inviteTTL = 15 * time.Minute

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 与 auth/pair 码表同源(0/O/1/I 排除)

// NormalizeCode 手输码清洗(§13.3 PairViewModel.clean 同构):去空格/横线 + 大写化;
// 8 位时补回横线成存储形状。导出面:端点层与测试共用(契约级清洗行为)。
func NormalizeCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(s)) {
		if r == ' ' || r == '-' {
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if len(out) == 8 {
		return out[:4] + "-" + out[4:]
	}
	return out
}

func newTeamCode() string {
	raw := make([]byte, 8)
	for i := range raw {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		raw[i] = codeAlphabet[n.Int64()]
	}
	return string(raw[0:4]) + "-" + string(raw[4:8])
}

type inviteEntry struct {
	TeamID string
	Exp    time.Time
}

type InviteStore struct {
	mu  sync.Mutex
	m   map[[32]byte]inviteEntry // sha256(code) → entry(内存后端;键恒为哈希,明文不落)
	rdb *redis.Client            // 非 nil 时走 Redis(G4:重启不丢码)
}

func NewInviteStore() *InviteStore {
	s := &InviteStore{m: make(map[[32]byte]inviteEntry)}
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		rdb := redis.NewClient(&redis.Options{Addr: addr})
		if err := rdb.Ping(context.Background()).Err(); err == nil {
			s.rdb = rdb
		} else {
			rdb.Close()
			log.Println("team invite store: redis 不可达,回退内存(重启丢团码)")
		}
	}
	return s
}

func inviteKey(code string) string {
	h := sha256.Sum256([]byte(code))
	return "team:invite:" + hex.EncodeToString(h[:])
}

// NewCode 一团一活码(旧的作废);明文码只出现在返回值,存储侧恒为哈希。
func (s *InviteStore) NewCode(teamID string) string {
	code := newTeamCode()
	if s.rdb != nil {
		ctx := context.Background()
		iter := s.rdb.Scan(ctx, 0, "team:invite:*", 100).Iterator()
		for iter.Next(ctx) {
			if v, err := s.rdb.Get(ctx, iter.Val()).Result(); err == nil && v == teamID {
				s.rdb.Del(ctx, iter.Val())
			}
		}
		s.rdb.Set(ctx, inviteKey(code), teamID, inviteTTL)
		return code
	}
	h := sha256.Sum256([]byte(code))
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.m {
		if v.TeamID == teamID {
			delete(s.m, k)
		}
	}
	s.m[h] = inviteEntry{TeamID: teamID, Exp: time.Now().Add(inviteTTL)}
	return code
}

// Take 一次性兑换(烧码);过期/不存在返回空串。
func (s *InviteStore) Take(code string) string {
	if s.rdb != nil {
		v, err := s.rdb.GetDel(context.Background(), inviteKey(code)).Result()
		if err != nil || v == "" {
			return ""
		}
		return v
	}
	h := sha256.Sum256([]byte(code))
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[h]
	delete(s.m, h)
	if !ok || time.Now().After(e.Exp) {
		return ""
	}
	return e.TeamID
}

// RevokeTeam 解散/重置时全灭该团活码。
func (s *InviteStore) RevokeTeam(teamID string) {
	if s.rdb != nil {
		ctx := context.Background()
		iter := s.rdb.Scan(ctx, 0, "team:invite:*", 100).Iterator()
		for iter.Next(ctx) {
			if v, err := s.rdb.Get(ctx, iter.Val()).Result(); err == nil && v == teamID {
				s.rdb.Del(ctx, iter.Val())
			}
		}
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.m {
		if v.TeamID == teamID {
			delete(s.m, k)
		}
	}
}
