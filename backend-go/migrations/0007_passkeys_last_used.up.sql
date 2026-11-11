-- 0007:passkey 最近使用时间(Node db.json 的 cred.lastUsed;round-trip 保真用)
-- 缺失(NULL)=从未记录;导出时省略该键,与老数据同形。
ALTER TABLE passkeys ADD COLUMN IF NOT EXISTS last_used timestamptz;
