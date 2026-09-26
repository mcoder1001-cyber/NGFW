import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { vrfStaticEcmpFakeState } from '../../src/features/vrf-static-ecmp/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { op: runSecret(), ro: runSecret() };
const MP = { 'content-type': 'application/merge-patch+json' };

/**
 * F-vrf-static-ecmp through the API on the host PostgreSQL with the fake agent: VRFs (+ source VRF select) and weighted
 * ECMP / blackhole / next-hop-VRF static routes through the generic config routes, candidate → diff → commit →
 * rollback, the semantic 400s with pointers, the FIB browser (`/state/routes`, paged by the agent's ListRoutes) and the
 * Action bridge (`/actions/ping`, `/actions/traceroute`). Slot 1 names (loop1xx, 10.1.0.0/16) as the other e2e suites.
 */
describe('F-vrf-static-ecmp e2e (PostgreSQL + fake agent)', () => {
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

  const config = {
    interfaces: {
      loop101: { enabled: true, vrf: 'red', ipv4: ['10.1.101.1/24'] },
      loop102: { enabled: true, ipv4: ['10.1.102.1/24'] },
    },
    vrfs: {
      red: {
        id: 1001,
        description: 'customer red',
        sourceSelect: [{ prefix: '10.1.50.0/24', interface: 'loop102' }],
      },
      blue: { id: 1002 },
    },
    routing: {
      static: [
        {
          prefix: '0.0.0.0/0',
          vrf: 'red',
          nextHops: [
            { address: '10.1.101.2', weight: 3 },
            { address: '10.1.101.3', weight: 1 },
          ],
        },
        {
          prefix: '10.1.60.0/24',
          vrf: 'red',
          nextHops: [{ address: '10.1.102.2', vrf: 'default' }],
        },
        { prefix: '10.1.70.0/24', vrf: 'blue', blackhole: true },
      ],
    },
  };

  it('commits VRFs, source VRF select and ECMP routes; the diff shows them; rollback removes them', async () => {
    for (const key of ['interfaces', 'vrfs', 'routing'] as const) {
      const r = await h.call(op, 'PATCH', `/api/v1/config/${key}`, config[key], MP);
      expect(r.status, r.raw).toBe(200);
    }
    const d = await h.call(op, 'GET', '/api/v1/config/diff');
    expect(d.status).toBe(200);
    const pointers = (d.body.changes as { pointer: string }[]).map((c) => c.pointer);
    expect(pointers).toEqual(expect.arrayContaining(['/vrfs/red', '/routing/static']));
    const c = await h.call(op, 'POST', '/api/v1/config/commit?comment=ecmp');
    expect(c.status, c.raw).toBe(200);
    const rev = c.body.revision.id as number;
    // the agent received the new fields (proto mirror: sourceSelect, nextHops[].vrf)
    const applied = h.fake.current as Record<string, any>;
    expect(applied['vrfs'].red.sourceSelect).toEqual([
      { prefix: '10.1.50.0/24', interface: 'loop102' },
    ]);
    expect(applied['routing'].static[1].nextHops[0].vrf).toBe('default');
    expect(applied['routing'].static[0].nextHops.map((n: { weight: number }) => n.weight)).toEqual([
      3, 1,
    ]);

    // rollback to the revision before: the VRFs and routes are gone from the agent
    const rb = await h.call(op, 'POST', `/api/v1/config/rollback/${rev - 1}`);
    expect(rb.status, rb.raw).toBe(200);
    expect((h.fake.current as Record<string, any>)['routing']?.static ?? []).toEqual([]);
    // and back for the state tests below
    const again = await h.call(op, 'POST', `/api/v1/config/rollback/${rev}`);
    expect(again.status, again.raw).toBe(200);
  });

  it('rejects a next hop in an undeclared VRF with 400 problem+json and a pointer', async () => {
    const r = await h.call(
      op,
      'PATCH',
      '/api/v1/config/routing',
      {
        static: [
          {
            prefix: '10.1.61.0/24',
            vrf: 'red',
            nextHops: [{ address: '10.1.102.2', vrf: 'nope' }],
          },
        ],
      },
      MP,
    );
    // PATCH validates the candidate: either here or at commit, always 400 with the pointer
    const res =
      r.status === 400 ? r : await h.call(op, 'POST', '/api/v1/config/commit?comment=bad');
    expect(res.status, res.raw).toBe(400);
    expect(res.headers['content-type']).toContain('application/problem+json');
    expect(res.body.errors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          pointer: '/routing/static/0/nextHops/0/vrf',
          message: "VRF 'nope' does not exist",
        }),
      ]),
    );
    await h.call(op, 'POST', '/api/v1/config/discard');
  });

  it('rejects a single-path weight and an unknown source-select interface', async () => {
    const r = await h.call(
      op,
      'PATCH',
      '/api/v1/config/vrfs/red',
      { sourceSelect: [{ prefix: '10.1.51.0/24', interface: 'nope0' }] },
      MP,
    );
    const res =
      r.status === 400 ? r : await h.call(op, 'POST', '/api/v1/config/commit?comment=bad');
    expect(res.status, res.raw).toBe(400);
    expect(res.body.errors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ pointer: '/vrfs/red/sourceSelect/0/interface' }),
      ]),
    );
    await h.call(op, 'POST', '/api/v1/config/discard');
  });

  it('serves the FIB browser page by page from the agent (ListRoutes), per VRF and over all VRFs', async () => {
    const red = await h.call(ro, 'GET', '/api/v1/state/routes?vrf=red&page=1&pageSize=10');
    expect(red.status, red.raw).toBe(200);
    expect(red.body).toMatchObject({ page: 1, pageSize: 10, vrf: 'red', tableId: 1001 });
    const def = (red.body.items as { prefix: string }[]).find((i) => i.prefix === '0.0.0.0/0');
    expect(def).toMatchObject({
      vrf: 'red',
      origin: 'static',
      source: 'API',
      nextHops: [{ address: '10.1.101.2' }, { address: '10.1.101.3' }],
      paths: [expect.objectContaining({ weight: 3 }), expect.objectContaining({ weight: 1 })],
    });
    const listed = h.fake.calls.filter((c) => c.method === 'ListRoutes').at(-1)?.request as Record<
      string,
      unknown
    >;
    expect(listed).toMatchObject({ vrf: 'red', offset: 0, limit: 10 });

    // page 2 of size 1 over red: the agent gets offset 1, limit 1
    const p2 = await h.call(ro, 'GET', '/api/v1/state/routes?vrf=red&page=2&pageSize=1');
    expect(p2.body.items).toHaveLength(1);
    expect(h.fake.calls.at(-1)?.request).toMatchObject({ offset: 1, limit: 1 });

    // every VRF (the CLI's `show ip route`): default + blue + red, connected and static
    const all = await h.call(ro, 'GET', '/api/v1/state/routes?page=1&pageSize=100');
    expect(all.status).toBe(200);
    const vrfs = new Set((all.body.items as { vrf: string }[]).map((i) => i.vrf));
    expect([...vrfs].sort()).toEqual(['blue', 'default', 'red']);
    expect(all.body.items).toContainEqual(
      expect.objectContaining({ vrf: 'default', prefix: '10.1.102.0/24', origin: 'connected' }),
    );
    expect(all.body.total).toBe((all.body.items as unknown[]).length);

    const bh = await h.call(ro, 'GET', '/api/v1/state/routes?vrf=blue&source=API');
    expect(bh.body.items).toEqual([
      expect.objectContaining({
        prefix: '10.1.70.0/24',
        paths: [expect.objectContaining({ type: 'drop' })],
      }),
    ]);
    expect((await h.call(ro, 'GET', '/api/v1/state/routes?vrf=nope')).status).toBe(404);
    expect((await h.call(ro, 'GET', '/api/v1/state/routes?vrf=red&source=bogus')).status).toBe(400);
    expect((await h.call(ro, 'GET', '/api/v1/state/routes?vrf=red&pageSize=5000')).status).toBe(
      400,
    );
    // review M2: the window is bounded before the offset becomes a uint32 (no wrap-around to a wrong page)
    const calls = h.fake.calls.length;
    const deep = await h.call(ro, 'GET', '/api/v1/state/routes?vrf=red&page=4296&pageSize=1000');
    expect(deep.status, deep.raw).toBe(400);
    expect(deep.body.errors[0]).toMatchObject({ pointer: '/page' });
    expect(h.fake.calls.length).toBe(calls);
    expect(
      (await h.call(ro, 'GET', '/api/v1/state/routes?vrf=red&page=100&pageSize=1000')).status,
    ).toBe(200);
    // review M3 (TD-2): filters are safe text
    const ctl = await h.call(ro, 'GET', '/api/v1/state/routes?vrf=red&source=API%1B%5B2J');
    expect(ctl.status).toBe(400);
    expect(ctl.body.errors[0]).toMatchObject({ pointer: '/source' });
  });

  it('bridges /actions/ping to the agent and maps the agent’s refusals', async () => {
    vrfStaticEcmpFakeState.pingReplies = () => 2;
    const r = await h.call(op, 'POST', '/api/v1/actions/ping', {
      target: '10.1.102.2',
      count: 3,
      intervalMs: 200,
    });
    expect(r.status, r.raw).toBe(200);
    expect(r.body).toMatchObject({
      action: 'ping',
      lines: [expect.stringContaining('3 packets transmitted, 2 received')],
      done: { exitCode: 0, stats: { transmitted: '3', received: '2', loss_pct: '33' } },
    });
    expect(h.fake.calls.at(-1)).toMatchObject({
      method: 'Action',
      request: { ping: { target: '10.1.102.2', count: 3, intervalMs: 200 } },
    });
    vrfStaticEcmpFakeState.pingReplies = undefined;

    const vrf = await h.call(op, 'POST', '/api/v1/actions/ping', {
      target: '10.1.102.2',
      vrf: 'red',
    });
    expect(vrf.status, vrf.raw).toBe(400);
    expect(vrf.body.errors).toEqual([expect.objectContaining({ pointer: '/vrf' })]);
    const src = await h.call(op, 'POST', '/api/v1/actions/ping', {
      target: '10.1.102.2',
      source: '10.1.102.1',
    });
    expect(src.body.errors).toEqual([expect.objectContaining({ pointer: '/source' })]);
    const bad = await h.call(op, 'POST', '/api/v1/actions/ping', { target: 'vrx.example' });
    expect(bad.status).toBe(400);
    expect(bad.body.errors[0].pointer).toBe('/target');

    // review M1: VPP with worker threads → the agent's FAILED_PRECONDITION becomes 409 problem+json
    vrfStaticEcmpFakeState.workers = 2;
    const wk = await h.call(op, 'POST', '/api/v1/actions/ping', { target: '10.1.102.2' });
    vrfStaticEcmpFakeState.workers = undefined;
    expect(wk.status, wk.raw).toBe(409);
    expect(wk.body.detail).toContain('worker thread');
    // review M3 (TD-2): control characters in the action name or the VRF are refused
    expect((await h.call(op, 'POST', '/api/v1/actions/ping%1B')).status).toBe(400);
    const cvrf = await h.call(op, 'POST', '/api/v1/actions/ping', {
      target: '10.1.102.2',
      vrf: 'red\u202e',
    });
    expect(cvrf.status).toBe(400);
    expect(cvrf.body.errors[0].pointer).toBe('/vrf');

    const tr = await h.call(op, 'POST', '/api/v1/actions/traceroute', { target: '10.1.102.2' });
    expect(tr.status, tr.raw).toBe(501);
    expect((await h.call(op, 'POST', '/api/v1/actions/reboot')).status).toBe(501);
    expect((await h.call(op, 'POST', '/api/v1/actions/nope')).status).toBe(404);
    // readonly users cannot run actions (operator role, audited as a mutation)
    expect(
      (await h.call(ro, 'POST', '/api/v1/actions/ping', { target: '10.1.102.2' })).status,
    ).toBe(403);
    const audit = await h.call(admin, 'GET', '/api/v1/audit?limit=50');
    expect(JSON.stringify(audit.body)).toContain('/api/v1/actions/ping');
    // review L4: the audit row says what was pinged
    expect(JSON.stringify(audit.body)).toContain('actions/ping');
    const rows = (audit.body.items ?? audit.body) as { resource?: string; after?: unknown }[];
    expect(rows).toContainEqual(
      expect.objectContaining({
        resource: 'actions/ping',
        after: expect.objectContaining({ target: '10.1.102.2', count: 3, intervalMs: 200 }),
      }),
    );
  });
});
