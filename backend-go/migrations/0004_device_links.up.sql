-- 设备配对码(浏览器签发 → 新设备加 passkey):存库跨进程;h 绑定 challenge 防换码。
CREATE TABLE device_links (
  code_hash text PRIMARY KEY,     -- 码的 sha256(明码不落库)
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  h text NOT NULL,                -- 绑定随机数:challenge 与 link 的对不可互换
  exp timestamptz NOT NULL
);
