-- P5:per-user 配额行(前端配额卡数据源;缺失=套餐默认)+ admin_roles 初始 owner 索引。
CREATE TABLE user_quotas (
  user_id text PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  limits jsonb NOT NULL DEFAULT '{}',   -- {custom_exercises, media_mb, ai_calls_month}
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS admin_roles_active ON admin_roles (user_id) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS admin_audit_action ON admin_audit_log (action, created_at DESC);
