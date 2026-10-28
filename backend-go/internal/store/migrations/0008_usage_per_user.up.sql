-- 0008:计费维度从 org 落到 user(B2C 先行;org 化在多租户落地时再迁一次)。
-- 表是空的(生产未用过),直接改形最干净;usage 语义不变:resource+period 唯一计数。
ALTER TABLE usage_counters DROP CONSTRAINT usage_counters_pkey;
ALTER TABLE usage_counters DROP COLUMN org_id;
ALTER TABLE usage_counters ADD COLUMN user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE usage_counters ADD PRIMARY KEY (user_id, resource, period);
