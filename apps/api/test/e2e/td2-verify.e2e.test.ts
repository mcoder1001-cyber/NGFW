import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { hashPassword } from '../../src/auth/password.js';
import { CommitService } from '../../src/commit/commit.service.js';
import { VALKEY, type Valkey } from '../../src/infra/valkey.js';
import { countWhere, guardLongTests, within } from '../support/bounded.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

/**
 * TD-2 verify review (docs/status/tasks/TD-2-verify.md) fix round 2, D-102:
 *  V1 API keys minted while an admin reset runs never survive it (credential generation in app_user, key creation
 *     under FOR SHARE with a re-check of the caller) — continuous minting through JWTs AND chained API keys
 *  V2 a hash changed through the config API (stage → commit / confirm) = an admin reset: sessions, refresh chains and
 *     API keys end; audited `via: config`; minting during that commit does not survive either
 *  V3 Valkey failing after the commit: the reset still holds (PostgreSQL generation + in-process revocation)
 *  V4 the in-flight hash is replaced only after the transaction committed (an aborted reset leaves nothing behind)
 *  V5 keepApiKeys and a discarded key-owned candidate are audited explicitly
 */
const PW: Record<string, string> = {};
const USERS = ['minter', 'cfgv', 'cfgc', 'cfgrace', 'vkfail', 'inflt', 'svc', 'lockkey', 'selfk'];
for (const u of USERS) PW[u] = runSecret();

function cookieOf(headers: Record<string, unknown>): string {
  const raw = [headers['set-cookie']]
    .flat()
    .find((c) => String(c).startsWith('vrx_refresh=')) as string;
  return raw.split(';')[0]!;
}
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
const withKey = (key: string) => ({ authorization: `ApiKey ${key}` });

