import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { hashPassword } from '../../src/auth/password.js';
import { createApp } from '../../src/app.js';
import { TokensService } from '../../src/auth/tokens.service.js';
import { CommitService } from '../../src/commit/commit.service.js';
import { guardLongTests } from '../support/bounded.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

/**
 * TD-2 review fixes (docs/status/tasks/TD-2-review.md, D-097), each with the reviewer's scenario:
 *  H1 password hashes are never restored by reconcile / confirm / confirm-timeout revert / rollback / import
 *  H2 a refresh chain racing a password reset never survives it (≥ 50 runs, parallel refreshes + a racing login)
 *  M1 an admin reset revokes the target's API keys (keepApiKeys opt-out; self-service keeps them)
 *  M2 a wrong `current` counts toward the target's lockout; per-target rate limit
 *  L3 access-token revocations survive an API restart · L4 a deleted key's candidate lock is released at once
 *  + the WebSocket of the target closes on a reset.
 * Fake agent with a short Apply deadline (800 ms) so a slow Apply becomes a lost answer (M3 path).
 */
const PW: Record<string, string> = {};
const USERS = ['victim', 'staged', 'racer', 'keyop', 'selfie', 'wsuser'];
for (const u of USERS) PW[u] = runSecret();

function cookieOf(headers: Record<string, unknown>): string {
  const raw = [headers['set-cookie']]
    .flat()
    .find((c) => String(c).startsWith('vrx_refresh=')) as string;
  return raw.split(';')[0]!;
}
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

