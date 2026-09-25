import { Logger } from '@nestjs/common';
import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { hashPassword } from '../../src/auth/password.js';
import { TokensService } from '../../src/auth/tokens.service.js';
import { VALKEY, type Valkey } from '../../src/infra/valkey.js';
import { within } from '../support/bounded.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

/**
 * TD-4 — auth hardening follow-ups of TD-2 (D-100 (1)–(3)) on the host PostgreSQL + Valkey + fake agent:
 *  (1) POST /auth/login from a remote plain-HTTP peer → 403 tls-required, checked before the rate limiter, the lookup
 *      and argon2 (no failed login counted, lockout untouched), audited without the password
 *  (2) POST /auth/api-keys from a JWT session needs the current password (step-up); API-key callers must not send it
 *      (D-124: 400, never checked); failed creations are audited with their problem slug (review M1). The burst case
 *      (review H1: per-account budget before argon2, one `locked` body) is td4-stepup-burst.e2e.test.ts
 *  (3) disabling an account through the config API bumps the credential generation: access tokens, refresh chains
 *      and WebSockets end at the commit (at confirm for a confirmed commit); API keys are refused while disabled
 * Passwords are generated per run; the file checks at the end that none of them reached audit_log, system_event or
 * the API log (every Nest log call at every level, and anything else written to stdout/stderr).
 */
const PW: Record<string, string> = {};
const USERS = [
  'tlsuser',
  'stepper',
  'locker',
  'keyuser',
  'stale4',
  'racer4',
  'dis',
  'disc',
  'dual',
];
for (const u of USERS) PW[u] = runSecret();
/** every password this file ever sends (wrong ones too): none may be stored or logged */
const SENT = new Set<string>();
const REMOTE = '192.0.2.10';
const MAX_FAILURES = 3;

function cookieOf(headers: Record<string, unknown>): string {
  const raw = [headers['set-cookie']]
    .flat()
    .find((c) => String(c).startsWith('vrx_refresh=')) as string;
  return raw.split(';')[0]!;
}
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
const withKey = (key: string) => ({ authorization: `ApiKey ${key}` });
const TLS = 'https://vrx.dev/problems/tls-required';

