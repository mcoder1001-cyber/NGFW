import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { hashPassword } from '../../src/auth/password.js';
import { VALKEY, type Valkey } from '../../src/infra/valkey.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';
import { auditReasons, via } from '../support/proxy.js';

/**
 * TD-10b review 2.3a on the host PostgreSQL + Valkey: the login lockout is keyed by (user, client address) behind the
 * trusted proxy; the last admin who could still log in from an address is throttled, never locked; the account-wide
 * lock of a session-held password check never hits the last admin; a root-only break-glass clears every lock.
 * VRX_LOGIN_MAX_FAILURES = 3 to keep it short. Client addresses are TEST-NET (RFC 5737).
 */
const MAX = 3;
const PW: Record<string, string> = {
  victim: runSecret(),
  stepper: runSecret(),
  admin2: runSecret(),
};
const A = '198.51.100.1';
const B = '198.51.100.2';

describe('TD-10b 2.3a lockout per (user, client address), last admin throttled, break-glass', () => {
  let h: Harness;
  let admin: string;
  let kv: Valkey;

  const login = (username: string, password: string, client: string) =>
    via(h, 'POST', '/api/v1/auth/login', { username, password }, { client });
  const row = async (u: string) =>
    (
      await h.db.execute(
        sql`select id, credential_gen as gen, locked_until from app_user where username = ${u}`,
      )
    ).rows[0] as { id: number; gen: number; locked_until: Date | null };
  const lockAt = async (u: string, client: string) => {
    const r = await row(u);
    return kv.get(`lk:${r.id}:${r.gen}:${client}`);
  };

  beforeAll(async () => {
    h = await startHarness({
      VRX_LOGIN_MAX_FAILURES: String(MAX),
      VRX_LOGIN_RATE_PER_MIN: '1000',
      VRX_PASSWORD_RATE_PER_MIN: '1000',
    });
    PW['admin'] = h.adminPassword;
    kv = h.app.get<Valkey>(VALKEY);
    admin = await h.login('admin', h.adminPassword);
    // admin2 starts DISABLED: until it is enabled, the bootstrap admin is the only (last) admin
    const users = [
      { username: 'admin', role: 'admin' },
      { username: 'victim', role: 'operator', passwordHash: await hashPassword(PW['victim']!) },
      { username: 'stepper', role: 'operator', passwordHash: await hashPassword(PW['stepper']!) },
      {
        username: 'admin2',
        role: 'admin',
        disabled: true,
        passwordHash: await hashPassword(PW['admin2']!),
      },
    ];
    expect((await h.call(admin, 'PUT', '/api/v1/config/management/users', users)).status).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=td10b')).status).toBe(200);
  });
  afterAll(async () => h?.close());

  it('a wrong-password burst from address A locks the user for A only — the owner logs in from B; nothing account-wide', async () => {
    for (let i = 0; i < MAX; i++) expect((await login('victim', `wrong-${i}`, A)).status).toBe(401);
    const fromA = await login('victim', PW['victim']!, A);
    expect(fromA.status).toBe(401);
    expect(fromA.body.detail).toBe('invalid credentials'); // no oracle: the same answer as a wrong password
    const fromB = await login('victim', PW['victim']!, B);
    console.log(
      `2.3a victim after ${MAX} failures from A: from A ${fromA.status}, from B ${fromB.status}`,
    );
    expect(fromB.status).toBe(200);
    expect(await lockAt('victim', A)).toBe('lock');
    expect((await row('victim')).locked_until).toBeNull();
    expect(await auditReasons(h, 'auth.login', 'victim')).toEqual([
      'bad-password',
      'bad-password',
      'bad-password-locked',
      'locked',
      null, // the success from B
    ]);
    const src = await h.db.execute(
      sql`select distinct source_ip from audit_log where action = 'auth.login' and username = 'victim' order by 1`,
    );
    expect(src.rows.map((r) => r['source_ip'])).toEqual([A, B]);
  });

  it('the LAST admin is throttled (1 s, 2 s …), never locked: after the delay the right password works from the same address', async () => {
    const C = '198.51.100.3';
    for (let i = 0; i < MAX; i++) expect((await login('admin', `wrong-${i}`, C)).status).toBe(401);
    expect(await lockAt('admin', C)).toBe('throttle');
    const during = await login('admin', PW['admin']!, C);
    expect(during.status).toBe(401); // inside the 1 s window even the right password is refused (no oracle)
    expect((await login('admin', PW['admin']!, '198.51.100.4')).status).toBe(200); // other addresses unaffected
    await new Promise((r) => setTimeout(r, 1_200));
    const after = await login('admin', PW['admin']!, C);
    console.log(
      `2.3a last admin from C: inside the throttle ${during.status}, after 1.2 s ${after.status}`,
    );
    expect(after.status).toBe(200);
    const reasons = await auditReasons(h, 'auth.login', 'admin');
    // …, bad-password ×2, bad-password-throttled, throttled, (success elsewhere), (success from C)
    expect(reasons.slice(-6)).toEqual([
      'bad-password',
      'bad-password',
      'bad-password-throttled',
      'throttled',
      null,
      null,
    ]);
    expect(reasons).toContain('bad-password-throttled');
    expect(reasons).toContain('throttled');
    expect(reasons).not.toContain('bad-password-locked');
    const ev = await h.db.execute(
      sql`select severity, subsystem, data from system_event where code = 'LOGIN_THROTTLED'`,
    );
    expect(ev.rows[0]).toMatchObject({
      severity: 'warning',
      subsystem: 'auth',
      data: { user: 'admin', client: C },
    });
  });

  it('a session-held password check never locks the last admin account-wide; an operator it still locks (TD-4 unchanged)', async () => {
    const key = (tok: string, current: string) =>
      h.call(tok, 'POST', '/api/v1/auth/api-keys', { name: `k-${runSecret()}`, current });
    const seen: string[] = [];
    for (let i = 0; i < MAX; i++) {
      const r = await key(admin, `wrong-${i}`);
      seen.push(`${r.status} ${String(r.body.type).split('/').pop()}`);
    }
    const right = await key(admin, PW['admin']!);
    console.log(
      `2.3a last admin, ${MAX} wrong step-ups: ${seen.join(', ')}; then the right one: ${right.status}`,
    );
    expect(seen).toEqual(Array(MAX).fill('403 forbidden'));
    expect(right.status).toBe(201);
    expect((await row('admin')).locked_until).toBeNull();

    const op = await h.login('stepper', PW['stepper']!);
    const opSeen: string[] = [];
    for (let i = 0; i < MAX; i++) {
      const r = await key(op, `wrong-${i}`);
      opSeen.push(`${r.status} ${String(r.body.type).split('/').pop()}`);
    }
    expect(opSeen).toEqual(['403 forbidden', '403 forbidden', '403 locked']);
    expect((await row('stepper')).locked_until).not.toBeNull();
    // account-wide: login refused from every address
    expect((await login('stepper', PW['stepper']!, B)).status).toBe(401);
  });

  it('two enabled admins: one may be locked at an address, the other is then the last one there and only throttled — also under parallel bursts', async () => {
    const users = (await h.call(admin, 'GET', '/api/v1/config/management/users')).body as {
      username: string;
    }[];
    const i = users.findIndex((u) => u.username === 'admin2');
    expect(
      (await h.call(admin, 'PATCH', `/api/v1/config/management/users/${i}`, { disabled: false }))
        .status,
    ).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=enable')).status).toBe(200);

    const D = '198.51.100.5';
    for (let n = 0; n < MAX; n++) await login('admin2', `wrong-${n}`, D);
    expect(await lockAt('admin2', D)).toBe('lock'); // the bootstrap admin can still log in from D
    for (let n = 0; n < MAX; n++) await login('admin', `wrong-${n}`, D);
    expect(await lockAt('admin', D)).toBe('throttle'); // … and now is the last admin who can
    expect((await login('admin2', PW['admin2']!, B)).status).toBe(200);

    const F = '198.51.100.6';
    await Promise.all(
      Array.from({ length: 4 * MAX }, (_, n) =>
        login(n % 2 === 0 ? 'admin' : 'admin2', `burst-${n}`, F),
      ),
    );
    const states = [await lockAt('admin', F), await lockAt('admin2', F)];
    console.log(`2.3a parallel bursts from F on both admins → ${JSON.stringify(states)}`);
    expect(states.filter((s) => s === 'lock').length).toBeLessThanOrEqual(1);
  });

  it('break-glass (root-only vrx-authctl): lists and clears every lock of a user — per-address and account-wide — audited', async () => {
    const { listLocks, unlockUser } = await import('../../src/auth/break-glass.js');
    const { main } = await import('../../src/auth/break-glass-cli.js');
    const { AuditService } = await import('../../src/audit/audit.service.js');
    const { SystemEventsService } = await import('../../src/audit/system-events.service.js');
    const deps = {
      db: h.db,
      kv,
      prefix: h.env.VRX_VALKEY_PREFIX,
      audit: h.app.get(AuditService),
      events: h.app.get(SystemEventsService),
    };
    const locks = await listLocks(deps);
    expect(locks).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          username: 'victim',
          scope: 'address',
          client: A,
          state: 'locked',
        }),
        expect.objectContaining({ username: 'stepper', scope: 'account', state: 'locked' }),
      ]),
    );
    // the CLI end to end (as root; the settings the API runs with — never printed)
    const out: string[] = [];
    const err: string[] = [];
    const code = await main(['unlock', 'victim'], {
      getuid: () => 0,
      env: {
        VRX_DATABASE_URL: h.env.VRX_DATABASE_URL,
        VRX_VALKEY_DB: String(h.env.VRX_VALKEY_DB),
        VRX_VALKEY_PREFIX: h.env.VRX_VALKEY_PREFIX,
      },
      out: (l) => out.push(l),
      err: (l) => err.push(l),
    });
    console.log(`break-glass CLI: exit ${code}; ${out.join(' | ')}${err.join(' | ')}`);
    expect(code).toBe(0);
    expect(out[0]).toMatch(
      /^unlocked 'victim' \(id \d+\): account-wide lock was not set, [1-9]\d* per-address/,
    );
    expect((await login('victim', PW['victim']!, A)).status).toBe(200);

    const r = await unlockUser(deps, 'stepper');
    expect(r).toMatchObject({ username: 'stepper', accountLockCleared: true });
    expect((await login('stepper', PW['stepper']!, B)).status).toBe(200);
    await expect(unlockUser(deps, 'nobody')).rejects.toThrow("no user named 'nobody'");

    const rows = (
      await h.db.execute(
        sql`select user_id, username, resource, after, result from audit_log where action = 'auth.break-glass-unlock' order by id`,
      )
    ).rows;
    expect(rows).toEqual([
      expect.objectContaining({
        user_id: null,
        username: 'root (break-glass)',
        resource: 'user/victim',
        result: 'success',
      }),
      expect.objectContaining({
        resource: 'user/stepper',
        after: { accountLockCleared: true, addressKeysCleared: expect.any(Number) },
      }),
    ]);
    const ev = await h.db.execute(
      sql`select count(*)::int as n from system_event where code = 'BREAK_GLASS_UNLOCK'`,
    );
    expect(ev.rows[0]!['n']).toBe(2);
  });
});
