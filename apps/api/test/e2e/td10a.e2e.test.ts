import type { NestFastifyApplication } from '@nestjs/platform-fastify';
import { IssueSeverity } from '@ngfw/proto';
import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { createApp } from '../../src/app.js';
import { SystemEventsService } from '../../src/audit/system-events.service.js';
import { CommitService } from '../../src/commit/commit.service.js';
import type { Db } from '../../src/db/db.js';
import { SecretsService } from '../../src/secrets/secrets.service.js';
import { startHarness, type Harness } from '../support/harness.js';

/**
 * TD-10a on PostgreSQL (REVIEW-2026-09-24): 2.4b the commit lock holds across API processes (advisory lock), 2.2 a
 * rollback's secret versions and 2.5 the agent's warnings live in config_pending (migration 0004) and survive an API
 * restart, 2.1 a confirm retried after a lost answer is confirmed, 2.3f a secret delete never leaves running pointing
 * at a deleted secret. Fixture secrets follow VRX_TEST_PSK_<id>.
 */
describe('TD-10a commit engine on PostgreSQL', () => {
  let h: Harness;
  let admin: string;
  const slot = () => Number(/(\d+)$/.exec(h.prefix)?.[1] ?? '5');
  const psk = (tag: string) => `VRX_TEST_PSK_TD10A_${tag}_${Date.now()}`;

  beforeAll(async () => {
    h = await startHarness();
    admin = await h.login('admin', h.adminPassword);
  });
  afterAll(async () => h?.close());

  /** A second vrx-api process on the same database, JWT key and agent (an upgrade overlap, tools/app + a unit). */
  async function secondApi(): Promise<NestFastifyApplication> {
    const app = await createApp({ env: h.env, logger: ['error'] });
    await app.init();
    await app.getHttpAdapter().getInstance().ready();
    await app.get(CommitService).resumePending();
    return app;
  }
  async function call2(
    app: NestFastifyApplication,
    method: 'GET' | 'POST',
    url: string,
  ): Promise<{ status: number; body: any }> {
    const r = await app.inject({ method, url, headers: { authorization: `Bearer ${admin}` } });
    return { status: r.statusCode, body: r.body ? JSON.parse(r.body) : undefined };
  }
  const applies = () => h.fake.calls.filter((c) => c.method === 'Apply').length;

  it('2.4b a second API process gets 409 commit-busy while the first one commits', async () => {
    const app2 = await secondApi();
    try {
      await h.call(admin, 'PUT', `/api/v1/config/interfaces/loop${slot()}41`, {
        ipv4: [`10.${slot()}.141.1/24`],
      });
      const n = applies();
      h.fake.applyDelayMs = 2500;
      const first = h.call(admin, 'POST', '/api/v1/config/commit?comment=first');
      await vi.waitFor(() => expect(applies()).toBe(n + 1), { timeout: 5000, interval: 20 });
      const t0 = Date.now();
      const second = await call2(app2, 'POST', '/api/v1/config/commit?comment=second');
      expect(second.status).toBe(409);
      expect(second.body.type).toBe('https://vrx.dev/problems/commit-busy');
      expect(Date.now() - t0).toBeLessThan(2500);
      expect(applies()).toBe(n + 1); // the second process never reached the agent
      const f = await first;
      expect(f.status).toBe(200);
      expect(f.body.status).toBe('applied');
    } finally {
      h.fake.applyDelayMs = 0;
      await app2.close();
    }
  });

  it('2.2 + 2.5 a confirmed rollback confirmed by a restarted API restores the secret versions; warnings survive', async () => {
    const name = `tac-${h.prefix}`;
    const ref = `psk/${name}`;
    expect(
      (await h.call(admin, 'POST', '/api/v1/secrets', { kind: 'psk', name, value: psk('v1') })).body
        .version,
    ).toBe(1);
    await h.call(admin, 'PATCH', '/api/v1/config/management/aaa', {
      tacacs: { servers: [{ address: `10.${slot()}.0.9`, secretRef: ref }] },
    });
    const c1 = await h.call(admin, 'POST', '/api/v1/config/commit?comment=v1');
    expect(c1.status).toBe(200);
    const rot = await h.call(admin, 'POST', '/api/v1/secrets?replace=true', {
      kind: 'psk',
      name,
      value: psk('v2'),
    });
    expect(rot.body.version).toBe(2);
    await h.call(admin, 'PATCH', '/api/v1/config/system', { hostname: `td10a-${h.prefix}` });
    expect((await h.call(admin, 'POST', '/api/v1/config/commit')).status).toBe(200);

    h.fake.dryRunIssues = () => [
      {
        pointer: '/system',
        message: 'fyi: TD-10a warning',
        severity: IssueSeverity.ISSUE_SEVERITY_WARNING,
        rule: 'agent.unsupported-field',
      },
    ];
    try {
      const rb = await h.call(
        admin,
        'POST',
        `/api/v1/config/rollback/${c1.body.revision.id}?confirm=30`,
      );
      expect(rb.status).toBe(200);
      expect(rb.body.status).toBe('pending');
    } finally {
      h.fake.dryRunIssues = undefined;
    }
    const row = await h.db.execute(sql`select restore_secrets, warnings from config_pending`);
    expect(row.rows[0]).toEqual({
      restore_secrets: { [ref]: 1 },
      warnings: [
        { pointer: '/system', message: 'fyi: TD-10a warning', rule: 'agent.unsupported-field' },
      ],
    });

    // the API process that made the rollback is gone; the next one confirms it
    const app2 = await secondApi();
    try {
      const c = await call2(app2, 'POST', '/api/v1/config/commit/confirm');
      expect(c.status).toBe(200);
      expect(c.body.status).toBe('confirmed');
      expect(c.body.warnings).toEqual([
        { pointer: '/system', message: 'fyi: TD-10a warning', rule: 'agent.unsupported-field' },
      ]);
      const cur = await h.db.execute(sql`select version from secret where ref = ${ref}`);
      expect(cur.rows[0]).toEqual({ version: 1 });
      const pinned = await h.db.execute(
        sql`select secret_versions from config_revision where id = ${c.body.revision.id}`,
      );
      expect(pinned.rows[0]).toEqual({ secret_versions: { [ref]: 1 } });
    } finally {
      await app2.close();
    }
  });

  it('2.1 a confirm retried after a lost answer is 200 confirmed (Health.last_txn_id), not 409 reverted', async () => {
    await h.call(admin, 'PATCH', '/api/v1/config/system', { hostname: `td10a-c-${h.prefix}` });
    const c = await h.call(admin, 'POST', '/api/v1/config/commit?confirm=30');
    expect(c.body.status).toBe('pending');
    h.fake.confirmPending(); // the agent confirmed; the answer never reached the API
    const r = await h.call(admin, 'POST', '/api/v1/config/commit/confirm');
    expect(r.status).toBe(200);
    expect(r.body).toMatchObject({ status: 'confirmed', txnId: c.body.txnId });
    const rev = await h.db.execute(
      sql`select count(*)::int as n from config_revision where txn_id = ${c.body.txnId}`,
    );
    expect(rev.rows[0]).toEqual({ n: 1 });
  });

  it('2.3f a secret delete with a commit landing between its reads never leaves running on a deleted secret', async () => {
    const name = `race-${h.prefix}`;
    const ref = `psk/${name}`;
    expect(
      (await h.call(admin, 'POST', '/api/v1/secrets', { kind: 'psk', name, value: psk('race') }))
        .body.created,
    ).toBe(true);
    await h.call(admin, 'PATCH', '/api/v1/config/management/aaa', {
      tacacs: { servers: [{ address: `10.${slot()}.0.19`, secretRef: ref }] },
    });
    let commitStatus = 0;
    const hooked = hookDb(h.db, async () => {
      commitStatus = (await h.call(admin, 'POST', '/api/v1/config/commit?comment=race')).status;
    });
    const secrets = new SecretsService(
      hooked,
      h.env,
      h.app.get(SystemEventsService),
      h.app.get(CommitService),
    );
    const outcome = await secrets.delete('psk', name).then(
      () => 'deleted',
      (e: { getStatus?: () => number }) => e.getStatus?.() ?? String(e),
    );
    const running = await h.call(admin, 'GET', '/api/v1/config/management/aaa');
    const referenced = JSON.stringify(running.body).includes(ref);
    const exists = (await h.call(admin, 'GET', '/api/v1/secrets')).body.some(
      (s: { ref: string }) => s.ref === ref,
    );
    console.log(
      `2.3f race: commit ${commitStatus}, delete ${outcome}, running references it: ${referenced}, secret exists: ${exists}`,
    );
    expect(referenced && !exists).toBe(false);
    // clean up: drop the reference, then the secret
    await h.call(admin, 'PATCH', '/api/v1/config/management/aaa', { tacacs: { servers: [] } });
    await h.call(admin, 'POST', '/api/v1/config/commit');
    if (exists) await h.call(admin, 'DELETE', `/api/v1/secrets/psk/${name}`);
  });
});

