// Package captcha — 发短信前置的数字图形验证码(契约 authCaptcha;决议 Q3 自绘 SVG)。
// 4 位数字 + 扭曲/噪线/噪点的 SVG,答案存 Redis(captcha:<id>) 5 分钟;
// sms/send 消费核验后删除,刷新即重新取。
package captcha

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	idPrefix = "captcha:id:"
	ttl      = 5 * time.Minute
)

// New 生成 4 位数字验证码:返回 (captchaId, svgMarkup, err)。
func New(ctx context.Context, rdb *redis.Client) (string, string, error) {
	digits := make([]string, 4)
	ans := ""
	for i := range digits {
		n, _ := rand.Int(rand.Reader, big.NewInt(10))
		digits[i] = n.String()
		ans += n.String()
	}
	id := fmt.Sprintf("cp%d%s", time.Now().UnixNano(), randHex(4))
	if err := rdb.Set(ctx, idPrefix+id, ans, ttl).Err(); err != nil {
		return "", "", err
	}
	return id, svg(digits), nil
}

// Check 核验:正确则消费(删)。错误/过期 → false。
func Check(ctx context.Context, rdb *redis.Client, id, code string) bool {
	key := idPrefix + id
	v, err := rdb.GetDel(ctx, key).Result()
	return err == nil && v != "" && v == strings.TrimSpace(code)
}

// svg 手绘 160×48:每位数字独立随机位移/旋转/缩放,两条贝塞尔噪线,20 个噪点。
func svg(digits []string) string {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="160" height="48" viewBox="0 0 160 48" role="img">`)
	b.WriteString(`<rect width="160" height="48" fill="#f6f8f6" rx="8"/>`)
	// 噪线两条
	b.WriteString(`<path d="M0 30 C 40 12, 120 44, 160 18" stroke="#b9cbb9" stroke-width="1.6" fill="none"/>`)
	b.WriteString(`<path d="M0 14 C 50 34, 110 6, 160 34" stroke="#a8c2a8" stroke-width="1.4" fill="none"/>`)
	// 噪点
	for i := 0; i < 20; i++ {
		x, y := randInt(6, 154), randInt(6, 42)
		b.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="1" fill="#9db89d" opacity=".7"/>`, x, y))
	}
	// 四位数字:基线 32,x 从 26 起每 36 一位,随机旋转 ±18°、字号 26±4、双色调
	for i, d := range digits {
		x := 26 + i*36
		y := 40 + randInt(-4, 4)
		rot := randInt(-18, 18)
		size := 24 + randInt(0, 6)
		color := "#2f5e37"
		if i%2 == 1 {
			color = "#4a7c59"
		}
		fmtArgs := []any{x, y, size, color, rot, x + 10, d}
		b.WriteString(fmt.Sprintf(
			`<text x="%d" y="%d" font-family="Arial, sans-serif" font-size="%d" font-weight="700" fill="%s" transform="rotate(%d %d 32)">%s</text>`,
			fmtArgs...))
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func randInt(min, max int) int {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	return int(n.Int64()) + min
}
