// Package agent — AI-Native 的 Go 工具层(方案 v2 五.2,R3:算法单源=frontend src/lib)。
//
// onerm.go 是 frontend/src/lib/onerm.js 的逐行翻译(golden 等价验收,testdata/
// onerm-golden.json 由 scripts/golden/gen-onerm.mjs 用 JS 真函数生成)。
// 只翻译纯计算层 estimate1RM / calculate1RMAccuracy / formulaOf / weightedEstimate;
// bestSetOf 及以上(依赖 entry 结构)等数据接入工具在 tools.go 另行组装。
//
// 浮点纪律:JS 与 Go math.* 在 exp/pow 上允许 ULP 级差异,golden 比对用 1e-6
// 绝对容差(round 到 0.1/0.01 的结果应绝大多数精确相等)。JS Math.round(向 +∞)
// 与 Go math.Round(half-away-from-zero)仅在 .5 负数分歧——本层取整输入均非负。
package agent

import "math"

// 重复次数上界:onerm.js L20/L24(公式在高次数下失真,拒绝猜测)。
const (
	RepCap         = 12
	WeightedRepCap = 15
	DefaultFormula = "epley"
)

// formulaEntry:一条公式 + 它的加权参数(a=绝对可靠度,v(r)=随有效次数衰减)。
// **顺序 = JS 对象插入序**,weightedEstimate/ensembleCV 的浮点求和必须与 JS
// 同序,否则 ULP 级漂移会在 0.05 取整边界翻结果。
type formulaEntry struct {
	name string
	fn   func(w, r float64) float64
	a    float64
	v    func(r float64) float64
}

// stepV 还原 JS 的 `r <= k ? 1 : Math.max(floor, 1 - (r-k)*slope)`(onerm.js L67-73)。
func stepV(k, slope, floor float64) func(float64) float64 {
	return func(r float64) float64 {
		if r <= k {
			return 1
		}
		d := 1 - (r-k)*slope
		if d < floor {
			return floor
		}
		return d
	}
}

// formulaList 对应 onerm.js L28-36(FORMULAS)+ L66-75(WEIGHTS)按名合并。
var formulaList = []formulaEntry{
	{name: "epley", a: 0.95, v: stepV(6, 0.12, 0.4),
		fn: func(w, r float64) float64 { return w * (1 + r/30) }},
	{name: "brzycki", a: 1.10, v: stepV(8, 0.18, 0.3),
		fn: func(w, r float64) float64 { return w * 36 / (37 - r) }},
	{name: "lombardi", a: 0.85, v: stepV(5, 0.10, 0.4),
		fn: func(w, r float64) float64 { return w * math.Pow(r, 0.1) }},
	{name: "oconner", a: 0.90, v: stepV(6, 0.12, 0.4),
		fn: func(w, r float64) float64 { return w * (1 + r/40) }},
	{name: "mayhew", a: 0.95, v: stepV(8, 0.10, 0.5),
		fn: func(w, r float64) float64 { return w * 100 / (52.2 + 41.9*math.Exp(-0.055*r)) }},
	{name: "wathan", a: 0.90, v: stepV(7, 0.12, 0.4),
		fn: func(w, r float64) float64 { return w * 100 / (48.8 + 53.8*math.Exp(-0.075*r)) }},
	{name: "lander", a: 0.85, v: stepV(7, 0.15, 0.35),
		fn: func(w, r float64) float64 { return w * 100 / (101.3 - 2.67123*r) }},
}

// rirWeight 对应 L74 的 rir 条目(七公式之外的第八权重)。
var rirWeight = struct {
	a float64
	v func(float64) float64
}{a: 1.00, v: stepV(10, 0.15, 0.4)}

// rirPct 对应 L53-56:Tuchscherer/RTS 的 %1RM 表,下标 0 = 力竭 1 次。
var rirPct = [15]float64{
	100, 95.5, 92.2, 89.2, 86.3, 83.7, 81.1, 78.6, 76.2, 73.9,
	71.7, 69.5, 67.5, 65.5, 63.6,
}

