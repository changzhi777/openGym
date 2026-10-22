-- Web Push 订阅(Node db.subs 同义;endpoint 唯一)。
CREATE TABLE push_subs (
  endpoint text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  device_id text,
  p256dh text NOT NULL,
  auth text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON push_subs (user_id);
