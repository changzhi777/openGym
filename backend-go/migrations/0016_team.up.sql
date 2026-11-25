-- 0016_team — 团队教练域三表(团队模式;语义真源 = muhe-fit api-go 0014+0015 合并移植,
-- 2026-11-26 测试版 PWA 栈落地)。
-- 与 v1 coach-console 绑定模式(coach_assignments,0013)并存渐进:团队模式供
-- 建团/QR 邀请/快照分发/看板;教练建团资格复用 admin_roles 的 coach 角色闸门。
-- 展示名直接 join users.name(测试版 users 无 display_name 列,与 muhe-fit 差异点)。

CREATE TABLE teams (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_uid text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  archived_at timestamptz
);
-- 一教练一活团:活团唯一,归档后可再建
CREATE UNIQUE INDEX teams_one_live_per_owner ON teams (owner_uid) WHERE archived_at IS NULL;

CREATE TABLE team_members (
  team_id uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  uid text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role text NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
  scope jsonb NOT NULL DEFAULT '{"tier":"basic"}',
  joined_at timestamptz NOT NULL DEFAULT now(),
  ended_at timestamptz
);
CREATE INDEX team_members_of_uid ON team_members (uid) WHERE ended_at IS NULL;
CREATE INDEX team_members_of_team ON team_members (team_id) WHERE ended_at IS NULL;
-- 全行唯一(软删行与复活行同键,复活语义依赖 ON CONFLICT 命中;muhe-fit 0015 教训:
-- 0014 漏建此约束时 AddMember 的 ON CONFLICT 必报 42P10,首次入团即炸)
CREATE UNIQUE INDEX team_members_team_uid ON team_members (team_id, uid);

-- 训练计划分发:快照中转,教练不写学员档;状态机 distributed→received(M1 不用)→loaded/declined;
-- 重分发 = 新行 + superseded_by 版本链;撤回仅限 distributed。
CREATE TABLE plan_assignments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  team_id uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  coach_uid text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  member_uid text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  plan_name text NOT NULL,
  plan_snapshot jsonb NOT NULL,
  status text NOT NULL DEFAULT 'distributed' CHECK (status IN ('distributed', 'received', 'loaded', 'declined')),
  superseded_by uuid REFERENCES plan_assignments(id),
  distributed_at timestamptz NOT NULL DEFAULT now(),
  settled_at timestamptz
);
CREATE INDEX plan_assignments_pending ON plan_assignments (member_uid) WHERE status = 'distributed';
CREATE INDEX plan_assignments_of_team ON plan_assignments (team_id);
