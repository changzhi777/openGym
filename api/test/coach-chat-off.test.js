/* AI-Native 对话面冒烟(批次 2,方案 .zcf/沐禾健身-AINative重构方案-v2.md 五):coach
   chat 七端点在两栈的最小行为契约。Node 栈:契约已冻结,全部 501 not-implemented stub。
   Go 栈(真实现):无 BIGMODEL_API_KEY → POST chat 503 off(CoachOff 同语义,前端不裸跑);
   memory/usage 不依赖 provider(needs a session only)→ 200 真数据;confirm/events 对
   未知 id → 404。MUHE_GO=<go bin> 时拉真 Go 二进制(需 PG+Redis,testmode 建独立库);
   否则只断言 Node stub 侧。 */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const API = path.join(path.dirname(fileURLToPath(import.meta.url)), '..');
const GO = process.env.MUHE_GO || '';
const SECRET = crypto.randomBytes(32).toString('hex');
const ORIGIN = 'http://localhost:8080';
const UID = 'u_chat_1';

// Session cookie in server.js makeSession() / Go auth.Sessions 同格式同钥:`uid:exp:sv`
// + HMAC-SHA256(base64url)。灰度两栈 cookie 互通就是这条约定。
function mintSession(uid, sv = 0) {
  const payload = `${uid}:${Date.now() + 86400000}:${sv}`;
  return payload + '.' + crypto.createHmac('sha256', SECRET).update(payload).digest('base64url');
}

async function startServer(t) {
  const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'gym-chat-'));
  fs.writeFileSync(path.join(dataDir, 'secret'), SECRET, { mode: 0o600 });
  fs.writeFileSync(path.join(dataDir, 'db.json'), JSON.stringify({
    users: [{ id: UID, name: 'Chatter', created: new Date().toISOString() }],
    creds: [], subs: [], invites: []
  }));
  const env = { ...process.env, MUHE_TEST: '1', PORT: '0', DATA_DIR: dataDir, ORIGIN, RP_ID: 'localhost' };
  delete env.BIGMODEL_API_KEY; // 冒烟锚点:无 provider → Go 侧 503 off
  const child = spawn(GO || process.execPath, GO ? [] : ['server.js'], {
    cwd: API, stdio: ['ignore', 'pipe', 'pipe'], env
  });
  const h = { api: '', log: '' };
  child.stdout.on('data', d => {
    h.log += d;
    const m = /gym-api on :(\d+)/.exec(h.log);
    if (m && !h.api) h.api = `http://127.0.0.1:${m[1]}`;
  });
  child.stderr.on('data', d => { h.log += d; });
  await new Promise((resolve, reject) => {
    const t0 = Date.now();
    const tick = () => {
      if (h.api) return resolve();
      if (child.exitCode !== null) return reject(new Error(`server exited: ${h.log.slice(-500)}`));
      if (Date.now() - t0 > 20000) return reject(new Error('server boot timeout'));
      setTimeout(tick, 100);
    };
    tick();
  });
  t.after(() => child.kill('SIGKILL'));
  const jar = `gymsid=${mintSession(UID)}`;
  return async (method, p, body) => {
    const res = await fetch(`${h.api}${p}`, {
      method,
      headers: { cookie: jar, ...(body ? { 'content-type': 'application/json' } : {}) },
      body: body ? JSON.stringify(body) : undefined
    });
    let json = null;
    try { json = await res.json(); } catch { /* SSE/非 JSON 不强解 */ }
    return { status: res.status, json };
  };
}

test('coach chat surface: 501 stubs on Node, off-guarded real endpoints on Go', async t => {
  const call = await startServer(t);

  // ① POST /api/coach/chat — Node 501 stub;Go 无 provider → 503 off。
  const chat = await call('POST', '/api/coach/chat', { message: '今天练什么' });
  if (GO) {
    assert.equal(chat.status, 503, JSON.stringify(chat.json));
    assert.equal(chat.json.code, 'off');
  } else {
    assert.equal(chat.status, 501);
    assert.equal(chat.json.code, 'not-implemented');
  }

  // ② GET /api/coach/memory — needs a session only:Go 回空双列表真数据;Node 501。
  const memory = await call('GET', '/api/coach/memory');
  if (GO) {
    assert.equal(memory.status, 200);
    assert.deepEqual(memory.json, { narrative: [], constraints: [] });
  } else {
    assert.equal(memory.status, 501);
  }

  // ③ GET /api/coach/usage — Go 回三池聚合 + D1 限额;Node 501。
  const usage = await call('GET', '/api/coach/usage');
  if (GO) {
    assert.equal(usage.status, 200);
    assert.equal(usage.json.limits.messagesPerDay, 50);
    assert.equal(usage.json.limits.tokensPerMonth, 3000000);
    assert.deepEqual(usage.json.day, { messages: 0, chat: 0, review: 0, embedding: 0 });
    assert.deepEqual(usage.json.month, { messages: 0, chat: 0, review: 0, embedding: 0 });
  } else {
    assert.equal(usage.status, 501);
  }

  // ④ POST confirm 未知会话 → Go 404;Node 501。
  const confirm = await call('POST', '/api/coach/chat/nonexistent/confirm', { proposalId: 'p1', decision: 'accept' });
  if (GO) {
    assert.equal(confirm.status, 404);
  } else {
    assert.equal(confirm.status, 501);
  }

  // ⑤ GET events 未知会话 → Go 404;Node 501。
  const events = await call('GET', '/api/coach/chat/nonexistent/events');
  if (GO) {
    assert.equal(events.status, 404);
  } else {
    assert.equal(events.status, 501);
  }

  // ⑥ DELETE 记忆未知条目 → Go 404;Node 501。
  const del = await call('DELETE', '/api/coach/memory/n999');
  if (GO) {
    assert.equal(del.status, 404);
  } else {
    assert.equal(del.status, 501);
  }

  // ⑦ {id} 分发器可达性:admin export(Node 曾漏映射 404,2026-11-18 审查轮修复)。
  //    契约一致性测试只查键存在不查 URL 可达——这条按真实 URL 打,双栈都要过。
  const exported = await call('GET', '/api/admin/users/u_chat_1/export');
  if (GO) {
    // 非管理员会话:路由已注册,落在鉴权/角色守卫(401/403),绝不能是 404。
    assert.ok(exported.status === 401 || exported.status === 403,
      `expected auth guard on a registered route, got ${exported.status}`);
  } else {
    assert.equal(exported.status, 501);
    assert.equal(exported.json.code, 'not-implemented');
  }
});
