-- 0001_init.sql — 沐禾健身 SaaS 后端初始 schema(fork,Go)
-- 方案:.zcf/plan/current/saas-backend-go.md §二(12 核心 + Admin 四件套 + 计费四件)
-- 原则:可查询/分析的关系化;整块读写的留 JSONB;字段级 merge 落 states.doc/doc_ts。

CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- gen_random_uuid()

-- ===== 认证与租户 =====
CREATE TABLE users (
  id text PRIMARY KEY,
  name text NOT NULL,
  email text UNIQUE,
  disabled boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE passkeys (
  id text PRIMARY KEY,                       -- credential id
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  public_key bytea NOT NULL,
  counter bigint NOT NULL DEFAULT 0,
  transports text[],
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON passkeys (user_id);

CREATE TABLE orgs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  type text NOT NULL DEFAULT 'personal' CHECK (type IN ('personal', 'team')),
  name text NOT NULL,
  owner_user_id text REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE org_members (
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role text NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member', 'coach')),
  joined_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (org_id, user_id)
);

-- ===== 核心:字段级 merge 载体(替代 state-<uid>.json) =====
CREATE TABLE states (
  user_id text PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  org_id uuid REFERENCES orgs(id),
  doc jsonb NOT NULL DEFAULT '{}',           -- settings 等 ~30 个小对象
  doc_ts jsonb NOT NULL DEFAULT '{}',        -- 每字段 _ts 映射(与 doc 同构)
  rev bigint NOT NULL DEFAULT 0,
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- ===== 高频关系化(M2 分析直接 SQL) =====
CREATE TABLE workouts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  org_id uuid REFERENCES orgs(id),
  date text NOT NULL,                        -- YYYY-MM-DD(与客户端一致)
  routine_id text,
  started_at timestamptz,
  ended_at timestamptz,
  note text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON workouts (user_id, date DESC);

CREATE TABLE sets (
  id bigserial PRIMARY KEY,
  workout_id uuid NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
  exercise_id text NOT NULL,
  idx int NOT NULL,
  reps int,
  weight numeric,
  rir int,
  is_warmup boolean NOT NULL DEFAULT false,
  done_at timestamptz,
  UNIQUE (workout_id, idx)
);

CREATE TABLE routines (
  id text PRIMARY KEY,                       -- 客户端自有 id(与 state 文档一致)
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  org_id uuid REFERENCES orgs(id),
  name text NOT NULL,
  payload jsonb NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON routines (user_id);

CREATE TABLE exercises (
  id text PRIMARY KEY,                       -- catalogue 4 位 id;自定义加前缀
  owner_user_id text REFERENCES users(id) ON DELETE CASCADE,  -- NULL = 系统 5,632 条
  name text NOT NULL,
  body_part text,
  equipment text,
  payload jsonb NOT NULL DEFAULT '{}'
);

CREATE TABLE bodyweights (
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date text NOT NULL,
  kg numeric NOT NULL,
  PRIMARY KEY (user_id, date)
);

CREATE TABLE measurements (
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date text NOT NULL,
  payload jsonb NOT NULL,
  PRIMARY KEY (user_id, date)
);

CREATE TABLE media (
  sha256 text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind text NOT NULL,
  bytes bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- ===== Admin 四件套(§Admin 落库细节) =====
CREATE TYPE admin_role AS ENUM ('super_admin', 'ops', 'support');

CREATE TABLE admin_roles (
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role admin_role NOT NULL,
  granted_by text REFERENCES users(id),
  granted_at timestamptz NOT NULL DEFAULT now(),
  revoked_at timestamptz,
  PRIMARY KEY (user_id, role)
);

CREATE TABLE admin_audit_log (
  id bigserial PRIMARY KEY,
  admin_user_id text NOT NULL,
  action text NOT NULL,
  target_type text,
  target_id text,
  detail jsonb,
  ip inet,
  user_agent text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON admin_audit_log (admin_user_id, created_at DESC);
CREATE INDEX ON admin_audit_log (target_type, target_id);
-- append-only:由应用账号权限保证(REVOKE UPDATE, DELETE),迁移期先注释
-- REVOKE UPDATE, DELETE ON admin_audit_log FROM PUBLIC;

-- ===== 计费四件套(§多租户与计费,provider 无关) =====
CREATE TABLE plans (
  id text PRIMARY KEY,                       -- free / pro / team
  name text NOT NULL,
  price_cents int NOT NULL DEFAULT 0,
  currency text NOT NULL DEFAULT 'CNY',
  interval text NOT NULL DEFAULT 'month',
  limits jsonb NOT NULL DEFAULT '{}'
);

CREATE TABLE subscriptions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  plan_id text NOT NULL REFERENCES plans(id),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'past_due', 'canceled')),
  period_start timestamptz NOT NULL DEFAULT now(),
  period_end timestamptz NOT NULL,
  provider text,
  provider_ref text,
  trial_ends_at timestamptz
);
CREATE INDEX ON subscriptions (org_id) WHERE status = 'active';

CREATE TABLE usage_counters (
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  resource text NOT NULL,
  period text NOT NULL,                      -- 'YYYY-MM'
  used bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (org_id, resource, period)
);

CREATE TABLE invoices (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  period text NOT NULL,
  amount_cents int NOT NULL,
  currency text NOT NULL DEFAULT 'CNY',
  status text NOT NULL DEFAULT 'open',
  paid_at timestamptz,
  provider_ref text
);

-- ===== RLS:两 DB 角色为 P3(数据核心)准备,P1 骨架先立结构 =====
-- CREATE ROLE app_user NOINHERIT; CREATE ROLE app_admin NOINHERIT;  -- 由 migrations/0002 建,需超级用户
