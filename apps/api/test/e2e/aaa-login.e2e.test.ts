import dgram from 'node:dgram';
import { createHash } from 'node:crypto';
import type { AddressInfo } from 'node:net';
import { sql } from 'drizzle-orm';
import { InvalidCredentialsError } from 'ldapts';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AaaService } from '../../src/features/aaa/aaa.service.js';
import type { LdapClientLike } from '../../src/features/aaa/ldap.js';
import { totpCode } from '../../src/features/aaa/totp.js';
import { startHarness, type Harness } from '../support/harness.js';

/**
 * F-aaa-login e2e (PostgreSQL + Valkey + fake agent): the login walks management.aaa.order — RADIUS (live local UDP
 * responder) and LDAP (the ldapts client seam, in-process fake directory) → roleMap → shadow user → session;
 * reject/unmapped/fallback; TOTP MFA (login-time enrolment, replay refused, single-use recovery codes); a stale
 * session stops at a raised MFA policy; no shared secret / bind password / TOTP seed in GET config or audit rows.
 */
const MP = { 'content-type': 'application/merge-patch+json' };
const RSECRET = 'VRX_TEST_PSK_FAAA_RADIUS';
const LSECRET = 'VRX_TEST_PSK_FAAA_LDAP';

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

/** RADIUS users: w1bob/bob-pw → Class netadmins; w1carol/carol-pw → Class nobody (unmapped). */
const RUSERS: Record<string, { pw: string; group: string }> = {
  w1bob: { pw: 'bob-pw', group: 'netadmins' },
  w1carol: { pw: 'carol-pw', group: 'nobody' },
};

function ldapFake(): LdapClientLike {
  return {
    startTLS: async () => undefined,
    bind: async (dn, pw) => {
      if (dn === 'cn=svc,dc=x' && pw === LSECRET) return;
      if (dn === 'uid=w1alice,dc=x' && pw === 'alice-pw') return;
      throw new InvalidCredentialsError();
    },
    search: async (_b, o) => ({
      searchEntries:
        o.filter.toLowerCase() === '(uid=w1alice)' // directories match names case-insensitively
          ? [{ dn: 'uid=w1alice,dc=x', memberOf: ['cn=admins,dc=x'] }]
          : [],
    }),
    unbind: async () => undefined,
  };
}

