import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { runSecret, startHarness, type Harness } from '../support/harness.js';
import { cookieOf } from '../support/proxy.js';

/**
 * TD-10b — PENDING-session-revocation OPTION 1 (the manager's recommendation; the product owner's decision is still
 * open, decision-policy §4). Its own commit and its own suite (review C3): if the answer is another option, the
 * merger drops that commit — datastore/pg-repo.ts `syncUsers` + datastore/repo.ts `PasswordReset.reasons` + this file —
 * and nothing else changes. On the host PostgreSQL + Valkey: a demotion or a deletion committed through the config API
 * ends the user's sessions at once (the JWT carried the old role / a deleted account), like a disable (D-100 (3)).
 */
const PW: Record<string, string> = { demo: runSecret(), promo: runSecret(), gone: runSecret() };

describe('TD-10b PENDING-session-revocation option 1: demotion and deletion end the sessions', () => {
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
  const users = async () =>
    (await h.call(admin, 'GET', '/api/v1/config/management/users')).body as {
      username: string;
    }[];
  const commit = async () =>
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=td10b')).status).toBe(200);

  beforeAll(async () => {
    h = await startHarness({ VRX_LOGIN_RATE_PER_MIN: '1000' });
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [
      { username: 'demo', role: 'operator', password: PW['demo']! },
      { username: 'promo', role: 'readonly', password: PW['promo']! },
      { username: 'gone', role: 'operator', password: PW['gone']! },
    ]);
  });
  afterAll(async () => h?.close());

  it('a DEMOTED user’s old token dies at once (it carried the old role); a promotion ends nothing', async () => {
    const d = await login('demo');
    const p = await login('promo');
    const list = await users();
    for (const [u, role] of [
      ['demo', 'readonly'],
      ['promo', 'operator'],
    ] as const) {
      const i = list.findIndex((x) => x.username === u);
      expect(
        (await h.call(admin, 'PATCH', `/api/v1/config/management/users/${i}`, { role })).status,
      ).toBe(200);
    }
    await commit();
    const after = {
      demoMe: await me(d.token),
      demoRefresh: (await refresh(d.cookie)).status,
      promoMe: await me(p.token),
    };
    console.log(`demotion: ${JSON.stringify(after)}`);
    expect(after).toEqual({ demoMe: 401, demoRefresh: 401, promoMe: 200 });
    const again = await h.call(undefined, 'POST', '/api/v1/auth/login', {
      username: 'demo',
      password: PW['demo'],
    });
    expect(again.body.user).toMatchObject({ username: 'demo', role: 'readonly' });
    // the promoted user gets the new role at the next refresh
    const pr = await refresh(p.cookie);
    expect(pr.body.user).toMatchObject({ role: 'operator' });
  });

  it('a DELETED user’s access token dies at once', async () => {
    const g = await login('gone');
    expect((await h.call(g.token, 'GET', '/api/v1/config')).status).toBe(200);
    const kept = (await users()).filter((u) => u.username !== 'gone');
    expect((await h.call(admin, 'PUT', '/api/v1/config/management/users', kept)).status).toBe(200);
    await commit();
    const after = (await h.call(g.token, 'GET', '/api/v1/config')).status;
    console.log(`deletion: the deleted user's token on GET /api/v1/config → ${after}`);
    expect(after).toBe(401);
    expect((await refresh(g.cookie)).status).toBe(401);
  });
});
