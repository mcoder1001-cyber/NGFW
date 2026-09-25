// WEB-2: the request sequence of the Secrets page against the REAL P06 API on slot 1 (no mocks).
// Reads the admin password from $PWFILE; prints only refs, statuses and problem types (never a value).
import { readFileSync } from 'node:fs';

const BASE = process.env.BASE ?? 'http://127.0.0.1:3100';
const VALUE = 'VRX_TEST_PSK_WEB2';
const REF = 'psk/w1-web2';
const pw = readFileSync(process.env.PWFILE, 'utf8').trim();

async function req(method, path, body, token) {
  const r = await fetch(BASE + path, {
    method,
    headers: { ...(body === undefined ? {} : { 'content-type': 'application/json' }), ...(token ? { authorization: `Bearer ${token}` } : {}) },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  const text = await r.text();
  return { status: r.status, json: text ? JSON.parse(text) : null, text };
}
const ok = (cond, what) => {
  console.log(`${cond ? 'PASS' : 'FAIL'}  ${what}`);
  if (!cond) process.exitCode = 1;
};

const login = await req('POST', '/api/v1/auth/login', { username: 'admin', password: pw });
ok(login.status === 200, `login admin → ${login.status}`);
const T = login.json.accessToken;
// re-runnable: drop leftovers of an earlier run (candidate edit, the test secret)
await req('POST', '/api/v1/config/discard', undefined, T);
await req('DELETE', '/api/v1/secrets/psk/w1-web2', undefined, T);

let r = await req('POST', '/api/v1/secrets', { kind: 'psk', name: 'w1-web2', value: VALUE }, T);
ok(r.status === 200 && r.json.ref === REF && r.json.created === true, `create → ${r.status} ${JSON.stringify(r.json)}`);
ok(!r.text.includes(VALUE), 'create answer does not contain the value');

r = await req('POST', '/api/v1/secrets', { kind: 'psk', name: 'w1-web2', value: VALUE }, T);
ok(r.status === 409 && r.json.type.endsWith('/secret-exists'), `create again → ${r.status} ${r.json.type} (page: name-field error)`);
ok(!r.text.includes(VALUE), '409 problem does not echo the value');

r = await req('POST', '/api/v1/secrets?replace=true', { kind: 'psk', name: 'w1-web2', value: VALUE }, T);
ok(r.status === 200 && r.json.created === false && r.json.version === 2, `rotate (?replace=true) → ${r.status} ${JSON.stringify(r.json)}`);

r = await req('POST', '/api/v1/secrets', { kind: 'psk', name: 'w1-web2', value: 'x'.repeat(65537) }, T);
ok(r.status === 400 && (r.json.errors ?? []).some((e) => e.pointer === '/value'), `oversized value → ${r.status} pointers ${JSON.stringify((r.json.errors ?? []).map((e) => e.pointer))} (page: value-field error)`);

r = await req('GET', '/api/v1/secrets', undefined, T);
const mine = (r.json ?? []).find((s) => s.ref === REF);
ok(r.status === 200 && mine && !('value' in mine) && !r.text.includes(VALUE), `list → ${r.status} ${JSON.stringify(mine)} (no value)`);

r = await req('PATCH', '/api/v1/config/management', { aaa: { radius: { servers: [{ address: '10.1.0.1', secretRef: REF }] } } }, T);
ok(r.status === 200, `candidate references ${REF} → ${r.status}`);
r = await req('DELETE', '/api/v1/secrets/psk/w1-web2', undefined, T);
ok(r.status === 409 && r.json.type.endsWith('/secret-in-use'), `delete while referenced → ${r.status} ${r.json.type} ${JSON.stringify(r.json.errors)}`);
r = await req('POST', '/api/v1/config/discard', undefined, T);
ok(r.status === 200, `discard candidate → ${r.status}`);

const { createApiClient } = await import(process.env.API_CLIENT);
const client = createApiClient(BASE);
client.use({ onRequest: ({ request }) => { request.headers.set('authorization', `Bearer ${T}`); return request; } });
const del = await client.DELETE('/api/v1/secrets/{kind}/{name}', { params: { path: { kind: 'psk', name: 'w1-web2' } } });
ok(del.response.status === 204, `delete via the generated client (the page's api.DELETE) → ${del.response.status}`);
r = await req('DELETE', '/api/v1/secrets/psk/w1-web2', undefined, T);
ok(r.status === 404, `delete again → ${r.status}`);
