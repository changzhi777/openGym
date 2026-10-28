/* Media GC over the Go backend: the sweep endpoint reads the caller's stored state, keeps what
   it references, deletes the rest (grace 0 — "reset everything"), and never deletes anything for
   a profile whose state cannot be read. Uploads land as <hash>.<ext>, the shape the Node server
   uses, so either stack can read and reclaim the other's files. Go server in a child
   (MUHE_GO=...); skipped under Node, whose media store is the reference. */
import { test } from 'node:test';
import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { boundPort } from './helpers.mjs';

const API = path.join(path.dirname(fileURLToPath(import.meta.url)), '..');
const GO = process.env.MUHE_GO || '';
const SECRET = crypto.randomBytes(32).toString('hex');
const ORIGIN = 'http://localhost:8080';
const sha = b => crypto.createHash('sha256').update(b).digest('hex');

// Minimal valid webp: RIFF + 4 length bytes + "WEBP" — the only positions Sniff looks at.
const webp = tag => Buffer.concat([Buffer.from('RIFF\x00\x00\x00\x00WEBPVTFT'), Buffer.from(tag)]);

function mintSession(uid) {
  const payload = `${uid}:${Date.now() + 86400000}:0`;
  return payload + '.' + crypto.createHmac('sha256', SECRET).update(payload).digest('base64url');
}

const U1 = 'u_media_1';

async function startServer(t) {
  const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'gym-mediasweep-'));
  fs.writeFileSync(path.join(dataDir, 'secret'), SECRET, { mode: 0o600 });
  fs.writeFileSync(path.join(dataDir, 'db.json'), JSON.stringify({
    users: [{ id: U1, name: 'Media Tester', created: new Date().toISOString() }],
    creds: [], subs: [], invites: []
  }));
  const child = spawn(GO, [], {
    cwd: API, stdio: ['ignore', 'pipe', 'pipe'],
    env: { ...process.env, MUHE_TEST: '1', PORT: '0', DATA_DIR: dataDir, ORIGIN, RP_ID: 'localhost' }
  });
  const h = { api: '', log: '', dataDir };
  child.stdout.on('data', d => h.log += d);
  child.stderr.on('data', d => h.log += d);
  t.after(() => { child.kill('SIGKILL'); fs.rmSync(dataDir, { recursive: true, force: true }); });
  h.api = `http://127.0.0.1:${await boundPort(child, () => h.log)}`;
  const cookie = `gymsid=${mintSession(U1)}`;
  h.json = async (method, p, body) => {
    const r = await fetch(`${h.api}${p}`, {
      method,
      headers: { 'Content-Type': 'application/json', 'Sec-Fetch-Site': 'same-origin', Cookie: cookie },
      body: body === undefined ? undefined : JSON.stringify(body)
    });
    return { status: r.status, body: await r.json().catch(() => null) };
  };
  h.upload = async hash => {
    const data = webp(hash.slice(0, 8));
    const r = await fetch(`${h.api}/api/media/${sha(data)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/octet-stream', 'Sec-Fetch-Site': 'same-origin', Cookie: cookie },
      body: data
    });
    return { status: r.status, body: await r.json().catch(() => null), sha: sha(data), size: data.length };
  };
  h.get = async hash => (await fetch(`${h.api}/api/media/${hash}`, { headers: { Cookie: cookie } })).status;
  return h;
}

test('sweep keeps what the state references and deletes the rest', { skip: !GO && 'Go backend only' }, async t => {
  const h = await startServer(t);
  const a = await h.upload('keep-me');          // will stay referenced
  const b = await h.upload('drop-me');          // will not
  assert.equal(a.status, 200, JSON.stringify(a.body));
  assert.equal(b.status, 200, JSON.stringify(b.body));
  assert.equal(await h.get(a.sha), 200, 'upload is readable back');
  assert.equal(await h.get(b.sha), 200);

  // A fresh profile answers {state:null, rev:0}; store a state referencing only the first file.
  const cur = await h.json('GET', '/api/data');
  assert.equal(cur.status, 200);
  const put = await h.json('PUT', '/api/data', {
    state: { workouts: [{ id: 'w1', name: 'W', media: [{ hash: a.sha }] }] },
    baseRev: cur.body.rev
  });
  assert.equal(put.status, 200, JSON.stringify(put.body));

  const sweep = await h.json('POST', '/api/media/sweep', {});
  assert.equal(sweep.status, 200, JSON.stringify(sweep.body));
  assert.equal(sweep.body.removed, 1);
  assert.equal(sweep.body.freedBytes, b.size);
  assert.equal(sweep.body.usage.count, 1);
  assert.equal(sweep.body.usage.bytes, a.size);
  assert.equal(await h.get(a.sha), 200, 'referenced file stays');
  assert.equal(await h.get(b.sha), 404, 'unreferenced file is gone');
});

test('nothing is deleted for a profile whose state cannot be read', { skip: !GO && 'Go backend only' }, async t => {
  const h = await startServer(t);
  const a = await h.upload('no-state');
  assert.equal(a.status, 200);
  const sweep = await h.json('POST', '/api/media/sweep', {});
  assert.equal(sweep.status, 200, JSON.stringify(sweep.body));
  assert.equal(sweep.body.removed, 0, 'no state means no inferences');
  assert.equal(await h.get(a.sha), 200);
});

test('missing answers usage alongside the hashes', { skip: !GO && 'Go backend only' }, async t => {
  const h = await startServer(t);
  const a = await h.upload('usage');
  const r = await h.json('POST', '/api/media/missing', { hashes: [a.sha, 'f'.repeat(64)] });
  assert.equal(r.status, 200, JSON.stringify(r.body));
  assert.deepEqual(r.body.missing, ['f'.repeat(64)]);
  assert.equal(r.body.usage.count, 1);
  assert.equal(r.body.usage.bytes, a.size);
});
