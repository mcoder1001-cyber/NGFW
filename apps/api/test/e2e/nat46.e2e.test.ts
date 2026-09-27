import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { ro: runSecret() };

/**
 * F-nat46 on the host PostgreSQL with the fake agent: `nat.nat46` through the generic pointer routes (candidate →
 * commit reaches the agent's Apply), validation failures → 400 problem+json with the pointer (duplicate IPv4 service
 * address, a non-/96 client prefix, overlap with a nat.map domain, an unknown interface), and the read-only state
 * routes (running mappings, RFC 6052 client address; RBAC readonly may read). Slot 1 names and addresses; the client
 * prefix is the slot's /96, never 64:ff9b::/96 (shared-host rules).
 */
describe('nat46 e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let ro: string;
  const mp = { 'content-type': 'application/merge-patch+json' };

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [{ username: 'ro1', role: 'readonly', password: PW.ro }]);
    ro = await h.login('ro1', PW.ro);
    const ifs = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/interfaces',
      {
        'host-w1l0': { enabled: true, ipv4: ['10.1.1.1/24'] },
        'host-w1w0': { enabled: true, ipv6: ['fd00:1:2::1/64'] },
      },
      mp,
    );
    expect(ifs.status).toBe(200);
  });
  afterAll(async () => h?.close());

  const nat46 = {
    clientPrefix: 'fd00:1:46::/96',
    interfaces: ['host-w1l0', 'host-w1w0'],
    mappings: [{ name: 'web', ipv4: '10.1.2.80', ipv6: 'fd00:1:2::80' }],
  };

  it('commits nat.nat46 through the generic nat routes', async () => {
    expect((await h.call(admin, 'PATCH', '/api/v1/config/nat', { nat46 }, mp)).status).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=nat46');
    expect(c.status).toBe(200);
    expect(c.body.status).toBe('applied');
    const applied = h.fake.calls.filter((x) => x.method === 'Apply').at(-1)?.request as {
      desiredState: { nat?: Record<string, unknown> };
    };
    expect(applied.desiredState.nat).toMatchObject({
      nat46: { clientPrefix: 'fd00:1:46::/96', mappings: [{ name: 'web', ipv4: '10.1.2.80' }] },
    });
  });

  it('refuses invalid NAT46 with 400 problem+json and the pointer', async () => {
    const dup = {
      nat46: {
        ...nat46,
        mappings: [...nat46.mappings, { name: 'web2', ipv4: '10.1.2.80', ipv6: 'fd00:1:2::81' }],
      },
    };
    const mapOverlap = {
      map: {
        domains: [
          {
            name: 'mt',
            mode: 'map-t',
            ipv4Prefix: '10.1.2.0/24',
            ipv6Prefix: 'fd00:1:64::/48',
            ipv6Source: 'fd00:1:ff::/96',
            eaBitsLength: 8,
          },
        ],
      },
    };
    for (const [patch, pointer] of [
      [dup, '/nat/nat46/mappings/1/ipv4'],
      [{ nat46: { ...nat46, clientPrefix: 'fd00:1:46::/64' } }, '/nat/nat46/clientPrefix'],
      [mapOverlap, '/nat/nat46/mappings/0/ipv4'],
      [{ nat46: { ...nat46, interfaces: ['host-w1x9'] } }, '/nat/nat46/interfaces/0'],
    ] as const) {
      expect((await h.call(admin, 'PATCH', '/api/v1/config/nat', patch, mp)).status).toBe(200);
      const c = await h.call(admin, 'POST', '/api/v1/config/commit');
      expect(c.status, pointer).toBe(400);
      expect(c.headers['content-type']).toMatch(/^application\/problem\+json/);
      expect(c.body.errors).toContainEqual(expect.objectContaining({ pointer }));
      await h.call(admin, 'POST', '/api/v1/config/discard');
    }
  });

  it('reads the running mappings and the client address (readonly may read)', async () => {
    const r = await h.call(ro, 'GET', '/api/v1/state/nat/nat46');
    expect(r.status).toBe(200);
    expect(r.body).toEqual({
      configured: true,
      clientPrefix: 'fd00:1:46::/96',
      interfaces: ['host-w1l0', 'host-w1w0'],
      mappings: [
        { name: 'web', domain: 'nat46-web', ipv4: '10.1.2.80', ipv6: 'fd00:1:2::80', mtu: null },
      ],
    });
    const c = await h.call(ro, 'GET', '/api/v1/state/nat/nat46/client?ipv4=10.1.1.2');
    expect(c.status).toBe(200);
    expect(c.body).toEqual({
      ipv4: '10.1.1.2',
      clientPrefix: 'fd00:1:46::/96',
      ipv6: 'fd00:1:46::a01:102',
    });
    expect((await h.call(ro, 'GET', '/api/v1/state/nat/nat46/client?ipv4=fd00::1')).status).toBe(400);
    expect((await h.call(ro, 'GET', '/api/v1/state/nat/nat46/client')).status).toBe(400);
  });

  it('refuses unauthenticated reads', async () => {
    expect((await h.call(undefined, 'GET', '/api/v1/state/nat/nat46')).status).toBe(401);
  });
});
