-- 0014_agent_chat_memory.up.sql — AI-Native 批次 2(方案 v2 五/六,D11/D14/D17)。
-- 对话会话 + 提议队列 + 结构化约束 + 叙事记忆 + 三池 usage。
-- 用户 id 与全库惯例一致:text(REFERENCES users(id))。

-- 对话会话:状态机 active ⇄ waiting_confirm → closed。
CREATE TABLE coach_chat_sessions (
  id         text PRIMARY KEY,               -- 16 hex(与 coach job id 同形)
  user_id    text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  state      text NOT NULL DEFAULT 'active', -- active|waiting_confirm|closed
  history    jsonb NOT NULL DEFAULT '[]',    -- 中立消息数组(llm.Message 形状)
  usage_in   bigint NOT NULL DEFAULT 0,
  usage_out  bigint NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_ccs_user ON coach_chat_sessions(user_id, updated_at DESC);

-- 提议队列:Agent 唯一写入口(确认制红线)。decision NULL = 待决。
CREATE TABLE coach_proposals (
  id         text PRIMARY KEY,               -- 会话内序号 p1/p2…
  session_id text NOT NULL REFERENCES coach_chat_sessions(id) ON DELETE CASCADE,
  tool       text NOT NULL,                  -- propose_memory / propose_change
  input      jsonb NOT NULL,
  decision   text,                           -- accept | reject | NULL
  created_at timestamptz NOT NULL DEFAULT now(),
  decided_at timestamptz
);

-- 结构化约束(D11):伤病黑名单等硬约束。确认制写入;计划工具硬读取
-- (检索不到也生效——安全约束不赌 RAG 召回)。删除=inactive,留审计痕迹。
CREATE TABLE coach_constraints (
  id         bigserial PRIMARY KEY,
  user_id    text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  text       text NOT NULL,
  active     boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_cc_user_active ON coach_constraints(user_id) WHERE active;

-- 叙事记忆(D14 用户记忆区;批次 3 起主写 WeKnora,本表为 D15 降级镜像)。
CREATE TABLE coach_memory_narrative (
  id       bigserial PRIMARY KEY,
  user_id  text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  text     text NOT NULL,
  active   boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_cmn_user_active ON coach_memory_narrative(user_id) WHERE active;

-- 三池 usage 事件(D12/D13'):chat/review/embedding,ledger 聚合源。
CREATE TABLE coach_usage_events (
  id         bigserial PRIMARY KEY,
  user_id    text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  pool       text NOT NULL,                 -- chat | review | embedding
  tokens_in  bigint NOT NULL DEFAULT 0,
  tokens_out bigint NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_cue_user_time ON coach_usage_events(user_id, created_at);
