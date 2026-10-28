// Package media — 自定义动作媒体存储(P4 核心校验读写 + P5 GC/配额)。
// 文件布局与 Node 兼容:<DATA_DIR>/media/<uid>/<sha256>.<ext>,marks 走同格式 .gc.json,
// 灰度期两栈可互为读写互为回收。
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	MaxImageMB = 2
	MaxGifMB   = 8
	MaxVideoMB = 40
	MaxTotal   = 40 * 1024 * 1024 // 单请求上限(宽松,类型上限在 sniff 后精准判)

	gcGraceDays = 14 // 与 Node MEDIA_GC_GRACE_DAYS 默认一致(后台回收用;sweep 端点传 0)
)

type Service struct {
	Dir          string
	Enabled      bool
	QuotaBytes   int64 // 0 = 不限(与 Node MEDIA_QUOTA_MB=0 同义)
	VideoSec     int   // 视频时长上限(+1s 宽限,与 Node 同)
	MinFreeBytes int64 // 磁盘水位下限,低于则拒收;0 = 不查
}

func New(dataDir string) *Service {
	quota := int64(200)
	if v := os.Getenv("MEDIA_QUOTA_MB"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			quota = n
		}
	}
	videoSec := 60
	if v := os.Getenv("MEDIA_VIDEO_MAX_SEC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			videoSec = n
		}
	}
	minFree := int64(512)
	if v := os.Getenv("MEDIA_MIN_FREE_MB"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			minFree = n
		}
	}
	return &Service{
		Dir: filepath.Join(dataDir, "media"), Enabled: os.Getenv("MEDIA_UPLOADS") != "0",
		QuotaBytes: quota * 1024 * 1024, VideoSec: videoSec, MinFreeBytes: minFree * 1024 * 1024,
	}
}

// Sniff 魔数识别(P4:webp/gif/mp4/jpeg/png;返回类型与大小上限)。
func Sniff(head []byte) (ext string, limitMB int, ok bool) {
	if len(head) >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "WEBP" {
		return "webp", MaxImageMB, true
	}
	if len(head) >= 6 && (string(head[0:6]) == "GIF87a" || string(head[0:6]) == "GIF89a") {
		return "gif", MaxGifMB, true
	}
	if len(head) >= 8 && string(head[4:8]) == "ftyp" {
		return "mp4", MaxVideoMB, true
	}
	if len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF {
		return "jpg", MaxImageMB, true
	}
	if len(head) >= 8 && head[0] == 0x89 && string(head[1:4]) == "PNG" {
		return "png", MaxImageMB, true
	}
	return "", 0, false
}

// Store 校验并落盘;返回错误码供 handler 映射(与 Node MESSAGES 同义)。
// 名字带扩展(<hash>.<ext>,Node 同形);同 hash 已存旧类型时换名覆盖。
func (s *Service) Store(uid, wantHash string, data []byte) (ok bool, code string) {
	if !s.Enabled {
		return false, "media-off"
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != strings.ToLower(wantHash) {
		return false, "media-hash"
	}
	ext, limitMB, sniffOK := Sniff(data)
	if !sniffOK {
		return false, "media-type"
	}
	if len(data) > limitMB*1024*1024 {
		return false, "media-too-large"
	}
	if ext == "mp4" { // 时长探针:坏结构拒绝,超长拒绝,未知时长放行(与 Node 同宽)
		meta, ok := MP4Info(data)
		if !ok {
			return false, "media-invalid"
		}
		if meta.DurationSec != nil && *meta.DurationSec > float64(s.VideoSec)+1 {
			return false, "media-too-long"
		}
	}
	dir := filepath.Join(s.Dir, uid)
	if s.MinFreeBytes > 0 { // 磁盘水位:低于则拒收,留空间给同步/会话等写路径
		var st syscall.Statfs_t
		if err := syscall.Statfs(dir, &st); err == nil {
			if free := int64(st.Bavail) * int64(st.Bsize); free-int64(len(data)) < s.MinFreeBytes {
				return false, "storage-full"
			}
		}
	}
	files := scanDir(dir)
	if old, exists := files[wantHash]; exists { // 同字节已存在:幂等,只刷新类型名
		if old.ext != ext {
			os.Remove(filepath.Join(dir, wantHash+"."+old.ext))
		}
	} else if s.QuotaBytes > 0 {
		var used int64
		for _, f := range files {
			used += f.size
		}
		if used+int64(len(data)) > s.QuotaBytes {
			return false, "media-quota"
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, ".tmp"), 0o700); err != nil { // 父目录一并建
		return false, "io"
	}
	tmp := filepath.Join(dir, ".tmp", wantHash)
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return false, "io"
	}
	if err := os.Rename(tmp, filepath.Join(dir, wantHash+"."+ext)); err != nil {
		return false, "io"
	}
	return true, ""
}

