import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import {
  installLldpFake,
  type LldpFake,
} from '../../src/features/loopback-bvi-gso-lldp-span/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { ro: runSecret() };
const MP = { 'content-type': 'application/merge-patch+json' };

/**
 * F-loopback-bvi-gso-lldp-span on the host PostgreSQL with the fake agent: GSO, mirror sessions, LLDP and nsim through
 * the generic pointer routes, the semantic 400s (mirror destination = source, the D-105 loopback reservation, the nsim
 * wheel bound) with their pointers, `/state/lldp/neighbors` from the LldpNeighbors RPC (server-side paged) and the
 * mirror / GSO leaves in `/state/interfaces` items. Names follow the slot rules (loop<N>xxx).
 */
describe('loopback / GSO / LLDP / mirroring / nsim e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let ro: string;
  let fake: LldpFake;
  const BVI = 'loop7101';
  const MON = 'loop7102';

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [{ username: 'ro1', role: 'readonly', password: PW.ro }]);
    ro = await h.login('ro1', PW.ro);
  });
  afterAll(async () => h?.close());

  const commitExpect400 = async (patch: unknown, pointer: string, message: RegExp) => {
    expect((await h.call(admin, 'PATCH', '/api/v1/config', patch, MP)).status).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit');
    expect(c.status).toBe(400);
    expect(c.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect(c.body).toMatchObject({ status: 400, tier: 'semantic' });
    expect(c.body.errors).toContainEqual(
      expect.objectContaining({ pointer, message: expect.stringMatching(message) }),
    );
    expect((await h.call(admin, 'POST', '/api/v1/config/discard')).body).toEqual({
      discarded: true,
    });
  };

  it('an agent without the RPC answers 501', async () => {
    const r = await h.call(ro, 'GET', '/api/v1/state/lldp/neighbors');
    expect(r.status).toBe(501);
    expect(r.headers['content-type']).toMatch(/^application\/problem\+json/);
  });

  it('a mirror destination equal to its source is a 400 problem+json with the pointer', async () => {
    await commitExpect400(
      { interfaces: { [BVI]: { enabled: true, mirror: [{ destination: BVI }] } } },
      `/interfaces/${BVI}/mirror/0/destination`,
      /cannot copy loop7101 to itself/,
    );
  });

  it('loop16000–loop16383 are refused (D-105) and the nsim wheel is bounded', async () => {
    await commitExpect400(
      { interfaces: { loop16001: { enabled: true } } },
      '/interfaces/loop16001',
      /reserved for the agent's quarantine holders/,
    );
    process.env['VRX_NSIM'] = 'lab'; // the semantic bound is checked once the lab gate lets the commit through
    await commitExpect400(
      { services: { nsim: { delayMs: 10000, bandwidthMbps: 100000, packetSize: 64 } } },
      '/services/nsim/delayMs',
      /scheduler wheel/,
    );
    delete process.env['VRX_NSIM'];
  });

  it('nsim is a lab tool: without VRX_NSIM=lab a commit that carries services.nsim is a 409 with its pointer (review M2)', async () => {
    delete process.env['VRX_NSIM'];
    expect(
      (
        await h.call(
          admin,
          'PATCH',
          '/api/v1/config',
          { services: { nsim: { delayMs: 20, bandwidthMbps: 100 } } },
          MP,
        )
      ).status,
    ).toBe(200);
    const applies = h.fake.calls.filter((x) => x.method === 'Apply').length;
    const c = await h.call(admin, 'POST', '/api/v1/config/commit');
    expect(c.status).toBe(409);
    expect(c.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect(c.body).toMatchObject({ status: 409, detail: expect.stringMatching(/VRX_NSIM=lab/) });
    expect(c.body.errors).toEqual([expect.objectContaining({ pointer: '/services/nsim' })]);
    expect(h.fake.calls.filter((x) => x.method === 'Apply')).toHaveLength(applies); // nothing reached the agent
    expect((await h.call(admin, 'POST', '/api/v1/config/discard')).body).toEqual({
      discarded: true,
    });
  });

  it('commits a loopback BVI with GSO, mirroring, LLDP and nsim; state shows them', async () => {
    process.env['VRX_NSIM'] = 'lab'; // the lab gate on (review M2)
    fake = await installLldpFake(h.fake, {
      heard: {
        [BVI]: { chassisId: '02:00:00:00:71:01', portId: 'Ethernet7', ttl: 120, agoSec: 7 },
      },
    });
    const patch = await h.call(
      admin,
      'PATCH',
      '/api/v1/config',
      {
        interfaces: {
          [BVI]: {
            enabled: true,
            ipv4: ['10.7.101.1/24'],
            l2: { bridgeDomain: 'lan', bvi: true },
            gso: true,
            mirror: [{ destination: MON }, { destination: MON, direction: 'rx', level: 'l2' }],
          },
          [MON]: { enabled: true },
        },
        routing: { l2: { bridgeDomains: { lan: { id: 7101 } } } },
        services: {
          lldp: {
            enabled: true,
            interfaces: [{ interface: MON }, { interface: BVI, portDescription: 'w7 bvi' }],
          },
          nsim: { delayMs: 20, bandwidthMbps: 100, outputInterfaces: [MON] },
        },
      },
      MP,
    );
    expect(patch.status).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=lbgs');
    expect(c.status).toBe(200);

    // the running document carries the Zod defaults of the new leaves
    const run = await h.call(ro, 'GET', '/api/v1/config/interfaces/loop7101');
    expect(run.status).toBe(200);
    expect(run.body).toMatchObject({
      gso: true,
      mirror: [
        { destination: MON, direction: 'both', level: 'device' },
        { destination: MON, direction: 'rx', level: 'l2' },
      ],
    });
    const nsim = await h.call(ro, 'GET', '/api/v1/config/services/nsim');
    expect(nsim.body).toMatchObject({ delayMs: 20, packetSize: 1500, dropFraction: 0 });

    // /state/interfaces: the agent's Retrieve view (`config`) carries GSO and the mirror sessions
    const st = await h.call(ro, 'GET', '/api/v1/state/interfaces');
    expect(st.status).toBe(200);
    const item = (st.body.items as { name: string; config: Record<string, unknown> | null }[]).find(
      (i) => i.name === BVI,
    );
    expect(item?.config).toMatchObject({
      gso: true,
      mirror: expect.arrayContaining([expect.objectContaining({ destination: MON })]),
    });

    // LLDP table: ordered by interface, marked with the running configuration, paged
    const n = await h.call(ro, 'GET', '/api/v1/state/lldp/neighbors');
    expect(n.status).toBe(200);
    expect(n.body).toMatchObject({ page: 1, pageSize: 100, total: 2 });
    expect(n.body.items).toEqual([
      expect.objectContaining({
        interface: BVI,
        heard: true,
        chassisId: '02:00:00:00:71:01',
        chassisIdSubtype: 'mac-address',
        portId: 'Ethernet7',
        ttl: 120,
        lastHeardSecAgo: 7,
        configured: true,
        portDescription: 'w7 bvi',
      }),
      expect.objectContaining({
        interface: MON,
        heard: false,
        configured: true,
        portDescription: null,
      }),
    ]);
    const p2 = await h.call(ro, 'GET', '/api/v1/state/lldp/neighbors?page=2&pageSize=1');
    expect(p2.body).toMatchObject({ page: 2, pageSize: 1, total: 2 });
    expect(p2.body.items.map((i: { interface: string }) => i.interface)).toEqual([MON]);
    expect((await h.call(ro, 'GET', '/api/v1/state/lldp/neighbors?pageSize=1001')).status).toBe(
      400,
    );
    expect(fake.table()).toHaveLength(2);
    // the API always passes its own owner (proto.md §6)
    expect(h.fake.calls.filter((c) => c.method === 'LldpNeighbors').at(-1)?.request).toMatchObject({
      owner: h.prefix,
    });
  });

  it('with the gate off again, neither a commit nor a rollback may carry services.nsim (409)', async () => {
    delete process.env['VRX_NSIM'];
    const revs = await h.call(admin, 'GET', '/api/v1/config/revisions');
    const items = (revs.body.items ?? revs.body) as { id: number }[];
    const last = Math.max(...items.map((r) => r.id));
    expect(
      (await h.call(admin, 'PATCH', '/api/v1/config', { interfaces: { [MON]: { mtu: 1500 } } }, MP))
        .status,
    ).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit')).status).toBe(409);
    expect((await h.call(admin, 'POST', '/api/v1/config/discard')).status).toBe(200);
    const rb = await h.call(admin, 'POST', `/api/v1/config/rollback/${last}`);
    expect(rb.status).toBe(409);
    expect(rb.body.errors).toEqual([expect.objectContaining({ pointer: '/services/nsim' })]);
  });

  it('the readonly role cannot change LLDP; rollback removes every leaf', async () => {
    const denied = await h.call(
      ro,
      'PATCH',
      '/api/v1/config',
      { services: { lldp: { enabled: false } } },
      MP,
    );
    expect(denied.status).toBe(403);
    const revs = await h.call(admin, 'GET', '/api/v1/config/revisions');
    expect(revs.status).toBe(200);
    const items = (revs.body.items ?? revs.body) as { id: number }[];
    const first = Math.min(...items.map((r) => r.id));
    const rb = await h.call(admin, 'POST', `/api/v1/config/rollback/${first}`);
    expect(rb.status).toBe(200);
    const n = await h.call(ro, 'GET', '/api/v1/state/lldp/neighbors');
    expect(n.body.total).toBe(0);
    const itf = await h.call(ro, 'GET', `/api/v1/config/interfaces/${BVI}`);
    expect(itf.status).toBe(404);
  });
});
