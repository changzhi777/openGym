// tools.go — 工具注册表(方案 v2 五.5.2:四件套注册,写操作=提议而非执行)。
//
// 与标准 agent 框架的本质差异落在这里:Proposal=true 的工具永远不会被 Exec,
// 调用即转确认队列——"未经确认写入为零"(AI 三红线之三)的实现载体。
// 工具实现来自两路:① 算法单源翻译(如 onerm.go,数据接入在数据工具组装时调用);
// ② WeKnora 知识/记忆检索(批次 3,HTTP 内网)。
package agent

import (
	"context"
	"fmt"
	"sort"

	"github.com/changzhi777/openGym/backend-go/internal/llm"
)

// Tool 是注册给 tool-loop 的一个工具。Exec 在 Proposal=true 时必须为 nil
// (构造时由 validate 检查),循环层永不调用。
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any // JSON Schema 形状,直接喂给模型
	Proposal    bool           // true = 提议型:调用→确认队列,不执行
	// ExtractRefs 从工具结果里抽取引用 id(如 catalogue 动作 id),随 tool_call
	// 事件带给前端渲染来源卡片。nil = 无引用语义。
	ExtractRefs func(result string) []string
	// Exec 执行只读工具。返回字符串进上下文(循环层截断到 MaxToolBytes)。
	Exec func(ctx context.Context, input map[string]any) (string, error)
}

// Registry 是工具注册表(定长,构建后不变,可并发读)。
type Registry []Tool

// NewRegistry 构造并校验:名字非空、只读工具必须有 Exec、提议工具不得有 Exec。
func NewRegistry(tools ...Tool) (Registry, error) {
	seen := map[string]bool{}
	for i := range tools {
		t := &tools[i]
		if t.Name == "" {
			return nil, fmt.Errorf("agent: tool #%d has empty name", i)
		}
		if seen[t.Name] {
			return nil, fmt.Errorf("agent: duplicate tool %q", t.Name)
		}
		seen[t.Name] = true
		if t.Proposal && t.Exec != nil {
			return nil, fmt.Errorf("agent: proposal tool %q must not carry Exec", t.Name)
		}
		if !t.Proposal && t.Exec == nil {
			return nil, fmt.Errorf("agent: read-only tool %q must carry Exec", t.Name)
		}
	}
	return Registry(tools), nil
}

// Defs 输出给模型的工具契约(顺序 = 注册序,稳定)。
func (r Registry) Defs() []llm.ToolDef {
	defs := make([]llm.ToolDef, 0, len(r))
	for _, t := range r {
		defs = append(defs, llm.ToolDef{Name: t.Name, Description: t.Description, InputSchema: t.Schema})
	}
	return defs
}

// Get 按名取工具;未注册返回 nil。
func (r Registry) Get(name string) *Tool {
	for i := range r {
		if r[i].Name == name {
			return &r[i]
		}
	}
	return nil
}

// Names 返回全部工具名(排序,用于日志与治理台展示)。
func (r Registry) Names() []string {
	names := make([]string, 0, len(r))
	for _, t := range r {
		names = append(names, t.Name)
	}
	sort.Strings(names)
	return names
}

// execReadonly 执行一个只读工具并把结果截断到 maxBytes。
func (r Registry) execReadonly(ctx context.Context, t *Tool, input map[string]any, maxBytes int) (string, error) {
	out, err := t.Exec(ctx, input)
	if err != nil {
		return "", err
	}
	return truncateRunes(out, maxBytes), nil
}

// truncateRunes 按 UTF-8 字节截断(不切半个 rune,截断处补省略标记)。
func truncateRunes(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	cut := s[:maxBytes]
	// 回退到完整 rune 边界。
	for len(cut) > 0 && !isRuneStart(cut[len(cut)-1]) {
		cut = cut[:len(cut)-1]
	}
	return cut + "…[截断]"
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }
