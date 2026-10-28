// mp4 探针 — 从 box 头读 brand 与时长(Node media.js mp4Info 同义)。
// 只用已读进内存的字节;解析不了返回 ok=false(调用方按 media-invalid 拒)。
// fragmented 文件(Safari MediaRecorder)的 mvhd 为 0,总时长在 mvex/mehd — 与 Node 同样处理。
package media

import (
	"encoding/binary"
	"math"
)

const (
	moovMax  = 8 << 20 // 真实 60s 片段的 moov 远小于 1MB;超限当解析失败
	maxBoxes = 10000
)

type MP4Meta struct {
	Brand       string
	DurationSec *float64 // nil = 未知(fragmented 且无 mehd)
}

type mp4box struct {
	typ  string
	body []byte
	end  int
}

// boxAt 读 pos 处的 box 头;size=1 走 16 字节 largesize,size=0 到结尾。
func boxAt(b []byte, pos int) (mp4box, bool) {
	if len(b)-pos < 8 {
		return mp4box{}, false
	}
	size := uint64(binary.BigEndian.Uint32(b[pos:]))
	headerLen := 8
	if size == 1 {
		if len(b)-pos < 16 {
			return mp4box{}, false
		}
		size = binary.BigEndian.Uint64(b[pos+8:])
		if size > math.MaxInt32 { // 现实文件远达不到;超界视为坏头
			return mp4box{}, false
		}
		headerLen = 16
	} else if size == 0 {
		size = uint64(len(b) - pos)
	}
	if size < uint64(headerLen) || pos+int(size) > len(b) {
		return mp4box{}, false
	}
	return mp4box{typ: string(b[pos+4 : pos+8]), body: b[pos+headerLen : pos+int(size)], end: pos + int(size)}, true
}

func u64At(b []byte, off int) uint64 { return binary.BigEndian.Uint64(b[off:]) }
func u32At(b []byte, off int) uint32 { return binary.BigEndian.Uint32(b[off:]) }

// unknownDur 「未知」时长:0,或全 1(v0 是 0xffffffff,v1 经 boxAt 的大小钳制后以 MaxInt32 判)。
func unknownDur(d uint64, v1 bool) bool {
	return d == 0 || (v1 && d >= math.MaxInt32) || (!v1 && d == 0xffffffff)
}

// MP4Info 解析 brand 与时长。第一个顶层 box 必须是 ftyp;moov 可前可后。
func MP4Info(data []byte) (MP4Meta, bool) {
	var meta MP4Meta
	count := 0
	var moovBody []byte
	for pos := 0; pos < len(data); {
		if count++; count > maxBoxes {
			return meta, false
		}
		b, ok := boxAt(data, pos)
		if !ok {
			return meta, false
		}
		if count == 1 {
			if b.typ != "ftyp" || len(b.body) < 4 {
				return meta, false
			}
			meta.Brand = string(b.body[:4])
		}
		if b.typ == "moov" {
			moovBody = b.body
			break
		}
		pos = b.end
	}
	if moovBody == nil || len(moovBody)+8 > moovMax {
		return meta, false
	}
	var timescale uint32
	var dur, fragDur *uint64
	sawMvhd := false
	for pos := 0; pos < len(moovBody); {
		if count++; count > maxBoxes {
			return meta, false
		}
		b, ok := boxAt(moovBody, pos)
		if !ok {
			return meta, false
		}
		switch b.typ {
		case "mvhd":
			if !sawMvhd {
				sawMvhd = true
				ts, d := parseMvhd(b.body)
				timescale, dur = ts, d
			}
		case "mvex":
			// fragmented:总时长在 mehd(同一 timescale)。
			for p := 0; p < len(b.body); {
				if count++; count > maxBoxes {
					return meta, false
				}
				c, ok := boxAt(b.body, p)
				if !ok {
					break
				}
				if c.typ == "mehd" {
					if d := parseMehd(c.body); d != nil {
						fragDur = d
					}
				}
				p = c.end
			}
		}
		pos = b.end
	}
	if !sawMvhd || timescale == 0 {
		return meta, false
	}
	ticks := dur
	if ticks == nil {
		ticks = fragDur
	}
	if ticks == nil {
		return meta, true // 时长未知(brand 有效),调用方按 unknown 跳过时长校验
	}
	sec := math.Round(float64(*ticks)/float64(timescale)*1000) / 1000
	meta.DurationSec = &sec
	return meta, true
}

// parseMvhd mvhd 的 timescale 与 duration;字段缺失/越界 → nil duration,结构坏 → false。
func parseMvhd(b []byte) (uint32, *uint64) {
	if len(b) < 4 {
		return 0, nil
	}
	v1 := b[0] == 1
	if !v1 && len(b) < 20 || v1 && len(b) < 32 {
		return 0, nil
	}
	var ts uint32
	var d uint64
	if v1 {
		ts = u32At(b, 20)
		d = u64At(b, 24)
	} else {
		ts = u32At(b, 12)
		d = uint64(u32At(b, 16))
	}
	if unknownDur(d, v1) {
		return ts, nil
	}
	return ts, &d
}

// parseMehd mehd 的 fragment duration(version 0 在 4 字节,version 1 在 8 字节)。
func parseMehd(b []byte) *uint64 {
	if len(b) < 8 {
		return nil
	}
	v1 := b[0] == 1 && len(b) >= 12
	var d uint64
	if v1 {
		d = u64At(b, 4)
	} else {
		d = uint64(u32At(b, 4))
	}
	if unknownDur(d, v1) {
		return nil
	}
	return &d
}
