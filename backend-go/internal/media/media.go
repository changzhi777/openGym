// Package media — 自定义动作媒体存储(P4 骨架:核心校验与读写;完整配额/GC/mp4 探针后补)。
// 文件布局与 Node 兼容:<DATA_DIR>/media/<uid>/<sha256>,灰度期两栈可互为读写。
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	MaxImageMB = 2
	MaxGifMB   = 8
	MaxVideoMB = 40
	MaxTotal   = 40 * 1024 * 1024 // 单请求上限(宽松,类型上限在 sniff 后精准判)
)

type Service struct {
	Dir     string
	Enabled bool
}

func New(dataDir string) *Service {
	return &Service{Dir: filepath.Join(dataDir, "media"), Enabled: os.Getenv("MEDIA_UPLOADS") != "0"}
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
func (s *Service) Store(uid, wantHash string, data []byte) (ok bool, code string) {
	if !s.Enabled {
		return false, "media-off"
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != strings.ToLower(wantHash) {
		return false, "media-hash"
	}
	_, limitMB, sniffOK := Sniff(data)
	if !sniffOK {
		return false, "media-type"
	}
	if len(data) > limitMB*1024*1024 {
		return false, "media-too-large"
	}
	dir := filepath.Join(s.Dir, uid)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, "io"
	}
	tmp := filepath.Join(dir, wantHash+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return false, "io"
	}
	if err := os.Rename(tmp, filepath.Join(dir, wantHash)); err != nil {
		return false, "io"
	}
	return true, ""
}

// File 只读调用者自己的目录(Node 同义:别人的文件=不存在)。
func (s *Service) File(uid, hash string) (string, error) {
	if !validHash(hash) {
		return "", errors.New("bad hash")
	}
	p := filepath.Join(s.Dir, uid, hash)
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		return "", errors.New("media-missing")
	}
	return p, nil
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
