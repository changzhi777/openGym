-- passkey 元数据(#95:Settings 列表显示;与 Node creds 的 created/lastUsed/name 对应)。
ALTER TABLE passkeys ADD COLUMN name text;
ALTER TABLE passkeys ADD COLUMN last_used timestamptz;
