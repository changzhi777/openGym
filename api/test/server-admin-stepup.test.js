/* Admin step-up: a sensitive action needs proof that whoever holds the cookie is the owner
   right now — a passkey assertion made for this request. The server takes a one-use challenge
   from /api/login/options, checks the credential belongs to the signed-in account, verifies the
   signature, and only then opens the 5-minute window. Real Go server in a child (MUHE_GO=...);
   skipped under the Node server, where POST /api/admin/step-up is still a 501 stub. */
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
const b64u = b => Buffer.from(b).toString('base64url');
const sha = b => crypto.createHash('sha256').update(b).digest();

/* One in-memory passkey: a P-256 keypair and the COSE shape go-webauthn stores, plus the
   attestation/assertion JSON navigator.credentials would hand back. Same construction as
   server-first-user-admin.test.js — copied rather than shared so the upstream test keeps its
   own copy through a rebase. */
function softPasskey() {
  const { privateKey, publicKey } = crypto.generateKeyPairSync('ec', { namedCurve: 'P-256' });
  const jwk = publicKey.export({ format: 'jwk' });
  const cose = Buffer.concat([
    Buffer.from([0xa5, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21, 0x58, 0x20]), Buffer.from(jwk.x, 'base64url'),
    Buffer.from([0x22, 0x58, 0x20]), Buffer.from(jwk.y, 'base64url')
  ]);
  const raw = crypto.randomBytes(16);
  const id = b64u(raw);
  let counter = 0;
  return {
    id,
    row: (userId, extra = {}) => ({ id, userId, publicKey: cose.toString('base64url'), counter: 0, transports: ['internal'], ...extra }),
    assertion(challenge, { userHandle } = {}) {
      const clientDataJSON = Buffer.from(JSON.stringify({ type: 'webauthn.get', challenge, origin: ORIGIN, crossOrigin: false }));
      const c = Buffer.alloc(4); c.writeUInt32BE(++counter);
      const authData = Buffer.concat([sha('localhost'), Buffer.from([0x05]), c]);
      const signature = crypto.sign('sha256', Buffer.concat([authData, sha(clientDataJSON)]), privateKey);
      return {
        id, rawId: id, type: 'public-key', clientExtensionResults: {}, authenticatorAttachment: 'platform',
        response: { clientDataJSON: b64u(clientDataJSON), authenticatorData: b64u(authData), signature: b64u(signature), userHandle: userHandle || null }
      };
    }
  };
}

// Session cookie in server.js makeSession() format: `uid:exp:sv` + HMAC-SHA256 over SECRET.
function mintSession(uid, sv = 0) {
  const payload = `${uid}:${Date.now() + 86400000}:${sv}`;
  return payload + '.' + crypto.createHmac('sha256', SECRET).update(payload).digest('base64url');
}

const ADMIN = 'u_adm_1', VICTIM = 'u_vic_1';

async function startServer(t, { key }) {
  const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'gym-stepup-'));
  fs.writeFileSync(path.join(dataDir, 'secret'), SECRET, { mode: 0o600 });
  fs.writeFileSync(path.join(dataDir, 'db.json'), JSON.stringify({
    users: [
      { id: ADMIN, name: 'Adminna', created: new Date().toISOString(), admin: true },
      { id: VICTIM, name: 'Mallory', created: new Date().toISOString() }
    ],
    creds: key ? [key.row(ADMIN)] : [], subs: [], invites: []
  }));
  const child = spawn(GO, [], {
    cwd: API, stdio: ['ignore', 'pipe', 'pipe'],
    env: { ...process.env, MUHE_TEST: '1', PORT: '0', DATA_DIR: dataDir, ORIGIN, RP_ID: 'localhost' }
  });
  const h = { api: '', log: '' };
  child.stdout.on('data', d => h.log += d);
  child.stderr.on('data', d => h.log += d);
  t.after(() => { child.kill('SIGKILL'); fs.rmSync(dataDir, { recursive: true, force: true }); });
  h.api = `http://127.0.0.1:${await boundPort(child, () => h.log)}`;
  h.req = async (method, p, { body, cookie } = {}) => {
    const r = await fetch(`${h.api}${p}`, {
      method,
      headers: { 'Content-Type': 'application/json', 'Sec-Fetch-Site': 'same-origin', ...(cookie ? { Cookie: cookie } : {}) },
      body: body === undefined ? undefined : JSON.stringify(body)
    });
    return { status: r.status, body: await r.json() };
  };
  return h;
}

