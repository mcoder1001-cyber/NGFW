import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { installBridgeL2Fake, type BridgeL2Fake } from '../../src/features/bridge-l2/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { ro: runSecret() };
const MP = { 'content-type': 'application/merge-patch+json' };

/**
 * F-bridge-l2 on the host PostgreSQL with the fake agent: configuration through the generic pointer routes
 * (`interfaces.<if>.l2`, `routing.l2`), the semantic 400 for a second membership, and the two state routes served
 * from the BridgeDomainState / BridgeDomainMacs RPCs (`/state/l2/bridge-domains`, `…/{id}/macs`, server-side
 * paged). Names follow the slot rules (host-<prefix>l0 / w0, loop<N>xxx, bridge-domain ids N000–N999).
 */
describe('state/l2 bridge domains e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let ro: string;
  let fake: BridgeL2Fake;
  let L: string;
  let W: string;
  const BVI = 'loop7000';

  beforeAll(async () => {
    h = await startHarness({});
    L = `host-${h.prefix}l0`;
    W = `host-${h.prefix}w0`;
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [{ username: 'ro1', role: 'readonly', password: PW.ro }]);
    ro = await h.login('ro1', PW.ro);
  });
  afterAll(async () => h?.close());

  it('an agent without the RPCs answers 501', async () => {
    const r = await h.call(ro, 'GET', '/api/v1/state/l2/bridge-domains');
    expect(r.status).toBe(501);
    expect(r.headers['content-type']).toMatch(/^application\/problem\+json/);
  });

  it('a second membership of one interface is a 400 problem+json with its pointer', async () => {
    fake = await installBridgeL2Fake(h.fake, { learned: { 7001: 3 } });
    const patch = await h.call(
      admin,
      'PATCH',
      '/api/v1/config',
      {
        interfaces: { [L]: { enabled: true, l2: { bridgeDomain: 'lan' } } },
        routing: {
          l2: { bridgeDomains: { lan: { id: 7001 } }, xconnects: { [L]: { tx: W } } },
        },
      },
      MP,
    );
    expect(patch.status).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit');
    expect(c.status).toBe(400);
    expect(c.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect(c.body).toMatchObject({ status: 400, tier: 'semantic' });
    expect(c.body.errors).toContainEqual(
      expect.objectContaining({
        pointer: `/routing/l2/xconnects/${L}`,
        message: expect.stringMatching(/already a member of bridge domain 'lan'/),
      }),
    );
    expect((await h.call(admin, 'POST', '/api/v1/config/discard')).body).toEqual({
      discarded: true,
    });
  });

  it('commits a bridge domain and serves its live state merged with running and candidate', async () => {
    const patch = await h.call(
      admin,
      'PATCH',
      '/api/v1/config',
      {
        interfaces: {
          [BVI]: { enabled: true, ipv4: ['10.7.0.1/24'], l2: { bridgeDomain: 'lan', bvi: true } },
          [L]: {
            enabled: true,
            l2: { bridgeDomain: 'lan' },
            subinterfaces: {
              '100': {
                vlanId: 100,
                enabled: true,
                l2: { bridgeDomain: 'lan', shg: 1, tagRewrite: { op: 'pop-1' } },
              },
            },
          },
        },
        routing: {
          l2: {
            bridgeDomains: {
              lan: {
                id: 7001,
                macAgeMin: 5,
                staticMacs: [{ mac: '02:00:00:00:70:01', interface: L }],
              },
            },
          },
        },
      },
      MP,
    );
    expect(patch.status).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=bridge-l2');
    expect(c.status).toBe(200);

    const st = await h.call(ro, 'GET', '/api/v1/state/l2/bridge-domains');
    expect(st.status).toBe(200);
    expect(st.body.items).toHaveLength(1);
    expect(st.body.items[0]).toMatchObject({
      name: 'lan',
      id: 7001,
      hasPendingChange: false,
      running: { id: 7001, macAgeMin: 5, flood: true, learn: true },
      state: {
        id: 7001,
        bvi: BVI,
        macAgeMin: 5,
        learnedMacs: 3,
        staticMacs: 2,
        members: expect.arrayContaining([
          expect.objectContaining({ interface: BVI, portType: 'bvi' }),
          expect.objectContaining({ interface: `${L}.100`, shg: 1, tagRewrite: 'pop-1' }),
        ]),
      },
    });

    // a candidate edit shows as pending on the row
    await h.call(
      admin,
      'PATCH',
      '/api/v1/config/routing/l2/bridgeDomains/lan',
      { macAgeMin: 10 },
      MP,
    );
    const pend = await h.call(ro, 'GET', '/api/v1/state/l2/bridge-domains');
    expect(pend.body.items[0]).toMatchObject({
      name: 'lan',
      hasPendingChange: true,
      running: { macAgeMin: 5 },
    });
    await h.call(admin, 'POST', '/api/v1/config/discard');
  });

  it('pages the MAC table server-side and validates the page parameters', async () => {
    const p1 = await h.call(
      ro,
      'GET',
      '/api/v1/state/l2/bridge-domains/7001/macs?page=1&pageSize=3',
    );
    expect(p1.status).toBe(200);
    expect(p1.body).toMatchObject({ page: 1, pageSize: 3, total: 4 });
    expect(p1.body.items).toHaveLength(3);
    const p2 = await h.call(
      ro,
      'GET',
      '/api/v1/state/l2/bridge-domains/7001/macs?page=2&pageSize=3',
    );
    expect(p2.body.items).toHaveLength(1);
    const macs = [...p1.body.items, ...p2.body.items].map((m: { mac: string }) => m.mac);
    expect(macs).toEqual([...macs].sort());
    expect(macs).toContain('02:00:00:00:70:01');
    const call = h.fake.calls.filter((x) => x.method === 'BridgeDomainMacs').at(-1);
    expect(call?.request).toMatchObject({ bdId: 7001, offset: 3, limit: 3, owner: h.prefix });

    expect((await h.call(ro, 'GET', '/api/v1/state/l2/bridge-domains/7999/macs')).status).toBe(404);
    expect(
      (await h.call(ro, 'GET', '/api/v1/state/l2/bridge-domains/7001/macs?pageSize=1001')).status,
    ).toBe(400);
    expect((await h.call(ro, 'GET', '/api/v1/state/l2/bridge-domains/0/macs')).status).toBe(400);
    expect((await h.call(undefined, 'GET', '/api/v1/state/l2/bridge-domains')).status).toBe(401);
  });

  it('rollback returns the members to L3 and removes the bridge domain', async () => {
    const revs = await h.call(admin, 'GET', '/api/v1/config/revisions');
    expect(revs.status).toBe(200);
    const items = (revs.body.items ?? revs.body) as { id: number }[];
    const first = Math.min(...items.map((r) => r.id));
    const rb = await h.call(admin, 'POST', `/api/v1/config/rollback/${first}`);
    expect(rb.status).toBe(200);
    const st = await h.call(ro, 'GET', '/api/v1/state/l2/bridge-domains');
    expect(st.body.items).toEqual([]);
    expect(fake.state()).toEqual([]);
  });
});
