// invite_test.go — 团码内存后端语义保护网(零 Redis 零端口):
// 码形/哈希存储(明文不落)/一团一活码/一次性兑换/TTL/撤销。
package team

import (
	"crypto/sha256"
	"regexp"
	"strings"
	"testing"
	"time"
)

func memoryInviteStore(t *testing.T) *InviteStore {
	t.Helper()
	t.Setenv("REDIS_ADDR", "") // 强制内存回退
	return NewInviteStore()
}

func TestTeamCodeFormat(t *testing.T) {
	re := regexp.MustCompile(`^[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{4}-[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{4}$`)
	for i := 0; i < 20; i++ {
		code := newTeamCode()
		if !re.MatchString(code) {
			t.Fatalf("团码形 XXXX-XXXX 且排除易混字符, got %q", code)
		}
		if strings.ContainsAny(code, "0O1I") {
			t.Fatalf("不得含 0/O/1/I, got %q", code)
		}
	}
}

func TestInviteHashOnlyStorage(t *testing.T) {
	s := memoryInviteStore(t)
	code := s.NewCode("team-1")
	h := sha256.Sum256([]byte(code))
	if _, ok := s.m[h]; !ok {
		t.Fatal("存储键应为 sha256(明文码)")
	}
	if len(s.m) != 1 {
		t.Fatalf("应恰存一条, got %d", len(s.m))
	}
	if e := s.m[h]; e.TeamID != "team-1" || !e.Exp.After(time.Now()) {
		t.Fatalf("entry 应含 teamID 与未过期 TTL, got %+v", e)
	}
}

func TestInviteOneActivePerTeam(t *testing.T) {
	s := memoryInviteStore(t)
	c1 := s.NewCode("team-1")
	c2 := s.NewCode("team-1") // 旧的作废
	if s.Take(c1) != "" {
		t.Fatal("旧团码应已作废")
	}
	if got := s.Take(c2); got != "team-1" {
		t.Fatalf("新团码应兑出 teamID, got %q", got)
	}
}

func TestInviteIndependentTeams(t *testing.T) {
	s := memoryInviteStore(t)
	c1 := s.NewCode("team-1")
	if got := s.Take(s.NewCode("team-2")); got != "team-2" {
		t.Fatalf("他团出码不得波及, got %q", got)
	}
	if s.Take(c1) != "team-1" {
		t.Fatal("team-1 活码应不受影响")
	}
}

func TestInviteTakeIsOneTime(t *testing.T) {
	s := memoryInviteStore(t)
	code := s.NewCode("team-1")
	if got := s.Take(code); got != "team-1" {
		t.Fatalf("首取应得 teamID, got %q", got)
	}
	if s.Take(code) != "" {
		t.Fatal("一次性:二取应为空")
	}
}

func TestInviteRevokeTeam(t *testing.T) {
	s := memoryInviteStore(t)
	code := s.NewCode("team-1")
	s.RevokeTeam("team-1")
	if s.Take(code) != "" {
		t.Fatal("解散应全灭该团活码")
	}
}

func TestNormalizeCode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"k7wq2mzp", "K7WQ-2MZP"},      // 小写 → 大写 + 补横线
		{"K7WQ-2MZP", "K7WQ-2MZP"},     // 已规范
		{" k7 wq 2m zp ", "K7WQ-2MZP"}, // 空格混杂
		{"K7WQ2M", "K7WQ2M"},           // 非 8 位原样(大写化)
	}
	for _, c := range cases {
		if got := NormalizeCode(c.in); got != c.want {
			t.Fatalf("NormalizeCode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
