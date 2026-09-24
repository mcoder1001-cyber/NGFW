import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { setFakeNeighbors } from '../../src/features/neighbors-ra/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { ro: runSecret() };

/**
 * F-neighbors-ra on the host PostgreSQL with the fake agent: the neighbour/RA/proxy leaves go through candidate →
 * validate → commit like every leaf (pointer routes, no feature code), `GET /api/v1/state/neighbors` (ListNeighbors,
 * paged by the agent) replaces the P06 501 stub, and `POST /api/v1/actions/arp-flush` is this feature's static route
 * (it beats the generic `POST /api/v1/actions/:action`) and is audited. Slot names: host-w9l0, 10.9.0.0/16, table 9xxx.
 */
describe('neighbors-ra e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let ro: string;
  const L = 'host-w9l0';

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [{ username: 'ro1', role: 'readonly', password: PW.ro }]);
    ro = await h.login('ro1', PW.ro);
  });
  afterAll(async () => h?.close());

  it('commits RA, proxy-ARP and static neighbours through the generic pointer routes', async () => {
    const mp = { 'content-type': 'application/merge-patch+json' };
    const patch = await h.call(
      admin,
      'PATCH',
      '/api/v1/config',
      {
        vrfs: { w9red: { id: 9001, proxyArpRanges: [{ low: '10.9.3.10', high: '10.9.3.20' }] } },
        interfaces: {
          [L]: {
            enabled: true,
            ipv4: ['10.9.1.1/24'],
            ipv6: ['2001:db8:9:1::1/64'],
            proxyArp: true,
            ipv6Ra: { suppress: false, prefixes: { '2001:db8:9:1::/64': {} } },
          },
        },
        routing: {
          neighbors: { static: [{ interface: L, ip: '10.9.1.50', mac: '02:00:00:00:09:50' }] },
        },
      },
      mp,
    );
    expect(patch.status).toBe(200);
    const diff = await h.call(admin, 'GET', '/api/v1/config/diff');
    expect(JSON.stringify(diff.body)).toContain('/routing/neighbors');
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=neighbors-ra')).status).toBe(
      200,
    );
    const applied = h.fake.calls.filter((c) => c.method === 'Apply').at(-1)!.request as {
      desiredState: {
        routing: { neighbors: { static: unknown[] } };
        interfaces: Record<string, { ipv6Ra: unknown }>;
      };
    };
    expect(applied.desiredState.routing.neighbors.static).toHaveLength(1);
    expect(applied.desiredState.interfaces[L]!.ipv6Ra).toMatchObject({
      suppress: false,
      lifetimeSec: 600,
    });
    const running = await h.call(ro, 'GET', `/api/v1/config/interfaces/${L}/ipv6Ra`);
    expect(running.body).toEqual({
      suppress: false,
      managed: false,
      other: false,
      lifetimeSec: 600,
      maxIntervalSec: 200,
      minIntervalSec: 150,
      prefixes: {
        '2001:db8:9:1::/64': {
          validSec: 2592000,
          preferredSec: 604800,
          offLink: false,
          noAutoconfig: false,
        },
      },
    });
  });

  it('static neighbour outside every connected subnet → 400 problem+json with the pointer (acceptance)', async () => {
    await h.call(admin, 'PUT', '/api/v1/config/routing/neighbors/static', [
      { interface: L, ip: '10.9.1.50', mac: '02:00:00:00:09:50' },
      { interface: L, ip: '10.9.200.5', mac: '02:00:00:00:09:51' },
    ]);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit');
    expect(c.status).toBe(400);
    expect(c.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect(c.body).toMatchObject({
      status: 400,
      tier: 'semantic',
      type: 'https://vrx.dev/problems/validation',
    });
    expect(c.body.errors).toContainEqual({
      pointer: '/routing/neighbors/static/1/ip',
      message: `10.9.200.5 is outside every connected subnet of ${L}`,
    });
    // RA intervals VPP refuses are schema errors at edit time, also with a pointer
    const ra = await h.call(
      admin,
      'PATCH',
      `/api/v1/config/interfaces/${L}/ipv6Ra`,
      { maxIntervalSec: 100, minIntervalSec: 90 },
      {
        'content-type': 'application/merge-patch+json',
      },
    );
    expect(ra.status).toBe(400);
    expect(ra.body.errors).toContainEqual(
      expect.objectContaining({ pointer: `/interfaces/${L}/ipv6Ra/minIntervalSec` }),
    );
    expect((await h.call(admin, 'POST', '/api/v1/config/discard')).body).toEqual({
      discarded: true,
    });
  });

  it('GET /state/neighbors: the agent table, filtered and paged (readonly may read)', async () => {
    setFakeNeighbors(h.fake, [
      { interface: L, ip: '10.9.1.7', mac: '02:00:00:00:09:07', ageSec: 4.5 },
      { interface: L, ip: '2001:db8:9:1::7', mac: '02:00:00:00:09:08', ageSec: 1 },
      {
        interface: 'loop901',
        ip: '10.9.5.7',
        mac: '02:00:00:00:95:07',
        vrf: 'w9red',
        tableId: 9001,
      },
    ]);
    const all = await h.call(ro, 'GET', '/api/v1/state/neighbors');
    expect(all.status).toBe(200);
    expect(all.body).toMatchObject({ page: 1, pageSize: 100, total: 4 });
    expect(all.body.items[0]).toEqual({
      interface: L,
      ip: '10.9.1.50',
      mac: '02:00:00:00:09:50',
      family: 'ipv4',
      state: 'static',
      noFibEntry: false,
      ageSec: 0,
      vrf: 'default',
      tableId: 0,
    });
    const page = await h.call(
      ro,
      'GET',
      '/api/v1/state/neighbors?state=dynamic&family=ipv4&sort=age&dir=desc&page=1&pageSize=1',
    );
    expect(page.body).toMatchObject({
      total: 2,
      pageSize: 1,
      items: [{ ip: '10.9.1.7', ageSec: 4.5 }],
    });
    const req = h.fake.calls.filter((c) => c.method === 'ListNeighbors').at(-1)!.request;
    expect(req).toMatchObject({
      state: 'dynamic',
      family: 'ipv4',
      sort: 'age',
      descending: true,
      offset: 0,
      limit: 1,
      owner: h.prefix,
    });
    const second = await h.call(
      ro,
      'GET',
      '/api/v1/state/neighbors?state=dynamic&page=2&pageSize=2',
    );
    expect(second.body).toMatchObject({ page: 2, total: 3 });
    expect(second.body.items.map((i: { ip: string }) => i.ip)).toEqual(['10.9.5.7']);
    expect((await h.call(ro, 'GET', '/api/v1/state/neighbors?vrf=w9red')).body.items).toHaveLength(
      1,
    );
    const bad = await h.call(ro, 'GET', '/api/v1/state/neighbors?family=ipx&pageSize=5000');
    expect(bad.status).toBe(400);
    expect(bad.body.errors.map((e: { pointer: string }) => e.pointer).sort()).toEqual([
      '/family',
      '/pageSize',
    ]);
  });

  it('POST /actions/arp-flush is this feature’s route: RBAC, body validation, audited, agent errors as problems', async () => {
    expect((await h.call(ro, 'POST', '/api/v1/actions/arp-flush', {})).status).toBe(403);
    const bad = await h.call(admin, 'POST', '/api/v1/actions/arp-flush', {
      family: 'ipx',
      extra: 1,
    });
    expect(bad.status).toBe(400);
    expect(bad.body.errors.map((e: { pointer: string }) => e.pointer).sort()).toEqual([
      '/extra',
      '/family',
    ]);
    // The shared fake agent's Action handler (fake-agent.ts, not this feature's) ends every Action with
    // call.destroy(UNIMPLEMENTED), which never reaches the client: the API answers 504 after VRX_AGENT_TIMEOUT_MS
    // (5 s here). Either way the request went to the agent through this route — the generic route would have
    // answered 404 "unknown action 'arp-flush'" — and the failure is a problem+json (questions Q9).
    const r = await h.call(admin, 'POST', '/api/v1/actions/arp-flush', {
      interface: L,
      family: 'ipv4',
    });
    expect([501, 504]).toContain(r.status);
    expect(r.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect(r.body.type).toMatch(/^https:\/\/vrx\.dev\/problems\/agent-(unimplemented|timeout)$/);
    const action = h.fake.calls.filter((c) => c.method === 'Action').at(-1)!.request;
    expect(action).toEqual({ arpFlush: { interface: L, family: 'ipv4' } });
    const audit = await h.call(admin, 'GET', '/api/v1/audit?limit=10');
    expect(audit.body.items).toContainEqual(
      expect.objectContaining({
        action: 'POST /api/v1/actions/arp-flush',
        resource: `arp-flush/${L}`,
        before: { interface: L, family: 'ipv4' },
        result: 'failure',
        status: r.status,
      }),
    );
    // the generic action route still owns the other names
    expect((await h.call(admin, 'POST', '/api/v1/actions/ping')).status).toBe(501);
  });
});
