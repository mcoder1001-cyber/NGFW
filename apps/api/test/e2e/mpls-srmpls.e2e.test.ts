import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { op: runSecret(), ro: runSecret() };
const MP = { 'content-type': 'application/merge-patch+json' };

/**
 * F-mpls-srmpls through the API on the host PostgreSQL with the fake agent: `routing.mpls` through the generic config
 * routes (candidate → diff → commit → rollback), the schema and semantic 400s with pointers (label 5, a duplicate
 * (table, label, eos), a missing SR policy), and the live state routes `/state/routing/mpls/{fib,tunnels}` (paged by
 * the agent's MplsState). Slot 5 names (loop5xxx, 10.5.0.0/16, labels 5×10000…).
 */
describe('F-mpls-srmpls e2e (PostgreSQL + fake agent)', () => {
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

  const mpls = {
    interfaces: ['loop5001'],
    tables: { '5001': {} },
    labelRoutes: [
      {
        table: 5001,
        label: 50016,
        paths: [{ nextHop: '10.5.1.2', interface: 'loop5001', outLabels: [50017, 50018] }],
      },
      { label: 50020, eos: false, paths: [{ interface: 't1', outLabels: [50021], weight: 2 }] },
    ],
    ipBindings: [{ label: 50040, vrf: 'red', prefix: '10.5.40.0/24' }],
    tunnels: {
      t1: { paths: [{ nextHop: '10.5.1.2', interface: 'loop5001', outLabels: [50050] }] },
    },
    sr: {
      policies: { '50100': { segmentLists: [{ labels: [50101, 50102] }] } },
      steering: [{ prefix: '10.5.60.0/24', bsid: 50100, vpnLabel: 50061 }],
    },
  };

  async function commitOrPatchError(body: unknown) {
    const r = await h.call(op, 'PATCH', '/api/v1/config/routing', body, MP);
    // PATCH validates the candidate: either here or at commit, always 400 with the pointer
    const res =
      r.status === 400 ? r : await h.call(op, 'POST', '/api/v1/config/commit?comment=bad');
    await h.call(op, 'POST', '/api/v1/config/discard');
    return res;
  }

  it('commits routing.mpls; the agent receives it with the defaults filled; rollback removes it', async () => {
    let r = await h.call(
      op,
      'PATCH',
      '/api/v1/config/interfaces',
      { loop5001: { ipv4: ['10.5.1.1/24'] } },
      MP,
    );
    expect(r.status, r.raw).toBe(200);
    r = await h.call(op, 'PATCH', '/api/v1/config/vrfs', { red: { id: 5010 } }, MP);
    expect(r.status, r.raw).toBe(200);
    r = await h.call(op, 'PATCH', '/api/v1/config/routing', { mpls }, MP);
    expect(r.status, r.raw).toBe(200);
    const d = await h.call(op, 'GET', '/api/v1/config/diff');
    expect(d.status).toBe(200);
    expect((d.body.changes as { pointer: string }[]).map((c) => c.pointer)).toEqual(
      expect.arrayContaining(['/routing/mpls']),
    );
    const c = await h.call(op, 'POST', '/api/v1/config/commit?comment=mpls');
    expect(c.status, c.raw).toBe(200);
    const rev = c.body.revision.id as number;
    const applied = (h.fake.current as Record<string, any>)['routing'].mpls;
    expect(applied.labelRoutes[0]).toEqual({
      table: 5001,
      label: 50016,
      eos: true,
      paths: [{ nextHop: '10.5.1.2', interface: 'loop5001', outLabels: [50017, 50018], weight: 1 }],
    });
    expect(applied.sr.steering[0]).toEqual({
      vrf: 'default',
      prefix: '10.5.60.0/24',
      bsid: 50100,
      vpnLabel: 50061,
    });
    expect(applied.tunnels.t1.l2Only).toBe(false);

    const rb = await h.call(op, 'POST', `/api/v1/config/rollback/${rev - 1}`);
    expect(rb.status, rb.raw).toBe(200);
    expect((h.fake.current as Record<string, any>)['routing']?.mpls).toBeUndefined();
    const again = await h.call(op, 'POST', `/api/v1/config/rollback/${rev}`);
    expect(again.status, again.raw).toBe(200);
  });

  it('label 5 → 400 problem+json with the pointer of the label', async () => {
    const res = await commitOrPatchError({
      mpls: { ...mpls, labelRoutes: [{ label: 5, paths: [{ interface: 'loop5001' }] }] },
    });
    expect(res.status, res.raw).toBe(400);
    expect(res.headers['content-type']).toContain('application/problem+json');
    expect(res.body.errors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ pointer: '/routing/mpls/labelRoutes/0/label' }),
      ]),
    );
  });

  it('a duplicate (table, label, eos) → 400 with the pointer of the second route', async () => {
    const res = await commitOrPatchError({
      mpls: {
        ...mpls,
        labelRoutes: [
          ...mpls.labelRoutes,
          { table: 5001, label: 50016, paths: [{ interface: 'loop5001' }] },
        ],
      },
    });
    expect(res.status, res.raw).toBe(400);
    expect(res.body.errors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ pointer: '/routing/mpls/labelRoutes/2/label' }),
      ]),
    );
  });

  it('steering into a missing SR policy → 400 with the pointer of the BSID', async () => {
    const res = await commitOrPatchError({
      mpls: { ...mpls, sr: { ...mpls.sr, steering: [{ prefix: '10.5.61.0/24', bsid: 50200 }] } },
    });
    expect(res.status, res.raw).toBe(400);
    expect(res.body.errors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ pointer: '/routing/mpls/sr/steering/0/bsid' }),
      ]),
    );
  });

  it('GET /state/routing/mpls/fib pages one table through the agent (read-only users included)', async () => {
    const r = await h.call(ro, 'GET', '/api/v1/state/routing/mpls/fib?table=0&page=2&pageSize=2');
    expect(r.status, r.raw).toBe(200);
    expect(r.body).toMatchObject({ page: 2, pageSize: 2, total: 6, tableId: 0 }); // 0, 1, 2 (VPP), 50020, binding 50040, BSID 50100
    expect(r.body.items.map((e: { label: number }) => e.label)).toEqual([2, 50020]);
    expect(r.body.tables).toEqual([
      { tableId: 0, name: 'vrx:0' },
      { tableId: 5001, name: `${h.fake.owner}:5001` },
    ]);
    const call = h.fake.calls.filter((x) => x.method === 'MplsState').at(-1);
    expect(call?.request).toMatchObject({ view: 'fib', tableId: 0, offset: 2, limit: 2 });

    const one = await h.call(ro, 'GET', '/api/v1/state/routing/mpls/fib?table=5001&label=50016');
    expect(one.status, one.raw).toBe(200);
    expect(one.body.items).toEqual([
      {
        label: 50016,
        eos: true,
        payload: 'ip4',
        paths: [
          {
            type: 'normal',
            proto: 'ip4',
            nextHop: '10.5.1.2',
            interface: 'loop5001',
            tableId: 0,
            outLabels: [50017, 50018],
            weight: 1,
            preference: 0,
          },
        ],
      },
    ]);
    expect((await h.call(ro, 'GET', '/api/v1/state/routing/mpls/fib?table=3001')).status).toBe(404);
    expect((await h.call(ro, 'GET', '/api/v1/state/routing/mpls/fib?label=5x')).status).toBe(400);
    expect(
      (await h.call(ro, 'GET', '/api/v1/state/routing/mpls/fib?page=101&pageSize=1000')).status,
    ).toBe(400);
    expect((await h.call(undefined, 'GET', '/api/v1/state/routing/mpls/fib')).status).toBe(401);
  });

  it('GET /state/routing/mpls/tunnels lists the configured tunnel', async () => {
    const r = await h.call(ro, 'GET', '/api/v1/state/routing/mpls/tunnels');
    expect(r.status, r.raw).toBe(200);
    expect(r.body.items).toEqual([
      expect.objectContaining({
        name: 't1',
        interface: 'mpls-tunnel0',
        owned: true,
        l2Only: false,
      }),
    ]);
  });
});
