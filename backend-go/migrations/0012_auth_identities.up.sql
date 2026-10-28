-- 认证重构:身份表模型(手机号/微信为可挂接凭证;passkey/配对退役)。
CREATE TABLE auth_identities (
  id bigserial PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('phone','wx')),
  identifier text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (kind, identifier)
);
CREATE INDEX auth_identities_user ON auth_identities (user_id);
DROP TABLE IF EXISTS device_links;
DROP TABLE IF EXISTS passkeys;
