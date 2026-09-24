import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { ro: runSecret() };

/**
 * P08 `/api/v1/state/interfaces` (merged live + config view) and `/state/interfaces/{name}/counters` on the host
 * PostgreSQL with the fake agent: live state from the InterfaceState RPC, Retrieve (`config`, its pre-P08 meaning,
 * D-105), the running configuration (`running`), counters and `hasPendingChange` from the candidate. Names follow the slot rules (host-w1l0 / w1w0, 10.1.0.0/16).
 */
describe('state/interfaces e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let ro: string;
  const L = 'host-w1l0';
  const W = 'host-w1w0';

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [{ username: 'ro1', role: 'readonly', password: PW.ro }]);
    ro = await h.login('ro1', PW.ro);
  });
  afterAll(async () => h?.close());

  it('merges live state, running config, Retrieve and counters; flags pending changes', async () => {
    const mp = { 'content-type': 'application/merge-patch+json' };
    expect(
      (
        await h.call(
          admin,
          'PATCH',
          '/api/v1/config/interfaces',
          {
            [L]: { enabled: true, mtu: 1400, description: 'lan', ipv4: ['10.1.1.1/24'] },
            [W]: {
              enabled: true,
              ipv4: ['10.1.2.1/24'],
              subinterfaces: { '100': { vlanId: 100, enabled: true, ipv4: ['10.1.100.1/24'] } },
            },
          },
          mp,
        )
      ).status,
    ).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=p08')).status).toBe(200);

    const st = await h.call(ro, 'GET', '/api/v1/state/interfaces');
    expect(st.status).toBe(200);
    const by = new Map(
      (st.body.items as { name: string }[]).map((i) => [i.name, i as Record<string, unknown>]),
    );
    expect([...by.keys()]).toEqual([L, W, `${W}.100`]);
    expect(by.get(L)).toMatchObject({
      kind: 'interface',
      parent: null,
      state: { adminUp: true, mtu: 1400, ipv4: ['10.1.1.1/24'], description: 'lan' },
      config: { enabled: true, mtu: 1400, ipv4: ['10.1.1.1/24'] }, // Retrieve view (as before P08)
      running: { enabled: true, mtu: 1400, description: 'lan', ipv4: ['10.1.1.1/24'] },
      counters: { name: L },
      hasPendingChange: false,
    });
    expect(by.get(L)).not.toHaveProperty('actual'); // dropped in fix round 1: it duplicated `config`

    // Retrieve drifts from running (VPP changed behind the API): `config` is the Retrieve view and `running` the
    // running configuration (D-105). The fake's Retrieve echoes the applied document, so without the drift the two
    // fields are equal and swapping them in the controller would pass (re-review T2/R2).
    const retrieved = h.fake.current['interfaces'] as Record<string, Record<string, unknown>>;
    const before = retrieved[L];
    retrieved[L] = { ...before, mtu: 9000 };
    try {
      const drift = await h.call(ro, 'GET', '/api/v1/state/interfaces');
      expect(drift.status).toBe(200);
      const d = (drift.body.items as { name: string }[]).find((i) => i.name === L) as {
        config: { mtu?: number };
        running: { mtu?: number };
        hasPendingChange: boolean;
      };
      expect(d.config.mtu).toBe(9000);
      expect(d.running.mtu).toBe(1400);
      expect(d.hasPendingChange).toBe(false); // running vs candidate, not vs the data plane
    } finally {
      retrieved[L] = before;
    }

    expect(by.get(W)).toMatchObject({ config: { subinterfaces: { '100': { vlanId: 100 } } } });
    expect(by.get(`${W}.100`)).toMatchObject({
      kind: 'subinterface',
      parent: W,
      state: { type: 'sub-interface', vlanId: 100, parent: W },
      config: { vlanId: 100, ipv4: ['10.1.100.1/24'] },
      running: { vlanId: 100, enabled: true, ipv4: ['10.1.100.1/24'] },
      hasPendingChange: false,
    });

    // an uncommitted change marks exactly that interface (and not its neighbour)
    await h.call(admin, 'PATCH', `/api/v1/config/interfaces/${L}`, { mtu: 1500 }, mp);
    const st2 = await h.call(ro, 'GET', '/api/v1/state/interfaces');
    const pending = (st2.body.items as { name: string; hasPendingChange: boolean }[])
      .filter((i) => i.hasPendingChange)
      .map((i) => i.name);
    expect(pending).toEqual([L]);
    // a candidate edit changes neither the Retrieve view nor the running configuration
    const l2 = (st2.body.items as { name: string }[]).find((i) => i.name === L) as Record<string, unknown>;
    expect(l2).toMatchObject({ config: { mtu: 1400 }, running: { mtu: 1400 } });
    expect((await h.call(admin, 'POST', '/api/v1/config/discard')).status).toBe(200);

    const c = await h.call(ro, 'GET', `/api/v1/state/interfaces/${L}/counters`);
    expect(c.status).toBe(200);
    expect(c.body).toMatchObject({ name: L, vppName: L, counters: { name: L } });
    expect(typeof c.body.counters.rxPackets).toBe('string'); // 64-bit counters are strings (D-039)
    expect((await h.call(ro, 'GET', '/api/v1/state/interfaces/nosuch0/counters')).status).toBe(404);
    expect((await h.call(ro, 'GET', '/api/v1/state/interfaces/..%2Fx/counters')).status).toBe(400);
  });

  it('lists live rows the agent does not manage and configured rows VPP lacks; an older agent still lists Retrieve (review N6)', async () => {
    // the configuration of the first test is committed: host-w1l0, host-w1w0, host-w1w0.100
    h.fake.liveExtra = [{ name: 'local0', type: 'local', adminUp: false, linkUp: false }];
    h.fake.liveMissing = new Set([W]);
    const st = await h.call(ro, 'GET', '/api/v1/state/interfaces');
    expect(st.status).toBe(200);
    const by = new Map(
      (st.body.items as { name: string }[]).map((i) => [i.name, i as Record<string, unknown>]),
    );
    expect([...by.keys()]).toEqual([L, W, `${W}.100`, 'local0']);
    expect(by.get('local0')).toMatchObject({
      kind: 'interface',
      parent: null,
      state: { managed: false, type: 'local', adminUp: false },
      config: null,
      running: null,
      hasPendingChange: false,
    });
    expect(by.get(W)).toMatchObject({ state: null, config: { enabled: true }, running: { enabled: true } });

    // an agent that predates the InterfaceState RPC answers UNIMPLEMENTED: the list comes from Retrieve alone
    h.fake.liveExtra = [];
    h.fake.liveMissing = new Set();
    h.fake.interfaceStateUnimplemented = true;
    try {
      const old = await h.call(ro, 'GET', '/api/v1/state/interfaces');
      expect(old.status).toBe(200);
      const items = old.body.items as { name: string; state: unknown; config: unknown }[];
      expect(items.map((i) => i.name)).toEqual([L, W, `${W}.100`]);
      expect(items.every((i) => i.state === null)).toBe(true);
      expect(items[0]).toMatchObject({ config: { enabled: true, mtu: 1400 } });
      const c = await h.call(ro, 'GET', `/api/v1/state/interfaces/${L}/counters`);
      expect(c.status).toBe(200); // falls back to the logical name as the VPP name
    } finally {
      h.fake.interfaceStateUnimplemented = false;
    }
  });
});