describe('TD-2 verify fixes e2e (round 2, D-102)', () => {
  let h: Harness;
  let admin: string;
  // T1 (TD-2-verify2.md): the long tests run through `guarded`; afterEach stops and awaits them, every wait is bounded
  const guarded = guardLongTests();

  beforeAll(async () => {
    h = await startHarness({
      VRX_PASSWORD_RATE_PER_MIN: '10000',
      VRX_LOGIN_RATE_PER_MIN: '10000',
    });
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(
      admin,
      USERS.map((u) => ({ username: u, role: 'operator', password: PW[u]! })),
    );
  });
  afterAll(async () => h?.close());

  const login = (u: string, p = PW[u]!) =>
    h.call(undefined, 'POST', '/api/v1/auth/login', { username: u, password: p });
  const token = async (u: string) => (await login(u)).body.accessToken as string;
  const reset = (u: string, body: Record<string, unknown> = {}) => {
    const next = runSecret();
    return h
      .call(admin, 'POST', `/api/v1/users/${u}/password`, { password: next, ...body })
      .then((r) => {
        if (r.status === 200) PW[u] = next;
        return r;
      });
  };
  const keyRows = async (u: string) =>
    Number(
      (
        await h.db.execute(
          sql`select count(*)::int as n from api_key k join app_user u on u.id = k.user_id where u.username = ${u}`,
        )
      ).rows[0]!['n'],
    );
  const userIndex = async (u: string) =>
    (
      (await h.call(admin, 'GET', '/api/v1/config/management/users')).body as { username: string }[]
    ).findIndex((x) => x.username === u);
  const works = async (key: string) =>
    (await h.call(undefined, 'GET', '/api/v1/auth/me', undefined, withKey(key))).status === 200;

  /**
   * The verify probe P1 as a test: while `trigger()` runs, 2 loops mint keys with the target's JWTs and 2 loops mint
   * keys with the target's API keys, each ApiKey loop chaining through the newest key it got (an attacker who keeps
   * a foothold). Returns what survived the trigger. The loops end in `finally` (and on `signal`), never later.
   */
  async function mintDuring(
    user: string,
    trigger: () => Promise<{ status: number }>,
    jitterMs: number,
    signal: AbortSignal,
  ) {
    const jwts = [await token(user), await token(user)];
    const seeds = [];
    for (const n of ['seed-a', 'seed-b']) {
      const k = await h.call(jwts[0], 'POST', '/api/v1/auth/api-keys', {
        name: n,
        current: PW[user],
      });
      expect(k.status).toBe(201);
      seeds.push(k.body.key as string);
    }
    const minted: { key: string; start: number; end: number; via: 'jwt' | 'apikey' }[] = [];
    let stop = false;
    let n = 0;
    const jwtLoop = async (t: string) => {
      while (!stop && !signal.aborted) {
        const start = performance.now();
        const r = await h.call(t, 'POST', '/api/v1/auth/api-keys', {
          name: `m${n++}`,
          current: PW[user],
        });
        if (r.status === 201)
          minted.push({ key: r.body.key, start, end: performance.now(), via: 'jwt' });
        else await sleep(1);
      }
    };
    const keyLoop = async (k0: string) => {
      let k = k0;
      while (!stop && !signal.aborted) {
        const start = performance.now();
        const r = await h.call(
          undefined,
          'POST',
          '/api/v1/auth/api-keys',
          { name: `c${n++}` },
          withKey(k),
        );
        if (r.status === 201) {
          minted.push({ key: r.body.key, start, end: performance.now(), via: 'apikey' });
          k = r.body.key;
        } else await sleep(1);
      }
    };
    const loops = [...jwts.map(jwtLoop), ...seeds.map(keyLoop)];
    let r: { status: number };
    let t0: number;
    let t1: number;
    try {
      await sleep(jitterMs);
      t0 = performance.now();
      r = await trigger();
      t1 = performance.now();
      await sleep(15);
    } finally {
      stop = true;
      await Promise.allSettled(loops);
    }
    await Promise.all(loops); // settled above: this only re-throws a loop's own error
    signal.throwIfAborted();
    expect(r.status).toBe(200);
    // every key minted in this run and both seeds, 8 checks at a time (sequential checks dominated V1 under load)
    const survivors = await countWhere([...minted.map((m) => m.key), ...seeds], works);
    const inFlight = minted.filter((m) => m.start < t1 && m.end > t0).length;
    return { minted: minted.length, inFlight, survivors, rows: await keyRows(user) };
  }

  // ------------------------------------------------------------------------------------------------ V1
  // 16–19 s on a quiet host, 43–155 s at load 15–25 (T1): an explicit budget, not the 30 s default
  it(
    'V1 — 40 runs: keys minted (JWT + chained API keys) while an admin reset runs → 0 working keys, 0 rows',
    guarded(async (signal) => {
      const tot = { minted: 0, inFlight: 0, survivors: 0, rows: 0 };
      for (let run = 0; run < 40; run++) {
        signal.throwIfAborted();
        const r = await mintDuring('minter', () => reset('minter'), run % 9, signal);
        tot.minted += r.minted;
        tot.inFlight += r.inFlight;
        tot.survivors += r.survivors;
        tot.rows += r.rows;
      }
      console.log(
        `V1: 40 admin resets, 4 minting loops each: minted ${tot.minted} keys (${tot.inFlight} by requests in flight during the reset) → working afterwards ${tot.survivors}, api_key rows left ${tot.rows}`,
      );
      expect(tot.inFlight).toBeGreaterThan(0);
      expect(tot.survivors).toBe(0);
      expect(tot.rows).toBe(0);
      // with the NEW password the user mints keys normally again
      const fresh = await token('minter');
      const k = await h.call(fresh, 'POST', '/api/v1/auth/api-keys', {
        name: 'after',
        current: PW['minter'],
      });
      expect(k.status).toBe(201);
      expect(await works(k.body.key)).toBe(true);
    }),
    300_000,
  );

  it('V1 — a session from before the reset cannot mint (401); the session a self-service change kept still can', async () => {
    const other = await token('selfk');
    const mine = await login('selfk');
    const mineTok = mine.body.accessToken as string;
    const next = runSecret();
    const self = await h.call(mineTok, 'POST', '/api/v1/users/selfk/password', {
      password: next,
      current: PW['selfk'],
    });
    expect(self.status).toBe(200);
    PW['selfk'] = next;
    expect(
      (
        await h.call(other, 'POST', '/api/v1/auth/api-keys', {
          name: 'x',
          current: PW['selfk'],
        })
      ).status,
    ).toBe(401);
    const k = await h.call(mineTok, 'POST', '/api/v1/auth/api-keys', {
      name: 'kept-session',
      current: PW['selfk'],
    });
    expect(k.status).toBe(201);
    // the kept chain was moved to the new generation: it refreshes, and the new token mints too
    const rf = await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, {
      cookie: cookieOf(mine.headers),
    });
    expect(rf.status).toBe(200);
    expect(
      (
        await h.call(rf.body.accessToken, 'POST', '/api/v1/auth/api-keys', {
          name: 'refreshed',
          current: PW['selfk'],
        })
      ).status,
    ).toBe(201);
  });

  // ------------------------------------------------------------------------------------------------ V2
  describe('V2 — D-102: a hash staged through the config API is an admin reset', () => {
    async function stageHash(u: string): Promise<string> {
      const next = runSecret();
      const i = await userIndex(u);
      const r = await h.call(admin, 'PATCH', `/api/v1/config/management/users/${i}`, {
        passwordHash: await hashPassword(next),
      });
      expect(r.status).toBe(200);
      return next;
    }

    it('commit: old sessions, refresh chain and API keys end; old password 401, new 200; audited via: config', async () => {
      const l = await login('cfgv');
      const tok = l.body.accessToken as string;
      const cookie = cookieOf(l.headers);
      const k = await h.call(tok, 'POST', '/api/v1/auth/api-keys', {
        name: 'cfg-key',
        current: PW['cfgv'],
      });
      expect(k.status).toBe(201);
      const old = PW['cfgv']!;
      const next = await stageHash('cfgv');
      // staged only: nothing happened yet
      expect((await h.call(tok, 'GET', '/api/v1/auth/me')).status).toBe(200);
      const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=reset%20cfgv');
      expect(c.status).toBe(200);
      PW['cfgv'] = next;
      const after = {
        me: (await h.call(tok, 'GET', '/api/v1/auth/me')).status,
        refresh: (await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, { cookie }))
          .status,
        apiKey: (await h.call(undefined, 'GET', '/api/v1/auth/me', undefined, withKey(k.body.key)))
          .status,
        keyRows: await keyRows('cfgv'),
        oldLogin: (await login('cfgv', old)).status,
        newLogin: (await login('cfgv')).status,
      };
      console.log(`V2 commit: ${JSON.stringify(after)}`);
      expect(after).toEqual({
        me: 401,
        refresh: 401,
        apiKey: 401,
        keyRows: 0,
        oldLogin: 401,
        newLogin: 200,
      });
      const audit = await h.db.execute(
        sql`select username, action, resource, after, result from audit_log
            where action = 'config.password-reset' and resource = 'user/cfgv' order by id desc limit 1`,
      );
      expect(audit.rows[0]).toEqual({
        username: 'admin',
        action: 'config.password-reset',
        resource: 'user/cfgv',
        after: {
          passwordSet: true,
          self: false,
          via: 'config',
          revision: c.body.revision.id,
          txnId: c.body.txnId,
          apiKeysRevoked: [{ id: k.body.id, name: 'cfg-key' }],
        },
        result: 'success',
      });
      const leak = await h.db.execute(
        sql`select count(*)::int as n from audit_log where coalesce(after::text, '') like '%argon2id%'`,
      );
      expect(leak.rows[0]).toEqual({ n: 0 });
    });

    it('confirmed commit: sessions live while pending, end at confirm; a NEW user with a hash is no reset', async () => {
      const l = await login('cfgc');
      const tok = l.body.accessToken as string;
      const k = await h.call(tok, 'POST', '/api/v1/auth/api-keys', {
        name: 'cfgc-key',
        current: PW['cfgc'],
      });
      const next = await stageHash('cfgc');
      // plus a brand-new user with a hash in the same commit
      const users = (await h.call(admin, 'GET', '/api/v1/config/management/users'))
        .body as unknown[];
      const fresh = runSecret();
      expect(
        (
          await h.call(admin, 'PATCH', `/api/v1/config/management/users/${users.length}`, {
            username: 'newbie',
            role: 'readonly',
            passwordHash: await hashPassword(fresh),
          })
        ).status,
      ).toBe(200);
      const c = await h.call(admin, 'POST', '/api/v1/config/commit?confirm=30');
      expect(c.body.status).toBe('pending');
      expect((await h.call(tok, 'GET', '/api/v1/auth/me')).status).toBe(200);
      expect(await works(k.body.key)).toBe(true);
      expect((await h.call(admin, 'POST', '/api/v1/config/commit/confirm')).status).toBe(200);
      PW['cfgc'] = next;
      expect((await h.call(tok, 'GET', '/api/v1/auth/me')).status).toBe(401);
      expect(await works(k.body.key)).toBe(false);
      expect((await login('cfgc')).status).toBe(200);
      expect((await login('newbie', fresh)).status).toBe(200);
      const rows = await h.db.execute(
        sql`select resource from audit_log where action = 'config.password-reset' order by id`,
      );
      expect(rows.rows.map((r) => r['resource'])).toEqual(['user/cfgv', 'user/cfgc']);
    });

    it(
      '10 runs: keys minted while a config-path reset commits → 0 working keys, 0 rows',
      guarded(async (signal) => {
        const tot = { minted: 0, inFlight: 0, survivors: 0, rows: 0 };
        for (let run = 0; run < 10; run++) {
          signal.throwIfAborted();
          const next = await stageHash('cfgrace');
          const r = await mintDuring(
            'cfgrace',
            async () => {
              const c = await h.call(admin, 'POST', '/api/v1/config/commit');
              if (c.status === 200) PW['cfgrace'] = next;
              return c;
            },
            run % 5,
            signal,
          );
          tot.minted += r.minted;
          tot.inFlight += r.inFlight;
          tot.survivors += r.survivors;
          tot.rows += r.rows;
        }
        console.log(
          `V2 race: 10 config-path resets: minted ${tot.minted} (${tot.inFlight} in flight during the commit) → working afterwards ${tot.survivors}, rows left ${tot.rows}`,
        );
        expect(tot.inFlight).toBeGreaterThan(0);
        expect(tot.survivors).toBe(0);
        expect(tot.rows).toBe(0);
      }),
      120_000,
    );
  });

  // ------------------------------------------------------------------------------------------------ V3
  it('V3 — Valkey fails after the commit: 200, old access token and refresh chain still refused, audited', async () => {
    const l = await login('vkfail');
    const tok = l.body.accessToken as string;
    const cookie = cookieOf(l.headers);
    const kv = h.app.get<Valkey>(VALKEY);
    const orig = kv.eval.bind(kv) as (...a: unknown[]) => Promise<unknown>;
    const spy = vi.spyOn(kv, 'eval').mockImplementation(((...a: unknown[]) => {
      // the revocation script (the only one that moves a kept family) fails like a Valkey restart would
      if (String(a[0]).includes('KEEPTTL'))
        return Promise.reject(new Error('simulated Valkey outage'));
      return orig(...a);
    }) as never);
    let r;
    try {
      r = await reset('vkfail');
    } finally {
      spy.mockRestore();
    }
    expect(r.status).toBe(200);
    // Valkey was NOT cleaned: the old family is still there …
    const fam = cookie.split('=')[1]!.split('.')[0]!;
    expect(await kv.exists(`rtfam:${fam}`)).toBe(1);
    // … and still nothing of the old session works
    expect((await h.call(tok, 'GET', '/api/v1/auth/me')).status).toBe(401);
    expect(
      (await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, { cookie })).status,
    ).toBe(401);
    expect((await login('vkfail')).status).toBe(200);
    const audit = await h.db.execute(
      sql`select after from audit_log where action = 'POST /api/v1/users/:name/password'
          and resource = 'user/vkfail' order by id desc limit 1`,
    );
    expect(audit.rows[0]!['after']).toEqual({
      passwordSet: true,
      self: false,
      apiKeysRevoked: [],
      revocationPersisted: false,
    });
  });

  // ------------------------------------------------------------------------------------------------ V4
  /** Bounded poll until a backend waits on a lock that backend `pid` holds (`pg_blocking_pids`) — the V4 reset. */
  async function blockedBy(pid: number, ms: number, signal: AbortSignal) {
    const until = performance.now() + ms;
    for (;;) {
      signal.throwIfAborted();
      const r = await h.db.execute(
        sql`select count(*)::int as n from pg_stat_activity
            where wait_event_type = 'Lock' and ${pid}::int = any(pg_blocking_pids(pid))`,
      );
      if (Number(r.rows[0]!['n']) > 0) return;
      if (performance.now() > until)
        throw new Error(
          `V4: no backend waited on the helper transaction (pid ${pid}) within ${ms} ms`,
        );
      await sleep(10);
    }
  }

  it(
    'V4 — a reset whose transaction aborts (deadlock) leaves the in-flight copy, the hash and the sessions alone',
    guarded(async (signal) => {
      const commits = h.app.get(CommitService);
      const spy = vi.spyOn(commits, 'replaceInflightHash');
      const tok = await token('inflt');
      const old = PW['inflt']!;
      const uid = (
        (await h.db.execute(sql`select id from app_user where username = 'inflt'`)).rows as {
          id: number;
        }[]
      )[0]!.id;
      let resetRes: { status: number } | undefined;
      let pending: Promise<unknown> | undefined;
      let helperErr: unknown;
      // a second connection holds the candidate row; the reset (app_user → api_key → candidate) waits on it; then that
      // connection wants the target's app_user row → deadlock; PostgreSQL aborts the reset (it has waited longest).
      // T1: every wait is bounded — lock_timeout on the helper, a poll until the reset really waits on the helper (not a
      // fixed sleep: a reset that had not locked app_user yet would wait on the helper, which waits on it in JavaScript,
      // a cycle PostgreSQL cannot see), and a bounded wait for the answer, so the helper transaction always ends.
      await h.db
        .transaction(async (tx) => {
          await tx.execute(sql`set local lock_timeout = '10s'`);
          const helper = Number(
            (await tx.execute(sql`select pg_backend_pid() as pid`)).rows[0]!['pid'],
          );
          await tx.execute(sql`select id from config_candidate where id = 1 for update`);
          pending = h
            .call(admin, 'POST', '/api/v1/users/inflt/password', { password: runSecret() })
            .then((r) => (resetRes = r));
          await blockedBy(helper, 15_000, signal);
          await tx.execute(sql`update app_user set last_login = last_login where id = ${uid}`);
          await within(pending, 15_000, 'V4 reset');
        })
        .catch((e: unknown) => {
          helperErr = e;
        });
      // the helper transaction has ended either way, so the reset can answer now
      if (pending !== undefined) await within(pending, 15_000, 'V4 reset after the helper');
      console.log(`V4: reset answered ${resetRes?.status} after a deadlock`);
      expect(helperErr).toBeUndefined();
      expect(resetRes?.status).toBe(500);
      expect(spy).not.toHaveBeenCalled();
      spy.mockRestore();
      expect((await login('inflt', old)).status).toBe(200);
      expect((await h.call(tok, 'GET', '/api/v1/auth/me')).status).toBe(200);
      // and a successful reset calls it once the new hash is visible to other connections (committed)
      let visibleAtCall: Promise<boolean> | undefined;
      const spy2 = vi.spyOn(commits, 'replaceInflightHash').mockImplementation((_n, hash) => {
        visibleAtCall = h.db
          .execute(sql`select password_hash = ${hash} as v from app_user where id = ${uid}`)
          .then((r) => r.rows[0]!['v'] === true);
      });
      expect((await reset('inflt')).status).toBe(200);
      spy2.mockRestore();
      expect(await visibleAtCall).toBe(true);
    }),
    60_000,
  );

  // ------------------------------------------------------------------------------------------------ V5
  it('V5 — keepApiKeys and a discarded key-owned candidate are audited (and the discard is answered)', async () => {
    const t = await token('svc');
    await h.call(t, 'POST', '/api/v1/auth/api-keys', { name: 'svc-1', current: PW['svc'] });
    await h.call(t, 'POST', '/api/v1/auth/api-keys', { name: 'svc-2', current: PW['svc'] });
    expect((await reset('svc', { keepApiKeys: true })).status).toBe(200);
    const a1 = await h.db.execute(
      sql`select after from audit_log where action = 'POST /api/v1/users/:name/password'
          and resource = 'user/svc' order by id desc limit 1`,
    );
    expect(a1.rows[0]!['after']).toEqual({
      passwordSet: true,
      self: false,
      apiKeysRevoked: [],
      keepApiKeys: true,
      apiKeysKept: 2,
    });

    const lt = await token('lockkey');
    const k = await h.call(lt, 'POST', '/api/v1/auth/api-keys', {
      name: 'lock-holder',
      current: PW['lockkey'],
    });
    expect(
      (
        await h.call(
          undefined,
          'PATCH',
          '/api/v1/config/interfaces/loop7777',
          { mtu: 1400 },
          withKey(k.body.key),
        )
      ).status,
    ).toBe(200);
    const r = await reset('lockkey');
    expect(r.body).toEqual({
      self: false,
      apiKeysRevoked: [{ id: k.body.id, name: 'lock-holder' }],
      discardedCandidate: true,
    });
    const a2 = await h.db.execute(
      sql`select after from audit_log where action = 'POST /api/v1/users/:name/password'
          and resource = 'user/lockkey' order by id desc limit 1`,
    );
    expect(a2.rows[0]!['after']).toMatchObject({ discardedCandidate: true });
    expect((await h.call(admin, 'GET', '/api/v1/config/lock')).body.locked).toBe(false);
  });
});
