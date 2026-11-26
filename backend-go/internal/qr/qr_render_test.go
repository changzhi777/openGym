// qr_render_test.go — qr 包纯测试：尺寸限幅、PNG 渲染双模式（纯码/品牌码）、logo 资产解码。
// 来源：MuTag qr 包移植时的伴随测试（2026-11-18 批次 D）。
package qr

import (
	"bytes"
	"image/png"
	"testing"
)

func TestClampSize(t *testing.T) {
	cases := map[int]int{
		0:    512,  // 缺省
		-5:   512,  // 负数走缺省
		10:   64,   // 下限钳制
		5000: 1024, // 上限钳制
		512:  512,  // 合法直通
	}
	for in, want := range cases {
		if got := ClampSize(in); got != want {
			t.Errorf("ClampSize(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestRenderPNG(t *testing.T) {
	for _, logo := range []bool{false, true} {
		b, err := Render("https://mufit.qingmulife.com/?link=ABCD-EFGH-JKLM", 256, logo)
		if err != nil {
			t.Fatalf("Render(logo=%v): %v", logo, err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("png decode (logo=%v): %v", logo, err)
		}
		if got := img.Bounds().Dx(); got != 256 {
			t.Errorf("size = %d, want 256", got)
		}
	}
}

func TestLogoDecode(t *testing.T) {
	if len(logoPNG) == 0 {
		t.Fatal("logoPNG 解码为空——logoB64 常量损坏")
	}
	// PNG 魔数校验（\x89PNG）
	if logoPNG[0] != 0x89 || logoPNG[1] != 'P' || logoPNG[2] != 'N' || logoPNG[3] != 'G' {
		t.Fatalf("logoPNG 不是 PNG: % x", logoPNG[:4])
	}
}
