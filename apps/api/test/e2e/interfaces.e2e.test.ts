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
});
