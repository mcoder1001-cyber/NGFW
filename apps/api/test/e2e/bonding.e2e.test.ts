import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { installBondingFake, type BondingFake } from '../../src/features/bonding/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { ro: runSecret() };
const MP = { 'content-type': 'application/merge-patch+json' };

/**
 * F-bonding `GET /api/v1/state/interfaces/bonds` on the host PostgreSQL with the fake agent: bonds are configured
 * through the generic pointer routes, the live view comes from the BondState RPC (fake: features/bonding/fake.ts) and
 * is merged with the running and candidate configuration; semantic validation answers 400 problem+json with the
 * pointer of the offending membership. Slot names: fixture taps tap<slot>00…, bond ids from the slot's range.
 */
describe('state/interfaces/bonds e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let ro: string;
  let fake: BondingFake;
  let B0: string;
  let B1: string;
  let T0: string;
  let T1: string;
  let T2: string;

  beforeAll(async () => {
    h = await startHarness({});
    const slot = Number(/(\d+)$/.exec(h.prefix)?.[1] ?? '1');
    [B0, B1] = [`BondEthernet${slot * 1000}`, `BondEthernet${slot * 1000 + 1}`];
    [T0, T1, T2] = [0, 1, 2].map((i) => `tap${slot * 1000 + i}`) as [string, string, string];
    fake = await installBondingFake(h.fake);
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [{ username: 'ro1', role: 'readonly', password: PW.ro }]);
    ro = await h.login('ro1', PW.ro);
  });
  afterAll(async () => h?.close());

  it('commits an LACP bond through the pointer routes and serves its live state', async () => {
    const patch = {
      [B0]: {
        enabled: true,
        ipv4: ['10.6.10.1/24'],
        bond: {
          mode: 'lacp',
          loadBalance: 'l34',
          members: { [T0]: {}, [T1]: { passive: true } },
        },
      },
      [T0]: { enabled: true },
      [T1]: { enabled: true },
    };
    expect((await h.call(admin, 'PATCH', '/api/v1/config/interfaces', patch, MP)).status).toBe(200);
    // before the commit: the bond exists only in the candidate
    const before = await h.call(ro, 'GET', '/api/v1/state/interfaces/bonds');
    expect(before.status).toBe(200);
    expect(before.body).toMatchObject({
      live: true,
      items: [{ name: B0, state: null, running: null, hasPendingChange: true }],
    });
    expect(before.body.items[0].candidate).toMatchObject({ mode: 'lacp', loadBalance: 'l34' });

    const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=f-bonding');
    expect(c.status).toBe(200);
    const st = await h.call(ro, 'GET', '/api/v1/state/interfaces/bonds');
    expect(st.status).toBe(200);
    expect(st.body.items).toHaveLength(1);
    const item = st.body.items[0];
    expect(item).toMatchObject({
      name: B0,
      hasPendingChange: false,
      running: {
        mode: 'lacp',
        loadBalance: 'l34',
        numaOnly: false,
        members: {
          [T0]: { passive: false, longTimeout: false },
          [T1]: { passive: true, longTimeout: false },
        },
      },
      state: {
        vppName: B0,
        mode: 'lacp',
        loadBalance: 'l34',
        adminUp: true,
        memberCount: 2,
        activeMemberCount: 0,
      },
    });
    expect(item.state.members.map((m: { interface: string }) => m.interface)).toEqual([T0, T1]);
    expect(item.state.members[1]).toMatchObject({
      passive: true,
      lacp: {
        rxState: 'defaulted',
        muxState: 'detached',
        actor: { stateFlags: ['activity', 'timeout', 'aggregation', 'defaulted'] },
        partner: { system: '00:00:00:00:00:00', stateFlags: [] },
      },
    });
    // the agent was asked for its own owner's view
    expect(h.fake.calls.filter((x) => x.method === 'BondState').at(-1)?.request).toMatchObject({
      owner: h.prefix,
    });
    // the configuration itself is served by the generic routes
    const cfg = await h.call(ro, 'GET', `/api/v1/config/interfaces/${B0}/bond/members`);
    expect(cfg.status).toBe(200);
    expect(Object.keys(cfg.body)).toEqual([T0, T1]);
  });

  it('member already in another bond → 400 problem+json pointing at the second membership', async () => {
    const patch = {
      [B1]: { enabled: true, bond: { mode: 'active-backup', members: { [T1]: { weight: 10 } } } },
    };
    expect((await h.call(admin, 'PATCH', '/api/v1/config/interfaces', patch, MP)).status).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit');
    expect(c.status).toBe(400);
    expect(c.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect(c.body).toMatchObject({ status: 400, tier: 'semantic' });
    expect(c.body.errors).toContainEqual(
      expect.objectContaining({
        pointer: `/interfaces/${B1}/bond/members/${T1}`,
        message: expect.stringMatching(new RegExp(`already a member of ${B0}`)),
      }),
    );
    // the pending candidate shows up with its change flag; running is untouched
    const st = await h.call(ro, 'GET', '/api/v1/state/interfaces/bonds');
    expect(st.body.items.map((i: { name: string }) => i.name)).toEqual([B0, B1]);
    expect(st.body.items[1]).toMatchObject({
      name: B1,
      state: null,
      running: null,
      hasPendingChange: true,
    });
    expect((await h.call(admin, 'POST', '/api/v1/config/discard')).body).toEqual({
      discarded: true,
    });
  });

  it('mode-dependent options fail validation with their pointer', async () => {
    const patch = {
      [B1]: { bond: { mode: 'xor', loadBalance: 'l23', members: { [T2]: { weight: 5 } } } },
      [T2]: { enabled: true, ipv4: ['10.6.12.1/24'] },
    };
    expect((await h.call(admin, 'PATCH', '/api/v1/config/interfaces', patch, MP)).status).toBe(200);
    const v = await h.call(admin, 'POST', '/api/v1/config/validate');
    expect(v.status).toBe(400);
    const pointers = (v.body.errors as { pointer: string }[]).map((e) => e.pointer).sort();
    expect(pointers).toEqual([
      `/interfaces/${B1}/bond/members/${T2}/weight`,
      `/interfaces/${T2}/ipv4`,
    ]);
    await h.call(admin, 'POST', '/api/v1/config/discard');
  });

  it('rollback removes the bond from running and from the live view', async () => {
    const revs = await h.call(ro, 'GET', '/api/v1/config/revisions');
    expect(revs.status).toBe(200);
    const withBond = await h.call(ro, 'GET', '/api/v1/state/interfaces/bonds');
    expect(withBond.body.items).toHaveLength(1);
    // delete the bond (the member NICs stay configured as plain interfaces)
    expect((await h.call(admin, 'DELETE', `/api/v1/config/interfaces/${B0}`)).status).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=no-bond')).status).toBe(200);
    const after = await h.call(ro, 'GET', '/api/v1/state/interfaces/bonds');
    expect(after.body).toMatchObject({ live: true, items: [] });
    expect(fake.state()).toEqual([]);
  });

  it('an agent without BondState still lists the configured bonds (live: false)', async () => {
    const patch = {
      [B0]: { bond: { mode: 'round-robin', members: { [T0]: {} } } },
      [T0]: { enabled: true },
    };
    expect((await h.call(admin, 'PATCH', '/api/v1/config/interfaces', patch, MP)).status).toBe(200);
    fake.opts.unimplemented = true;
    try {
      const st = await h.call(ro, 'GET', '/api/v1/state/interfaces/bonds');
      expect(st.status).toBe(200);
      expect(st.body).toMatchObject({
        live: false,
        items: [{ name: B0, state: null, running: null, hasPendingChange: true }],
      });
    } finally {
      fake.opts.unimplemented = false;
      await h.call(admin, 'POST', '/api/v1/config/discard');
    }
  });

  it('requires authentication', async () => {
    expect((await h.call(undefined, 'GET', '/api/v1/state/interfaces/bonds')).status).toBe(401);
  });
});
