// media 四端点(P4 骨架):PUT/GET /api/media/{hash}、missing、sweep。
package media

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/changzhi777/openGym/backend-go/internal/auth"
)

type Handlers struct {
	DB       *pgxpool.Pool
	Sessions *auth.Sessions
	Svc      *Service
}

// Put PUT /api/media/{hash} — 校验 sha256+类型上限后落盘;元数据进 media 表。
func (h *Handlers) Put(c *gin.Context) {
	u, ok := auth.UserOf(c, h.DB, h.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	hash := c.Param("hash")
	raw, _ := c.Get("rawBody")
	data, _ := raw.([]byte)
	if data == nil {
		var err error
		data, err = io.ReadAll(io.LimitReader(c.Request.Body, MaxTotal+1))
		if err != nil || len(data) > MaxTotal {
			c.JSON(413, gin.H{"error": "that file is too large"})
			return
		}
	}
	if ok, code := h.Svc.Store(u.ID, hash, data); !ok {
		c.JSON(codeStatus(code), gin.H{"error": codeMsg(code)})
		return
	}
	h.DB.Exec(c.Request.Context(),
		`INSERT INTO media (sha256, user_id, kind, bytes) VALUES ($1,$2,$3,$4)
		 ON CONFLICT (sha256) DO NOTHING`, hash, u.ID, "file", len(data))
	c.JSON(200, gin.H{"ok": true})
}

// Get GET /api/media/{hash} — 只读自己的目录;sha 命名天然可长缓存。
func (h *Handlers) Get(c *gin.Context) {
	u, ok := auth.UserOf(c, h.DB, h.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	p, err := h.Svc.File(u.ID, c.Param("hash"))
	if err != nil {
		c.JSON(404, gin.H{"error": "media-missing"})
		return
	}
	c.Header("Content-Type", ContentType(p))
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	st, _ := os.Stat(p)
	if st != nil {
		c.Header("Content-Length", strconv.FormatInt(st.Size(), 10))
	}
	c.File(p)
}

// Missing POST /api/media/missing — {hashes:[...]} → 本地没有的那些 + 用量(Node 同形)。
func (h *Handlers) Missing(c *gin.Context) {
	u, ok := auth.UserOf(c, h.DB, h.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	var body struct {
		Hashes []string `json:"hashes"`
	}
	if raw, ok2 := c.Get("rawBody"); ok2 {
		if b, ok3 := raw.([]byte); ok3 {
			jsonUnmarshal(b, &body)
		}
	}
	miss := []string{}
	for _, hash := range body.Hashes {
		if _, err := h.Svc.File(u.ID, hash); err != nil {
			miss = append(miss, hash)
		}
	}
	c.JSON(200, gin.H{"missing": miss, "usage": h.Svc.Usage(u.ID)})
}

// Sweep POST /api/media/sweep — "重置一切":当前 state 不引用的立即删(grace=0)。
// state 读不到(无行/坏文档)时什么都不删 —— 宁可留着,不可误删。
func (h *Handlers) Sweep(c *gin.Context) {
	u, ok := auth.UserOf(c, h.DB, h.Sessions)
	if !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	var doc map[string]any
	err := h.DB.QueryRow(c.Request.Context(),
		`SELECT doc FROM states WHERE user_id = $1`, u.ID).Scan(&doc)
	if err != nil || doc == nil { // 无 state:跳过回收,只报用量
		c.JSON(200, gin.H{"removed": 0, "freedBytes": 0, "usage": h.Svc.Usage(u.ID)})
		return
	}
	removed, freed := h.Svc.Sweep(u.ID, ReferencedHashes(doc), 0)
	c.JSON(http.StatusOK, gin.H{"removed": removed, "freedBytes": freed, "usage": h.Svc.Usage(u.ID)})
}

func codeStatus(code string) int {
	switch code {
	case "media-hash", "media-type", "media-too-large":
		return 400
	case "media-quota", "media-too-long":
		return 413
	case "media-invalid":
		return 415
	case "storage-full":
		return 507
	case "media-off":
		return 404
	}
	return 500
}

func codeMsg(code string) string {
	switch code {
	case "media-hash":
		return "the hash does not match the content"
	case "media-type":
		return "that file type is not accepted"
	case "media-quota":
		return "media quota exceeded"
	case "media-too-large":
		return "that file is too large"
	case "media-too-long":
		return "that video is too long"
	case "media-invalid":
		return "that video could not be read"
	case "storage-full":
		return "storage is full"
	case "media-off":
		return "media uploads are off"
	}
	return "could not store the file"
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

// SweepAll 后台引用计数回收:遍历媒体目录(目录即真相——有文件才需要回收),每个 uid
// 读 PG state 后按 grace 回收。state 读不到的 uid 跳过:绝不在没有 state 时推断。
func (h *Handlers) SweepAll(graceMs int64) {
	entries, err := os.ReadDir(h.Svc.Dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		var doc map[string]any
		err := h.DB.QueryRow(context.Background(),
			`SELECT doc FROM states WHERE user_id = $1`, e.Name()).Scan(&doc)
		if err != nil || doc == nil {
			continue
		}
		h.Svc.Sweep(e.Name(), ReferencedHashes(doc), graceMs)
	}
}