const adminCookie = () => `gymsid=${mintSession(ADMIN)}`;

test('step-up verifies the assertion before it opens the window', { skip: !GO && 'Go backend only' }, async t => {
  const key = softPasskey();
  const h = await startServer(t, { key });
  const cookie = adminCookie();

  // No window yet: the sensitive action is refused with the code stepUp.js retries on.
  let r = await h.req('POST', '/api/admin/user/disable', { cookie, body: { id: VICTIM, disabled: true } });
  assert.equal(r.status, 403);
  assert.equal(r.body.code, 'passkey-required');

  // A step-up with nothing to verify is refused the same way.
  r = await h.req('POST', '/api/admin/step-up', { cookie, body: {} });
  assert.equal(r.status, 403);
  assert.equal(r.body.code, 'passkey-required');

  // A credential that belongs to someone else never opens this session's window.
  const stranger = softPasskey();
  const x = (await h.req('POST', '/api/login/options', { body: {} })).body;
  r = await h.req('POST', '/api/admin/step-up', { cookie, body: { cid: x.cid, credential: stranger.assertion(x.options.challenge) } });
  assert.equal(r.status, 403, JSON.stringify(r.body));
  assert.equal(r.body.code, 'passkey');

  // A signature over a challenge the server never issued is refused.
  r = await h.req('POST', '/api/admin/step-up', { cookie, body: { cid: 'made-up', credential: key.assertion(x.options.challenge) } });
  assert.equal(r.status, 403);
  assert.equal(r.body.code, 'challenge-expired');

  // The real thing: challenge from /api/login/options, assertion by the account's own key.
  // userHandle carries the account id, the way a browser returns it for a discoverable key.
  const { cid, options } = (await h.req('POST', '/api/login/options', { body: {} })).body;
  r = await h.req('POST', '/api/admin/step-up', { cookie, body: { cid, credential: key.assertion(options.challenge, { userHandle: b64u(ADMIN) }) } });
  assert.equal(r.status, 200, JSON.stringify(r.body) + '\n' + h.log);
  assert.equal(r.body.ok, true);
  assert.ok(r.body.until > Date.now(), 'window ends in the future');

  // The same challenge is gone from the store: replaying it is not a second proof.
  r = await h.req('POST', '/api/admin/step-up', { cookie, body: { cid, credential: key.assertion(options.challenge) } });
  assert.equal(r.status, 403);
  assert.equal(r.body.code, 'challenge-expired');

  // Within the window the action goes through.
  r = await h.req('POST', '/api/admin/user/disable', { cookie, body: { id: VICTIM, disabled: true } });
  assert.equal(r.status, 200, JSON.stringify(r.body));
});

test('a signature made for another origin fails verification', { skip: !GO && 'Go backend only' }, async t => {
  const key = softPasskey();
  const h = await startServer(t, { key });
  const { cid, options } = (await h.req('POST', '/api/login/options', { body: {} })).body;
  const cred = key.assertion(options.challenge, { userHandle: b64u(ADMIN) });
  // Re-sign the client data with a different origin, as a phishing page would.
  const forged = { ...cred, response: { ...cred.response } };
  const cd = Buffer.from(JSON.stringify({ type: 'webauthn.get', challenge: options.challenge, origin: 'http://evil.example', crossOrigin: false }));
  forged.response.clientDataJSON = b64u(cd);
  const r = await h.req('POST', '/api/admin/step-up', { cookie: adminCookie(), body: { cid, credential: forged } });
  assert.equal(r.status, 403, JSON.stringify(r.body));
  assert.equal(r.body.code, 'passkey');
});

test('an account with no passkey cannot step up', { skip: !GO && 'Go backend only' }, async t => {
  const h = await startServer(t, { key: null });
  const { cid, options } = (await h.req('POST', '/api/login/options', { body: {} })).body;
  const r = await h.req('POST', '/api/admin/step-up', {
    cookie: adminCookie(), body: { cid, credential: softPasskey().assertion(options.challenge) }
  });
  assert.equal(r.status, 403, JSON.stringify(r.body));
  assert.equal(r.body.code, 'passkey');
});
