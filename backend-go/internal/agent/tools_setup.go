// tools_setup.go — 工具集装配(批次 2 口径 + 批次 3 知识检索)。
//
// 清单:estimate_1rm(纯计算真工具)+ 两个提议工具(确认制红线载体)
// + search_knowledge(批次 3:WeKnora 全局知识区检索;WEKNORA_* 未配置时不注册
// = D15 降级形态一)。
// 数据读取工具(get_week_plan 等)在批次 4 对照 frontend src/lib 的 states 结构
// 逐一翻译 + golden,不瞎猜文档形状。
// 用户记忆区的 WeKnora KB 副本推批次 4:记忆条目的检索价值已由 system prompt
// 注入(DefaultSystem 读本地表)实现,per-user KB 管理待前端联调时按需上——
// 本地表仍是读写主存,D15 降级语义不变。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/changzhi777/openGym/backend-go/internal/store"
	"github.com/changzhi777/openGym/backend-go/internal/weknora"
)

// DefaultTools 装配生产工具集。wk 为 nil(未配置 WeKnora)时不注册检索工具。
// globalKB 是全局知识区(共享只读)的 KB id——空串则工具退化为"未配置"应答。
func DefaultTools(db *pgxpool.Pool, wk *weknora.Client, globalKB string) Registry {
	tools := []Tool{
		{
			Name:        "estimate_1rm",
			Description: "由一组(重量kg,次数[,RIR])估算 1RM(kg)。未知公式回退 epley;超过重复上限返回 null。公式可选: epley/brzycki/lombardi/oconner/mayhew/wathen/lander/weighted。",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"weight":  map[string]any{"type": "number", "description": "重量 kg"},
					"reps":    map[string]any{"type": "integer", "description": "次数"},
					"formula": map[string]any{"type": "string", "description": "可选,默认 epley"},
					"rir":     map[string]any{"type": "integer", "description": "可选,储备次数"},
				},
				"required": []string{"weight", "reps"},
			},
			Exec: func(_ context.Context, input map[string]any) (string, error) {
				w := numOf(input["weight"])
				r := numOf(input["reps"])
				if w == 0 || r == 0 {
					return "", fmt.Errorf("weight/reps required")
				}
				formula, _ := input["formula"].(string)
				var rir *float64
				if v := input["rir"]; v != nil {
					rv := numOf(v)
					rir = &rv
				}
				est := Estimate1RM(w, r, FormulaOf(formula), rir)
				if est == nil {
					out, _ := json.Marshal(map[string]any{"estimate": nil, "note": "超出诚实回答范围(非法输入或次数超上限)"})
					return string(out), nil
				}
				acc := Calc1RMAccuracy(r, rir, FormulaOf(formula))
				out, err := json.Marshal(map[string]any{"estimate": *est, "unit": "kg", "confidence": acc})
				if err != nil {
					return "", err
				}
				return string(out), nil
			},
		},
		{
			Name:        "propose_constraint",
			Description: "提议一条硬约束(伤病回避/禁忌动作等)。用户确认后即刻生效于所有计划生成;未经确认绝不写入。",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"text": map[string]any{"type": "string", "description": "约束内容,用户的原话优先"}},
				"required":   []string{"text"},
			},
			Proposal: true,
		},
		{
			Name:        "propose_memory",
			Description: "提议记住一条长期偏好(器材/时长/习惯等)。用户确认后写入;未经确认绝不写入。",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"text": map[string]any{"type": "string", "description": "记忆内容"}},
				"required":   []string{"text"},
			},
			Proposal: true,
		},
	}
	// get_week_plan(批次 6 后段:"先查周计划再改计划"的最小里程碑收官件):
	// 计划决策链(plan.go,queue.js 全文翻译 + golden 8 场景等价验收)。
	tools = append(tools, Tool{
		Name:        "get_week_plan",
		Description: "读取用户的训练计划:未来七天的日历视图(每天练什么、由何种规则决定:coach/钉选/覆盖/周模板/休息)、今天的 session、以及进行中的教练周进度。",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Exec: func(ctx context.Context, _ map[string]any) (string, error) {
			uid := UIDFrom(ctx)
			if uid == "" {
				return "", fmt.Errorf("需要会话才能读取训练计划")
			}
			doc, err := store.GetStateDoc(ctx, db, uid)
			if err != nil {
				return "", err
			}
			var st planS
			if err := json.Unmarshal(doc.Doc, &st); err != nil {
				return "", fmt.Errorf("state unreadable: %w", err)
			}
			wp, err := st.WeekPlan(time.Now().Format("2006-01-02"))
			if err != nil {
				return "", err
			}
			out, err := json.Marshal(wp)
			if err != nil {
				return "", err
			}
			return string(out), nil
		},
	})
	// get_bodyweight(批次 6 营养域 v1):体重序列 + 目标差(只读;D2 一期只读
	// 体重曲线+训练数据,不建饮食表)。uid 从 ctx(WithUID,runTurn 注入)。
	tools = append(tools, Tool{
		Name:        "get_bodyweight",
		Description: "读取用户体重记录:按时间排序的称重、当前目标与差值。用于体重趋势/目标进展类问题。",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{
				"from": map[string]any{"type": "string", "description": "可选,起始日 YYYY-MM-DD"},
				"to":   map[string]any{"type": "string", "description": "可选,结束日,默认今天"},
			},
		},
		Exec: func(ctx context.Context, input map[string]any) (string, error) {
			uid := UIDFrom(ctx)
			if uid == "" {
				return "", fmt.Errorf("需要会话才能读取体重记录")
			}
			doc, err := store.GetStateDoc(ctx, db, uid)
			if err != nil {
				return "", err
			}
			var st struct {
				Bodyweight []struct {
					D string  `json:"d"`
					W float64 `json:"w"`
				} `json:"bodyweight"`
				TargetW *float64 `json:"targetW"`
				Unit    string   `json:"unit"`
			}
			if err := json.Unmarshal(doc.Doc, &st); err != nil {
				return "", fmt.Errorf("state unreadable: %w", err)
			}
			from, _ := input["from"].(string)
			to, _ := input["to"].(string)
			if to == "" {
				to = "9999-12-31"
			}
			type entry struct {
				Date  string   `json:"date"`
				Weight float64 `json:"weight"`
				Delta *float64 `json:"deltaVsGoal,omitempty"`
			}
			entries := make([]entry, 0, len(st.Bodyweight))
			var latest *entry
			for _, b := range st.Bodyweight {
				if from != "" && b.D < from {
					continue
				}
				if b.D > to {
					continue
				}
				e := entry{Date: b.D, Weight: b.W}
				if st.TargetW != nil {
					d := math.Round((b.W-*st.TargetW)*10) / 10
					e.Delta = &d
				}
				entries = append(entries, e)
				latest = &entries[len(entries)-1] // 按日期排序前最后一个 = 倒序插入点;doc 顺序即时序
			}
			// doc 内 bodyweight 不保证有序——按日期排(与 MCP get_bodyweight 同口径)。
			sort.Slice(entries, func(i, j int) bool { return entries[i].Date < entries[j].Date })
			if len(entries) > 0 {
				last := entries[len(entries)-1]
				latest = &last
			}
			unit := st.Unit
			if unit == "" {
				unit = "kg"
			}
			out, err := json.Marshal(map[string]any{
				"unit": unit,
				"goal": st.TargetW,
				"count": len(entries),
				"latest": latest,
				"entries": entries,
			})
			if err != nil {
				return "", err
			}
			return string(out), nil
		},
	})
	if wk != nil {
		tools = append(tools, Tool{
			Name:        "search_knowledge",
			Description: "检索健身知识库(动作要领/训练科学/常识)。返回相关片段与来源文件;检索不到时如实告知用户。",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string", "description": "检索问题,如 深蹲标准动作 要领"}},
				"required":   []string{"query"},
			},
			ExtractRefs: func(result string) []string {
				// 结果 JSON 每条含 ref_id(catalogue 动作 id);宽容解析。
				var parsed struct {
					Results []struct {
						RefID string `json:"ref_id"`
					} `json:"results"`
				}
				if err := json.Unmarshal([]byte(result), &parsed); err != nil {
					return nil
				}
				refs := make([]string, 0, len(parsed.Results))
				for _, r := range parsed.Results {
					if r.RefID != "" {
						refs = append(refs, r.RefID)
					}
				}
				return refs
			},
			Exec: func(ctx context.Context, input map[string]any) (string, error) {
				query, _ := input["query"].(string)
				query = strings.TrimSpace(query)
				if query == "" {
					return "", fmt.Errorf("query required")
				}
				if globalKB == "" {
					return `{"note":"知识库未配置"}`, nil
				}
				chunks, err := wk.Search(ctx, weknora.SearchRequest{
					Query: query, KnowledgeBaseIDs: []string{globalKB}, MatchCount: 5,
				})
				if err != nil {
					return "", err // D15:错误由 loop 回填,回合不中断
				}
				if len(chunks) == 0 {
					return `{"results": [], "note": "知识库无相关内容"}`, nil
				}
				type hit struct {
					Content  string  `json:"content"`
					Score    float64 `json:"score"`
					Source   string  `json:"source,omitempty"`
					RefID    string  `json:"ref_id,omitempty"`
				}
				hits := make([]hit, 0, len(chunks))
				for _, ch := range chunks {
					hits = append(hits, hit{Content: ch.Content, Score: ch.Score, Source: ch.Filename, RefID: ch.KnowledgeID})
				}
				out, err := json.Marshal(map[string]any{"results": hits})
				if err != nil {
					return "", err
				}
				return string(out), nil
			},
		})
	}
	reg, err := NewRegistry(tools...)
	if err != nil {
		// 全部为仓库内常量定义,构造错误只可能是未来维护引入——fail fast。
		panic(fmt.Errorf("agent: default tools: %w", err))
	}
	return reg
}

// numOf 宽松取数(JSON decode 的 float64 / 字符串数字都收)。
func numOf(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case string:
		var f float64
		if _, err := fmt.Sscanf(n, "%g", &f); err == nil {
			return f
		}
	}
	return 0
}
