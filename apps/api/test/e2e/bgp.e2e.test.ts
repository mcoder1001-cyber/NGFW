import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { bgpFakeState } from '../../src/features/bgp/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { op: runSecret(), ro: runSecret() };
const MP = { 'content-type': 'application/merge-patch+json' };

/**
 * P12 through the API on the host PostgreSQL with the fake agent: linux-cp pairs (`interfaces.<n>.lcp`), prefix lists,
 * route maps and BGP through the generic config routes, candidate → diff → commit → rollback, the semantic 400s with
 * pointers (P12's routing.bgp-* rules), `GET /state/bgp` (RoutingState) and the FRR annotation / `proto` filter of the
 * FIB browser. Slot 1 names (loop1xx, 10.1.0.0/16) as the other e2e suites.
 */
describe('P12 BGP e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let op: string;
  let ro: string;

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [
      { username: 'op1', role: 'operator', password: PW.op },
      { username: 'ro1', role: 'readonly', password: PW.ro },
    ]);
    op = await h.login('op1', PW.op);
    ro = await h.login('ro1', PW.ro);
  });
  afterAll(async () => h?.close());

  const interfaces = {
    loop111: { enabled: true, ipv4: ['10.1.111.1/24'], lcp: { hostIfName: 'w1-l111' } },
    loop112: { enabled: true, ipv4: ['10.1.112.1/24'], lcp: {} },
  };
  const routing = {
    policy: {
      prefixLists: {
        'pl-low': {
          family: 'ipv4',
          rules: [{ seq: 5, action: 'permit', prefix: '10.1.64.0/18', le: 25 }],
        },
      },
      routeMaps: {
        'rm-in': {
          entries: [
            { seq: 10, action: 'deny', match: { prefixList: 'pl-low' } },
            { seq: 20, action: 'permit', set: { localPref: 150 } },
          ],
        },
      },
    },
    bgp: {
      asn: 65010,
      routerId: '10.1.111.1',
      neighbors: {
        '10.1.111.2': {
          remoteAs: 65011,
          description: 'peer one',
          updateSource: 'loop111',
          afi: { ipv4Unicast: { routeMapIn: 'rm-in', routeMapOut: 'rm-in' } },
        },
        '10.1.112.2': { remoteAs: 65012, shutdown: true },
      },
      networks: [{ prefix: '10.1.250.0/24' }],
      redistribute: { connected: {} },
    },
  };

  it('commits linux-cp pairs, filters and BGP; the agent receives them; rollback removes them', async () => {
    const r1 = await h.call(op, 'PATCH', '/api/v1/config/interfaces', interfaces, MP);
    expect(r1.status, r1.raw).toBe(200);
    const r2 = await h.call(op, 'PATCH', '/api/v1/config/routing', routing, MP);
    expect(r2.status, r2.raw).toBe(200);
    const d = await h.call(op, 'GET', '/api/v1/config/diff');
    const pointers = (d.body.changes as { pointer: string }[]).map((c) => c.pointer);
    expect(pointers).toEqual(expect.arrayContaining(['/routing/bgp', '/interfaces/loop111']));
    const c = await h.call(op, 'POST', '/api/v1/config/commit?comment=bgp');
    expect(c.status, c.raw).toBe(200);
    const rev = c.body.revision.id as number;
    const applied = h.fake.current as Record<string, any>;
    expect(applied['interfaces'].loop111.lcp).toEqual({ hostIfName: 'w1-l111', hostIfType: 'tap' });
    expect(applied['routing'].bgp.neighbors['10.1.111.2'].afi.ipv4Unicast.routeMapIn).toBe('rm-in');
    expect(applied['routing'].policy.routeMaps['rm-in'].entries).toHaveLength(2);

    const rb = await h.call(op, 'POST', `/api/v1/config/rollback/${rev - 1}`);
    expect(rb.status, rb.raw).toBe(200);
    expect((h.fake.current as Record<string, any>)['routing']?.bgp).toBeUndefined();
    const again = await h.call(op, 'POST', `/api/v1/config/rollback/${rev}`);
    expect(again.status, again.raw).toBe(200);
  });

  it('rejects FRR references FRR cannot see with 400 problem+json and pointers', async () => {
    // an update-source interface without a linux-cp pair, and a route tag on an agent-programmed route
    const r = await h.call(
      op,
      'PATCH',
      '/api/v1/config',
      {
        interfaces: { loop113: { enabled: true, ipv4: ['10.1.113.1/24'] } },
        routing: {
          static: [{ prefix: '10.1.70.0/24', blackhole: true, tag: 7 }],
          bgp: { neighbors: { '10.1.113.2': { remoteAs: 65013, updateSource: 'loop113' } } },
        },
      },
      MP,
    );
    const res =
      r.status === 400 ? r : await h.call(op, 'POST', '/api/v1/config/commit?comment=bad');
    expect(res.status, res.raw).toBe(400);
    expect(res.headers['content-type']).toContain('application/problem+json');
    expect(res.body.errors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ pointer: '/routing/bgp/neighbors/10.1.113.2/updateSource' }),
        expect.objectContaining({ pointer: '/routing/static/0/tag' }),
      ]),
    );
    await h.call(op, 'POST', '/api/v1/config/discard');

    const bad = await h.call(
      op,
      'PATCH',
      '/api/v1/config/interfaces',
      { 'TenGigabitEthernet0/1/0': { lcp: {} } },
      MP,
    );
    const res2 =
      bad.status === 400 ? bad : await h.call(op, 'POST', '/api/v1/config/commit?comment=bad2');
    expect(res2.status, res2.raw).toBe(400);
    expect(res2.body.errors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          pointer: '/interfaces/TenGigabitEthernet0~11~10/lcp/hostIfName',
        }),
      ]),
    );
    await h.call(op, 'POST', '/api/v1/config/discard');
  });

  it('serves the live BGP state (neighbours, prefixes, flaps) and the linux-cp pairs', async () => {
    bgpFakeState.neighborState['10.1.111.2'] = 'Established';
    bgpFakeState.prefixesReceived['10.1.111.2'] = 50;
    const s = await h.call(ro, 'GET', '/api/v1/state/bgp');
    expect(s.status, s.raw).toBe(200);
    expect(s.body).toMatchObject({ frrRunning: true, frrVersion: '10.7.1' });
    expect(s.body.instances).toEqual([
      expect.objectContaining({
        vrf: 'default',
        asn: 65010,
        neighbors: [
          expect.objectContaining({
            address: '10.1.111.2',
            state: 'Established',
            prefixesReceived: 50,
            uptimeSec: 3600,
            description: 'peer one',
          }),
          expect.objectContaining({
            address: '10.1.112.2',
            state: 'Idle (Admin)',
            prefixesReceived: 0,
          }),
        ],
      }),
    ]);
    expect(s.body.lcpPairs).toEqual([
      { interface: 'loop111', hostIfName: 'w1-l111', hostIfType: 'tap' },
      { interface: 'loop112', hostIfName: 'loop112', hostIfType: 'tap' },
    ]);
  });

  it('annotates FRR routes in the FIB browser and filters them by protocol', async () => {
    bgpFakeState.frrRoutes = [
      { vrf: 'default', prefix: '10.1.128.0/25', protocol: 'bgp', nextHop: '10.1.111.2' },
      { vrf: 'default', prefix: '10.1.128.128/25', protocol: 'bgp', nextHop: '10.1.111.2' },
      { vrf: 'default', prefix: '10.1.200.0/24', protocol: 'static', nextHop: '10.1.112.2' },
    ];
    const r = await h.call(ro, 'GET', '/api/v1/state/routes?vrf=default&proto=bgp&pageSize=50');
    expect(r.status, r.raw).toBe(200);
    expect(r.body).toMatchObject({ protoFiltered: true, total: 3 });
    expect(r.body.items).toEqual([
      expect.objectContaining({
        prefix: '10.1.128.0/25',
        origin: 'frr',
        proto: 'bgp',
        source: 'lcp-rt-dynamic',
      }),
      expect.objectContaining({ prefix: '10.1.128.128/25', origin: 'frr', proto: 'bgp' }),
    ]);
    expect(h.fake.calls.filter((c) => c.method === 'ListRoutes').at(-1)?.request).toMatchObject({
      source: 'lcp-rt-dynamic',
    });
    expect(
      (await h.call(ro, 'GET', '/api/v1/state/routes?vrf=default&proto=bgp&pageSize=500')).status,
    ).toBe(400);
    expect(
      (await h.call(ro, 'GET', '/api/v1/state/routes?vrf=default&proto=bgp&source=API')).status,
    ).toBe(400);
    expect((await h.call(ro, 'GET', '/api/v1/state/routes?vrf=default&proto=eigrp')).status).toBe(
      400,
    );
    // without proto, connected routes keep their origin
    const all = await h.call(ro, 'GET', '/api/v1/state/routes?vrf=default');
    expect(all.body.items).toContainEqual(
      expect.objectContaining({ prefix: '10.1.111.0/24', origin: 'connected' }),
    );
  });
});
