-- 教练工作台 v1:coach 角色 + 教练-学员绑定关系(绑定模式:教练只看绑定学员)。
-- 角色复用 admin_roles 表(RequireCoach 闸门机制现成;coach/super_admin 均可访问)。
-- ⚠️ admin_roles 的活跃唯一索引(user_id WHERE revoked_at IS NULL)意味着一个用户
-- 只能持有一个活跃角色:super_admin 无需另授 coach(直接以 super 身份访问工作台);
-- 纯教练用户授 coach。兼任场景 v1 不支持。
ALTER TYPE admin_role ADD VALUE IF NOT EXISTS 'coach';

CREATE TABLE coach_assignments (
  id bigserial PRIMARY KEY,
  coach_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  student_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (coach_id, student_id)
);
CREATE INDEX coach_assignments_coach ON coach_assignments (coach_id);
