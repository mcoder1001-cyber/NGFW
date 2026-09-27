import dgram from 'node:dgram';
import { createHash } from 'node:crypto';
import type { AddressInfo } from 'node:net';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { startHarness, type Harness } from '../support/harness.js';

/**
 * F-aaa (increment 1): the admin backend-test route against a live RADIUS server; role mapping; unknown backend 501;
 * operator forbidden.
 */
const MP = { 'content-type': 'application/merge-patch+json' };
const RSECRET = 'e2e-radius';

function decodePw(enc: Buffer, secret: string, auth: Buffer): string {
  const out = Buffer.alloc(enc.length);
  let prev = auth;
  for (let i = 0; i < enc.length; i += 16) {
    const b = createHash('md5').update(secret).update(prev).digest();
    for (let j = 0; j < 16; j++) out[i + j] = (enc[i + j] ?? 0) ^ (b[j] ?? 0);
    prev = enc.subarray(i, i + 16);
  }
  return out.toString('utf8').replace(/\0+$/, '');
}

describe('F-aaa e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let op: string;
  let radius: dgram.Socket;
  let rport: number;

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [
      { username: 'op1', role: 'operator', password: 'Op1-pw-1234567890' }, // gitleaks:allow — test fixture (dummy password / RFC 6238 test vector), never a real secret
    ]);
    op = await h.login('op1', 'Op1-pw-1234567890');
    // a local RADIUS server: accepts bob/bob-pw, replies Class=netadmins
    radius = dgram.createSocket('udp4');
    radius.on('message', (msg, rinfo) => {
      const id = msg[1] ?? 0;
      const auth = msg.subarray(4, 20);
      let user = '',
        pass = '';
      let i = 20;
      while (i + 2 <= msg.length) {
        const type = msg[i] ?? 0,
          len = msg[i + 1] ?? 0;
        if (len < 2) break;
        const val = msg.subarray(i + 2, i + len);
        if (type === 1) user = val.toString('utf8');
        else if (type === 2) pass = decodePw(val, RSECRET, auth);
        i += len;
      }
      const ok = user === 'bob' && pass === 'bob-pw';
      const g = Buffer.from('netadmins');
      const attrs = ok ? Buffer.concat([Buffer.from([25, 2 + g.length]), g]) : Buffer.alloc(0);
      const header = Buffer.alloc(20);
      header[0] = ok ? 2 : 3;
      header[1] = id;
      header.writeUInt16BE(20 + attrs.length, 2);
      createHash('md5')
        .update(Buffer.concat([header.subarray(0, 4), auth, attrs, Buffer.from(RSECRET)]))
        .digest()
        .copy(header, 4);
      radius.send(Buffer.concat([header, attrs]), rinfo.port, rinfo.address);
    });
    await new Promise<void>((r) => radius.bind(0, '127.0.0.1', r));
    rport = (radius.address() as AddressInfo).port;

    // secret + config
    const s = await h.call(admin, 'POST', '/api/v1/secrets', {
      kind: 'psk',
      name: 'radius',
      value: RSECRET,
    });
    expect(s.status, s.raw).toBe(200);
    const cfg = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/management',
      {
        aaa: {
          order: ['local', 'radius'],
          radius: {
            servers: [
              { address: '127.0.0.1', authPort: rport, secretRef: 'psk/radius', timeoutSec: 2 },
            ],
          },
          roleMap: [{ group: 'netadmins', role: 'operator' }],
        },
      },
      MP,
    );
    expect(cfg.status, cfg.raw).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=aaa')).status).toBe(200);
  });
  afterAll(async () => {
    radius?.close();
    await h?.close();
  });

  it('authenticates a good credential and maps the role', async () => {
    const r = await h.call(admin, 'POST', '/api/v1/actions/aaa/test', {
      method: 'radius',
      username: 'bob',
      password: 'bob-pw',
    });
    expect(r.status, r.raw).toBe(200);
    expect(r.body).toMatchObject({ reachable: true, authenticated: true, role: 'operator' });
    expect(r.body.groups).toContain('netadmins');
  });

  it('reports a rejected credential', async () => {
    const r = await h.call(admin, 'POST', '/api/v1/actions/aaa/test', {
      method: 'radius',
      username: 'bob',
      password: 'wrong',
    });
    expect(r.status, r.raw).toBe(200);
    expect(r.body).toMatchObject({ reachable: true, authenticated: false });
  });

  it('501 for a not-yet-implemented backend, 403 for an operator', async () => {
    const ni = await h.call(admin, 'POST', '/api/v1/actions/aaa/test', {
      method: 'ldap',
      username: 'x',
      password: 'y',
    });
    expect(ni.status).toBe(501);
    const forbidden = await h.call(op, 'POST', '/api/v1/actions/aaa/test', {
      method: 'radius',
      username: 'bob',
      password: 'bob-pw',
    });
    expect(forbidden.status).toBe(403);
  });
});
