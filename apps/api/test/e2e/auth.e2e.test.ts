import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

/** Passwords of this run only — generated, never literal (gitleaks, 00-CONTEXT secrets rule). */
const PW = {
  op: runSecret(),
  ro: runSecret(),
  ro2: runSecret(),
  victim: runSecret(),
  racer: runSecret(),
};

function refreshCookie(headers: Record<string, unknown>): { value: string; attrs: string } {
  const raw = [headers['set-cookie']]
    .flat()
    .find((c) => String(c).startsWith('vrx_refresh=')) as string;
  expect(raw).toBeDefined();
  const value = raw.split(';')[0]!.slice('vrx_refresh='.length);
  return { value, attrs: raw.toLowerCase() };
}

describe('auth e2e (argon2id, JWT + rotating refresh, API keys, lockout, rate limit)', () => {
  let h: Harness;
  let admin: string;

  beforeAll(async () => {
    h = await startHarness({ VRX_LOGIN_RATE_PER_MIN: '100', VRX_LOGIN_MAX_FAILURES: '10' });
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [
      { username: 'op2', role: 'operator', password: PW.op },
      { username: 'ro2', role: 'readonly', password: PW.ro },
      { username: 'victim', role: 'operator', password: PW.victim },
      { username: 'racer', role: 'operator', password: PW.racer },
    ]);
  });
  afterAll(async () => h?.close());

  it('bootstrap admin was seeded with an argon2id hash (D-048)', async () => {
    const rows = await h.db.execute(
      sql`select username, role, source, password_hash like '$argon2id$%' as argon from app_user order by id`,
    );
    expect(rows.rows[0]).toEqual({
      username: 'admin',
      role: 'admin',
      source: 'bootstrap',
      argon: true,
    });
  });

  it('login returns a 15-minute bearer token and an httpOnly SameSite=Strict refresh cookie', async () => {
    const r = await h.call(undefined, 'POST', '/api/v1/auth/login', {
      username: 'op2',
      password: PW.op,
    });
    expect(r.status).toBe(200);
    expect(r.body).toMatchObject({
      tokenType: 'Bearer',
      expiresIn: 900,
      user: { username: 'op2', role: 'operator' },
    });
    expect(r.body).not.toHaveProperty('refreshToken');
    const c = refreshCookie(r.headers);
    expect(c.attrs).toContain('httponly');
    expect(c.attrs).toContain('samesite=strict');
    expect(c.attrs).toContain('secure'); // review L3: Secure by default
    expect(c.attrs).toContain('path=/api/v1/auth');
    const me = await h.call(r.body.accessToken, 'GET', '/api/v1/auth/me');
    expect(me.body).toMatchObject({ username: 'op2', role: 'operator', via: 'jwt' });
  });

  it('refresh rotates; replaying a used refresh token revokes the whole family', async () => {
    const l = await h.call(undefined, 'POST', '/api/v1/auth/login', {
      username: 'ro2',
      password: PW.ro,
    });
    const first = refreshCookie(l.headers).value;
    const r1 = await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, {
      cookie: `vrx_refresh=${first}`,
    });
    expect(r1.status).toBe(200);
    const second = refreshCookie(r1.headers).value;
    expect(second).not.toBe(first);
    // replay of the rotated-away token: 401, and the family (incl. the new token) is dead
    const replay = await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, {
      cookie: `vrx_refresh=${first}`,
    });
    expect(replay.status).toBe(401);
    const r2 = await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, {
      cookie: `vrx_refresh=${second}`,
    });
    expect(r2.status).toBe(401);
    expect((await h.call(undefined, 'POST', '/api/v1/auth/refresh')).status).toBe(401);
  });

  it('logout kills the refresh token', async () => {
    const l = await h.call(undefined, 'POST', '/api/v1/auth/login', {
      username: 'ro2',
      password: PW.ro,
    });
    const tok = refreshCookie(l.headers).value;
    expect(
      (
        await h.call(undefined, 'POST', '/api/v1/auth/logout', undefined, {
          cookie: `vrx_refresh=${tok}`,
        })
      ).status,
    ).toBe(204);
    expect(
      (
        await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, {
          cookie: `vrx_refresh=${tok}`,
        })
      ).status,
    ).toBe(401);
  });

  it('API keys: shown once, capped by role, revocable', async () => {
    const op = await h.login('op2', PW.op);
    const k = await h.call(op, 'POST', '/api/v1/auth/api-keys', {
      name: 'automation',
      role: 'readonly',
    });
    expect(k.status).toBe(201);
    expect(k.body.key).toMatch(/^vrxk_/);
    expect(k.body.role).toBe('readonly');
    const key = k.body.key as string;
    const asKey = (method: 'GET' | 'PATCH', url: string, body?: unknown) =>
      h.call(undefined, method, url, body, { authorization: `ApiKey ${key}` });
    expect((await asKey('GET', '/api/v1/config')).status).toBe(200);
    expect((await asKey('GET', '/api/v1/auth/me')).body).toMatchObject({
      effectiveRole: 'readonly',
      via: 'apikey',
    });
    expect((await asKey('PATCH', '/api/v1/config/system', { hostname: 'k' })).status).toBe(403);
    const list = await h.call(op, 'GET', '/api/v1/auth/api-keys');
    expect(list.raw).not.toContain(key);
    expect(list.body).toEqual([expect.objectContaining({ name: 'automation', role: 'readonly' })]);
    const rows = await h.db.execute(sql`select hash from api_key`);
    expect(JSON.stringify(rows.rows)).not.toContain(key);
    expect((await h.call(op, 'DELETE', `/api/v1/auth/api-keys/${k.body.id}`)).status).toBe(204);
    expect((await asKey('GET', '/api/v1/config')).status).toBe(401);
    // an operator cannot mint an admin key
    const up = await h.call(op, 'POST', '/api/v1/auth/api-keys', {
      name: 'escalate',
      role: 'admin',
    });
    expect(up.body.role).toBe('operator');
  });

  it('password change (self-service, also for readonly)', async () => {
    const ro = await h.login('ro2', PW.ro);
    expect(
      (
        await h.call(ro, 'POST', '/api/v1/auth/password', {
          current: 'wrong',
          password: PW.ro2,
        })
      ).status,
    ).toBe(403);
    expect(
      (
        await h.call(ro, 'POST', '/api/v1/auth/password', {
          current: PW.ro,
          password: PW.ro2,
        })
      ).status,
    ).toBe(204);
    await h.login('ro2', PW.ro2);
  });

  it('lockout after 10 failures: the right password is refused until the lock expires', async () => {
    for (let i = 0; i < 10; i++) {
      const r = await h.call(undefined, 'POST', '/api/v1/auth/login', {
        username: 'victim',
        password: `nope-${i}`,
      });
      expect(r.status).toBe(401);
      expect(r.body.detail).toBe('invalid credentials');
    }
    const r = await h.call(undefined, 'POST', '/api/v1/auth/login', {
      username: 'victim',
      password: PW.victim,
    });
    expect(r.status).toBe(401);
    const audit = await h.db.execute(
      sql`select after->>'reason' as reason from audit_log where action = 'auth.login' and username = 'victim' order by id`,
    );
    const reasons = audit.rows.map((x) => x['reason']);
    expect(reasons.slice(-2)).toEqual(['bad-password-locked', 'locked']);
    const u = await h.db.execute(
      sql`select locked_until > now() as locked from app_user where username = 'victim'`,
    );
    expect(u.rows[0]).toEqual({ locked: true });
    // an admin can unlock by expiring the lock (no API for that in P06): the right password then works
    await h.db.execute(
      sql`update app_user set locked_until = now() - interval '1 second' where username = 'victim'`,
    );
    await h.login('victim', PW.victim);
  });

  it('review H1: 60 PARALLEL wrong passwords still lock the account; the right password is then refused', async () => {
    const tries = await Promise.all(
      Array.from({ length: 60 }, (_, i) =>
        h.call(undefined, 'POST', '/api/v1/auth/login', {
          username: 'racer',
          password: `wrong-${i}`,
        }),
      ),
    );
    expect(tries.every((t) => t.status === 401)).toBe(true);
    const u = await h.db.execute(
      sql`select locked_until > now() as locked from app_user where username = 'racer'`,
    );
    expect(u.rows[0]).toEqual({ locked: true });
    const ok = await h.call(undefined, 'POST', '/api/v1/auth/login', {
      username: 'racer',
      password: PW.racer,
    });
    expect(ok.status).toBe(401);
    const last = await h.db.execute(
      sql`select after->>'reason' as reason from audit_log where action = 'auth.login' and username = 'racer' order by id desc limit 1`,
    );
    expect(last.rows[0]).toEqual({ reason: 'locked' });
  });

  it('review L3: a password change ends the other sessions (refresh families revoked)', async () => {
    const l1 = await h.call(undefined, 'POST', '/api/v1/auth/login', {
      username: 'op2',
      password: PW.op,
    });
    const other = refreshCookie(l1.headers).value;
    const token = l1.body.accessToken as string;
    const next = runSecret();
    expect(
      (await h.call(token, 'POST', '/api/v1/auth/password', { current: PW.op, password: next }))
        .status,
    ).toBe(204);
    PW.op = next;
    expect(
      (
        await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, {
          cookie: `vrx_refresh=${other}`,
        })
      ).status,
    ).toBe(401);
    await h.login('op2', PW.op);
  });

  it('login rate limit per source IP → 429', async () => {
    let last = 0;
    for (let i = 0; i < 120 && last !== 429; i++) {
      last = (
        await h.call(undefined, 'POST', '/api/v1/auth/login', {
          username: 'nobody',
          password: 'wrong',
        })
      ).status;
    }
    expect(last).toBe(429);
    const audit = await h.db.execute(
      sql`select count(*)::int as n from audit_log where after->>'reason' = 'rate-limited'`,
    );
    expect(audit.rows[0]!['n']).toBeGreaterThan(0);
  });
});
