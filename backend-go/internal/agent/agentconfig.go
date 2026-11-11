// agentconfig.go — AI-Native 治理配置(批次 6,D6/治理台 v1):MAX_TURNS 与审核词表。
//
// 存 instance_settings(key=coach_agent_config);每个对话回合读取(热生效,
// 无重启)。缺省回安全值(4 轮/空词表);坏 JSON 按未配置处理(与 coach_limits
// 同防御)。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentConfig 治理台可编辑的两项。
type AgentConfig struct {
	MaxTurns    int      `json:"maxTurns"`
	ReviewWords []string `json:"reviewWords"`
}

// DefaultAgentConfig 方案 v2 默认(MAX_TURNS=4;词表空——env MUHE_REVIEW_WORDS
// 仍是初始兜底,见 Service.reviewerFor 的合并语义)。
var DefaultAgentConfig = AgentConfig{MaxTurns: 4, ReviewWords: []string{}}

const agentConfigKey = "coach_agent_config"

// GetAgentConfig 读生效配置;未配置/坏值回默认。
func GetAgentConfig(ctx context.Context, db *pgxpool.Pool) (AgentConfig, error) {
	out := DefaultAgentConfig
	row := db.QueryRow(ctx, `SELECT value FROM instance_settings WHERE key=$1`, agentConfigKey)
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.MaxTurns <= 0 {
		return DefaultAgentConfig, nil
	}
	return out, nil
}

// SetAgentConfig 全量写入(调用方已做范围校验)。
func SetAgentConfig(ctx context.Context, db *pgxpool.Pool, c AgentConfig) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `INSERT INTO instance_settings (key, value, updated_at) VALUES ($1,$2,now())
		ON CONFLICT (key) DO UPDATE SET value=$2, updated_at=now()`, agentConfigKey, raw)
	if err != nil {
		return fmt.Errorf("agent: set agent config: %w", err)
	}
	return nil
}

// reviewerFor 组装生效审核链:配置词表 + env 词表(初始兜底)并集 → LLM 复检
// (provider 存在时)。每次 Chat 调用一次——配置热生效。
func (s *Service) reviewerFor(ctx context.Context) Reviewer {
	var chain Chained
	cfg, err := GetAgentConfig(ctx, s.DB)
	if err == nil && len(cfg.ReviewWords) > 0 {
		chain = append(chain, NewWordlistReviewer(cfg.ReviewWords))
	}
	if s.EnvWords != "" {
		chain = append(chain, NewWordlistReviewer(strings.Split(s.EnvWords, ",")))
	}
	if s.Provider != nil {
		chain = append(chain, NewLLMReviewer(s.Provider))
	}
	if len(chain) == 0 {
		return nil
	}
	return chain
}
