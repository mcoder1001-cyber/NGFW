import { sql } from 'drizzle-orm';
import { randomBytes } from 'node:crypto';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { TokensService } from '../../src/auth/tokens.service.js';
import { Bus } from '../../src/infra/bus.js';
import { VALKEY, type Valkey } from '../../src/infra/valkey.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';
import { cookieOf, eventually } from '../support/proxy.js';

/**
 * TD-10b on the host PostgreSQL + Valkey — review 2.3c (logout ends the session's access tokens, per sid) and 2.3e
 * (logout and refresh failures are audited). Demotion/deletion (PENDING-session-revocation option 1) is in its own
 * suite and commit: td10b-session-revocation.e2e.test.ts.
 */
const PW: Record<string, string> = {
  op: runSecret(),
  off: runSecret(),
};

describe('TD-10b sessions: per-sid logout, audited refresh/logout', () => {
  let h: Harness;
  let admin: string;

  const login = async (u: string) => {
    const r = await h.call(undefined, 'POST', '/api/v1/auth/login', {
      username: u,
      password: PW[u],
    });
    expect(r.status).toBe(200);
    return { token: r.body.accessToken as string, cookie: cookieOf(r.headers, 'vrx_refresh')! };
  };
  const me = async (tok: string) => (await h.call(tok, 'GET', '/api/v1/auth/me')).status;
  const refresh = (cookie: string) =>
    h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, { cookie });
  const logout = (headers: Record<string, string>) =>
    h.call(undefined, 'POST', '/api/v1/auth/logout', undefined, headers);
  const audit = async (action: string, extra = sql``) =>
    (
      await h.db.execute(
        sql`select user_id, username, source_ip, resource, after, result, status from audit_log where action = ${action} ${extra} order by id`,
      )
    ).rows;
  const idOf = async (u: string) =>
    Number((await h.db.execute(sql`select id from app_user where username = ${u}`)).rows[0]!['id']);
  const patchUser = async (u: string, patch: Record<string, unknown>) => {
    const users = (await h.call(admin, 'GET', '/api/v1/config/management/users')).body as {
      username: string;
    }[];
    const i = users.findIndex((x) => x.username === u);
    expect(
      (await h.call(admin, 'PATCH', `/api/v1/config/management/users/${i}`, patch)).status,
    ).toBe(200);
  };
  const commit = async () =>
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=td10b')).status).toBe(200);

  beforeAll(async () => {
    h = await startHarness({ VRX_LOGIN_RATE_PER_MIN: '1000' });
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [
      { username: 'op', role: 'operator', password: PW['op']! },
      { username: 'off', role: 'operator', password: PW['off']! },
    ]);
  });
  afterAll(async () => h?.close());

  it('2.3c logout (refresh cookie) ends THAT session’s access tokens at once; the user’s other session stays; audited', async () => {
    const s1 = await login('op');
    const s2 = await login('op');
    expect((await logout({ cookie: s1.cookie })).status).toBe(204);
    const after = { s1: await me(s1.token), s2: await me(s2.token) };
    console.log(`2.3c after logout of session 1: ${JSON.stringify(after)}`);
    expect(after).toEqual({ s1: 401, s2: 200 });
    expect((await refresh(s1.cookie)).status).toBe(401);
    expect((await refresh(s2.cookie)).status).toBe(200);
    const rows = await audit('auth.logout');
    expect(rows).toEqual([
      {
        user_id: await idOf('op'),
        username: 'op',
        source_ip: '127.0.0.1',
        resource: 'user/op',
        after: { via: 'refresh-cookie' },
        result: 'success',
        status: 204,
      },
    ]);
  });

  it('2.3c the revocation survives an API restart (atrevsid in Valkey, reloaded at boot)', async () => {
    const s = await login('op');
    expect((await logout({ cookie: s.cookie })).status).toBe(204);
    const restarted = new TokensService(h.env, h.app.get<Valkey>(VALKEY), new Bus());
    await restarted.loadRevocations();
    expect(await restarted.verifyAccess(s.token)).toBeNull();
    const other = await login('op');
    expect(await restarted.verifyAccess(other.token)).toMatchObject({ username: 'op' });
  });

  it('2.3c a Bearer logout (the CLI) ends that session too — access token and refresh chain', async () => {
    const s = await login('op');
    expect((await logout({ authorization: `Bearer ${s.token}` })).status).toBe(204);
    expect(await me(s.token)).toBe(401);
    expect((await refresh(s.cookie)).status).toBe(401);
    expect((await audit('auth.logout')).at(-1)).toMatchObject({ after: { via: 'bearer' } });
  });

  it('a forged `<family>.<junk>` logs nobody out (before: anyone knowing a family id ended that session)', async () => {
    const s = await login('op');
    const family = s.cookie.split('=')[1]!.split('.')[0]!;
    const forged = `vrx_refresh=${family}.${randomBytes(32).toString('base64url')}`;
    expect((await logout({ cookie: forged })).status).toBe(204);
    const r = await refresh(s.cookie);
    console.log(
      `forged logout → the victim's refresh ${r.status}, access token ${await me(s.token)}`,
    );
    expect(r.status).toBe(200);
    expect(await me(s.token)).toBe(200);
    // counted, aggregated, without a user
    const agg = await eventually(async () =>
      (await audit('auth.logout', sql`and after->>'reason' = 'unknown-token'`)).at(0),
    );
    expect(agg).toMatchObject({
      user_id: null,
      result: 'failure',
      after: { aggregated: { count: 1 } },
    });
  });

  it('review L4: a logout racing a refresh of the same cookie still ends the session', async () => {
    const s = await login('op');
    const kv = h.app.get<Valkey>(VALKEY);
    const set = kv.set.bind(kv) as (...a: unknown[]) => Promise<unknown>;
    // widen the refresh's window between taking the token and marking it used (where the old code had one)
    const spy = vi.spyOn(kv, 'set').mockImplementation((async (...a: unknown[]) => {
      if (String(a[0]).startsWith('rtused:')) await new Promise((r) => setTimeout(r, 300));
      return set(...a);
    }) as never);
    let r: Awaited<ReturnType<typeof refresh>> | undefined;
    let out: Awaited<ReturnType<typeof logout>> | undefined;
    try {
      const refreshing = refresh(s.cookie);
      await new Promise((res) => setTimeout(res, 100));
      out = await logout({ cookie: s.cookie });
      r = await refreshing;
    } finally {
      spy.mockRestore();
    }
    const after = {
      logout: out!.status,
      refresh: r!.status,
      oldToken: await me(s.token),
      newChain:
        r!.status === 200 ? (await refresh(cookieOf(r!.headers, 'vrx_refresh')!)).status : 401,
      newToken: r!.status === 200 ? await me(r!.body.accessToken as string) : 401,
    };
    console.log(`L4 logout racing a refresh: ${JSON.stringify(after)}`);
    expect(after).toMatchObject({ logout: 204, oldToken: 401, newChain: 401, newToken: 401 });
  });

  it('2.3e refresh-token reuse: the family AND its access tokens die; the audit row names the user', async () => {
    const s = await login('op');
    const r1 = await refresh(s.cookie);
    expect(r1.status).toBe(200);
    const token1 = r1.body.accessToken as string;
    expect((await refresh(s.cookie)).status).toBe(401); // replay of the rotated-away token
    expect(await me(token1)).toBe(401);
    expect(
      (await audit('auth.refresh', sql`and after->>'reason' like 'refresh-token-reuse%'`)).at(-1),
    ).toMatchObject({
      user_id: await idOf('op'),
      username: 'op',
      resource: 'user/op',
      status: 401,
    });
  });

  it('2.3e refresh failures carry their reason; junk is aggregated; no cookie at all writes nothing', async () => {
    const before = (await audit('auth.refresh')).length;
    expect((await refresh('vrx_refresh=')).status).toBe(401);
    expect((await h.call(undefined, 'POST', '/api/v1/auth/refresh')).status).toBe(401);
    expect((await audit('auth.refresh')).length).toBe(before);
    const junk = `vrx_refresh=${randomBytes(12).toString('base64url')}.${randomBytes(32).toString('base64url')}`;
    for (let i = 0; i < 12; i++) expect((await refresh(junk)).status).toBe(401);
    const agg = await eventually(async () => {
      const rows = await audit('auth.refresh', sql`and after->>'reason' = 'unknown-token'`);
      return rows.length === 2 ? rows : null;
    });
    expect(
      agg!.map((r) => (r['after'] as { aggregated: { count: number } }).aggregated.count),
    ).toEqual([1, 10]);
    // a disabled user's chain: revoked by the disable (TD-4), the refresh names the user
    const s = await login('off');
    await patchUser('off', { disabled: true });
    await commit();
    expect((await refresh(s.cookie)).status).toBe(401);
    expect(
      (await audit('auth.refresh', sql`and username = 'off'`)).map(
        (r) => (r['after'] as { reason: string }).reason,
      ),
    ).toEqual(['session-ended']);
  });
});
