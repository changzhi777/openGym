// media 四端点(P4 骨架):PUT/GET /api/media/{hash}、missing、sweep。
package media

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"

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

// Missing POST /api/media/missing — {hashes:[...]} → 本地没有的那些(同步补拉用)。
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
	c.JSON(200, gin.H{"missing": miss})
}

// Sweep POST /api/media/sweep — P4 骨架:安全 no-op(完整引用计数 GC 随 P5 补)。
func (h *Handlers) Sweep(c *gin.Context) {
	if _, ok := auth.UserOf(c, h.DB, h.Sessions); !ok {
		c.JSON(401, gin.H{"error": "not signed in"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"removed": 0})
}

func codeStatus(code string) int {
	switch code {
	case "media-hash", "media-type", "media-too-large":
		return 400
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
	case "media-too-large":
		return "that file is too large"
	case "media-off":
		return "media uploads are off"
	}
	return "could not store the file"
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
