-- 邀请码(与 Node db.invites 同语义:16hex 码精确比对、一次性、可撤销)。
-- revoked 列服务于历史数据导入(Node 老数据可带 revoked:true);现行 revoke 端点仍物理删除。
CREATE TABLE admin_invites (
  code text PRIMARY KEY,
  note text NOT NULL DEFAULT '',
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  used_by text REFERENCES users(id),
  used_at timestamptz,
  revoked boolean NOT NULL DEFAULT false
);
