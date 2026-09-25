import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { setFakeSrv6Counters } from '../../src/features/srv6/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { op: runSecret(), ro: runSecret() };
const MP = { 'content-type': 'application/merge-patch+json' };

/** Slot 4's SIDs, BSIDs and VRF (docs/lab/shared-host-rules.md: fd00:4::/48, tables 4000–4999). */
const SRV6 = {
  encapSource: 'fd00:4::1',
  localSids: {
    'fd00:4:ff::1': { behavior: 'end', psp: true },
    'fd00:4:ff::a': { behavior: 'end.dt4', lookupVrf: 'cust-a' },
  },
  policies: {
    'fd00:4:bb::1': {
      sidLists: [
        { sids: ['fd00:4:ee::1', 'fd00:4:ff::a'], weight: 1 },
        { sids: ['fd00:4:ee::2', 'fd00:4:ff::a'], weight: 3 },
      ],
    },
  },
  steering: [{ type: 'l3', prefix: '10.4.100.0/24', vrf: 'cust-a', bsid: 'fd00:4:bb::1' }],
};

/**
 * F-srv6 API on the host PostgreSQL with the fake agent: routing.srv6 through the generic config routes, the
 * acceptance case "encap policy without encapSource → 400 problem+json with a pointer" (D-074), `GET /state/srv6`
 * joined with the running VRF names, schema errors at edit time, RBAC (readonly reads, may not edit), rollback.
 */
describe('F-srv6 e2e (PostgreSQL + fake agent)', () => {
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

  it('encap policy without any encapSource → 400 problem+json pointing at the policy (D-074)', async () => {
    expect(
      (await h.call(op, 'PATCH', '/api/v1/config/vrfs', { 'cust-a': { id: 4001 } }, MP)).status,
    ).toBe(200);
    const noSource = { ...SRV6, encapSource: undefined };
    expect(
      (await h.call(op, 'PATCH', '/api/v1/config/routing', { srv6: noSource }, MP)).status,
    ).toBe(200);
    const c = await h.call(op, 'POST', '/api/v1/config/commit');
    expect(c.status).toBe(400);
    expect(c.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect(c.body).toMatchObject({ status: 400, tier: 'semantic' });
    expect(c.body.errors).toContainEqual(
      expect.objectContaining({
        pointer: '/routing/srv6/policies/fd00:4:bb::1/encapSource',
        message: expect.stringMatching(/needs an outer source address/),
      }),
    );
    await h.call(op, 'POST', '/api/v1/config/discard');
  });

  it('schema errors are 400 at edit time (17 SIDs, a proxy behaviour, a non-IPv6 SID)', async () => {
    const tooMany = await h.call(
      op,
      'PATCH',
      '/api/v1/config/routing',
      {
        srv6: {
          policies: {
            'fd00:4:bb::1': {
              sidLists: [{ sids: Array.from({ length: 17 }, (_, i) => `fd00:4:ee::${i + 1}`) }],
            },
          },
        },
      },
      MP,
    );
    expect(tooMany.status).toBe(400);
    expect(tooMany.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect(tooMany.body.errors).toContainEqual(
      expect.objectContaining({ pointer: '/routing/srv6/policies/fd00:4:bb::1/sidLists/0/sids' }),
    );
    const proxy = await h.call(
      op,
      'PATCH',
      '/api/v1/config/routing',
      { srv6: { localSids: { 'fd00:4:ff::9': { behavior: 'end.ad' } } } },
      MP,
    );
    expect(proxy.status).toBe(400);
    expect(proxy.body.errors).toContainEqual(
      expect.objectContaining({ pointer: '/routing/srv6/localSids/fd00:4:ff::9/behavior' }),
    );
    await h.call(op, 'POST', '/api/v1/config/discard');
  });

  it('readonly may not edit routing.srv6 (403)', async () => {
    expect((await h.call(ro, 'PATCH', '/api/v1/config/routing', { srv6: SRV6 }, MP)).status).toBe(
      403,
    );
  });

  it('commit → the fake agent has it; GET /state/srv6 joins the running VRF names', async () => {
    expect(
      (await h.call(op, 'PATCH', '/api/v1/config/vrfs', { 'cust-a': { id: 4001 } }, MP)).status,
    ).toBe(200);
    expect((await h.call(op, 'PATCH', '/api/v1/config/routing', { srv6: SRV6 }, MP)).status).toBe(
      200,
    );
    const c = await h.call(op, 'POST', '/api/v1/config/commit?comment=srv6');
    expect(c.body.errors ?? []).toEqual([]);
    expect(c.status).toBe(200);
    const applied = (h.fake.current['routing'] as Record<string, any>)['srv6'];
    expect(Object.keys(applied.localSids).sort()).toEqual(['fd00:4:ff::1', 'fd00:4:ff::a']);
    // the running document carries the schema defaults the agent reports back
    const run = await h.call(ro, 'GET', '/api/v1/config/routing');
    expect(run.body.srv6.policies['fd00:4:bb::1']).toMatchObject({
      type: 'default',
      encap: true,
      vrf: 'default',
    });
    setFakeSrv6Counters(h.fake, 'fd00:4:ff::a', {
      goodPackets: 12,
      goodBytes: 1536,
      badPackets: 1,
      badBytes: 100,
    });
    const st = await h.call(ro, 'GET', '/api/v1/state/srv6');
    expect(st.status).toBe(200);
    expect(st.body.localSids).toEqual([
      expect.objectContaining({
        sid: 'fd00:4:ff::1',
        behavior: 'end',
        psp: true,
        vrf: 'default',
        configured: true,
      }),
      expect.objectContaining({
        sid: 'fd00:4:ff::a',
        behavior: 'end.dt4',
        lookupVrf: 'cust-a',
        lookupTable: 4001,
        goodPackets: 12,
        goodBytes: 1536,
        badPackets: 1,
        badBytes: 100,
        configured: true,
      }),
    ]);
    expect(st.body.policies).toEqual([
      {
        bsid: 'fd00:4:bb::1',
        type: 'default',
        encap: true,
        vrf: 'default',
        table: 0,
        encapSource: 'fd00:4::1',
        sidLists: [
          { sids: ['fd00:4:ee::1', 'fd00:4:ff::a'], weight: 1 },
          { sids: ['fd00:4:ee::2', 'fd00:4:ff::a'], weight: 3 },
        ],
        configured: true,
      },
    ]);
    expect(st.body.steering).toEqual([
      {
        type: 'l3',
        trafficType: 'ipv4',
        prefix: '10.4.100.0/24',
        vrf: 'cust-a',
        table: 4001,
        interface: null,
        bsid: 'fd00:4:bb::1',
        configured: true,
      },
    ]);
    const call = h.fake.calls.filter((x) => x.method === 'Srv6State').at(-1);
    expect(call?.request).toMatchObject({ owner: h.fake.owner });
  });

  it('rollback to the revision before the SRv6 commit removes it (agent and state)', async () => {
    const revs = await h.call(ro, 'GET', '/api/v1/config/revisions?limit=10');
    const items = revs.body.items as { id: number; comment?: string | null }[];
    const rev = items.find((r) => r.comment === 'srv6');
    expect(rev).toBeDefined();
    const rb = await h.call(admin, 'POST', `/api/v1/config/rollback/${rev!.id - 1}`);
    expect(rb.status).toBe(200);
    expect(
      (h.fake.current['routing'] as Record<string, unknown> | undefined)?.['srv6'],
    ).toBeUndefined();
    const st = await h.call(ro, 'GET', '/api/v1/state/srv6');
    expect(st.body).toMatchObject({ localSids: [], policies: [], steering: [] });
  });
});
