// media 单测:引用集与 api/test/fixtures/media-refs.json 共享 fixture(两栈同一答案);
// 文件名带扩展(Node 兼容)、GC 的 marks/grace、配额。
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func shaOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

const fixture = "../../../api/test/fixtures/media-refs.json"

func TestReferencedHashesMatchesSharedFixture(t *testing.T) {
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	var f struct {
		Cases []struct {
			Name   string         `json:"name"`
			State  map[string]any `json:"state"`
			Expect []string       `json:"expect"`
		} `json:"referencedHashes"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	for _, c := range f.Cases {
		got := ReferencedHashes(c.State)
		if len(got) != len(c.Expect) {
			t.Errorf("%s: got %d hashes, want %d (%v)", c.Name, len(got), len(c.Expect), c.Expect)
			continue
		}
		for _, h := range c.Expect {
			if !got[h] {
				t.Errorf("%s: missing %s", c.Name, h)
			}
		}
	}
}

func testService(t *testing.T) *Service {
	t.Helper()
	return &Service{Dir: t.TempDir(), Enabled: true, QuotaBytes: 1 << 30}
}

// 一个最小合法 webp:RIFF + 4 字节长度 + "WEBP"(Sniff 只看这两个位置)。
func webp(tag string) []byte {
	return append([]byte("RIFF\x00\x00\x00\x00WEBPVTFT"), []byte(tag)...)
}

func TestStoreNamesFileWithExtension(t *testing.T) {
	s := testService(t)
	data := webp("alpha")
	if ok, code := s.Store("u1", shaOf(data), data); !ok {
		t.Fatalf("store: %s", code)
	}
	// 目录里是 <hash>.webp(Node 同形),裸名不存在。
	entries, _ := os.ReadDir(filepath.Join(s.Dir, "u1"))
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	if !names[shaOf(data)+".webp"] {
		t.Fatalf("want %s.webp on disk, have %v", shaOf(data), names)
	}
	if p, err := s.File("u1", shaOf(data)); err != nil || !strings.HasSuffix(p, ".webp") {
		t.Fatalf("File: %v %v", p, err)
	}
}

func TestSweepKeepsReferencedRemovesRest(t *testing.T) {
	s := testService(t)
	keep, drop := webp("keep"), webp("drop")
	for _, d := range [][]byte{keep, drop} {
		if ok, code := s.Store("u1", shaOf(d), d); !ok {
			t.Fatalf("store: %s", code)
		}
	}
	refs := map[string]bool{shaOf(keep): true}
	removed, freed := s.Sweep("u1", refs, 0)
	if removed != 1 || freed != int64(len(drop)) {
		t.Fatalf("removed=%d freed=%d, want 1/%d", removed, freed, len(drop))
	}
	if _, err := s.File("u1", shaOf(keep)); err != nil {
		t.Fatal("referenced file was removed")
	}
	if _, err := s.File("u1", shaOf(drop)); err == nil {
		t.Fatal("unreferenced file survived")
	}
	u := s.Usage("u1")
	if u["count"] != 1 || u["bytes"] != int64(len(keep)) {
		t.Fatalf("usage: %v", u)
	}
}

func TestSweepGraceWindowHoldsFreshUnreferenced(t *testing.T) {
	s := testService(t)
	data := webp("later")
	s.Store("u1", shaOf(data), data)
	// 首轮 sweep 给未引用文件打 mark(今天),14 天窗口内不删。
	if removed, _ := s.Sweep("u1", map[string]bool{}, gcGraceDays*24*3600*1000); removed != 0 {
		t.Fatalf("grace should hold, removed=%d", removed)
	}
	// 窗口放宽到 0:立即删。
	if removed, _ := s.Sweep("u1", map[string]bool{}, 0); removed != 1 {
		t.Fatalf("grace=0 should remove, removed=%d", removed)
	}
}

func TestStoreEnforcesQuota(t *testing.T) {
	s := testService(t)
	s.QuotaBytes = int64(len(webp("one")) + 1) // 只够一个文件
	first := webp("one")
	if ok, code := s.Store("u1", shaOf(first), first); !ok {
		t.Fatalf("first: %s", code)
	}
	second := webp("two-over-quota")
	if ok, code := s.Store("u1", shaOf(second), second); ok || code != "media-quota" {
		t.Fatalf("second: ok=%v code=%s, want media-quota", ok, code)
	}
	// 同一文件的重复上传是幂等的,不占第二份配额。
	if ok, code := s.Store("u1", shaOf(first), first); !ok {
		t.Fatalf("re-store same bytes: %s", code)
	}
}