// File 只读调用者自己的目录(Node 同义:别人的文件=不存在);按类型名探测。
func (s *Service) File(uid, hash string) (string, error) {
	if !validHash(hash) {
		return "", errors.New("bad hash")
	}
	dir := filepath.Join(s.Dir, uid)
	for _, ext := range []string{"webp", "gif", "mp4", "jpg", "png"} {
		p := filepath.Join(dir, hash+"."+ext)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("media-missing")
}

func validHash(h string) bool {
	if len(h) != 64 {
		return false
	}
	for _, c := range h {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// ContentType 按扩展/魔数给 Content-Type。
func ContentType(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	head := make([]byte, 16)
	n, _ := f.Read(head)
	ext, _, ok := Sniff(head[:n])
	if !ok {
		return "application/octet-stream"
	}
	switch ext {
	case "webp":
		return "image/webp"
	case "gif":
		return "image/gif"
	case "mp4":
		return "video/mp4"
	case "jpg":
		return "image/jpeg"
	case "png":
		return "image/png"
	}
	return fmt.Sprintf("application/octet-stream")
}

/* ---------- P5:目录事实、引用集、marks、GC ---------- */

type storedFile struct {
	ext  string
	size int64
}

// scanDir 一次 readdir 建目录事实(<hash>.<ext>;隐藏名与坏名跳过——磁盘是唯一真相)。
func scanDir(dir string) map[string]storedFile {
	out := map[string]storedFile{}
	names, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, n := range names {
		if n.IsDir() || strings.HasPrefix(n.Name(), ".") {
			continue
		}
		h, ext, found := strings.Cut(n.Name(), ".")
		if !found || !validHash(h) || ext == "" {
			continue
		}
		if info, err := n.Info(); err == nil {
			out[h] = storedFile{ext: ext, size: info.Size()}
		}
	}
	return out
}

// ReferencedHashes 一个 state 文件引用的媒体集(customEx[].media 与 workouts[].media 的
// hash 及其 poster;宁多勿漏——这集合决定"保留")。与 api/media.js 同义,fixture 共用。
func ReferencedHashes(state map[string]any) map[string]bool {
	out := map[string]bool{}
	add := func(m any) {
		ref, ok := m.(map[string]any)
		if !ok {
			return
		}
		if h, ok := ref["hash"].(string); ok && validHash(h) {
			out[h] = true
		}
		if p, ok := ref["poster"].(map[string]any); ok {
			if h, ok := p["hash"].(string); ok && validHash(h) {
				out[h] = true
			}
		}
	}
	if list, ok := state["customEx"].([]any); ok {
		for _, c := range list {
			ce, ok := c.(map[string]any)
			if !ok {
				continue
			}
			if m, ok := ce["media"].(map[string]any); ok {
				add(m)
			}
		}
	}
	if list, ok := state["workouts"].([]any); ok {
		for _, w := range list {
			wo, ok := w.(map[string]any)
			if !ok {
				continue
			}
			ml, ok := wo["media"].([]any) // 键缺失/形状不对都不算引用
			if !ok {
				continue
			}
			for _, m := range ml {
				add(m)
			}
		}
	}
	return out
}

// gcMarks <uid>/.gc.json = {hash: 离开 state 的时刻(ms)}。每次现读(几百字节,磁盘即真相);
// 读不了 = 无 marks:grace 从头计,只会更晚删。
func (s *Service) gcMarks(uid string) map[string]float64 {
	raw, err := os.ReadFile(filepath.Join(s.Dir, uid, ".gc.json"))
	if err != nil {
		return map[string]float64{}
	}
	var m map[string]float64
	if json.Unmarshal(raw, &m) != nil {
		return map[string]float64{}
	}
	return m
}

func (s *Service) writeGCMarks(uid string, m map[string]float64) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	dir := filepath.Join(s.Dir, uid)
	tmp := filepath.Join(dir, ".gc.json.tmp")
	if os.WriteFile(tmp, b, 0o600) != nil {
		return
	}
	os.Rename(tmp, filepath.Join(dir, ".gc.json"))
}

// Sweep 按引用集回收:未引用且超过 grace 的文件删除。state 不可读由调用方先判
// (skipped 语义:绝不在没有 state 的情况下推断)。
func (s *Service) Sweep(uid string, refs map[string]bool, graceMs int64) (removed int, freed int64) {
	dir := filepath.Join(s.Dir, uid)
	files := scanDir(dir)
	if len(files) == 0 {
		return 0, 0
	}
	marks := s.gcMarks(uid)
	t := float64(time.Now().UnixMilli())
	changed := false
	// reconcile:仍引用的销 mark;未引用的无 mark 则打点(Node 同序)。
	for h := range files {
		if refs[h] {
			if _, ok := marks[h]; ok {
				delete(marks, h)
				changed = true
			}
		} else if _, ok := marks[h]; !ok {
			marks[h] = t
			changed = true
		}
	}
	for h := range marks {
		if _, ok := files[h]; !ok {
			delete(marks, h)
			changed = true
		}
	}
	for h, f := range files {
		if refs[h] {
			continue
		}
		if graceMs > 0 && t-marks[h] < float64(graceMs) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, h+"."+f.ext)); err == nil || errors.Is(err, os.ErrNotExist) {
			delete(marks, h)
			changed = true
			removed++
			freed += f.size
		}
	}
	if changed {
		s.writeGCMarks(uid, marks)
	}
	return removed, freed
}

// Usage 目录用量(Node usageOf 同形:bytes/count/quotaBytes)。
func (s *Service) Usage(uid string) map[string]any {
	files := scanDir(filepath.Join(s.Dir, uid))
	var bytes int64
	for _, f := range files {
		bytes += f.size
	}
	return map[string]any{"bytes": bytes, "count": len(files), "quotaBytes": s.QuotaBytes}
}

// CleanTmp 启动时清各 uid 的 .tmp(崩溃残留的上传中转;此刻没有上传在途)。
func (s *Service) CleanTmp() {
	entries, _ := os.ReadDir(s.Dir)
	for _, e := range entries {
		if e.IsDir() {
			os.RemoveAll(filepath.Join(s.Dir, e.Name(), ".tmp"))
		}
	}
}
