-- 实例开关(契约 AdminSettings;本期唯一键 invite_only,PUT 持久化/GET 答生效值)。
CREATE TABLE instance_settings (
  key text PRIMARY KEY,
  value jsonb NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);