/**
 * A Drizzle database whose SECOND `select` (and the first `transaction`) runs `hook` before it executes — the moment
 * between the delete's reference reads that review 2.3f names. Test-only.
 */
function hookDb(db: Db, hook: () => Promise<void>): Db {
  let fired = false;
  let selects = 0;
  const once = async () => {
    if (fired) return;
    fired = true;
    await hook();
  };
  const wrap = (q: object): object =>
    new Proxy(q, {
      get(t, prop) {
        const v = Reflect.get(t, prop, t) as unknown;
        if (prop === 'then') {
          return (res: (x: unknown) => unknown, rej: (e: unknown) => unknown) =>
            once().then(() => (v as (a: unknown, b: unknown) => unknown).call(t, res, rej), rej);
        }
        if (typeof v !== 'function') return v;
        return (...a: unknown[]) => {
          const r = (v as (...x: unknown[]) => unknown).apply(t, a);
          return r !== null && typeof r === 'object' ? wrap(r) : r;
        };
      },
    });
  return new Proxy(db, {
    get(t, prop) {
      const v = Reflect.get(t, prop, t) as unknown;
      if (prop === 'select') {
        return (...a: unknown[]) => {
          selects += 1;
          const q = (v as (...x: unknown[]) => object).apply(t, a);
          return selects === 2 ? wrap(q) : q;
        };
      }
      if (prop === 'transaction') {
        return async (...a: unknown[]) => {
          await once();
          return (v as (...x: unknown[]) => unknown).apply(t, a);
        };
      }
      return typeof v === 'function' ? (v as (...x: unknown[]) => unknown).bind(t) : v;
    },
  }) as Db;
}
