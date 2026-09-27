import http from 'node:http';
import type { AddressInfo } from 'node:net';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AlarmsService } from '../../src/features/dashboard-prom-alarms/index.js';
import { startHarness, type Harness } from '../support/harness.js';

/**
 * F-dashboard-prom-alarms API: alarm rules commit, the engine raises/clears with hysteresis, webhook targets are
 * notified, /state/alarms lists them, ack records the actor, and /state/dashboard summarises. Samples are driven
 * through the engine directly (the live StreamStats subscription is host-side).
 */
const MP = { 'content-type': 'application/merge-patch+json' };

describe('F-dashboard-prom-alarms e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let svc: AlarmsService;
  let hookServer: http.Server;
  let hookUrl: string;
  const hooks: { auth: string | null; body: unknown }[] = [];

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    hookServer = http.createServer((req, res) => {
      let body = '';
      req.on('data', (c) => (body += c));
      req.on('end', () => {
        hooks.push({
          auth: req.headers['authorization'] ?? null,
          body: body ? JSON.parse(body) : null,
        });
        res.writeHead(204).end();
      });
    });
    await new Promise<void>((r) => hookServer.listen(0, '127.0.0.1', r));
    hookUrl = `http://127.0.0.1:${(hookServer.address() as AddressInfo).port}/hook`;
    svc = h.app.get(AlarmsService);
    svc.now = () => 1_000_000; // fixed clock; tests pass explicit sample times
  });
  afterAll(async () => {
    hookServer?.close();
    await h?.close();
  });

  it('commits rules+targets, raises a link-down alarm, notifies the webhook, lists, acks, clears', async () => {
    const p = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/management',
      {
        alarms: {
          targets: { ops: { kind: 'webhook', url: hookUrl } },
          rules: {
            wanlink: {
              metric: 'interface_link_down',
              op: 'ge',
              threshold: 1,
              severity: 'critical',
              targets: ['ops'],
            },
          },
        },
      },
      MP,
    );
    expect(p.status, p.raw).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=alarms');
    expect(c.status, c.raw).toBe(200);
    // the commit fires reload() via the bus; give the microtask a tick
    await new Promise((r) => setTimeout(r, 50));

    // link down on wan0 → raise
    await svc.ingestSamples(
      [{ metric: 'interface_link_down', instance: 'wan0', value: 1 }],
      1_000_000,
    );
    await new Promise((r) => setTimeout(r, 50));
    let list = await h.call(admin, 'GET', '/api/v1/state/alarms?active=true');
    expect(list.status, list.raw).toBe(200);
    expect(list.body.items).toEqual([
      expect.objectContaining({
        rule: 'wanlink',
        instance: 'wan0',
        severity: 'critical',
        state: 'active',
      }),
    ]);
    expect(hooks.at(-1)?.body).toMatchObject({ kind: 'raised', rule: 'wanlink', instance: 'wan0' });

    const id = list.body.items[0].id as number;
    // re-raise is idempotent (still one active row, no second webhook)
    const hooksBefore = hooks.length;
    await svc.ingestSamples(
      [{ metric: 'interface_link_down', instance: 'wan0', value: 1 }],
      1_000_001,
    );
    expect(hooks.length).toBe(hooksBefore);

    // ack records the actor
    const ack = await h.call(admin, 'POST', `/api/v1/actions/alarms/${id}/ack`);
    expect(ack.status, ack.raw).toBe(200);
    expect(ack.body).toEqual({ acked: true });
    const ack2 = await h.call(admin, 'POST', `/api/v1/actions/alarms/${id}/ack`);
    expect(ack2.status).toBe(404); // already acked

    // link back up → clear + webhook
    await svc.ingestSamples(
      [{ metric: 'interface_link_down', instance: 'wan0', value: 0 }],
      1_000_002,
    );
    await new Promise((r) => setTimeout(r, 50));
    list = await h.call(admin, 'GET', '/api/v1/state/alarms?active=true');
    expect(list.body.items).toEqual([]);
    expect(hooks.at(-1)?.body).toMatchObject({
      kind: 'cleared',
      rule: 'wanlink',
      instance: 'wan0',
    });
    const all = await h.call(admin, 'GET', '/api/v1/state/alarms');
    expect(all.body.items[0]).toMatchObject({ state: 'cleared', ackedBy: 'admin' });
  });

  it('summarises active alarms and agent reachability on /state/dashboard', async () => {
    await svc.ingestSamples(
      [{ metric: 'interface_link_down', instance: 'wan9', value: 1 }],
      1_000_100,
    );
    await new Promise((r) => setTimeout(r, 30));
    const d = await h.call(admin, 'GET', '/api/v1/state/dashboard');
    expect(d.status, d.raw).toBe(200);
    expect(d.body.alarms.active).toBeGreaterThanOrEqual(1);
    expect(d.body.alarms.bySeverity.critical).toBeGreaterThanOrEqual(1);
    expect(typeof d.body.agent.reachable).toBe('boolean');
  });

  it('rejects a rule referencing an unknown target (semantic tier)', async () => {
    await h.call(
      admin,
      'PATCH',
      '/api/v1/config/management',
      {
        alarms: {
          rules: { bad: { metric: 'worker_cpu_percent', threshold: 90, targets: ['ghost'] } },
        },
      },
      MP,
    );
    const bad = await h.call(admin, 'POST', '/api/v1/config/commit?comment=badrule');
    expect(bad.status, bad.raw).toBe(400);
    expect(JSON.stringify(bad.body.errors)).toContain('/management/alarms/rules/bad/targets/0');
  });
});
