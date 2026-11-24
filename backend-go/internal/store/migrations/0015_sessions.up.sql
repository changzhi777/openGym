-- 0015_sessions — 会话与设备管理(2026-11-25)。
-- 每次登录签发一条会话(sid 旁路 cookie gymsid_sid;主 HMAC cookie 格式不变,
-- 与 Node 字节级兼容不破坏)。无 sid 的旧会话cookie 兼容有效,重登即纳入管理。
CREATE TABLE sessions (
    id          text PRIMARY KEY,                 -- crypto/rand 32B hex
    user_id     text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_name text NOT NULL DEFAULT '',
    user_agent  text NOT NULL DEFAULT '',
    ip          text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    revoked_at  timestamptz
);
CREATE INDEX idx_sessions_user ON sessions(user_id, created_at DESC);
