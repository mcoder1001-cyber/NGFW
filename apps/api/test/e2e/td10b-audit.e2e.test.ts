import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuditService } from '../../src/audit/audit.service.js';
import { CommitService } from '../../src/commit/commit.service.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';
import { cookieAttrs, cookieOf, eventually, via } from '../support/proxy.js';

/**
 * TD-10b on the host PostgreSQL + Valkey: 2.3b (the client behind the trusted proxy: transport rule, per-client rate
 * limit, audit address), 2.3e (401 mutations audited in aggregate; an audit write failure is reported; privileged
 * routes fail closed), 2.3g (/api/docs in a browser through the session's docs cookie) and the manager's addendum
 * (password set: 409 commit-busy after 1 s instead of an unbounded wait behind a commit).
 */
const RATE = 6;
const PW: Record<string, string> = { op: runSecret(), op2: runSecret() };
const X = '203.0.113.10';

describe('TD-10b audit gaps, trusted proxy, /api/docs, commit-busy', () => {
  let h: Harness;
  let admin: string;

  const rows = async (where: ReturnType<typeof sql>) =>
    (
      await h.db.execute(
        sql`select user_id, username, source_ip, action, after, result, status from audit_log where ${where} order by id`,
      )
    ).rows;
  const block = (name: string, action: string) =>
    h.db.execute(
      sql.raw(
        `alter table audit_log add constraint ${name} check (action <> '${action}') not valid`,
      ),
    );
  const unblock = (name: string) =>
    h.db.execute(sql.raw(`alter table audit_log drop constraint ${name}`));

  beforeAll(async () => {
    h = await startHarness({ VRX_LOGIN_RATE_PER_MIN: String(RATE) });
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [
      { username: 'op', role: 'operator', password: PW['op']! },
      { username: 'op2', role: 'operator', password: PW['op2']! },
    ]);
  });
  afterAll(async () => h?.close());

  it('2.3b behind the trusted proxy the CLIENT counts: plain HTTP from another machine → 403 tls-required; TLS → 200, audited with the client address', async () => {
    const cred = { username: 'op', password: PW['op'] };
    const plain = await via(h, 'POST', '/api/v1/auth/login', cred, { client: X, proto: 'http' });
    const tls = await via(h, 'POST', '/api/v1/auth/login', cred, { client: X, proto: 'https' });
    console.log(
      `2.3b via the proxy from ${X}: plain HTTP ${plain.status} ${plain.body.type ?? ''}, TLS ${tls.status}`,
    );
    expect(plain.status).toBe(403);
    expect(plain.body.type).toBe('https://vrx.dev/problems/tls-required');
    expect(tls.status).toBe(200);
    expect(await rows(sql`action = 'auth.login' and username = 'op'`)).toEqual([
      expect.objectContaining({ source_ip: X, after: { reason: 'tls-required' }, status: 403 }),
      expect.objectContaining({ source_ip: X, result: 'success', status: 200 }),
    ]);
  });

  it('2.3b the login rate limit is per client, not one global bucket behind the proxy', async () => {
    const P = '203.0.113.20';
    const Q = '203.0.113.21';
    const statuses: number[] = [];
    for (let i = 0; i <= RATE; i++)
      statuses.push(
        (
          await via(
            h,
            'POST',
            '/api/v1/auth/login',
            { username: 'nobody', password: 'x' },
            {
              client: P,
            },
          )
        ).status,
      );
    const q = (
      await via(
        h,
        'POST',
        '/api/v1/auth/login',
        { username: 'nobody', password: 'x' },
        { client: Q },
      )
    ).status;
    console.log(`2.3b ${RATE + 1} logins from P: ${statuses.join(',')}; then one from Q: ${q}`);
    expect(statuses.at(-1)).toBe(429);
    expect(q).toBe(401);
  });

  it('2.3e unauthenticated mutations leave a bounded trace: one row per client, route and minute at the 1st, 10th … attempt; GETs none', async () => {
    for (let i = 0; i < 12; i++)
      expect((await via(h, 'POST', '/api/v1/config/commit', undefined, { client: X })).status).toBe(
        401,
      );
    expect(
      (
        await via(h, 'POST', '/api/v1/config/commit', undefined, {
          client: X,
          token: 'abc.def.ghi',
        })
      ).status,
    ).toBe(401);
    expect((await via(h, 'GET', '/api/v1/config', undefined, { client: X })).status).toBe(401);
    const got = await eventually(async () => {
      const r = await rows(sql`status = 401 and action like 'POST /api/v1/config/commit'`);
      return r.length === 3 ? r : null;
    });
    console.log(`2.3e 13 unauthenticated commits → ${got?.length} audit rows`);
    expect(got).toEqual([
      expect.objectContaining({
        user_id: null,
        source_ip: X,
        result: 'failure',
        after: expect.objectContaining({
          reason: 'no-credentials',
          aggregated: expect.objectContaining({ count: 1 }),
        }),
      }),
      expect.objectContaining({
        after: expect.objectContaining({
          reason: 'no-credentials',
          aggregated: expect.objectContaining({ count: 10 }),
        }),
      }),
      expect.objectContaining({
        after: expect.objectContaining({ reason: 'invalid-credentials' }),
      }),
    ]);
    expect(await rows(sql`status = 401 and action like 'GET %'`)).toEqual([]);
  });

  it('2.3e an audit write failure: an ordinary mutation still succeeds (fail open) but it is counted and becomes a system_event', async () => {
    const audit = h.app.get(AuditService);
    const before = audit.writeFailures;
    await block('td10b_block_patch', 'PATCH /api/v1/config/*');
    try {
      expect(
        (await h.call(admin, 'PATCH', '/api/v1/config/system', { hostname: 'td10b' })).status,
      ).toBe(200);
    } finally {
      await unblock('td10b_block_patch');
    }
    expect(audit.writeFailures).toBe(before + 1);
    const ev = await h.db.execute(
      sql`select severity, subsystem, data from system_event where code = 'AUDIT_WRITE_FAILED'`,
    );
    console.log(
      `2.3e fail-open: counter ${before} → ${audit.writeFailures}; event ${JSON.stringify(ev.rows[0])}`,
    );
    expect(ev.rows[0]).toMatchObject({
      severity: 'error',
      subsystem: 'audit',
      data: { action: 'PATCH /api/v1/config/*', code: '23514', failuresTotal: before + 1 },
    });
  });

  it('2.3e privileged routes fail CLOSED: no audit row first → 503 audit-unavailable and nothing changed', async () => {
    const keys = async () =>
      Number((await h.db.execute(sql`select count(*)::int as n from api_key`)).rows[0]!['n']);
    const k0 = await keys();
    await block('td10b_block_keys', 'POST /api/v1/auth/api-keys');
    await block('td10b_block_pw', 'POST /api/v1/users/:name/password');
    let key;
    let pw;
    try {
      key = await h.call(admin, 'POST', '/api/v1/auth/api-keys', {
        name: 'blocked',
        current: h.adminPassword,
      });
      pw = await h.call(admin, 'POST', '/api/v1/users/op2/password', { password: runSecret() });
    } finally {
      await unblock('td10b_block_keys');
      await unblock('td10b_block_pw');
    }
    console.log(
      `2.3e fail-closed: key creation ${key.status} ${key.body.type}; password set ${pw.status}`,
    );
    expect(key.status).toBe(503);
    expect(key.body.type).toBe('https://vrx.dev/problems/audit-unavailable');
    expect(pw.status).toBe(503);
    expect(await keys()).toBe(k0);
    expect(
      (
        await h.call(undefined, 'POST', '/api/v1/auth/login', {
          username: 'op2',
          password: PW['op2'],
        })
      ).status,
    ).toBe(200); // the password did not change
    // with the audit log back: one row per request, written before and completed after
    const ok = await h.call(admin, 'POST', '/api/v1/auth/api-keys', {
      name: 'after',
      current: h.adminPassword,
    });
    expect(ok.status).toBe(201);
    expect(await rows(sql`action = 'POST /api/v1/auth/api-keys'`)).toEqual([
      expect.objectContaining({
        username: 'admin',
        result: 'success',
        status: 201,
        after: { name: 'after', role: 'admin', via: 'jwt' },
      }),
    ]);
    expect(await rows(sql`after ? 'incomplete'`)).toEqual([]);
  });

  it('2.3g /api/docs works in a browser: login sets the docs cookie; the Swagger UI and its spec load with it; logout ends it', async () => {
    const l = await h.call(undefined, 'POST', '/api/v1/auth/login', {
      username: 'op',
      password: PW['op'],
    });
    const attrs = cookieAttrs(l.headers, 'vrx_docs')!;
    expect(attrs).toContain('path=/api/docs');
    expect(attrs).toContain('httponly');
    expect(attrs).toContain('samesite=strict');
    expect(attrs).toMatch(/max-age=(900|899)/);
    const docs = cookieOf(l.headers, 'vrx_docs')!;
    const page = await h.call(undefined, 'GET', '/api/docs', undefined, { cookie: docs });
    const spec = await h.call(undefined, 'GET', '/api/docs/swagger-ui-init.js', undefined, {
      cookie: docs,
    });
    const bare = await h.call(undefined, 'GET', '/api/docs');
    console.log(
      `2.3g docs with the cookie: page ${page.status}, init.js ${spec.status}; without: ${bare.status}`,
    );
    expect(page.status).toBe(200);
    expect(spec.status).toBe(200);
    expect(spec.raw).toContain('"openapi": "3.1.0"');
    expect(bare.status).toBe(401);
    expect(bare.body.detail).toMatch(/log in to the web UI/);
    const out = await h.call(undefined, 'POST', '/api/v1/auth/logout', undefined, {
      cookie: cookieOf(l.headers, 'vrx_refresh')!,
    });
    expect(cookieAttrs(out.headers, 'vrx_docs')).toMatch(
      /vrx_docs=;.*(max-age=0|expires=thu, 01 jan 1970)/,
    );
    expect((await h.call(undefined, 'GET', '/api/docs', undefined, { cookie: docs })).status).toBe(
      401,
    );
  });

  it('manager addendum (TD-10a M2): a password set behind a busy commit lock answers 409 commit-busy after ~1 s, and changes nothing', async () => {
    const hold = h.app.get(CommitService).exclusive(() => new Promise((r) => setTimeout(r, 3_000)));
    const t0 = Date.now();
    const r = await h.call(admin, 'POST', '/api/v1/users/op/password', { password: runSecret() });
    const took = Date.now() - t0;
    await hold;
    console.log(`commit-busy: ${r.status} ${r.body.type} after ${took} ms (lock held 3 s)`);
    expect(r.status).toBe(409);
    expect(r.body).toMatchObject({
      type: 'https://vrx.dev/problems/commit-busy',
      retryAfterSec: 2,
    });
    expect(took).toBeLessThan(2_500);
    // the abandoned section did nothing when its turn came: the old password still works
    expect(
      (
        await h.call(undefined, 'POST', '/api/v1/auth/login', {
          username: 'op',
          password: PW['op'],
        })
      ).status,
    ).toBe(200);
    expect(
      (await rows(sql`action = 'POST /api/v1/users/:name/password' and status = 409`)).length,
    ).toBe(1);
  });
});