// rirEstimate 对应 L58-61。
func rirEstimate(w, effectiveReps float64) float64 {
	idx := int(math.Round(effectiveReps)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx > len(rirPct)-1 {
		idx = len(rirPct) - 1
	}
	return w * 100 / rirPct[idx]
}

// ensembleCV 对应 L78-83:七公式估计值的变异系数(尺度不变)。求和顺序 =
// formulaList 插入序(与 JS Object.entries 同序)。
func ensembleCV(w, effectiveReps float64) float64 {
	vals := make([]float64, 0, len(formulaList))
	for _, f := range formulaList {
		vals = append(vals, f.fn(w, effectiveReps))
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	mean := sum / float64(len(vals))
	var vsum float64
	for _, v := range vals {
		vsum += (v - mean) * (v - mean)
	}
	return math.Sqrt(vsum/float64(len(vals))) / mean
}

// weightedEstimate 对应 L88-100:七公式 + RIR 表的加权平均(不取整,内部用)。
func weightedEstimate(w, effectiveReps float64) float64 {
	var total, wsum float64
	for _, f := range formulaList {
		wt := f.a * f.v(effectiveReps)
		total += f.fn(w, effectiveReps) * wt
		wsum += wt
	}
	rirWt := rirWeight.a * rirWeight.v(effectiveReps)
	total += rirEstimate(w, effectiveReps) * rirWt
	wsum += rirWt
	return total / wsum
}

// FormulaOf 对应 L46-49:未识别的取默认(旧/新端互不破坏)。
func FormulaOf(f string) string {
	if f == "weighted" {
		return f
	}
	for _, e := range formulaList {
		if e.name == f {
			return f
		}
	}
	return DefaultFormula
}

// Estimate1RM 对应 L108-134 estimate1RM:由一组(重量,次数[,RIR])估 1RM。
// 返回 nil 表示"诚实回答不了"(非法输入/超上界)。rir nil 即 JS null。
// r==1 且无有效 RIR 是测量而非估计,原样返回(1 位小数)。
func Estimate1RM(w, reps float64, formula string, rir *float64) *float64 {
	if math.IsNaN(w) || math.IsInf(w, 0) || math.IsNaN(reps) || math.IsInf(reps, 0) {
		return nil
	}
	if w <= 0 || reps < 1 {
		return nil
	}
	validRir := rir != nil && *rir >= 0

	// 1 次力竭(无 RIR 或 RIR=0):测量,不是估计。
	if reps == 1 && (!validRir || *rir == 0) {
		v := math.Round(w*10) / 10
		return &v
	}

	if formula == "weighted" {
		eff := reps
		if validRir {
			eff = reps + *rir
		}
		if eff > WeightedRepCap {
			return nil
		}
		est := weightedEstimate(w, eff)
		if math.IsNaN(est) || math.IsInf(est, 0) || est <= 0 {
			return nil
		}
		v := math.Round(est*10) / 10
		return &v
	}

	if reps > RepCap {
		return nil
	}
	var fn func(w, r float64) float64 = func(w, r float64) float64 { return w * (1 + r/30) } // DefaultFormula
	for _, f := range formulaList {
		if f.name == formula {
			fn = f.fn
			break
		}
	}
	est := w
	if reps != 1 {
		est = fn(w, math.Round(reps))
	}
	if math.IsNaN(est) || math.IsInf(est, 0) || est <= 0 {
		return nil
	}
	v := math.Round(est*10) / 10
	return &v
}

// Calc1RMAccuracy 对应 L200-222 calculate1RMAccuracy:估计置信度(0-1)。
// 覆盖次数衰减、RIR 主观性、(加权模式)公式间离散度。
func Calc1RMAccuracy(reps float64, rir *float64, formula string) float64 {
	r := reps
	if math.IsNaN(r) || math.IsInf(r, 0) || r < 1 {
		return 0
	}
	// JS: r === 1 && rir === 0(null 不等于 0)
	if r == 1 && rir != nil && *rir == 0 {
		return 1
	}
	if r > WeightedRepCap {
		return 0
	}
	repFactor := 1.0
	if r != 1 {
		repFactor = 1 - 0.5*(r-1)/(WeightedRepCap-1)
	}
	rirFactor := 1.0
	if rir != nil && !math.IsNaN(*rir) && !math.IsInf(*rir, 0) && *rir > 0 {
		rirFactor = 0.92
	}
	spreadFactor := 1.0
	if formula == "weighted" && r >= 2 {
		sf := 1 - ensembleCV(100, r)/0.15*0.5
		if sf < 0.5 {
			sf = 0.5
		}
		spreadFactor = sf
	}
	return math.Round(repFactor*rirFactor*spreadFactor*100) / 100
}
