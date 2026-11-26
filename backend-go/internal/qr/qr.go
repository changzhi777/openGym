// Package qr QR 图渲染：skip2/go-qrcode 薄封装。
// 职责收敛：尺寸限幅、纠错级别、中心 logo 叠加、PNG 字节输出——业务语义（内容构造/鉴权/限流）留在 api 层。
// 来源：MuTag backend/internal/qr 移植（2026-11-18，授权引用）；品牌 logo 走 logo.go 的 base64 内嵌常量。
package qr

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	qrcode "github.com/skip2/go-qrcode"
)

// 尺寸限幅：下限防 skip2 对过小尺寸报错，上限防恶意拉大图打内存。
const (
	minSize = 64
	maxSize = 1024
	defSize = 512
)

// 中心 logo 占比（相对码宽）：垫片 22% 白底 + logo 18%——High 级 30% 容错内安全。
const (
	logoPadPct = 22
	logoPct    = 18
)

// ClampSize 规整用户传入尺寸：缺省 512，越界钳到 [64,1024]。
func ClampSize(size int) int {
	if size <= 0 {
		return defSize
	}
	if size < minSize {
		return minSize
	}
	if size > maxSize {
		return maxSize
	}
	return size
}

// Render QR 内容 → PNG 字节。
// withLogo=true：纠错 Medium→High（30% 容错盖中心遮蔽）+ 中心白垫片/logo 贴图（品牌码）。
func Render(content string, size int, withLogo bool) ([]byte, error) {
	level := qrcode.Medium
	if withLogo {
		level = qrcode.High
	}
	pngBytes, err := qrcode.Encode(content, level, ClampSize(size))
	if err != nil {
		return nil, fmt.Errorf("qr render: %w", err)
	}
	if !withLogo {
		return pngBytes, nil
	}
	return overlayLogo(pngBytes)
}

// overlayLogo 码图中心叠 logo：白底垫片 + 最近邻缩放贴图（资产 64px → 约 18% 宽，锯齿可忽略）。
func overlayLogo(qrPNG []byte) ([]byte, error) {
	qrImg, err := png.Decode(bytes.NewReader(qrPNG))
	if err != nil {
		return nil, fmt.Errorf("qr decode: %w", err)
	}
	logoImg, err := png.Decode(bytes.NewReader(logoPNG))
	if err != nil {
		return nil, fmt.Errorf("logo decode: %w", err)
	}

	b := qrImg.Bounds()
	W := b.Dx()
	padW := W * logoPadPct / 100
	logoW := W * logoPct / 100

	dst := image.NewRGBA(image.Rect(0, 0, W, W))
	draw.Draw(dst, dst.Bounds(), qrImg, b.Min, draw.Src)

	// 白底垫片（保证 logo 与码模块的对比度）
	p0 := (W - padW) / 2
	draw.Draw(dst, image.Rect(p0, p0, p0+padW, p0+padW), &image.Uniform{color.White}, image.Point{}, draw.Src)

	// logo 最近邻缩放（64 → logoW）后居中贴图
	scaled := image.NewRGBA(image.Rect(0, 0, logoW, logoW))
	lb := logoImg.Bounds()
	for y := 0; y < logoW; y++ {
		sy := lb.Min.Y + y*lb.Dy()/logoW
		for x := 0; x < logoW; x++ {
			sx := lb.Min.X + x*lb.Dx()/logoW
			scaled.Set(x, y, logoImg.At(sx, sy))
		}
	}
	l0 := (W - logoW) / 2
	draw.Draw(dst, image.Rect(l0, l0, l0+logoW, l0+logoW), scaled, image.Point{}, draw.Over)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("qr encode: %w", err)
	}
	return buf.Bytes(), nil
}
