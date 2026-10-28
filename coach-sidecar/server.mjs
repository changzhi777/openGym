#!/usr/bin/env node
/* coach-sidecar — Go 栈的 AI 教练过渡进程(fork)。
 *
 * 复用 api/coach 的全部逻辑(路由/任务/配置/适配器),作为独立小服务挂在 127.0.0.1。
 * 鉴权完全由 Go 完成:Go 反代时以 X-Coach-UID(已验会话)与 X-Coach-Admin(已验角色)注入,
 * 本进程自己不做会话/角色判定——它只被同机的 Go API 访问。
 * 数据桥:COACH_STATE_FROM=go 时 readState/listUserIds(jobs.js)经 Go 的 /internal 端点读 PG;
 * 教练自有记录(DATA/coach/)、配置(coach.json)、handle 秘钥(secret)仍走共享 DATA_DIR 文件。
 *
 * 用法: NODE_ENV=production COACH_STATE_FROM=go GO_API=http://127.0.0.1:3100 \
 *        INTERNAL_TOKEN=<与 Go 相同> DATA_DIR=<同 Go> node coach-sidecar/server.mjs
 */
import http from 'node:http';
import { coachRoutes } from '../api/coach/routes.js';
import * as coachJobs from '../api/coach/jobs.js';
import * as cfgStore from '../api/coach/config.js';
import { startCadence } from '../api/coach/cadence.js';
import { startWarmup } from '../api/coach/warmup.js';

const PORT = +(process.env.PORT || 3111);
const MAX_BODY = 5 * 1024 * 1024;

const json = (res, code, obj) => {
  const body = JSON.stringify(obj);
  res.writeHead(code, { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(body) });
  res.end(body);
};

const readBody = req => new Promise((resolve, reject) => {
  const chunks = []; let size = 0, over = false;
  req.on('data', d => {
    size += d.length;
    if (over) return;             // already refused: drain quietly so the answer gets through
    if (size > MAX_BODY) { over = true; chunks.length = 0; reject(http413()); return; }
    chunks.push(d);
  });
  req.on('end', () => {
    if (over) return;
    if (!chunks.length) return resolve({});
    try { resolve(JSON.parse(Buffer.concat(chunks).toString('utf8'))); }
    catch { resolve({}); }        // 与 Node 主进程 readBody 同宽:坏 JSON 当空体
  });
  req.on('error', reject);
});
const http413 = () => Object.assign(new Error('body too large'), { status: 413 });

// 鉴权已由 Go 完成:uid/admin 只从注入头读,不解析 cookie。
const readSession = req => {
  const uid = req.headers['x-coach-uid'];
  return typeof uid === 'string' && uid ? { id: uid } : null;
};
const requireAdmin = (req, res) => {
  if (req.headers['x-coach-admin'] === '1') return readSession(req);
  json(res, 403, { error: 'admin required' });
  return null;
};

const routes = coachRoutes({ json, readBody, readSession, requireAdmin });

const server = http.createServer(async (req, res) => {
  let pathname;
  try { pathname = new URL(req.url, 'http://localhost').pathname; }
  catch { return json(res, 400, { error: 'bad url' }); }
  // Go 回读面:同 INTERNAL_TOKEN 相认(未设 token 时全部 404,fail-closed)。
  if (pathname.startsWith('/internal/coach/')) {
    if (!process.env.INTERNAL_TOKEN || req.headers.authorization !== `Bearer ${process.env.INTERNAL_TOKEN}`) {
      return json(res, 404, { error: 'no such route' });
    }
    if (req.method === 'GET' && pathname === '/internal/coach/public-config') {
      return json(res, 200, cfgStore.publicConfig());
    }
    return json(res, 404, { error: 'no such route' });
  }
  const handler = routes[`${req.method} ${pathname}`];
  if (!handler) return json(res, 404, { error: 'no such route' });
  try { await handler(req, res); }
  catch (e) {
    if (!res.headersSent) json(res, e.status || 500, { error: e.message || 'internal' });
    else res.end();
    if (!e.status) console.error('coach-sidecar:', e);
  }
});

server.listen(PORT, '127.0.0.1', () => console.log(`coach-sidecar on 127.0.0.1:${PORT} (go=${process.env.COACH_STATE_FROM === 'go'})`));

coachJobs.recoverOnBoot();
// A ready proposal is the one event worth a notification(FR-38/E4)——推送在 Go 侧,走内部端点。
coachJobs.setProposalHook((uid, pending) => {
  const n = (pending?.changes || []).length;
  if (!n) return;
  goPost('/internal/coach/push', {
    uid, title: 'Your Coach has been reading',
    body: n === 1 ? '1 suggestion after this week' : `${n} suggestions after this week`,
    tag: 'coach-proposal', url: '#/coach'
  });
});
// 每日巡检的群体在 PG(jobs.listUserIds 已走桥);时区从各 profile 的 state 读(同样走桥)。
// userNow 与 server.js 同实现:星期按用户自己的日历算。
function userNow(tz) {
  try {
    const parts = new Intl.DateTimeFormat('en-CA', {
      timeZone: tz, hour12: false,
      year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'
    }).formatToParts(new Date());
    const g = t => parts.find(p => p.type === t)?.value;
    const date = `${g('year')}-${g('month')}-${g('day')}`;
    return { date, hhmm: `${g('hour')}:${g('minute')}`, weekday: new Date(date + 'T12:00:00Z').getUTCDay() };
  } catch { return null; } // 坏时区:跳过这个用户,不猜
}
startCadence({ users: () => coachJobs.listUserIds().map(id => ({ id })), userNow });
startWarmup();

function goPost(p, obj) {
  fetch(`${process.env.GO_API || 'http://127.0.0.1:3100'}${p}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${process.env.INTERNAL_TOKEN || ''}` },
    body: JSON.stringify(obj)
  }).catch(() => { /* 推送尽力而为:Go 不在时静默 */ });
}
