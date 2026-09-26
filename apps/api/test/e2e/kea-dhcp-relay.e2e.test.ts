import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { fakeLease, setFakeDhcp } from '../../src/features/kea-dhcp-relay/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { op: runSecret(), ro: runSecret() };
const MP = { 'content-type': 'application/merge-patch+json' };

/**
 * F-kea-dhcp-relay on the host PostgreSQL with the fake agent: a Kea server + relay + DHCP client through the generic
 * pointer routes, commit, `/state/dhcp/leases` (paged, filtered, server status and pool usage from DhcpLeases),
 * `/state/dhcp/relays` (running vs Retrieve), `/state/interfaces/{name}/dhcp-client`; validation failures → 400
 * problem+json with pointers (pool outside its subnet: schema tier; reservation inside a pool, DHCP client + static
 * address: semantic tier); rollback removes the server and relay. Names and addresses follow slot 1's rules.
 */
describe('kea-dhcp-relay e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let op: string;
  let ro: string;
  const L = 'host-w1l0';
  const W = 'host-w1w0';
  const C = 'host-w1c0';

  beforeAll(async () => {
    h = await startHarness({});
    const admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [
      { username: 'op1', role: 'operator', password: PW.op },
      { username: 'ro1', role: 'readonly', password: PW.ro },
    ]);
    op = await h.login('op1', PW.op);
    ro = await h.login('ro1', PW.ro);
  });
  afterAll(async () => h?.close());

  const lan = {
    enabled: true,
    vrf: 'w1-dhcp',
    interfaces: [L],
    leaseTimeSec: 600,
    subnets: {
      lan: {
        subnet: '10.1.1.0/24',
        pools: [{ start: '10.1.1.100', end: '10.1.1.150' }],
        gateway: '10.1.1.1',
        dnsServers: ['10.1.1.1'],
        reservations: { printer: { mac: '02:00:00:00:01:01', ip: '10.1.1.20' } },
      },
    },
  };
  const relay = {
    vrf: 'w1-dhcp',
    interfaces: [L],
    servers: ['10.1.2.2'],
    sourceAddress: '10.1.2.1',
    description: 'LAN to the Kea test instance',
  };

  let rev1 = 0;
  it('commits a server, a relay and a DHCP client through the pointer routes', async () => {
    const base = await h.call(op, 'POST', '/api/v1/config/commit?comment=base');
    expect(base.status).toBe(200);
    expect(
      (
        await h.call(
          op,
          'PATCH',
          '/api/v1/config/vrfs',
          { 'w1-dhcp': { id: 1001 }, 'w1-cli': { id: 1002 } },
          MP,
        )
      ).status,
    ).toBe(200);
    expect(
      (
        await h.call(
          op,
          'PATCH',
          '/api/v1/config/interfaces',
          {
            [L]: { enabled: true, vrf: 'w1-dhcp', ipv4: ['10.1.1.1/24'] },
            [W]: { enabled: true, vrf: 'w1-dhcp', ipv4: ['10.1.2.1/24'] },
            [C]: {
              enabled: true,
              vrf: 'w1-cli',
              dhcpClient: { hostname: 'w1-client', setBroadcastFlag: true },
            },
          },
          MP,
        )
      ).status,
    ).toBe(200);
    expect((await h.call(op, 'PUT', '/api/v1/config/services/dhcp/servers/lan', lan)).status).toBe(
      200,
    );
    expect(
      (await h.call(op, 'PUT', '/api/v1/config/services/dhcp/relays/to-kea', relay)).status,
    ).toBe(200);
    const d = await h.call(ro, 'GET', '/api/v1/config/diff');
    expect(d.status).toBe(200);
    expect(JSON.stringify(d.body)).toContain('/services/dhcp/servers/lan');
    const c = await h.call(op, 'POST', '/api/v1/config/commit?comment=dhcp');
    expect(c.status).toBe(200);
    expect(c.body.status).toBe('applied');
    rev1 = c.body.revision.id as number;
    // the services document reached the agent
    expect(h.fake.current['services']).toMatchObject({
      dhcp: {
        servers: { lan: { vrf: 'w1-dhcp', interfaces: [L] } },
        relays: { 'to-kea': { servers: ['10.1.2.2'] } },
      },
    });
  });

  it('serves the lease page: server-side paging, filter, server status and pool usage', async () => {
    const leases = Array.from({ length: 30 }, (_, i) =>
      fakeLease({
        address: `10.1.1.${100 + i}`,
        hwAddress: `02:00:00:00:00:${(i + 16).toString(16)}`,
        hostname: `pc${i}`,
        server: 'lan',
        subnet: 'lan',
        subnetId: 1,
      }),
    );
    setFakeDhcp(h.fake, { leases, usage: { 1: { assigned: 30, total: 51 } } });
    const p2 = await h.call(ro, 'GET', '/api/v1/state/dhcp/leases?page=2&pageSize=10');
    expect(p2.status).toBe(200);
    expect(p2.body).toMatchObject({ page: 2, pageSize: 10, total: 30, truncated: false });
    expect(p2.body.items.map((l: { address: string }) => l.address)).toEqual(
      Array.from({ length: 10 }, (_, i) => `10.1.1.${110 + i}`),
    );
    expect(p2.body.servers).toContainEqual(
      expect.objectContaining({
        family: 'ipv4',
        running: true,
        active: true,
        actionRequired: '',
        subnets: [
          expect.objectContaining({
            server: 'lan',
            subnet: 'lan',
            prefix: '10.1.1.0/24',
            total: '51',
            assigned: '30',
          }),
        ],
      }),
    );
    const f = await h.call(
      ro,
      'GET',
      '/api/v1/state/dhcp/leases?filter=PC29&server=lan&family=ipv4',
    );
    expect(f.body.total).toBe(1);
    expect(f.body.items[0]).toMatchObject({
      address: '10.1.1.129',
      hostname: 'pc29',
      server: 'lan',
    });
    const req = h.fake.calls.filter((x) => x.method === 'DhcpLeases').at(-1)?.request as Record<
      string,
      unknown
    >;
    expect(req).toMatchObject({
      server: 'lan',
      family: 'ipv4',
      filter: 'PC29',
      page: 1,
      pageSize: 100,
      interface: '',
    });
    // a stopped daemon with an active configuration is a start request (D-079)
    setFakeDhcp(h.fake, { running: { ipv4: false, ipv6: true } });
    const stopped = await h.call(ro, 'GET', '/api/v1/state/dhcp/leases?family=ipv4');
    expect(stopped.body.servers[0]).toMatchObject({ running: false, actionRequired: 'start' });
    setFakeDhcp(h.fake, { running: { ipv4: true, ipv6: true } });
    // bad query → 400 problem+json
    const bad = await h.call(ro, 'GET', '/api/v1/state/dhcp/leases?pageSize=5000');
    expect(bad.status).toBe(400);
    expect(bad.headers['content-type']).toMatch(/^application\/problem\+json/);
  });

  it('lists relays (running vs Retrieve) and the interface DHCP client state', async () => {
    const r = await h.call(ro, 'GET', '/api/v1/state/dhcp/relays');
    expect(r.status).toBe(200);
    expect(r.body.items).toEqual([
      expect.objectContaining({
        name: 'to-kea',
        state: 'applied',
        config: expect.objectContaining({ sourceAddress: '10.1.2.1' }),
      }),
    ]);
    setFakeDhcp(h.fake, {
      clients: {
        [C]: {
          configured: true,
          state: 'BOUND',
          address: '10.1.1.120/24',
          router: '10.1.1.1',
          dnsServers: ['10.1.1.1'],
          hostname: 'w1-client',
        },
      },
    });
    const c = await h.call(ro, 'GET', `/api/v1/state/interfaces/${C}/dhcp-client`);
    expect(c.status).toBe(200);
    expect(c.body).toMatchObject({
      interface: C,
      configured: true,
      state: 'BOUND',
      address: '10.1.1.120/24',
      config: { hostname: 'w1-client', setBroadcastFlag: true },
    });
    const none = await h.call(ro, 'GET', `/api/v1/state/interfaces/${L}/dhcp-client`);
    expect(none.body).toMatchObject({ configured: false, config: null });
    expect(
      (await h.call(ro, 'GET', '/api/v1/state/interfaces/bad%20name/dhcp-client')).status,
    ).toBe(400);
  });

  it('pool outside its subnet → 400 problem+json with the pool pointer (schema tier)', async () => {
    const put = await h.call(
      op,
      'PUT',
      '/api/v1/config/services/dhcp/servers/lan/subnets/lan/pools/0',
      {
        start: '10.1.9.10',
        end: '10.1.9.20',
      },
    );
    expect(put.status).toBe(400);
    expect(put.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect(put.body.errors).toContainEqual(
      expect.objectContaining({ pointer: '/services/dhcp/servers/lan/subnets/lan/pools/0/start' }),
    );
  });

  it('reservation inside a pool and a DHCP client with a static address → 400 (semantic tier)', async () => {
    expect(
      (
        await h.call(
          op,
          'PUT',
          '/api/v1/config/services/dhcp/servers/lan/subnets/lan/reservations/inpool',
          { mac: '02:00:00:00:01:02', ip: '10.1.1.130' },
        )
      ).status,
    ).toBe(200);
    expect(
      (await h.call(op, 'PATCH', `/api/v1/config/interfaces/${C}`, { ipv4: ['10.1.3.1/24'] }, MP))
        .status,
    ).toBe(200);
    const c = await h.call(op, 'POST', '/api/v1/config/commit');
    expect(c.status).toBe(400);
    expect(c.body).toMatchObject({ status: 400, tier: 'semantic' });
    expect(c.body.errors).toContainEqual(
      expect.objectContaining({
        pointer: '/services/dhcp/servers/lan/subnets/lan/reservations/inpool/ip',
        message: expect.stringMatching(/inside pool 0/),
      }),
    );
    expect(c.body.errors).toContainEqual(
      expect.objectContaining({
        pointer: `/interfaces/${C}/ipv4`,
        message: expect.stringMatching(/DHCP client/),
      }),
    );
    expect((await h.call(op, 'POST', '/api/v1/config/discard')).body).toEqual({ discarded: true });
  });

  it('rollback to the revision before DHCP removes server, relay and client from what the agent holds', async () => {
    expect(rev1).toBeGreaterThan(1);
    const rb = await h.call(op, 'POST', `/api/v1/config/rollback/${rev1 - 1}`);
    expect([200, 201]).toContain(rb.status);
    const services = (h.fake.current['services'] ?? {}) as {
      dhcp?: { servers?: object; relays?: object };
    };
    expect(Object.keys(services.dhcp?.servers ?? {})).toEqual([]);
    expect(Object.keys(services.dhcp?.relays ?? {})).toEqual([]);
    const r = await h.call(ro, 'GET', '/api/v1/state/dhcp/relays');
    expect(r.body.items).toEqual([]);
  });
});
