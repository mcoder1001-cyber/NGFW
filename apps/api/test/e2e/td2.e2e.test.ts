import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { hashPassword } from '../../src/auth/password.js';
import { roomInRateWindow } from '../support/bounded.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

/**
 * TD-2 follow-ups of the P06 API on the host PostgreSQL + Valkey + fake agent:
 *  #1 POST /api/v1/users/{name}/password   #3 /health response   #4 control characters   #5 candidate lock per API key.
 * Passwords are generated per run (never literal). Interfaces follow the slot rules (loop7xx on slot 7).
 */
const PW = { op: runSecret(), ro: runSecret(), op3: runSecret(), admin2: runSecret() };

function cookieOf(headers: Record<string, unknown>): string {
  const raw = [headers['set-cookie']]
    .flat()
    .find((c) => String(c).startsWith('vrx_refresh=')) as string;
  return raw.split(';')[0]!;
}

describe('TD-2 e2e', () => {
  let h: Harness;
  let admin: string;
  const slot = () => Number(/(\d+)$/.exec(h.prefix)?.[1] ?? '7');
  const IF = () => `loop${slot()}01`;

  beforeAll(async () => {
    h = await startHarness({ VRX_PASSWORD_RATE_PER_MIN: '30', VRX_LOGIN_RATE_PER_MIN: '200' });
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [
      { username: 'op1', role: 'operator', password: PW.op },
      { username: 'ro1', role: 'readonly', password: PW.ro },
      { username: 'op3', role: 'operator', password: PW.op3 },
      { username: 'admin2', role: 'admin', password: PW.admin2 },
    ]);
  });
  afterAll(async () => h?.close());

  const apiKey = async (token: string, name: string): Promise<{ id: string; key: string }> => {
    const r = await h.call(token, 'POST', '/api/v1/auth/api-keys', { name });
    expect(r.status).toBe(201);
    return r.body;
  };
  const withKey = (key: string) => ({ authorization: `ApiKey ${key}` });

  // ---------------------------------------------------------------- #1
  describe('#1 POST /api/v1/users/{name}/password', () => {
    it('admin sets another user’s password: old one dead, new one works, every session of the target ends', async () => {
      const l = await h.call(undefined, 'POST', '/api/v1/auth/login', {
        username: 'op1',
        password: PW.op,
      });
      const opToken = l.body.accessToken as string;
      const opCookie = cookieOf(l.headers);
      expect((await h.call(opToken, 'GET', '/api/v1/auth/me')).status).toBe(200);
      const revsBefore = await h.call(admin, 'GET', '/api/v1/config/revisions');

      const next = runSecret() + runSecret();
      const r = await h.call(admin, 'POST', '/api/v1/users/op1/password', { password: next });
      expect(r.status).toBe(200);
      expect(r.body).toEqual({ self: false, apiKeysRevoked: [], discardedCandidate: false });

      // the target's sessions: access token refused, refresh cookie refused
      expect((await h.call(opToken, 'GET', '/api/v1/auth/me')).status).toBe(401);
      const ref = await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, {
        cookie: opCookie,
      });
      expect(ref.status).toBe(401);
      // old password refused, new one works; the admin's own session is untouched
      const old = await h.call(undefined, 'POST', '/api/v1/auth/login', {
        username: 'op1',
        password: PW.op,
      });
      expect(old.status).toBe(401);
      PW.op = next;
      await h.login('op1', PW.op);
      expect((await h.call(admin, 'GET', '/api/v1/auth/me')).status).toBe(200);

      // stored as argon2id in app_user only; no revision, no plaintext anywhere
      const row = await h.db.execute(
        sql`select password_hash like '$argon2id$%' as argon from app_user where username = 'op1'`,
      );
      expect(row.rows[0]).toEqual({ argon: true });
      const revsAfter = await h.call(admin, 'GET', '/api/v1/config/revisions');
      expect(revsAfter.body.total).toBe(revsBefore.body.total);
      const leak = await h.db.execute(
        sql`select
              (select count(*) from audit_log where coalesce(before::text,'') || coalesce(after::text,'') || coalesce(resource,'') like ${'%' + next + '%'})::int as audit,
              (select count(*) from config_revision where payload::text like ${'%' + next + '%'} or payload::text like '%argon2id%')::int as revisions,
              (select count(*) from system_event where coalesce(data::text,'') || message like ${'%' + next + '%'})::int as events`,
      );
      expect(leak.rows[0]).toEqual({ audit: 0, revisions: 0, events: 0 });
      // audited without the value
      const audit = await h.db.execute(
        sql`select username, action, resource, after, result, status from audit_log
            where action = 'POST /api/v1/users/:name/password' order by id desc limit 1`,
      );
      expect(audit.rows[0]).toEqual({
        username: 'admin',
        action: 'POST /api/v1/users/:name/password',
        resource: 'user/op1',
        after: { passwordSet: true, self: false, apiKeysRevoked: [] },
        result: 'success',
        status: 200,
      });
    });

    it('a user sets their own password with the current one; their session survives, the others end', async () => {
      const mine = await h.call(undefined, 'POST', '/api/v1/auth/login', {
        username: 'ro1',
        password: PW.ro,
      });
      const other = await h.call(undefined, 'POST', '/api/v1/auth/login', {
        username: 'ro1',
        password: PW.ro,
      });
      const token = mine.body.accessToken as string;
      const next = runSecret();

      const noCurrent = await h.call(token, 'POST', '/api/v1/users/ro1/password', {
        password: next,
      });
      expect(noCurrent.status).toBe(400);
      expect(noCurrent.headers['content-type']).toContain('application/problem+json');
      expect(noCurrent.body.errors).toEqual([expect.objectContaining({ pointer: '/current' })]);
      const wrong = await h.call(token, 'POST', '/api/v1/users/ro1/password', {
        password: next,
        current: runSecret(),
      });
      expect(wrong.status).toBe(403);
      const short = await h.call(token, 'POST', '/api/v1/users/ro1/password', {
        password: 'short',
        current: PW.ro,
      });
      expect(short.status).toBe(400);
      expect(short.body.errors[0].pointer).toBe('/password');

      const ok = await h.call(token, 'POST', '/api/v1/users/ro1/password', {
        password: next,
        current: PW.ro,
      });
      expect(ok.status).toBe(200);
      PW.ro = next;
      // own session (JWT + refresh family) survives; the other login session is gone
      expect((await h.call(token, 'GET', '/api/v1/auth/me')).status).toBe(200);
      expect(
        (
          await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, {
            cookie: cookieOf(mine.headers),
          })
        ).status,
      ).toBe(200);
      expect((await h.call(other.body.accessToken, 'GET', '/api/v1/auth/me')).status).toBe(401);
      expect(
        (
          await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, {
            cookie: cookieOf(other.headers),
          })
        ).status,
      ).toBe(401);
      await h.login('ro1', PW.ro);
    });

    it('RBAC: non-admins cannot set other users’ passwords (403, no user enumeration); unknown user 404 for admins', async () => {
      const op = await h.login('op3', PW.op3);
      const ro = await h.login('ro1', PW.ro);
      for (const target of ['op1', 'admin', 'nobody-here']) {
        expect(
          (await h.call(op, 'POST', `/api/v1/users/${target}/password`, { password: runSecret() }))
            .status,
        ).toBe(403);
        expect(
          (await h.call(ro, 'POST', `/api/v1/users/${target}/password`, { password: runSecret() }))
            .status,
        ).toBe(403);
      }
      const unknown = await h.call(admin, 'POST', '/api/v1/users/nobody-here/password', {
        password: runSecret(),
      });
      expect(unknown.status).toBe(404);
      // an admin API key may reset passwords too (automation)
      const k = await apiKey(admin, 'pw-reset');
      const next = runSecret();
      const r = await h.call(
        undefined,
        'POST',
        '/api/v1/users/op3/password',
        { password: next },
        withKey(k.key),
      );
      expect(r.status).toBe(200);
      expect((await h.call(op, 'GET', '/api/v1/auth/me')).status).toBe(401);
      PW.op3 = next;
      await h.login('op3', PW.op3);
      expect((await h.call(admin, 'DELETE', `/api/v1/auth/api-keys/${k.id}`)).status).toBe(204);
    });

    it('plain HTTP from a non-loopback peer is refused (TLS only); nothing changes', async () => {
      const next = runSecret();
      const res = await h.app.inject({
        method: 'POST',
        url: '/api/v1/users/op1/password',
        remoteAddress: '192.0.2.10',
        headers: { authorization: `Bearer ${admin}` },
        payload: { password: next },
      });
      expect(res.statusCode).toBe(403);
      expect(res.json()).toMatchObject({ type: 'https://vrx.dev/problems/tls-required' });
      await h.login('op1', PW.op); // unchanged
    });

    it('a hash staged in the candidate is replaced, so the next commit does not bring the old password back', async () => {
      const stale = await hashPassword(PW.op);
      const put = await h.call(admin, 'PATCH', '/api/v1/config/management', {
        users: [
          { username: 'admin', role: 'admin' },
          { username: 'op1', role: 'operator', passwordHash: stale },
          { username: 'ro1', role: 'readonly' },
          { username: 'op3', role: 'operator' },
          { username: 'admin2', role: 'admin' },
        ],
      });
      expect(put.status).toBe(200);
      const next = runSecret();
      expect(
        (await h.call(admin, 'POST', '/api/v1/users/op1/password', { password: next })).status,
      ).toBe(200);
      const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=td2-users');
      expect(c.status).toBe(200);
      const old = await h.call(undefined, 'POST', '/api/v1/auth/login', {
        username: 'op1',
        password: PW.op,
      });
      expect(old.status).toBe(401);
      PW.op = next;
      await h.login('op1', PW.op);
    });

    it('rate-limited per caller (429)', async () => {
      const h2 = h; // same app; a fresh caller: op3 changing its own password with wrong current passwords
      const op = await h2.login('op3', PW.op3);
      let last = 0;
      // T1 fix (TD-2-questions #9): each self attempt is paired with one on another user (403, which counts for the
      // caller only, never for the target op3). The per-caller limit then comes after about 15 target hits in any
      // 60 s bucket, so the admin reset below stays under the per-target limit. Before, the test passed only when the
      // RBAC test's three op3 calls fell into the same bucket.
      for (let i = 0; i < 40 && last !== 429; i++) {
        last = (await h2.call(op, 'POST', '/api/v1/users/op1/password', { password: runSecret() }))
          .status;
        if (last === 429) break;
        expect(last).toBe(403);
        last = (
          await h2.call(op, 'POST', '/api/v1/users/op3/password', {
            password: runSecret(),
            current: runSecret(),
          })
        ).status;
      }
      expect(last).toBe(429);
      // D-097 / review M2: the wrong guesses locked op3 (they count like failed logins); an admin reset unlocks
      expect(
        (
          await h.call(undefined, 'POST', '/api/v1/auth/login', {
            username: 'op3',
            password: PW.op3,
          })
        ).status,
      ).toBe(401);
      const next = runSecret();
      expect(
        (await h.call(admin, 'POST', '/api/v1/users/op3/password', { password: next })).status,
      ).toBe(200);
      PW.op3 = next;
      await h.login('op3', PW.op3);
    });

    // T1 fix (TD-2-questions #9): the ≤ 35 bound holds only when the whole count lands in one 60 s window (it took
    // 5–10 s here), so start it with at least 30 s left in the window; the wait is at most 30 s, hence the budget
    it('rate-limited per target too (review M2): two admins alternating on one user → 429', async () => {
      const admin2 = await h.login('admin2', PW.admin2);
      const callers = [admin, admin2];
      let last = 0;
      let n = 0;
      await roomInRateWindow(30_000);
      for (; n < 70 && last !== 429; n++) {
        const next = runSecret();
        last = (
          await h.call(callers[n % 2], 'POST', '/api/v1/users/op1/password', { password: next })
        ).status;
        if (last === 200) PW.op = next;
      }
      expect(last).toBe(429);
      expect(n).toBeLessThanOrEqual(35); // the per-caller limit (30) alone would allow ~60 here
    }, 90_000);
  });

  // ---------------------------------------------------------------- #3
  it('#3 GET /api/v1/health answers what the OpenAPI response schema says', async () => {
    const r = await h.call(undefined, 'GET', '/api/v1/health');
    expect(r.status).toBe(200);
    expect(Object.keys(r.body).sort()).toEqual(['service', 'status', 'time', 'version']);
    const doc = await h.call(admin, 'GET', '/api/docs-json');
    const schema =
      doc.body.paths['/api/v1/health'].get.responses['200'].content['application/json'].schema;
    expect(schema.required).toEqual(['status', 'service', 'version', 'time']);
  });

  // ---------------------------------------------------------------- #4
  describe('#4 control characters and bidi overrides → 400 problem+json with pointer', () => {
    it('commit / rollback comments', async () => {
      for (const c of ['%1B%5D0%3BPWNED%07', 'ok%0D%1B%5B2Kx', '%E2%80%AEevil', '%C2%9B31m']) {
        const r = await h.call(admin, 'POST', `/api/v1/config/commit?comment=${c}`);
        expect(r.status, c).toBe(400);
        expect(r.headers['content-type']).toContain('application/problem+json');
        expect(r.body.errors).toEqual([expect.objectContaining({ pointer: '/comment' })]);
        const rb = await h.call(admin, 'POST', `/api/v1/config/rollback/1?comment=${c}`);
        expect(rb.status).toBe(400);
        expect(rb.body.errors[0].pointer).toBe('/comment');
      }
      // Persian and a plain comment still work
      await h.call(admin, 'PATCH', `/api/v1/config/interfaces/${IF()}`, {
        ipv4: [`10.${slot()}.101.1/24`],
      });
      const ok = await h.call(
        admin,
        'POST',
        `/api/v1/config/commit?comment=${encodeURIComponent('تغییر آدرس — ok')}`,
      );
      expect(ok.status).toBe(200);
      expect(ok.body.revision.comment).toBe('تغییر آدرس — ok');
    });

    it('API key names, login usernames, path parameters, query parameters', async () => {
      const k = await h.call(admin, 'POST', '/api/v1/auth/api-keys', { name: 'ci\u001b[31m' });
      expect(k.status).toBe(400);
      expect(k.body.errors[0].pointer).toBe('/name');
      const l = await h.call(undefined, 'POST', '/api/v1/auth/login', {
        username: 'admin\u202e',
        password: 'x',
      });
      expect(l.status).toBe(400);
      expect(l.body.errors[0].pointer).toBe('/username');
      const u = await h.call(admin, 'POST', '/api/v1/users/op1%1B%5B2K/password', {
        password: runSecret(),
      });
      expect(u.status).toBe(400);
      expect(u.body.errors[0].pointer).toBe('/name');
      const s = await h.call(admin, 'DELETE', '/api/v1/secrets/psk/x%07y');
      expect(s.status).toBe(400);
      expect(s.body.errors[0].pointer).toBe('/name');
      const a = await h.call(admin, 'POST', '/api/v1/actions/ping%1B');
      expect(a.status).toBe(400);
      const v = await h.call(admin, 'GET', '/api/v1/state/routes?vrf=%E2%81%A6x');
      expect(v.status).toBe(400);
      expect(v.body.errors[0].pointer).toBe('/vrf');
    });

    it('configuration documents: values and member names the schema leaves open, and URL path segments', async () => {
      const d = await h.call(admin, 'PATCH', '/api/v1/config/acl', {
        lists: { [`td2-${slot()}`]: { description: 'x\u001b]52;c;AAAA\u0007', rules: [] } },
      });
      expect(d.status).toBe(400);
      expect(d.body.errors).toEqual([
        expect.objectContaining({
          pointer: `/acl/lists/td2-${slot()}/description`,
          rule: 'api.safe-text',
        }),
      ]);
      const key = await h.call(admin, 'PATCH', '/api/v1/config/routing', {
        bgp: { neighbors: { 'n\u001b[1A': {} } },
      });
      expect(key.status).toBe(400);
      expect(key.body.errors[0].pointer).toBe('/routing/bgp/neighbors/n\\u{1b}[1A');
      expect(key.raw).not.toContain('\\u001b');
      const path = await h.call(admin, 'PUT', '/api/v1/config/interfaces/loop%1B1', {});
      expect(path.status).toBe(400);
      expect(path.body.errors[0].pointer).toBe('/interfaces/loop\\u{1b}1');
      // nothing was staged by the refused edits
      expect((await h.call(admin, 'GET', '/api/v1/config/diff')).body.changes).toEqual([]);
    });
  });

  // ---------------------------------------------------------------- #5
  describe('#5 candidate + lock owner per API key (D-093)', () => {
    it('two keys of ONE user edit in parallel without interfering', async () => {
      const a = await apiKey(admin, 'pipeline-a');
      const b = await apiKey(admin, 'pipeline-b');
      const A = withKey(a.key);
      const B = withKey(b.key);
      const ifA = `loop${slot()}11`;
      const ifB = `loop${slot()}12`;

      // both start at the same moment: exactly one gets the lock, the other a 409 naming the key
      const [ra, rb] = await Promise.all([
        h.call(
          undefined,
          'PUT',
          `/api/v1/config/interfaces/${ifA}`,
          { ipv4: [`10.${slot()}.111.1/24`] },
          A,
        ),
        h.call(
          undefined,
          'PUT',
          `/api/v1/config/interfaces/${ifB}`,
          { ipv4: [`10.${slot()}.112.1/24`] },
          B,
        ),
      ]);
      expect([ra.status, rb.status].sort()).toEqual([200, 409]);
      const [win, winH, lose, loseH, loseIf, loseIp, loser] =
        ra.status === 200
          ? [a, A, rb, B, ifB, `10.${slot()}.112.1/24`, b]
          : [b, B, ra, A, ifA, `10.${slot()}.111.1/24`, a];
      expect(lose.body.lock).toMatchObject({ ownerId: 1, ownerKeyId: win.id });
      expect(lose.body.detail).toContain('API key');

      // the loser can neither edit, discard nor commit the winner's candidate
      expect(
        (await h.call(undefined, 'POST', '/api/v1/config/discard', undefined, loseH)).status,
      ).toBe(409);
      expect(
        (await h.call(undefined, 'POST', '/api/v1/config/commit', undefined, loseH)).status,
      ).toBe(409);
      expect(
        (
          await h.call(
            undefined,
            'PATCH',
            `/api/v1/config/interfaces/${loseIf}`,
            { mtu: 1400 },
            loseH,
          )
        ).status,
      ).toBe(409);
      // nor can the same user's interactive session; it sees who holds the lock
      const lock = await h.call(admin, 'GET', '/api/v1/config/lock');
      expect(lock.body).toMatchObject({
        locked: true,
        owner: 'admin',
        ownerKeyId: win.id,
        ownerKey: expect.stringMatching(/^pipeline-/),
      });
      expect((await h.call(admin, 'POST', '/api/v1/config/discard')).status).toBe(409);

      // the winner's candidate holds exactly its own change, and it commits it
      const diff = await h.call(undefined, 'GET', '/api/v1/config/diff', undefined, winH);
      expect(diff.body.changes.map((c: { pointer: string }) => c.pointer)).toEqual([
        `/interfaces/${ra.status === 200 ? ifA : ifB}`,
      ]);
      const cw = await h.call(
        undefined,
        'POST',
        '/api/v1/config/commit?comment=winner',
        undefined,
        winH,
      );
      expect(cw.status).toBe(200);

      // now the other pipeline gets its own, clean candidate
      const again = await h.call(
        undefined,
        'PUT',
        `/api/v1/config/interfaces/${loseIf}`,
        { ipv4: [loseIp] },
        loseH,
      );
      expect(again.status).toBe(200);
      expect((await h.call(admin, 'GET', '/api/v1/config/lock')).body.ownerKeyId).toBe(loser.id);
      const d2 = await h.call(undefined, 'GET', '/api/v1/config/diff', undefined, loseH);
      expect(d2.body.changes.map((c: { pointer: string }) => c.pointer)).toEqual([
        `/interfaces/${loseIf}`,
      ]);
      const cl = await h.call(
        undefined,
        'POST',
        '/api/v1/config/commit?comment=loser',
        undefined,
        loseH,
      );
      expect(cl.status).toBe(200);

      const running = await h.call(admin, 'GET', '/api/v1/config/interfaces');
      expect(running.body[ifA].ipv4).toEqual([`10.${slot()}.111.1/24`]);
      expect(running.body[ifB].ipv4).toEqual([`10.${slot()}.112.1/24`]);
      const revs = await h.call(admin, 'GET', '/api/v1/config/revisions?limit=2');
      expect(revs.body.items.map((r: { comment: string }) => r.comment)).toEqual([
        'loser',
        'winner',
      ]);

      // cleanup: the slot's interfaces and both keys
      await h.call(admin, 'DELETE', `/api/v1/config/interfaces/${ifA}`);
      await h.call(admin, 'DELETE', `/api/v1/config/interfaces/${ifB}`);
      expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=cleanup')).status).toBe(
        200,
      );
      for (const k of [a, b])
        expect((await h.call(admin, 'DELETE', `/api/v1/auth/api-keys/${k.id}`)).status).toBe(204);
    });

    it('interactive sessions of one user still share one candidate (unchanged)', async () => {
      const tab1 = await h.login('op1', PW.op);
      const tab2 = await h.login('op1', PW.op);
      expect(
        (await h.call(tab1, 'PATCH', `/api/v1/config/interfaces/${IF()}`, { mtu: 1400 })).status,
      ).toBe(200);
      expect(
        (await h.call(tab2, 'PATCH', `/api/v1/config/interfaces/${IF()}`, { mtu: 1450 })).status,
      ).toBe(200);
      const lock = await h.call(tab2, 'GET', '/api/v1/config/lock');
      expect(lock.body).toMatchObject({ owner: 'op1', ownerKeyId: null, ownerKey: null });
      expect((await h.call(tab1, 'POST', '/api/v1/config/discard')).body).toEqual({
        discarded: true,
      });
    });
  });

  // ---------------------------------------------------------------- #6
  describe('#6 secret leaves are visible as redacted changes (P07b review H1)', () => {
    it('hash-only edit → one redacted diff entry → commit → revision diff shows it redacted; the value never leaves', async () => {
      const newHash = await hashPassword(runSecret());
      const users = await h.call(admin, 'GET', '/api/v1/config/management/users');
      const i = (users.body as { username: string }[]).findIndex((u) => u.username === 'op3');
      expect(i).toBeGreaterThanOrEqual(0);

      const edit = await h.call(admin, 'PATCH', `/api/v1/config/management/users/${i}`, {
        passwordHash: newHash,
      });
      expect(edit.status).toBe(200);
      expect(edit.body.secretChanges).toEqual([
        { op: 'replace', pointer: `/management/users/${i}/passwordHash`, redacted: true },
      ]);
      const d = await h.call(admin, 'GET', '/api/v1/config/diff');
      expect(d.body.changes).toEqual([
        { op: 'replace', pointer: `/management/users/${i}/passwordHash`, redacted: true },
      ]);

      const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=op3-hash');
      expect(c.status).toBe(200);
      expect(c.body.status).not.toBe('unchanged');
      expect(c.body.revision.secretChanges).toEqual([
        { op: 'replace', pointer: `/management/users/${i}/passwordHash`, redacted: true },
      ]);
      const rev = c.body.revision.id as number;
      const rd = await h.call(admin, 'GET', `/api/v1/config/revisions/${rev}/diff`);
      expect(rd.status).toBe(200);
      expect(rd.body).toEqual({
        revision: rev,
        parent: c.body.revision.parentId,
        changes: [
          { op: 'replace', pointer: `/management/users/${i}/passwordHash`, redacted: true },
        ],
      });
      const one = await h.call(admin, 'GET', `/api/v1/config/revisions/${rev}`);
      expect(one.body.secretChanges).toHaveLength(1);
      expect((await h.call(admin, 'GET', '/api/v1/config/diff')).body.changes).toEqual([]);

      // the audit row of the edit says THAT the hash changed, never what
      const audit = await h.db.execute(
        sql`select before, after from audit_log where action = 'PATCH /api/v1/config/*'
            and resource = ${`/management/users/${i}`} order by id desc limit 1`,
      );
      expect(audit.rows[0]).toMatchObject({
        before: { username: 'op3', passwordHash: '<redacted>' },
        after: { username: 'op3', passwordHash: '<redacted:changed>' },
      });

      // the hash is nowhere a client or the logs can see it
      const hashTail = newHash.split('$').at(-1)!;
      const leak = await h.db.execute(
        sql`select
              (select count(*) from audit_log where coalesce(before::text,'') || coalesce(after::text,'') like ${'%' + hashTail + '%'})::int as audit,
              (select count(*) from config_revision where payload::text || secret_changes::text like ${'%' + hashTail + '%'})::int as revisions`,
      );
      expect(leak.rows[0]).toEqual({ audit: 0, revisions: 0 });
      for (const url of [
        '/api/v1/config',
        '/api/v1/config/candidate',
        '/api/v1/config/export',
        `/api/v1/config/revisions/${rev}`,
        `/api/v1/config/revisions/${rev}/diff`,
        '/api/v1/config/revisions',
        '/api/v1/audit?limit=50',
      ]) {
        const r = await h.call(admin, 'GET', url);
        expect(r.status, url).toBe(200);
        expect(r.raw, url).not.toContain(hashTail);
        expect(r.raw, url).not.toContain('$argon2id$');
      }
      // and app_user follows the committed hash (the user's password changed through the config path)
      const row = await h.db.execute(
        sql`select password_hash = ${newHash} as same from app_user where username = 'op3'`,
      );
      expect(row.rows[0]).toEqual({ same: true });
    });
  });
});
