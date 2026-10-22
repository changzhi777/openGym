-- session version:logout-all 时 +1,全部旧 cookie 失效(与 Node user.sv 同义)。
ALTER TABLE users ADD COLUMN sv bigint NOT NULL DEFAULT 0;