describe('F-aaa-login e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let radius: dgram.Socket;
  let rport: number;

  const login = (username: string, password: string) =>
    h.call(undefined, 'POST', '/api/v1/auth/login', { username, password });
  const setAaa = async (aaa: unknown) => {
    const p = await h.call(admin, 'PATCH', '/api/v1/config/management', { aaa }, MP);
    expect(p.status, p.raw).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=aaa');
    expect(c.status, c.raw).toBe(200);
  };

  beforeAll(async () => {
    h = await startHarness({ VRX_LOGIN_RATE_PER_MIN: '200' });
    admin = await h.login('admin', h.adminPassword);
    h.app.get(AaaService).ldapFactory = () => ldapFake();
    radius = dgram.createSocket('udp4');
    radius.on('message', (msg, rinfo) => {
      const id = msg[1] ?? 0;
      const auth = msg.subarray(4, 20);
      let user = '';
      let pass = '';
      let i = 20;
      while (i + 2 <= msg.length) {
        const type = msg[i] ?? 0;
        const len = msg[i + 1] ?? 0;
        if (len < 2) break;
        const val = msg.subarray(i + 2, i + len);
        if (type === 1) user = val.toString('utf8');
        else if (type === 2) pass = decodePw(val, RSECRET, auth);
        i += len;
      }
      const u = RUSERS[user];
      const ok = u !== undefined && u.pw === pass;
      const g = Buffer.from(u?.group ?? '');
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
    for (const [kind, name, value] of [
      ['psk', 'radius', RSECRET],
      ['password', 'ldap', LSECRET],
    ] as const) {
      const s = await h.call(admin, 'POST', '/api/v1/secrets', { kind, name, value });
      expect(s.status, s.raw).toBe(200);
    }
    await setAaa({
      order: ['local', 'radius', 'ldap'],
      radius: {
        servers: [
          { address: '127.0.0.1', authPort: rport, secretRef: 'psk/radius', timeoutSec: 1 },
        ],
      },
      ldap: {
        servers: [
          {
            url: 'ldaps://127.0.0.1:1',
            bindDn: 'cn=svc,dc=x',
            bindPasswordRef: 'password/ldap',
            baseDn: 'dc=x',
          },
        ],
      },
      roleMap: [
        { group: 'netadmins', role: 'operator' },
        { group: 'cn=admins,dc=x', role: 'admin' },
      ],
    });
  });
  afterAll(async () => {
    radius?.close();
    await h?.close();
  });

  it('RADIUS login → JWT with the mapped role; the shadow user has no local password', async () => {
    const r = await login('w1bob', 'bob-pw');
    expect(r.status, r.raw).toBe(200);
    expect(r.body.user).toMatchObject({ username: 'w1bob', role: 'operator' });
    const me = await h.call(r.body.accessToken, 'GET', '/api/v1/auth/me');
    expect(me.body).toMatchObject({ role: 'operator', via: 'jwt' });
    const rows = await h.db.execute(
      sql`select source, password_hash from app_user where username = 'w1bob'`,
    );
    expect(rows.rows[0]).toEqual({ source: 'external', password_hash: null });
    // D-100 for external principals: refused, never a lockout count
    const key = await h.call(r.body.accessToken, 'POST', '/api/v1/auth/api-keys', {
      name: 'k',
      current: 'bob-pw',
    });
    expect(key.status, key.raw).toBe(403);
    expect(key.body.type).toMatch(/external-principal$/);
  });

  it('wrong password → 401 (reject is final: LDAP is not tried after RADIUS answered)', async () => {
    const r = await login('w1bob', 'wrong');
    expect(r.status).toBe(401);
  });

  it('unmapped external user → 403 problem+json no-role-mapping', async () => {
    const r = await login('w1carol', 'carol-pw');
    expect(r.status, r.raw).toBe(403);
    expect(r.headers['content-type']).toMatch(/application\/problem\+json/);
    expect(r.body.type).toMatch(/no-role-mapping$/);
  });

  it('LDAP login (RADIUS rejects unknown users, so LDAP-only users come first in a separate order)', async () => {
    await setAaa({ order: ['ldap', 'local'] });
    const r = await login('w1alice', 'alice-pw');
    expect(r.status, r.raw).toBe(200);
    expect(r.body.user).toMatchObject({ username: 'w1alice', role: 'admin' });
    expect((await login('w1alice', 'nope')).status).toBe(401);
    // review 6: a name LDAP does not know falls through to the next method (the local admin behind ldap-first)
    const local = await login('admin', h.adminPassword);
    expect(local.status, local.raw).toBe(200);
    // review 6: case-folded — W1ALICE is the same shadow user
    const upper = await login('W1ALICE', 'alice-pw');
    expect(upper.status, upper.raw).toBe(200);
    expect(upper.body.user.id).toBe(r.body.user.id);
  });

  it('review 2: a configured user may not take over an external shadow account (commit 400 with pointer)', async () => {
    const put = await h.call(admin, 'PUT', '/api/v1/config/management/users', [
      { username: 'admin', role: 'admin' },
      {
        username: 'W1Bob',
        role: 'admin',
        passwordHash: '$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA',
      },
    ]);
    const c = put.status === 200 ? await h.call(admin, 'POST', '/api/v1/config/commit') : put;
    expect(c.status, c.raw).toBe(400);
    expect(JSON.stringify(c.body)).toMatch(/management\/users\/1\/username/);
    await h.call(admin, 'DELETE', '/api/v1/config/candidate');
    const rows = await h.db.execute(
      sql`select source, role, password_hash from app_user where lower(username) = 'w1bob'`,
    );
    expect(rows.rows).toEqual([{ source: 'external', role: 'operator', password_hash: null }]);
  });

  it('review 9: no local password for an external identity (admin reset or self) → 403 external-principal', async () => {
    await setAaa({ order: ['local', 'radius', 'ldap'] });
    const r = await h.call(admin, 'POST', '/api/v1/users/w1bob/password', {
      password: 'Long-enough-password-1',
    });
    expect(r.status, r.raw).toBe(403);
    expect(r.body.type).toMatch(/external-principal$/);
    const bob = await login('w1bob', 'bob-pw');
    expect(bob.status, bob.raw).toBe(200);
    const self = await h.call(bob.body.accessToken, 'POST', '/api/v1/auth/password', {
      current: 'bob-pw',
      password: 'Long-enough-password-1',
    });
    expect(self.status, self.raw).toBe(403);
    const lk = await h.db.execute(sql`select failed_logins from app_user where username = 'w1bob'`);
    expect(lk.rows[0]).toEqual({ failed_logins: 0 });
  });

  it('server down + fallbackLocal → the local admin logs in; fallbackLocal false → refused', async () => {
    await setAaa({
      order: ['radius'],
      radius: {
        servers: [{ address: '127.0.0.1', authPort: 9, secretRef: 'psk/radius', timeoutSec: 1 }],
      },
      fallbackLocal: true,
    });
    const ok = await login('admin', h.adminPassword);
    expect(ok.status, ok.raw).toBe(200);
    admin = ok.body.accessToken;
    await setAaa({ fallbackLocal: false });
    expect((await login('admin', h.adminPassword)).status).toBe(401);
    // review 6: ldap-only order, the admin is unknown to LDAP (not an answer) → break-glass fallback
    await setAaa({ order: ['ldap'], fallbackLocal: true });
    const bg = await login('admin', h.adminPassword);
    expect(bg.status, bg.raw).toBe(200);
    admin = bg.body.accessToken;
    // restore: local first
    await setAaa({ order: ['local'], fallbackLocal: true });
  });

  it('`aaa.order: [radius]` with no servers → 400 with a pointer', async () => {
    const p = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/management',
      { aaa: { order: ['radius'], radius: { servers: [] } } },
      MP,
    );
    const c = p.status === 200 ? await h.call(admin, 'POST', '/api/v1/config/commit') : p;
    expect(c.status, c.raw).toBe(400);
    expect(JSON.stringify(c.body)).toMatch(/aaa\/radius\/servers/);
    await h.call(admin, 'DELETE', '/api/v1/config/candidate');
  });

  it('TOTP: stale session refused, login-time enrolment, replay refused, recovery code single use', async () => {
    const stale = admin;
    // D-159 (review 5): enrolment is admin-issued — a one-time token, shown once
    const tok = await h.call(admin, 'POST', '/api/v1/auth/mfa/users/admin/enrolment-token');
    expect(tok.status, tok.raw).toBe(200);
    const token = tok.body.token as string;
    await setAaa({ mfa: { required: 'admins' } });
    // the policy cache is refreshed by the next login; the old session (no second factor) stops working
    const first = await login('admin', h.adminPassword);
    expect(first.status, first.raw).toBe(200);
    expect(first.body).toMatchObject({ mfaRequired: true, enrolled: false });
    expect(first.headers['set-cookie']).toBeUndefined();
    expect((await h.call(stale, 'GET', '/api/v1/auth/me')).status).toBe(401);

    // review 4: the transport rule of login applies to every MFA step
    for (const url of ['/api/v1/auth/mfa/enroll', '/api/v1/auth/mfa/verify']) {
      const res = await h.app.inject({
        method: 'POST',
        url,
        remoteAddress: '192.0.2.10',
        payload: {
          challenge: first.body.challenge,
          ...(url.endsWith('enroll') ? { token } : { code: '123456' }),
        },
      });
      expect(res.statusCode, url).toBe(403);
      expect(res.body).toMatch(/tls-required/);
    }
    // no token → refused; a wrong token burns the challenge's single enrolment attempt
    const none = await h.call(undefined, 'POST', '/api/v1/auth/mfa/enroll', {
      challenge: first.body.challenge,
    });
    expect(none.status).toBe(400);
    const wrong = await h.call(undefined, 'POST', '/api/v1/auth/mfa/enroll', {
      challenge: first.body.challenge,
      token: 'x'.repeat(32),
    });
    expect(wrong.status).toBe(401);
    const burnt = await h.call(undefined, 'POST', '/api/v1/auth/mfa/enroll', {
      challenge: first.body.challenge,
      token,
    });
    expect(burnt.status).toBe(401);
    const retry = await login('admin', h.adminPassword);
    first.body.challenge = retry.body.challenge;
    const en = await h.call(undefined, 'POST', '/api/v1/auth/mfa/enroll', {
      challenge: first.body.challenge,
      token,
    });
    expect(en.status, en.raw).toBe(200);
    const seed = en.body.secret as string;
    expect(en.body.otpauthUri).toMatch(/^otpauth:\/\/totp\//);
    const code = totpCode(seed);
    const bad = await h.call(undefined, 'POST', '/api/v1/auth/mfa/verify', {
      challenge: first.body.challenge,
      code: code === '000000' ? '111111' : '000000',
    });
    expect(bad.status).toBe(401);
    const v = await h.call(undefined, 'POST', '/api/v1/auth/mfa/verify', {
      challenge: first.body.challenge,
      code,
    });
    expect(v.status, v.raw).toBe(200);
    expect(v.body.recoveryCodes).toHaveLength(10);
    admin = v.body.accessToken;
    expect((await h.call(admin, 'GET', '/api/v1/auth/me')).status).toBe(200);
    // the challenge is single use
    const again = await h.call(undefined, 'POST', '/api/v1/auth/mfa/verify', {
      challenge: first.body.challenge,
      code,
    });
    expect(again.status).toBe(401);

    // replay: the same code on a new login is refused
    const second = await login('admin', h.adminPassword);
    expect(second.body).toMatchObject({ mfaRequired: true, enrolled: true });
    const replay = await h.call(undefined, 'POST', '/api/v1/auth/mfa/verify', {
      challenge: second.body.challenge,
      code,
    });
    expect(replay.status).toBe(401);
    // a recovery code works once
    const rc = v.body.recoveryCodes[0] as string;
    const rec = await h.call(undefined, 'POST', '/api/v1/auth/mfa/verify', {
      challenge: second.body.challenge,
      recoveryCode: rc,
    });
    expect(rec.status, rec.raw).toBe(200);
    admin = rec.body.accessToken;
    const third = await login('admin', h.adminPassword);
    const reuse = await h.call(undefined, 'POST', '/api/v1/auth/mfa/verify', {
      challenge: third.body.challenge,
      recoveryCode: rc,
    });
    expect(reuse.status).toBe(401);
    const st = await h.call(admin, 'GET', '/api/v1/auth/mfa');
    expect(st.body).toEqual({ enrolled: true, recoveryCodesLeft: 9, required: true });

    // no secret, bind password or seed in GET config or in the audit log
    const cfg = await h.call(admin, 'GET', '/api/v1/config/management');
    const audit = await h.db.execute(sql`select * from audit_log`);
    const mfaRow = await h.db.execute(sql`select seed from aaa_mfa`);
    const blob = JSON.stringify([cfg.body, audit.rows]);
    for (const s of [RSECRET, LSECRET, seed, 'bob-pw', 'alice-pw', h.adminPassword, rc]) {
      expect(blob).not.toContain(s);
    }
    expect(JSON.stringify(mfaRow.rows)).not.toContain(seed);
    // the external answers are audited (method, server, result)
    expect(blob).toMatch(
      /"method":"radius".*"result":"accept"|"result":"accept".*"method":"radius"/,
    );
  });

  it('admin MFA reset: the factor goes, the policy asks for a new enrolment', async () => {
    const r = await h.call(admin, 'DELETE', '/api/v1/auth/mfa/users/admin');
    expect(r.status, r.raw).toBe(204);
    const l = await login('admin', h.adminPassword);
    expect(l.body).toMatchObject({ mfaRequired: true, enrolled: false });
    // without a token issued by an admin there is no way to enrol (no trust-on-first-use)
    const e = await h.call(undefined, 'POST', '/api/v1/auth/mfa/enroll', {
      challenge: l.body.challenge,
      token: 'A'.repeat(32),
    });
    expect(e.status).toBe(401);
  });
});
