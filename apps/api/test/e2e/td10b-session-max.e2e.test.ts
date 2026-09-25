import { sql } from 'drizzle-orm';
import { decodeJwt } from 'jose';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { runSecret, startHarness, type Harness } from '../support/harness.js';
import { cookieAttrs, cookieOf } from '../support/proxy.js';

/**
 * TD-10b review 2.3d on the host PostgreSQL + Valkey: a login session has an ABSOLUTE lifetime (VRX_SESSION_MAX_SEC,
 * here 4 s) next to the sliding refresh TTL (7 days): refreshes do not extend it, the cookies and the access token
 * never outlive it, the refused refresh is audited as `session-expired`.
 */
const MAX = 5;
const PW = runSecret();
const maxAge = (headers: Record<string, unknown>, name: string) =>
  Number(/max-age=(\d+)/.exec(cookieAttrs(headers, name) ?? '')?.[1]);

describe('TD-10b 2.3d VRX_SESSION_MAX_SEC', () => {
  let h: Harness;

  beforeAll(async () => {
    h = await startHarness({ VRX_SESSION_MAX_SEC: String(MAX), VRX_LOGIN_RATE_PER_MIN: '1000' });
    const admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [{ username: 'u', role: 'operator', password: PW }]);
  });
  afterAll(async () => h?.close());

  it('refreshes do not extend a session past its maximum age; nothing issued outlives it', async () => {
    const t0 = Date.now();
    const l = await h.call(undefined, 'POST', '/api/v1/auth/login', {
      username: 'u',
      password: PW,
    });
    expect(l.status).toBe(200);
    const token = l.body.accessToken as string;
    const { exp, iat } = decodeJwt(token);
    const first = {
      expiresIn: l.body.expiresIn as number,
      tokenLife: exp! - iat!,
      refreshCookie: maxAge(l.headers, 'vrx_refresh'),
      docsCookie: maxAge(l.headers, 'vrx_docs'),
    };
    await new Promise((r) => setTimeout(r, 1_200));
    const r1 = await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, {
      cookie: cookieOf(l.headers, 'vrx_refresh')!,
    });
    expect(r1.status).toBe(200);
    const second = {
      expiresIn: r1.body.expiresIn as number,
      refreshCookie: maxAge(r1.headers, 'vrx_refresh'),
    };
    await new Promise((r) => setTimeout(r, Math.max(0, t0 + MAX * 1000 + 700 - Date.now())));
    const r2 = await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, {
      cookie: cookieOf(r1.headers, 'vrx_refresh')!,
    });
    console.log(
      `2.3d max ${MAX}s: login ${JSON.stringify(first)}; refresh at +1.2 s ${r1.status} ${JSON.stringify(second)}; refresh at +${MAX}.7 s ${r2.status}`,
    );
    for (const v of Object.values(first)) expect(v).toBeLessThanOrEqual(MAX);
    expect(second.refreshCookie).toBeLessThanOrEqual(MAX - 1);
    expect(second.expiresIn).toBeLessThanOrEqual(MAX - 1);
    expect(r2.status).toBe(401);
    const rows = await h.db.execute(
      sql`select username, after->>'reason' as reason from audit_log where action = 'auth.refresh' order by id`,
    );
    expect(rows.rows).toEqual([{ username: 'u', reason: 'session-expired' }]);
    // a fresh login starts a new session
    expect(
      (await h.call(undefined, 'POST', '/api/v1/auth/login', { username: 'u', password: PW }))
        .status,
    ).toBe(200);
  });
});
