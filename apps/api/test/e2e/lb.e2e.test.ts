import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { lbFake } from '../../src/features/lb/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { ro: runSecret(), op: runSecret() };

/**
 * F-lb on the host PostgreSQL with the fake agent: `services.lb` through the generic pointer routes (schema rules →
 * 400 problem+json with a pointer), `GET /api/v1/state/lb/vips` (agent LbState merged with the running configuration)
 * and `POST /api/v1/actions/lb/vips/{name}/flush` (agent LbFlushVip; audited). Slot 2 rules: VIPs 10.2.250.0/24,
 * servers 10.2.2.0/24, the NAT feature on host-w2l0.
 */
describe('F-lb e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let ro: string;
  let op: string;
  const mp = { 'content-type': 'application/merge-patch+json' };
  const L = 'host-w2l0';

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [
      { username: 'ro1', role: 'readonly', password: PW.ro },
      { username: 'op1', role: 'operator', password: PW.op },
    ]);
    ro = await h.login('ro1', PW.ro);
    op = await h.login('op1', PW.op);
  });
  afterAll(async () => h?.close());

  it('GRE4 VIP with an IPv6 server → 400 problem+json with a pointer (acceptance)', async () => {
    const r = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/services',
      {
        lb: {
          vips: {
            web: {
              prefix: '10.2.250.1/32',
              protocol: 'tcp',
              port: 80,
              encap: 'gre4',
              servers: [{ address: '2001:db8:2::10' }],
            },
          },
        },
      },
      mp,
    );
    expect(r.status).toBe(400);
    expect(String(r.headers['content-type'])).toContain('application/problem+json');
    expect(r.body.errors).toContainEqual({
      pointer: '/services/lb/vips/web/servers/0/address',
      message: 'encap gre4 needs IPv4 application servers, 2001:db8:2::10 is IPv6',
    });
  });

  it('commits services.lb, reports the VIPs live, flushes one; errors are problem+json', async () => {
    const patch = {
      lb: {
        vips: {
          web: {
            prefix: '10.2.250.1/32',
            protocol: 'tcp',
            port: 80,
            encap: 'gre4',
            servers: [{ address: '10.2.2.10' }, { address: '10.2.2.11', flushOnDelete: true }],
          },
          dns: {
            prefix: '10.2.250.3/32',
            protocol: 'udp',
            port: 53,
            encap: 'nat4',
            targetPort: 5353,
            servers: [{ address: '10.2.2.13' }],
          },
        },
        natInterfaces: [{ interface: L, family: 'ip4' }],
      },
    };
    expect(
      (
        await h.call(
          admin,
          'PATCH',
          '/api/v1/config/interfaces',
          { [L]: { ipv4: ['10.2.1.1/24'] } },
          mp,
        )
      ).status,
    ).toBe(200);
    expect((await h.call(admin, 'PATCH', '/api/v1/config/services', patch, mp)).status).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=f-lb')).status).toBe(200);

    lbFake(h.fake).opts = { removed: { web: ['10.2.2.9'] } };
    const st = await h.call(ro, 'GET', '/api/v1/state/lb/vips');
    expect(st.status).toBe(200);
    expect(st.body.totalVppVips).toBe(3);
    const items = st.body.items as Record<string, unknown>[];
    expect(items.map((i) => i['name'])).toEqual(['dns', 'web']);
    expect(items[1]).toMatchObject({
      name: 'web',
      prefix: '10.2.250.1/32',
      protocol: 'tcp',
      port: 80,
      encap: 'gre4',
      status: 'active',
      applied: true,
      vppEntries: 2,
      vppEncap: 'gre4',
      servers: [
        { address: '10.2.2.10', inUse: true, configured: true },
        { address: '10.2.2.11', inUse: true, configured: true },
        { address: '10.2.2.9', inUse: false, configured: false },
      ],
    });
    expect(items[0]).toMatchObject({
      name: 'dns',
      encap: 'nat4',
      targetPort: 5353,
      status: 'active',
    });
    const one = await h.call(ro, 'GET', '/api/v1/state/lb/vips?name=dns');
    expect((one.body.items as unknown[]).length).toBe(1);

    // flush: readonly may not (403), an operator may; unknown → 404; refused by the agent → 409
    expect((await h.call(ro, 'POST', '/api/v1/actions/lb/vips/web/flush')).status).toBe(403);
    const fl = await h.call(op, 'POST', '/api/v1/actions/lb/vips/web/flush');
    expect(fl.status).toBe(200);
    expect(fl.body).toEqual({ vip: 'lb.vip/10.2.250.1/32/tcp/80' });
    expect(lbFake(h.fake).flushed).toEqual(['web']);
    const nf = await h.call(op, 'POST', '/api/v1/actions/lb/vips/nope/flush');
    expect(nf.status).toBe(404);
    expect(String(nf.headers['content-type'])).toContain('application/problem+json');
    lbFake(h.fake).opts = { notFlushable: new Set(['dns']) };
    const pre = await h.call(op, 'POST', '/api/v1/actions/lb/vips/dns/flush');
    expect(pre.status).toBe(409);
    expect(pre.body.type).toContain('agent-precondition');

    // the flush is audited like every mutation
    const audit = await h.call(admin, 'GET', '/api/v1/audit?limit=20');
    expect(audit.status).toBe(200);
    const rows = JSON.stringify(audit.body);
    expect(rows).toContain('/api/v1/actions/lb/vips/:name/flush');
    expect(rows).toContain('services/lb/vips/web');

    // an agent without the RPCs: 501
    lbFake(h.fake).opts = { unimplemented: true };
    expect((await h.call(ro, 'GET', '/api/v1/state/lb/vips')).status).toBe(501);
    lbFake(h.fake).opts = {};

    // removal through the pointer route + commit: the state lists nothing configured
    expect((await h.call(admin, 'DELETE', '/api/v1/config/services/lb')).status).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=f-lb-remove')).status).toBe(
      200,
    );
    const empty = await h.call(ro, 'GET', '/api/v1/state/lb/vips');
    expect(empty.body.items).toEqual([]);
  });
});
