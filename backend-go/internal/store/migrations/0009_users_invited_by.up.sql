-- 邀请来源(迁移器 import 的 db.json users[].invitedBy 此前未落库,round-trip 丢字段)。
ALTER TABLE users ADD COLUMN invited_by text;