describe('TD-4 auth hardening e2e (D-100)', () => {
  let h: Harness;
  let admin: string;
  // the API log: every Nest log call at EVERY level (the harness prints errors only, so the logger is replaced by
  // one that records all of them), plus anything else written to stdout/stderr while this file runs
  const logged: string[] = [];
  const writes = { out: process.stdout.write, err: process.stderr.write };
  const record =
    (level: string) =>
    (message: unknown, ...rest: unknown[]) => {
      const line = `${level} ${JSON.stringify([message, ...rest])}`;
      logged.push(line);
      if (level === 'error') writes.err.call(process.stderr, `${line}\n`);
    };

  beforeAll(async () => {
    for (const [stream, orig] of [
      [process.stdout, writes.out],
      [process.stderr, writes.err],
    ] as const) {
      stream.write = ((chunk: unknown, ...rest: unknown[]) => {
        logged.push(String(chunk));
        return (orig as (...a: unknown[]) => boolean).call(stream, chunk, ...rest);
      }) as typeof stream.write;
    }
    h = await startHarness({
      VRX_LOGIN_MAX_FAILURES: String(MAX_FAILURES),
      VRX_LOGIN_RATE_PER_MIN: '1000',
      VRX_PASSWORD_RATE_PER_MIN: '1000',
    });
    h.app.useLogger({
      log: record('log'),
      error: record('error'),
      warn: record('warn'),
      debug: record('debug'),
      verbose: record('verbose'),
      fatal: record('fatal'),
    });
    // the capture is live: a Logger created like the services' own lands in it
    new Logger('TD-4 e2e').log('log capture is live');
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(
      admin,
      USERS.map((u) => ({ username: u, role: 'operator', password: PW[u]! })),
    );
  });
  afterAll(async () => {
    process.stdout.write = writes.out;
    process.stderr.write = writes.err;
    await h?.close();
  });

  const login = (u: string, p = PW[u]!) => {
    SENT.add(p);
    return h.call(undefined, 'POST', '/api/v1/auth/login', { username: u, password: p });
  };
  const token = async (u: string) => (await login(u)).body.accessToken as string;
  const createKey = (
    tok: string,
    body: Record<string, unknown>,
    headers?: Record<string, string>,
  ) => {
    if (typeof body['current'] === 'string') SENT.add(body['current']);
    return h.call(headers ? undefined : tok, 'POST', '/api/v1/auth/api-keys', body, headers);
  };
  /** `app.inject` from a remote peer over plain HTTP (the API bound to a public address by mistake). */
  const remote = async (url: string, payload: Record<string, unknown>, authorization?: string) => {
    for (const k of ['password', 'current'])
      if (typeof payload[k] === 'string') SENT.add(payload[k]);
    const res = await h.app.inject({
      method: 'POST',
      url,
      remoteAddress: REMOTE,
      headers: authorization ? { authorization } : {},
      payload,
    });
    return { status: res.statusCode, body: res.json() as Record<string, unknown> };
  };
  const userRow = async (u: string) =>
    (
      await h.db.execute(
        sql`select id, failed_logins, locked_until, credential_gen, disabled from app_user where username = ${u}`,
      )
    ).rows[0] as {
      id: number;
      failed_logins: number;
      locked_until: Date | null;
      credential_gen: number;
      disabled: boolean;
    };
  const keyRows = async (u: string) =>
    Number(
      (
        await h.db.execute(
          sql`select count(*)::int as n from api_key k join app_user u on u.id = k.user_id where u.username = ${u}`,
        )
      ).rows[0]!['n'],
    );
  const works = async (key: string) =>
    (await h.call(undefined, 'GET', '/api/v1/auth/me', undefined, withKey(key))).status;
  const me = async (tok: string) => (await h.call(tok, 'GET', '/api/v1/auth/me')).status;
  const userIndex = async (u: string) =>
    (
      (await h.call(admin, 'GET', '/api/v1/config/management/users')).body as { username: string }[]
    ).findIndex((x) => x.username === u);
  const stage = async (u: string, patch: Record<string, unknown>) => {
    const r = await h.call(
      admin,
      'PATCH',
      `/api/v1/config/management/users/${await userIndex(u)}`,
      patch,
    );
    expect(r.status).toBe(200);
  };

  // ------------------------------------------------------------------------------------------------ (1)
  it('(1) login from a remote plain-HTTP peer → 403 tls-required, checked first: no failed login, lockout and rate limit untouched; audited without the password; loopback unchanged', async () => {
    // one real failure from loopback first, so "unchanged" is a visible 1, not a default 0 — TD-10b: a failed LOGIN
    // counts in the (user, client address) lockout in Valkey (lockout.ts), app_user.failed_logins stays 0
    const kv = h.app.get<Valkey>(VALKEY);
    const fails = async (client: string) => {
      const r = await userRow('tlsuser');
      return kv.get(`lkf:${r.id}:${r.credential_gen}:${client}`);
    };
    expect((await login('tlsuser', `wrong-${runSecret()}`)).status).toBe(401);
    expect(await fails('127.0.0.1')).toBe('1');
    const wrong = `wrong-${runSecret()}`;
    // more attempts than VRX_LOGIN_MAX_FAILURES, right and wrong passwords alike
    for (let i = 0; i < MAX_FAILURES + 2; i++) {
      const r = await remote('/api/v1/auth/login', {
        username: 'tlsuser',
        password: i % 2 === 0 ? wrong : PW['tlsuser']!,
      });
      expect(r.status).toBe(403);
      expect(r.body).toMatchObject({ type: TLS, status: 403, title: 'TLS required' });
    }
    const after = await userRow('tlsuser');
    console.log(
      `(1) after ${MAX_FAILURES + 2} remote plain-HTTP logins: failed_logins ${after.failed_logins}, locked_until ${after.locked_until}`,
    );
    expect({
      failed: after.failed_logins,
      locked: after.locked_until,
      loopbackFails: await fails('127.0.0.1'),
      remoteFails: await fails(REMOTE),
    }).toEqual({ failed: 0, locked: null, loopbackFails: '1', remoteFails: null });
    // the per-IP rate limiter never counted the remote peer
    expect(await kv.exists(`rl:login:${REMOTE}:${Math.floor(Date.now() / 60_000)}`)).toBe(0);
    const rows = await h.db.execute(
      sql`select user_id, username, source_ip, resource, after, result, status from audit_log
          where action = 'auth.login' and after->>'reason' = 'tls-required' order by id`,
    );
    expect(rows.rows).toHaveLength(MAX_FAILURES + 2);
    expect(rows.rows[0]).toEqual({
      user_id: null,
      username: 'tlsuser',
      source_ip: REMOTE,
      resource: 'tlsuser',
      after: { reason: 'tls-required' },
      result: 'failure',
      status: 403,
    });
    // loopback: exactly as before (the success clears the one real failure)
    expect((await login('tlsuser')).status).toBe(200);
    expect(await fails('127.0.0.1')).toBeNull();
  });

  // ------------------------------------------------------------------------------------------------ (2)
  describe('(2) step-up: creating an API key from a JWT session needs the current password', () => {
    it('JWT: no current → 400 /current; remote plain HTTP → 403 tls-required; right current → 201, the key works; audited via: jwt', async () => {
      const tok = await token('stepper');
      const missing = await createKey(tok, { name: 'no-current' });
      expect(missing.status).toBe(400);
      expect(missing.body.errors).toEqual([
        { pointer: '/current', message: 'required when the caller is a login (JWT) session' },
      ]);
      // plain HTTP from a remote peer: refused before anything else (without and with the password)
      for (const body of [{ name: 'remote' }, { name: 'remote', current: PW['stepper']! }]) {
        const r = await remote('/api/v1/auth/api-keys', body, `Bearer ${tok}`);
        expect(r.status).toBe(403);
        expect(r.body).toMatchObject({ type: TLS });
      }
      expect(await keyRows('stepper')).toBe(0);
      expect((await userRow('stepper')).failed_logins).toBe(0);
      const k = await createKey(tok, { name: 'stepped-up', current: PW['stepper']! });
      expect(k.status).toBe(201);
      expect(k.body).toMatchObject({ name: 'stepped-up', role: 'operator' });
      expect(k.body).not.toHaveProperty('current');
      expect(await works(k.body.key)).toBe(200);
      const audit = await h.db.execute(
        sql`select username, resource, after, result, status from audit_log
            where action = 'POST /api/v1/auth/api-keys' and username = 'stepper' order by id`,
      );
      // review M1: a failed creation records the problem slug as its reason (nothing else, never the password)
      expect(audit.rows.map((r) => [r['status'], r['after']])).toEqual([
        [400, { name: 'no-current', via: 'jwt', reason: 'bad-request' }],
        [403, { name: 'remote', via: 'jwt', reason: 'tls-required' }],
        [403, { name: 'remote', via: 'jwt', reason: 'tls-required' }],
        [201, { name: 'stepped-up', role: 'operator', via: 'jwt' }],
      ]);
      expect(audit.rows[3]).toMatchObject({ resource: `api-key/${k.body.id}`, result: 'success' });
    });

    it(`wrong current → 403 and failed_logins +1; the ${MAX_FAILURES}th locks → 403 locked; locked: 403 locked even with the right password, no api_key row`, async () => {
      const tok = await token('locker');
      const seen: { status: number; type: unknown; failed: number; locked: boolean }[] = [];
      let lockingRaw = '';
      for (let i = 0; i < MAX_FAILURES; i++) {
        const r = await createKey(tok, { name: `guess-${i}`, current: `wrong-${runSecret()}` });
        lockingRaw = r.raw;
        const row = await userRow('locker');
        seen.push({
          status: r.status,
          type: r.body.type,
          failed: row.failed_logins,
          locked: row.locked_until !== null,
        });
      }
      console.log(`(2) wrong current ×${MAX_FAILURES}: ${JSON.stringify(seen)}`);
      expect(seen).toEqual([
        { status: 403, type: 'https://vrx.dev/problems/forbidden', failed: 1, locked: false },
        { status: 403, type: 'https://vrx.dev/problems/forbidden', failed: 2, locked: false },
        // registerFailure: the failure that reaches the maximum locks and resets the counter
        { status: 403, type: 'https://vrx.dev/problems/locked', failed: 0, locked: true },
      ]);
      const right = await createKey(tok, { name: 'while-locked', current: PW['locker']! });
      expect(right.status).toBe(403);
      expect(right.body).toMatchObject({ type: 'https://vrx.dev/problems/locked' });
      expect(await keyRows('locker')).toBe(0);
      // review H1: the guess that caused the lock (argon2 ran) and a guess refused while locked (no argon2): ONE body
      expect(lockingRaw).toBe(right.raw);
      const audit = await h.db.execute(
        sql`select status, after->>'reason' as reason from audit_log
            where action = 'POST /api/v1/auth/api-keys' and username = 'locker' order by id`,
      );
      expect(audit.rows.map((r) => `${r['status']} ${r['reason']}`)).toEqual([
        '403 forbidden',
        '403 forbidden',
        '403 locked',
        '403 locked',
      ]);
      // and login is locked too: the step-up and login share one lockout
      expect((await login('locker')).status).toBe(401);
    });

    it('API-key caller: no current needed → 201, audited via: apikey; a current it sends is refused (400, D-124), never checked', async () => {
      const tok = await token('keyuser');
      const k0 = await createKey(tok, { name: 'automation', current: PW['keyuser']! });
      expect(k0.status).toBe(201);
      const k1 = await createKey('', { name: 'from-a-key' }, withKey(k0.body.key));
      expect(k1.status).toBe(201);
      expect(await works(k1.body.key)).toBe(200);
      const audit = await h.db.execute(
        sql`select after from audit_log where action = 'POST /api/v1/auth/api-keys'
            and resource = ${`api-key/${k1.body.id}`}`,
      );
      expect(audit.rows).toEqual([
        { after: { name: 'from-a-key', role: 'operator', via: 'apikey' } },
      ]);
      // a password sent by an API-key caller is still a password: over plain HTTP from a remote peer → tls-required
      const r = await remote(
        '/api/v1/auth/api-keys',
        { name: 'k-remote', current: PW['keyuser']! },
        `ApiKey ${k0.body.key}`,
      );
      expect(r.body).toMatchObject({ type: TLS });
      // D-124 (review L1): over TLS it is refused, never checked — a role-capped key must not test its owner's password.
      // The right and a wrong password get the same 400; nothing counts toward the lockout, no key is created.
      const answers: string[] = [];
      for (const current of [`wrong-${runSecret()}`, PW['keyuser']!]) {
        const a = await createKey('', { name: 'k-current', current }, withKey(k0.body.key));
        expect(a.status).toBe(400);
        expect(a.body).toMatchObject({
          type: 'https://vrx.dev/problems/current-not-allowed-with-api-key',
          errors: [{ pointer: '/current', message: 'not allowed with Authorization: ApiKey' }],
        });
        answers.push(a.raw);
      }
      expect(answers[0]).toBe(answers[1]);
      expect((await userRow('keyuser')).failed_logins).toBe(0);
      expect(await keyRows('keyuser')).toBe(2);
      const refused = await h.db.execute(
        sql`select status, after from audit_log where action = 'POST /api/v1/auth/api-keys'
            and username = 'keyuser' and result = 'failure' order by id`,
      );
      expect(refused.rows.map((x) => [x['status'], x['after']])).toEqual([
        [403, { name: 'k-remote', via: 'apikey', reason: 'tls-required' }],
        [400, { name: 'k-current', via: 'apikey', reason: 'current-not-allowed-with-api-key' }],
        [400, { name: 'k-current', via: 'apikey', reason: 'current-not-allowed-with-api-key' }],
      ]);
    });

    it('a JWT whose generation is stale is refused before argon2 (401) and never feeds the lockout', async () => {
      const tok = await token('stale4');
      const { id } = await userRow('stale4');
      // the generation moved without this process revoking anything (e.g. after an API restart that lost Valkey)
      await h.db.execute(
        sql`update app_user set credential_gen = credential_gen + 1 where id = ${id}`,
      );
      expect(await me(tok)).toBe(200); // the access token itself is still accepted …
      for (const current of [`wrong-${runSecret()}`, PW['stale4']!]) {
        const r = await createKey(tok, { name: 'stale', current });
        expect(r.status).toBe(401); // … but it mints nothing, whatever the password
      }
      expect((await userRow('stale4')).failed_logins).toBe(0);
      expect(await keyRows('stale4')).toBe(0);
    });

    /** Bounded poll until a backend waits on a lock that backend `pid` holds. */
    async function blockedBy(pid: number, ms: number) {
      const until = performance.now() + ms;
      for (;;) {
        const r = await h.db.execute(
          sql`select count(*)::int as n from pg_stat_activity
              where wait_event_type = 'Lock' and ${pid}::int = any(pg_blocking_pids(pid))`,
        );
        if (Number(r.rows[0]!['n']) > 0) return;
        if (performance.now() > until)
          throw new Error(
            `no backend waited on the helper transaction (pid ${pid}) within ${ms} ms`,
          );
        await sleep(10);
      }
    }

    it('the checked password must belong to the committed generation: a change committing between the check and the insert → 401, no row (even for the kept session)', async () => {
      const l = await login('racer4');
      const tok = l.body.accessToken as string;
      const sid = cookieOf(l.headers).split('=')[1]!.split('.')[0]!;
      const { id, credential_gen: gen0 } = await userRow('racer4');
      const tokens = h.app.get(TokensService);
      let res: { status: number } | undefined;
      let pending: Promise<unknown> | undefined;
      // a helper transaction moves the generation and holds the row: the step-up reads the committed (old) row and
      // checks the password against it, then its key transaction waits on FOR SHARE. Before the helper commits, this
      // session is made the one a self-service change keeps (the worst case: the TD-2 V1 re-check alone would pass).
      await h.db.transaction(async (tx) => {
        await tx.execute(sql`set local lock_timeout = '10s'`);
        const helper = Number(
          (await tx.execute(sql`select pg_backend_pid() as pid`)).rows[0]!['pid'],
        );
        await tx.execute(
          sql`update app_user set credential_gen = credential_gen + 1 where id = ${id}`,
        );
        pending = createKey(tok, { name: 'raced', current: PW['racer4']! }).then((r) => (res = r));
        await blockedBy(helper, 15_000);
        await tokens.revokeUser(id, gen0 + 1, sid);
      });
      await within(pending!, 15_000, 'raced key creation');
      console.log(`(2) key creation racing a generation change: ${res?.status}`);
      expect(res?.status).toBe(401);
      expect(await keyRows('racer4')).toBe(0);
      // the kept session mints again once the password check and the transaction see the same generation
      const k = await createKey(tok, { name: 'after-race', current: PW['racer4']! });
      expect(k.status).toBe(201);
    });
  });

  // ------------------------------------------------------------------------------------------------ (3)
  describe('(3) disabling an account through the config API ends its sessions at once', () => {
    it('commit disabled: true → access token, refresh, WebSocket and API key end now; one audit row; re-enable → login 200, the old token stays dead, the key works again', async () => {
      await h.app.listen({ port: h.env.VRX_HTTP_PORT, host: '127.0.0.1' });
      const l = await login('dis');
      const tok = l.body.accessToken as string;
      const cookie = cookieOf(l.headers);
      const k = await createKey(tok, { name: 'dis-key', current: PW['dis']! });
      expect(k.status).toBe(201);
      const ws = new WebSocket(`ws://127.0.0.1:${h.env.VRX_HTTP_PORT}/api/v1/stream`, [
        'vrx.v1',
        `bearer.${tok}`,
      ]);
      await within(
        new Promise<void>((resolve, reject) => {
          ws.addEventListener('open', () => resolve());
          ws.addEventListener('error', () => reject(new Error('websocket error')));
        }),
        10_000,
        'websocket open',
      );
      const closed = new Promise<number>((resolve) =>
        ws.addEventListener('close', (e) => resolve(e.code)),
      );
      const gen0 = (await userRow('dis')).credential_gen;
      await stage('dis', { disabled: true });
      // staged only: nothing happened yet
      expect(await me(tok)).toBe(200);
      const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=disable%20dis');
      expect(c.status).toBe(200);
      const after = {
        me: await me(tok),
        refresh: (await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, { cookie }))
          .status,
        apiKey: await works(k.body.key),
        // not thrown: a socket that stays open shows up in the comparison next to the other fields
        ws: await within(closed, 10_000, 'websocket close').catch(() => 'still open'),
        genBump: (await userRow('dis')).credential_gen - gen0,
        keyRows: await keyRows('dis'),
        login: (await login('dis')).status,
      };
      console.log(`(3) after the disable commit: ${JSON.stringify(after)}`);
      expect(after).toEqual({
        me: 401,
        refresh: 401,
        apiKey: 401,
        ws: 4403,
        genBump: 1,
        keyRows: 1, // not deleted: refused while disabled (D-100 (3))
        login: 401,
      });
      const audit = await h.db.execute(
        sql`select username, action, resource, after, result from audit_log
            where resource = 'user/dis' and action like 'config.%' order by id`,
      );
      expect(audit.rows).toEqual([
        {
          username: 'admin',
          action: 'config.user-disabled',
          resource: 'user/dis',
          after: {
            disabled: true,
            via: 'config',
            revision: c.body.revision.id,
            txnId: c.body.txnId,
          },
          result: 'success',
        },
      ]);
      // re-enable: no bump, login works, the old access token stays dead, the API key works again
      await stage('dis', { disabled: false });
      expect((await h.call(admin, 'POST', '/api/v1/config/commit')).status).toBe(200);
      const back = {
        genBump: (await userRow('dis')).credential_gen - gen0,
        login: (await login('dis')).status,
        oldToken: await me(tok),
        apiKey: await works(k.body.key),
        auditRows: Number(
          (
            await h.db.execute(
              sql`select count(*)::int as n from audit_log where resource = 'user/dis' and action like 'config.%'`,
            )
          ).rows[0]!['n'],
        ),
      };
      console.log(`(3) after the re-enable commit: ${JSON.stringify(back)}`);
      expect(back).toEqual({ genBump: 1, login: 200, oldToken: 401, apiKey: 200, auditRows: 1 });
    });

    it('confirmed commit: the user stays alive while the commit is pending and is cut off at confirm', async () => {
      const tok = await token('disc');
      const k = await createKey(tok, { name: 'disc-key', current: PW['disc']! });
      await stage('disc', { disabled: true });
      const c = await h.call(admin, 'POST', '/api/v1/config/commit?confirm=30');
      expect(c.body.status).toBe('pending');
      const pendingState = { me: await me(tok), apiKey: await works(k.body.key) };
      expect((await h.call(admin, 'POST', '/api/v1/config/commit/confirm')).status).toBe(200);
      const confirmed = { me: await me(tok), apiKey: await works(k.body.key) };
      console.log(
        `(3) confirmed commit: pending ${JSON.stringify(pendingState)} → confirmed ${JSON.stringify(confirmed)}`,
      );
      expect(pendingState).toEqual({ me: 200, apiKey: 200 });
      expect(confirmed).toEqual({ me: 401, apiKey: 401 });
      const rows = await h.db.execute(
        sql`select count(*)::int as n from audit_log where action = 'config.user-disabled' and resource = 'user/disc'`,
      );
      expect(rows.rows[0]).toEqual({ n: 1 });
    });

    it('hash change + disable in one commit: ONE bump, two audit rows (the D-102 row as before); a user created disabled is no disable', async () => {
      const tok = await token('dual');
      const k = await createKey(tok, { name: 'dual-key', current: PW['dual']! });
      const gen0 = (await userRow('dual')).credential_gen;
      const next = runSecret();
      await stage('dual', { passwordHash: await hashPassword(next), disabled: true });
      // plus a brand-new user that is created disabled, in the same commit
      const users = (await h.call(admin, 'GET', '/api/v1/config/management/users'))
        .body as unknown[];
      await h.call(admin, 'PATCH', `/api/v1/config/management/users/${users.length}`, {
        username: 'born4',
        role: 'readonly',
        passwordHash: await hashPassword(runSecret()),
        disabled: true,
      });
      const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=reset%2Bdisable');
      expect(c.status).toBe(200);
      PW['dual'] = next;
      expect((await userRow('dual')).credential_gen - gen0).toBe(1);
      expect((await userRow('born4')).credential_gen).toBe(0);
      expect(await me(tok)).toBe(401);
      expect(await keyRows('dual')).toBe(0); // the hash change is an admin reset (D-102): keys revoked
      const audit = await h.db.execute(
        sql`select action, after from audit_log where resource in ('user/dual', 'user/born4')
            and action like 'config.%' order by id`,
      );
      expect(audit.rows).toEqual([
        {
          action: 'config.password-reset',
          after: {
            passwordSet: true,
            self: false,
            via: 'config',
            revision: c.body.revision.id,
            txnId: c.body.txnId,
            apiKeysRevoked: [{ id: k.body.id, name: 'dual-key' }],
          },
        },
        {
          action: 'config.user-disabled',
          after: {
            disabled: true,
            via: 'config',
            revision: c.body.revision.id,
            txnId: c.body.txnId,
          },
        },
      ]);
    });
  });

  // ------------------------------------------------------------------------------------------------ secrets
  it('no password sent by this file appears in audit_log, system_event or the API log', async () => {
    SENT.add(h.adminPassword);
    const found: string[] = [];
    for (const p of SENT) {
      const a = await h.db.execute(
        sql`select count(*)::int as n from audit_log t where strpos(to_jsonb(t)::text, ${p}) > 0`,
      );
      const e = await h.db.execute(
        sql`select count(*)::int as n from system_event t where strpos(to_jsonb(t)::text, ${p}) > 0`,
      );
      if (Number(a.rows[0]!['n']) > 0) found.push('audit_log');
      if (Number(e.rows[0]!['n']) > 0) found.push('system_event');
      if (logged.some((l) => l.includes(p))) found.push('API log');
    }
    expect(logged.some((l) => l.includes('log capture is live'))).toBe(true);
    const counts = await h.db.execute(
      sql`select (select count(*)::int from audit_log) as audit, (select count(*)::int from system_event) as events`,
    );
    console.log(
      `secrets: ${SENT.size} passwords checked against ${JSON.stringify(counts.rows[0])} rows and ${logged.length} API log lines → found in: ${JSON.stringify(found)}`,
    );
    expect(found).toEqual([]);
  });
});
