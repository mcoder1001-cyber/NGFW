import { NatSessionVariant } from '@ngfw/proto';
import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { fakeSession } from '../../src/features/nat44-ed-sessions/fake.js';
import { fakeNat64Session, natVariantsFake } from '../../src/features/nat44-ei-64-66-nptv6/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { op: runSecret(), ro: runSecret() };

/**
 * F-nat44-ei-64-66-nptv6 on the host PostgreSQL with the fake agent: NAT44-EI, NAT64 (the slot's /96, never
 * 64:ff9b::/96), NAT66 and NPTv6 through the generic pointer routes (candidate → commit; ED-only leaves with mode "ei"
 * and bad NPTv6 prefixes → 400 problem+json with the pointer), the paged EI and NAT64 session routes (the variant
 * reaches the agent), the NPTv6 state (running bindings, write-only marker) and the EI kill (reaches the agent's Action
 * with variant EI, audited, RBAC). Names and addresses follow the slot rules of slot 1.
 */
describe('nat44-ei / nat64 / nat66 / nptv6 e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let op: string;
  let ro: string;
  const mp = { 'content-type': 'application/merge-patch+json' };

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [
      { username: 'op1', role: 'operator', password: PW.op },
      { username: 'ro1', role: 'readonly', password: PW.ro },
    ]);
    op = await h.login('op1', PW.op);
    ro = await h.login('ro1', PW.ro);
    const ifs = await h.call(
      op,
      'PATCH',
      '/api/v1/config/interfaces',
      {
        'host-w1l0': { enabled: true, ipv4: ['10.1.1.1/24'], ipv6: ['fd00:1:1::1/64'] },
        'host-w1w0': { enabled: true, ipv4: ['10.1.2.1/24'], ipv6: ['fd00:1:2::1/64'] },
      },
      mp,
    );
    expect(ifs.status).toBe(200);
  });
  afterAll(async () => h?.close());

  const natDoc = {
    mode: 'ei',
    inside: ['host-w1l0'],
    outside: ['host-w1w0'],
    pools: [{ name: 'out', range: '10.1.2.100-10.1.2.103' }],
    staticMappings: [
      {
        name: 'web',
        protocol: 'tcp',
        local: { ip: '10.1.1.2', port: 80 },
        external: { ip: '10.1.2.110', port: 8080 },
      },
    ],
    nat64: {
      enabled: true,
      inside: ['host-w1l0'],
      outside: ['host-w1w0'],
      prefixes: [{ prefix: 'fd00:1:64::/96' }],
      pools: [{ range: '10.1.64.1-10.1.64.2' }],
    },
    nat66: {
      enabled: true,
      inside: ['host-w1l0'],
      outside: ['host-w1w0'],
      staticMappings: [{ local: 'fd00:1:1::66', external: 'fd00:1:2::66' }],
    },
    nptv6: {
      bindings: [
        {
          interface: 'host-w1w0',
          internal: 'fd00:1:10::/48',
          external: 'fd00:1:20::/48',
          description: 'site',
        },
      ],
    },
  };

  it('commits NAT44-EI, NAT64, NAT66 and NPTv6 through the generic nat routes', async () => {
    const p = await h.call(op, 'PATCH', '/api/v1/config/nat', natDoc, mp);
    expect(p.status).toBe(200);
    const c = await h.call(op, 'POST', '/api/v1/config/commit?comment=nat-ei');
    expect(c.status).toBe(200);
    expect(c.body.status).toBe('applied');
    const applied = h.fake.calls.filter((x) => x.method === 'Apply').at(-1)?.request as {
      desiredState: { nat?: Record<string, unknown> };
    };
    expect(applied.desiredState.nat).toMatchObject({
      mode: 'ei',
      nat64: { enabled: true, prefixes: [{ prefix: 'fd00:1:64::/96' }] },
      nat66: { enabled: true },
      nptv6: { bindings: [{ interface: 'host-w1w0', internal: 'fd00:1:10::/48' }] },
    });
  });

  it('mode "ei" with ED-only leaves, and NPTv6 prefixes of different lengths → 400 with the pointer', async () => {
    for (const [patch, pointer] of [
      [{ pools: [{ name: 'tn', range: '10.1.2.120', twiceNat: true }] }, '/nat/pools/0/twiceNat'],
      [
        {
          loadBalancedMappings: [
            {
              name: 'lb',
              protocol: 'tcp',
              external: { ip: '10.1.2.113', port: 443 },
              locals: [{ ip: '10.1.1.10', port: 8443 }],
            },
          ],
        },
        '/nat/loadBalancedMappings/0',
      ],
      [
        {
          nptv6: {
            bindings: [
              { interface: 'host-w1w0', internal: 'fd00:1:10::/48', external: 'fd00:1:20::/56' },
            ],
          },
        },
        '/nat/nptv6/bindings/0',
      ],
    ] as const) {
      expect((await h.call(op, 'PATCH', '/api/v1/config/nat', patch, mp)).status).toBe(200);
      const c = await h.call(op, 'POST', '/api/v1/config/commit');
      expect(c.status).toBe(400);
      expect(c.headers['content-type']).toMatch(/^application\/problem\+json/);
      expect(c.body.errors).toContainEqual(
        expect.objectContaining({ pointer: expect.stringMatching(new RegExp(`^${pointer}`)) }),
      );
      await h.call(op, 'POST', '/api/v1/config/discard');
    }
  });

  it('pages NAT44-EI sessions server-side with the EI variant', async () => {
    const st = natVariantsFake(h.fake);
    st.ei = [];
    for (let u = 0; u < 3; u++)
      for (let i = 0; i < 50; i++)
        st.ei.push(
          fakeSession({
            insideAddress: `10.1.1.${10 + u}`,
            insidePort: 10000 + i,
            outsideAddress: '10.1.2.100',
            outsidePort: 20000 + u * 100 + i,
          }),
        );
    const r = await h.call(ro, 'GET', '/api/v1/state/nat/ei/sessions?pageSize=100&page=2');
    expect(r.status).toBe(200);
    expect(r.body).toMatchObject({ page: 2, pageSize: 100, total: 150, totalUsers: 3 });
    expect(r.body.items).toHaveLength(50);
    expect(r.body.items[0]).toMatchObject({ insideAddress: '10.1.1.12', protocol: 'tcp' });
    const req = h.fake.calls.filter((x) => x.method === 'NatSessions').at(-1)?.request as {
      offset: number;
      limit: number;
      variant: number;
    };
    expect(req).toMatchObject({
      offset: 100,
      limit: 100,
      variant: NatSessionVariant.NAT_SESSION_VARIANT_EI,
    });
    const f = await h.call(ro, 'GET', '/api/v1/state/nat/ei/sessions?inside=10.1.1.11');
    expect(f.body.total).toBe(50);
    for (const bad of ['pageSize=1001', 'inside=10.1.1', 'port=70000']) {
      const b = await h.call(ro, 'GET', `/api/v1/state/nat/ei/sessions?${bad}`);
      expect(b.status, bad).toBe(400);
    }
    // the NAT44-ED route is unchanged (no variant)
    await h.call(ro, 'GET', '/api/v1/state/nat/sessions');
    const edReq = h.fake.calls.filter((x) => x.method === 'NatSessions').at(-1)?.request as {
      variant?: number;
    };
    expect(edReq.variant).toBeUndefined();
  });

  it('pages NAT64 sessions with the NAT64 variant (IPv6 client ↔ IPv4 pool ↔ IPv4 remote)', async () => {
    const st = natVariantsFake(h.fake);
    st.nat64 = [];
    for (let i = 0; i < 5; i++)
      st.nat64.push(
        fakeNat64Session({ insideAddress: `fd00:1::${10 + i}`, insidePort: 40000 + i }),
      );
    st.nat64.push(fakeNat64Session({ protocol: 'udp', externalPort: 53 }));
    const r = await h.call(ro, 'GET', '/api/v1/state/nat/nat64/sessions?pageSize=4');
    expect(r.status).toBe(200);
    expect(r.body).toMatchObject({ page: 1, pageSize: 4, total: 6 });
    expect(r.body.items).toHaveLength(4);
    expect(r.body.items[0]).toEqual({
      client: 'fd00:1::10',
      clientPort: 40000,
      poolAddress: '10.1.64.1',
      poolPort: 1024,
      remote: '10.1.2.2',
      remotePort: 80,
      remoteIpv6: 'fd00:1:64::a01:202',
      protocol: 'tcp',
      vrf: 'default',
      tableId: 0,
    });
    const u = await h.call(ro, 'GET', '/api/v1/state/nat/nat64/sessions?protocol=UDP');
    expect(u.body.total).toBe(1);
    const req = h.fake.calls.filter((x) => x.method === 'NatSessions').at(-1)?.request as {
      variant: number;
      filter: Record<string, unknown>;
    };
    expect(req).toMatchObject({
      variant: NatSessionVariant.NAT_SESSION_VARIANT_NAT64,
      filter: { protocol: 'udp' },
    });
    expect((await h.call(ro, 'GET', '/api/v1/state/nat/nat64/sessions?protocol=x')).status).toBe(
      400,
    );
  });

  it('shows the running NPTv6 bindings with the write-only marker', async () => {
    const r = await h.call(ro, 'GET', '/api/v1/state/nat/nptv6');
    expect(r.status).toBe(200);
    expect(r.body).toEqual({
      writeOnly: true,
      bindings: [
        {
          interface: 'host-w1w0',
          internal: 'fd00:1:10::/48',
          external: 'fd00:1:20::/48',
          description: 'site',
        },
      ],
    });
  });

  it('kills an EI session through the agent Action (variant EI), audited; RBAC and a bad body', async () => {
    const body = { protocol: 'tcp', insideAddress: '10.1.1.10', insidePort: 10005 };
    expect((await h.call(ro, 'POST', '/api/v1/actions/nat/ei/sessions/kill', body)).status).toBe(
      403,
    );
    const bad = await h.call(op, 'POST', '/api/v1/actions/nat/ei/sessions/kill', {
      ...body,
      insideAddress: 'fd00::1',
    });
    expect(bad.status).toBe(400);
    expect(bad.body.errors).toContainEqual(expect.objectContaining({ pointer: '/insideAddress' }));
    // the fake agent's generic Action answers UNIMPLEMENTED (F-nat44-ed-sessions Q5): the request reaches it with
    // the inside endpoint and variant EI, and the API maps the gRPC status (501)
    const k = await h.call(op, 'POST', '/api/v1/actions/nat/ei/sessions/kill', body);
    expect(k.status).toBe(501);
    const sent = h.fake.calls.filter((x) => x.method === 'Action').at(-1)?.request as {
      natSessionKill?: Record<string, unknown>;
    };
    expect(sent.natSessionKill).toEqual({
      ...body,
      externalAddress: '',
      externalPort: 0,
      vrf: '',
      variant: NatSessionVariant.NAT_SESSION_VARIANT_EI,
    });
    const audit = await h.db.execute(
      sql`select action, resource, result, status, username from audit_log where action like '%/actions/nat/ei/sessions/kill' order by id`,
    );
    const rows = (audit.rows as Record<string, unknown>[]).filter((r) => r['username'] === 'op1');
    expect(rows.map((r) => [r['result'], r['status']])).toEqual([
      ['failure', 400],
      ['failure', 501],
    ]);
    expect(rows.at(-1)).toMatchObject({
      action: 'POST /api/v1/actions/nat/ei/sessions/kill',
      resource: 'nat/ei/sessions/tcp/10.1.1.10:10005/default',
    });
  });
});
