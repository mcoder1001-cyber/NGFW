import { eq } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { appUser } from '../../src/db/schema.js';
import { startHarness, type Harness } from '../support/harness.js';
import { deadPort, startRadius, type RadiusTestServer } from '../support/radius-server.js';

/**
 * F-aaa-login (increment 2): `POST /auth/login` walks `management.aaa.order`.
 *
 * Covers: an external identity getting a session and a shadow account with the roleMap role; no roleMap match
 * refused; an external reject refused; the walk stopping at the first method that answers (the directory is never
 * consulted for a local user, right or wrong password); `fallbackLocal` on and off while the directory is down;
 * a shadow account surviving a config commit; a role change on re-login; and a directory being unable to log in as
 * a local account.
 */
const MP = { 'content-type': 'application/merge-patch+json' };
const RSECRET = 'e2e-login-radius';
const LOCAL_PW = 'Local-user-pw-1234567890';

describe('F-aaa-login e2e (login order, shadow accounts, fallback)', () => {
  let h: Harness;
  let admin: string;
  let radius: RadiusTestServer;
  let dead: number;

  /** Set management.aaa and commit; the AaaService policy cache reloads on the commit event. */
  const setAaa = async (aaa: Record<string, unknown>) => {
    const p = await h.call(admin, 'PATCH', '/api/v1/config/management', { aaa }, MP);
    expect(p.status, p.raw).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=aaa-login');
    expect(c.status, c.raw).toBe(200);
  };

  const login = (username: string, password: string) =>
    h.call(undefined, 'POST', '/api/v1/auth/login', { username, password });

  const userRow = async (username: string) =>
    (await h.db.select().from(appUser).where(eq(appUser.username, username)))[0];

  beforeAll(async () => {
    // this file logs in many times over (one per method-order case), so the per-client login budget is raised
    // as auth.e2e does — the limit itself is exercised there, not here.
    h = await startHarness({ VRX_LOGIN_RATE_PER_MIN: '200' });
    admin = await h.login('admin', h.adminPassword);
    radius = await startRadius(RSECRET, {
      bob: { password: 'bob-pw', groups: ['netadmins'] },
      carol: { password: 'carol-pw', groups: ['nobody'] },
      // same name as a local config user, to prove the directory cannot take that account over
      eve: { password: 'eve-pw', groups: ['netadmins'] },
      // a local user the directory also knows: local answers first, so this is never used
      loki: { password: 'radius-side-pw', groups: ['netadmins'] },
    });
    dead = await deadPort();
    const s = await h.call(admin, 'POST', '/api/v1/secrets', {
      kind: 'psk',
      name: 'radius',
      value: RSECRET,
    });
    expect(s.status, s.raw).toBe(200);
    // a local user with a password, and a config user with NO passwordHash (so the local step cannot decide for it)
    const put = await h.call(admin, 'PUT', '/api/v1/config/management/users', [
      { username: 'admin', role: 'admin' },
      { username: 'loki', role: 'readonly', passwordHash: await hash(LOCAL_PW) },
      { username: 'eve', role: 'readonly' },
    ]);
    expect(put.status, put.raw).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=users')).status).toBe(200);
  });
  afterAll(async () => {
    radius?.close();
    await h?.close();
  });

  describe('order [local, radius]', () => {
    beforeAll(async () => {
      await setAaa({
        order: ['local', 'radius'],
        radius: {
          servers: [
            { address: '127.0.0.1', authPort: radius.port, secretRef: 'psk/radius', timeoutSec: 2 },
          ],
        },
        roleMap: [{ group: 'netadmins', role: 'operator' }],
      });
    });

    it('an external identity gets a session, the roleMap role and a shadow account', async () => {
      const before = radius.seen.length;
      const r = await login('bob', 'bob-pw');
      expect(r.status, r.raw).toBe(200);
      expect(r.body.user).toMatchObject({ username: 'bob', role: 'operator' });
      expect(radius.seen.slice(before)).toEqual(['bob']);
      const row = await userRow('bob');
      expect(row).toMatchObject({ role: 'operator', source: 'aaa:radius', passwordHash: null });
      // the session works like any other
      const me = await h.call(r.body.accessToken, 'GET', '/api/v1/auth/me');
      expect(me.status, me.raw).toBe(200);
      expect(me.body).toMatchObject({ username: 'bob', role: 'operator' });
    });

    it('an authenticated identity with no roleMap match is refused', async () => {
      const r = await login('carol', 'carol-pw');
      expect(r.status, r.raw).toBe(401);
      expect(await userRow('carol')).toBeUndefined(); // no account is created
    });

    it('an external reject is refused', async () => {
      const r = await login('bob', 'wrong-pw');
      expect(r.status, r.raw).toBe(401);
    });

    it('a local user is decided locally — the directory is never consulted', async () => {
      const before = radius.seen.length;
      const ok = await login('loki', LOCAL_PW);
      expect(ok.status, ok.raw).toBe(200);
      expect(ok.body.user).toMatchObject({ username: 'loki', role: 'readonly' });
      // and a WRONG local password stops the walk instead of being replayed against the directory,
      // which would otherwise have let 'loki' in as an operator with its radius-side password
      const bad = await login('loki', 'not-the-local-password');
      expect(bad.status, bad.raw).toBe(401);
      expect(radius.seen.slice(before)).toEqual([]);
      expect(await userRow('loki')).toMatchObject({ role: 'readonly', source: 'config' });
    });

    it('the directory cannot log in as a local account of the same name', async () => {
      const r = await login('eve', 'eve-pw');
      expect(r.status, r.raw).toBe(401);
      // eve stays the local config account: not taken over, not re-sourced, not promoted
      expect(await userRow('eve')).toMatchObject({ role: 'readonly', source: 'config' });
    });

    it('a shadow account survives a configuration commit', async () => {
      expect(await userRow('bob')).toBeDefined();
      const put = await h.call(admin, 'PUT', '/api/v1/config/management/users', [
        { username: 'admin', role: 'admin' },
        { username: 'loki', role: 'readonly', passwordHash: await hash(LOCAL_PW) },
        { username: 'eve', role: 'readonly' },
      ]);
      expect(put.status, put.raw).toBe(200);
      expect(
        (await h.call(admin, 'POST', '/api/v1/config/commit?comment=users-again')).status,
      ).toBe(200);
      expect(await userRow('bob')).toMatchObject({ source: 'aaa:radius', role: 'operator' });
    });

    it('a shadow account cannot be used through the local step', async () => {
      // 'bob' has no password hash, so no local password can ever authenticate it; only the directory can
      const r = await login('bob', LOCAL_PW);
      expect(r.status, r.raw).toBe(401);
    });

    it('a roleMap change lands on the next login', async () => {
      await setAaa({
        order: ['local', 'radius'],
        roleMap: [{ group: 'netadmins', role: 'admin' }],
      });
      const r = await login('bob', 'bob-pw');
      expect(r.status, r.raw).toBe(200);
      expect(r.body.user).toMatchObject({ username: 'bob', role: 'admin' });
      expect(await userRow('bob')).toMatchObject({ role: 'admin' });
    });
  });

  describe('order [radius] with the directory down', () => {
    it('fallbackLocal lets a local user in', async () => {
      await setAaa({
        order: ['radius'],
        fallbackLocal: true,
        radius: {
          servers: [
            { address: '127.0.0.1', authPort: dead, secretRef: 'psk/radius', timeoutSec: 1 },
          ],
        },
        roleMap: [{ group: 'netadmins', role: 'operator' }],
      });
      const r = await login('loki', LOCAL_PW);
      expect(r.status, r.raw).toBe(200);
      expect(r.body.user).toMatchObject({ username: 'loki', role: 'readonly' });
    });

    it('without fallbackLocal the local user is refused', async () => {
      await setAaa({ order: ['radius'], fallbackLocal: false });
      const r = await login('loki', LOCAL_PW);
      expect(r.status, r.raw).toBe(401);
    });

    it('the fallback does not fire when the directory answered', async () => {
      await setAaa({
        order: ['radius'],
        fallbackLocal: true,
        radius: {
          servers: [
            { address: '127.0.0.1', authPort: radius.port, secretRef: 'psk/radius', timeoutSec: 2 },
          ],
        },
      });
      // radius answers (reject) for 'loki', so the local fallback must NOT run and let it in locally
      const r = await login('loki', LOCAL_PW);
      expect(r.status, r.raw).toBe(401);
    });
  });
});

/** argon2id hash for a config user's passwordHash field. */
async function hash(password: string): Promise<string> {
  const { hashPassword } = await import('../../src/auth/password.js');
  return hashPassword(password);
}