describe('TD-2 review fixes e2e', () => {
  let h: Harness;
  let admin: string;
  const slot = () => Number(/(\d+)$/.exec(h.prefix)?.[1] ?? '7');
  // TD-2-verify2 T1 (optional item): H2 runs through `guarded`; afterEach stops and awaits it
  const guarded = guardLongTests();

  beforeAll(async () => {
    h = await startHarness({
      VRX_AGENT_TIMEOUT_MS: '800',
      VRX_PASSWORD_RATE_PER_MIN: '1000',
      VRX_LOGIN_RATE_PER_MIN: '5000',
      VRX_LOGIN_MAX_FAILURES: '3',
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
  const reset = (u: string, body: Record<string, unknown> = {}) => {
    const next = runSecret();
    return h
      .call(admin, 'POST', `/api/v1/users/${u}/password`, { password: next, ...body })
      .then((r) => {
        if (r.status === 200) PW[u] = next;
        return { ...r, next };
      });
  };
  const userIndex = async (u: string) =>
    (
      (await h.call(admin, 'GET', '/api/v1/config/management/users')).body as { username: string }[]
    ).findIndex((x) => x.username === u);

  // ------------------------------------------------------------------------------------------------ H1
  describe('H1 — a snapshot never brings a password back', () => {
    it('reconcile after a lost Apply answer: the set password stays (hydrated AND staged hashes)', async () => {
      const oldVictim = PW['victim']!;
      const stagedOld = runSecret();
      const si = await userIndex('staged');
      // the in-flight document stages a hash for `staged` and carries victim's (hydrated) one
      expect(
        (
          await h.call(admin, 'PATCH', `/api/v1/config/management/users/${si}`, {
            passwordHash: await hashPassword(stagedOld),
          })
        ).status,
      ).toBe(200);
      await h.call(admin, 'PUT', `/api/v1/config/interfaces/loop${slot()}31`, {
        ipv4: [`10.${slot()}.131.1/24`],
      });
      h.fake.applyDelayMs = 2500;
      const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=lost');
      expect(c.status).toBe(504);
      expect(c.body.type).toContain('running-unknown');
      h.fake.applyDelayMs = 0;

      // during the outage: both passwords are set
      expect((await reset('victim')).status).toBe(200);
      expect((await reset('staged')).status).toBe(200);
      expect((await login('victim')).status).toBe(200);

      await sleep(2000); // the delayed Apply lands on the agent
      const sync = await h.app.get(CommitService).reconcile();
      expect(sync.state).toBe('in-sync');
      expect(sync.reason).toContain('revision saved');

      expect((await login('victim', oldVictim)).status).toBe(401);
      expect((await login('victim')).status).toBe(200);
      expect((await login('staged', stagedOld)).status).toBe(401);
      expect((await login('staged')).status).toBe(200);
    });

    it('confirmed commit: confirm, and the confirm-timeout revert, keep the password set meanwhile', async () => {
      const si = await userIndex('staged');
      for (const mode of ['confirm', 'revert'] as const) {
        const stagedOld = runSecret();
        await h.call(admin, 'PATCH', `/api/v1/config/management/users/${si}`, {
          passwordHash: await hashPassword(stagedOld),
        });
        const c = await h.call(
          admin,
          'POST',
          `/api/v1/config/commit?confirm=${mode === 'confirm' ? 30 : 2}`,
        );
        expect(c.status).toBe(200);
        expect(c.body.status).toBe('pending');
        expect((await reset('staged')).status).toBe(200);
        expect((await reset('victim')).status).toBe(200);
        if (mode === 'confirm') {
          expect((await h.call(admin, 'POST', '/api/v1/config/commit/confirm')).status).toBe(200);
        } else {
          for (let i = 0; i < 40; i++) {
            if ((await h.call(admin, 'GET', '/api/v1/config/commit/pending')).body.pending === null)
              break;
            await sleep(200);
          }
          await h.call(admin, 'POST', '/api/v1/config/discard');
        }
        expect((await login('staged', stagedOld)).status, mode).toBe(401);
        expect((await login('staged')).status, mode).toBe(200);
        expect((await login('victim')).status, mode).toBe(200);
      }
    });

    it('rollback to a revision from before the set keeps the current password', async () => {
      const revs = await h.call(admin, 'GET', '/api/v1/config/revisions?limit=500');
      const first = (revs.body.items as { id: number }[]).at(-1)!.id;
      const old = PW['victim']!;
      expect((await reset('victim')).status).toBe(200);
      const rb = await h.call(admin, 'POST', `/api/v1/config/rollback/${first}`);
      expect(rb.status).toBe(200);
      expect((await login('victim', old)).status).toBe(401);
      expect((await login('victim')).status).toBe(200);
    });

    it('import never brings hashes: they are listed as ignored, the current password stays', async () => {
      const old = runSecret();
      const exp = await h.call(admin, 'GET', '/api/v1/config/export');
      const doc = exp.body;
      const vi = (doc.management.users as { username: string }[]).findIndex(
        (u) => u.username === 'victim',
      );
      doc.management.users[vi].passwordHash = await hashPassword(old);
      const imp = await h.call(admin, 'POST', '/api/v1/config/import', doc);
      expect(imp.status).toBe(200);
      expect(imp.body.ignoredSecrets).toEqual([`/management/users/${vi}/passwordHash`]);
      expect((await h.call(admin, 'GET', '/api/v1/config/diff')).body.changes).toEqual([]);
      const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=import');
      expect(c.status).toBe(200);
      expect((await login('victim', old)).status).toBe(401);
      expect((await login('victim')).status).toBe(200);
    });
  });

  // ------------------------------------------------------------------------------------------------ H2
  // 13–20 s on a quiet host, over 30 s at load 12–16 (T1): an explicit budget, not the 30 s default
  it(
    'H2 — 60 runs: parallel refresh chains and a racing login during a reset → 0 survivors',
    guarded(async (signal) => {
      let survivors = 0;
      let raced = 0;
      for (let run = 0; run < 60; run++) {
        signal.throwIfAborted();
        const before = PW['racer']!;
        const sessions = await Promise.all([login('racer'), login('racer')]);
        const chains = sessions.map((s) => ({
          cookie: cookieOf(s.headers),
          token: s.body.accessToken as string,
          alive: true,
        }));
        let stop = false;
        const hammer = async (c: (typeof chains)[number]) => {
          while (!stop && !signal.aborted && c.alive) {
            const r = await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, {
              cookie: c.cookie,
            });
            if (r.status !== 200) c.alive = false;
            else {
              c.cookie = cookieOf(r.headers);
              c.token = r.body.accessToken;
            }
          }
        };
        const loops = chains.map(hammer);
        let racing: Awaited<ReturnType<typeof login>>;
        let done: Awaited<ReturnType<typeof reset>>;
        try {
          await sleep(run % 7);
          [racing, done] = await Promise.all([login('racer', before), reset('racer')]);
          await sleep(5);
        } finally {
          stop = true;
          await Promise.allSettled(loops);
        }
        await Promise.all(loops); // settled above: this only re-throws a loop's own error
        expect(done.status).toBe(200);
        const late =
          racing.status === 200
            ? [{ cookie: cookieOf(racing.headers), token: racing.body.accessToken as string }]
            : [];
        if (racing.status === 200) raced += 1;
        for (const c of [...chains, ...late]) {
          const me = await h.call(c.token, 'GET', '/api/v1/auth/me');
          const rf = await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, {
            cookie: c.cookie,
          });
          if (me.status === 200 || rf.status === 200) survivors += 1;
        }
      }
      console.log(
        `H2: 60 runs, ${60 * 2} hammered chains + ${raced} racing logins that got in → survivors ${survivors}`,
      );
      expect(survivors).toBe(0);
    }),
    180_000,
  );

  // ------------------------------------------------------------------------------------------------ M1 + L4
  describe('M1 — an admin reset revokes the target’s API keys (D-097)', () => {
    it('default: keys revoked, listed and audited; the key’s candidate lock is released', async () => {
      const op = (await login('keyop')).body.accessToken as string;
      const k1 = await h.call(op, 'POST', '/api/v1/auth/api-keys', { name: 'ci-1' });
      const k2 = await h.call(op, 'POST', '/api/v1/auth/api-keys', { name: 'ci-2' });
      const key = { authorization: `ApiKey ${k1.body.key}` };
      expect(
        (
          await h.call(
            undefined,
            'PATCH',
            `/api/v1/config/interfaces/loop${slot()}32`,
            { mtu: 1400 },
            key,
          )
        ).status,
      ).toBe(200);
      expect((await h.call(admin, 'GET', '/api/v1/config/lock')).body.ownerKeyId).toBe(k1.body.id);

      const r = await reset('keyop');
      expect(r.status).toBe(200);
      expect(r.body.self).toBe(false);
      expect(r.body.apiKeysRevoked.map((k: { name: string }) => k.name).sort()).toEqual([
        'ci-1',
        'ci-2',
      ]);
      expect((await h.call(undefined, 'GET', '/api/v1/auth/me', undefined, key)).status).toBe(401);
      // review L4: the lock of the revoked key is gone at once (candidate discarded, not handed to anyone)
      expect((await h.call(admin, 'GET', '/api/v1/config/lock')).body.locked).toBe(false);
      expect((await h.call(admin, 'GET', '/api/v1/config/diff')).body.changes).toEqual([]);
      const audit = await h.db.execute(
        sql`select after from audit_log where action = 'POST /api/v1/users/:name/password'
            and resource = 'user/keyop' order by id desc limit 1`,
      );
      expect((audit.rows[0]!['after'] as any).apiKeysRevoked).toHaveLength(2);
      void k2;
    });

    it('keepApiKeys: true keeps them (service users); a self-service change keeps them too', async () => {
      const op = (await login('keyop')).body.accessToken as string;
      const k = await h.call(op, 'POST', '/api/v1/auth/api-keys', { name: 'svc' });
      const key = { authorization: `ApiKey ${k.body.key}` };
      const r = await reset('keyop', { keepApiKeys: true });
      expect(r.body.apiKeysRevoked).toEqual([]);
      expect((await h.call(undefined, 'GET', '/api/v1/auth/me', undefined, key)).status).toBe(200);
      const op2 = (await login('keyop')).body.accessToken as string;
      const next = runSecret();
      const self = await h.call(op2, 'POST', '/api/v1/users/keyop/password', {
        password: next,
        current: PW['keyop'],
      });
      expect(self.status).toBe(200);
      expect(self.body).toEqual({ self: true, apiKeysRevoked: [], discardedCandidate: false });
      PW['keyop'] = next;
      expect((await h.call(undefined, 'GET', '/api/v1/auth/me', undefined, key)).status).toBe(200);
      expect((await h.call(op2, 'DELETE', `/api/v1/auth/api-keys/${k.body.id}`)).status).toBe(204);
    });

    it('L4: deleting a key releases the candidate lock it holds immediately', async () => {
      const k = await h.call(admin, 'POST', '/api/v1/auth/api-keys', { name: 'short-lived' });
      const key = { authorization: `ApiKey ${k.body.key}` };
      await h.call(
        undefined,
        'PATCH',
        `/api/v1/config/interfaces/loop${slot()}33`,
        { mtu: 1400 },
        key,
      );
      expect(
        (await h.call(admin, 'PATCH', '/api/v1/config/system', { hostname: 'x' })).status,
      ).toBe(409);
      expect((await h.call(admin, 'DELETE', `/api/v1/auth/api-keys/${k.body.id}`)).status).toBe(
        204,
      );
      expect((await h.call(admin, 'GET', '/api/v1/config/lock')).body.locked).toBe(false);
      expect((await h.call(admin, 'GET', '/api/v1/config/diff')).body.changes).toEqual([]);
    });
  });

  // ------------------------------------------------------------------------------------------------ M2
  it('M2 — wrong `current` counts toward the lockout; a locked account takes no more guesses', async () => {
    const tok = (await login('selfie')).body.accessToken as string;
    const guesses: number[] = [];
    for (let i = 0; i < 3; i++) {
      guesses.push(
        (
          await h.call(tok, 'POST', '/api/v1/users/selfie/password', {
            password: runSecret(),
            current: runSecret(),
          })
        ).status,
      );
    }
    expect(guesses).toEqual([403, 403, 403]);
    const row = await h.db.execute(
      sql`select locked_until > now() as locked from app_user where username = 'selfie'`,
    );
    expect(row.rows[0]).toEqual({ locked: true });
    // even the right current password is refused now, and so is a login
    const right = await h.call(tok, 'POST', '/api/v1/users/selfie/password', {
      password: runSecret(),
      current: PW['selfie'],
    });
    expect(right.status).toBe(403);
    expect(right.body.detail).toContain('locked');
    expect((await login('selfie')).status).toBe(401);
    // an admin reset unlocks
    expect((await reset('selfie')).status).toBe(200);
    expect((await login('selfie')).status).toBe(200);
  });

  // ------------------------------------------------------------------------------------------------ L3
  it('L3 — access-token revocations survive an API restart (Valkey, TTL = token lifetime)', async () => {
    const before = (await login('victim')).body.accessToken as string;
    expect((await reset('victim')).status).toBe(200);
    expect((await h.call(before, 'GET', '/api/v1/auth/me')).status).toBe(401);
    // a second API process with the same JWT key and Valkey: it loads the revocation at boot
    const app2 = await createApp({ env: h.env, logger: ['error'] });
    try {
      await app2.init();
      await app2.getHttpAdapter().getInstance().ready();
      expect(await app2.get(TokensService).loadRevocations()).toBeGreaterThan(0);
      const r = await app2.inject({
        method: 'GET',
        url: '/api/v1/auth/me',
        headers: { authorization: `Bearer ${before}` },
      });
      expect(r.statusCode).toBe(401);
      const fresh = (await login('victim')).body.accessToken as string;
      const ok = await app2.inject({
        method: 'GET',
        url: '/api/v1/auth/me',
        headers: { authorization: `Bearer ${fresh}` },
      });
      expect(ok.statusCode).toBe(200);
    } finally {
      await app2.close();
    }
  });

  // ------------------------------------------------------------------------------------------------ WS
  it('a reset closes the target’s WebSockets (4403)', async () => {
    await h.app.listen({ port: h.env.VRX_HTTP_PORT, host: '127.0.0.1' });
    const tok = (await login('wsuser')).body.accessToken as string;
    const ws = new WebSocket(`ws://127.0.0.1:${h.env.VRX_HTTP_PORT}/api/v1/stream`, [
      'vrx.v1',
      `bearer.${tok}`,
    ]);
    await new Promise<void>((resolve, reject) => {
      ws.addEventListener('open', () => resolve());
      ws.addEventListener('error', () => reject(new Error('websocket error')));
    });
    const closed = new Promise<number>((resolve) =>
      ws.addEventListener('close', (e) => resolve(e.code)),
    );
    expect((await reset('wsuser')).status).toBe(200);
    expect(await closed).toBe(4403);
  });
});
