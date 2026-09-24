import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { fakeSession, natFake } from '../../src/features/nat44-ed-sessions/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { op: runSecret(), ro: runSecret() };

/**
 * F-nat44-ed-sessions on the host PostgreSQL with the fake agent: NAT44-ED configuration through the generic pointer
 * routes (candidate → commit, overlapping / adjacent pools → 400 problem+json with the pointer), the paged session
 * browser (`pageSize=100` never returns more than 100, filters, 400 on a bad query), the summary joined with the
 * running pool names, and the kill action (reaches the agent's Action with the 5-tuple, is audited, RBAC). Names and
 * addresses follow the slot rules of slot 1 (host-w1l0 / host-w1w0, 10.1.0.0/16).
 */
describe('nat44-ed sessions e2e (PostgreSQL + fake agent)', () => {
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
        'host-w1l0': { enabled: true, ipv4: ['10.1.1.1/24'] },
        'host-w1w0': { enabled: true, ipv4: ['10.1.2.1/24'] },
      },
      mp,
    );
    expect(ifs.status).toBe(200);
  });
  afterAll(async () => h?.close());

  it('commits outbound PAT, 1:1 and a port forward through the generic nat routes', async () => {
    const p = await h.call(
      op,
      'PATCH',
      '/api/v1/config/nat',
      {
        inside: ['host-w1l0'],
        outside: ['host-w1w0'],
        pools: [
          { name: 'out', range: '10.1.2.100-10.1.2.103' },
          { name: 'wan', interface: 'host-w1w0' },
        ],
        staticMappings: [
          { name: 'one2one', local: { ip: '10.1.1.3' }, external: { ip: '10.1.2.111' } },
          {
            name: 'web',
            protocol: 'tcp',
            local: { ip: '10.1.1.2', port: 80 },
            external: { ip: '10.1.2.110', port: 8080 },
          },
        ],
      },
      mp,
    );
    expect(p.status).toBe(200);
    const c = await h.call(op, 'POST', '/api/v1/config/commit?comment=nat');
    expect(c.status).toBe(200);
    expect(c.body.status).toBe('applied');
    const applied = h.fake.calls.filter((x) => x.method === 'Apply').at(-1)?.request as {
      desiredState: { nat?: { pools: unknown[] } };
    };
    expect(applied.desiredState.nat?.pools).toHaveLength(2);
  });

  it('overlapping and adjacent pools → 400 problem+json with the pointer', async () => {
    for (const [second, rule] of [
      ['10.1.2.102-10.1.2.110', /overlaps pool 'out'/],
      ['10.1.2.104-10.1.2.110', /touches pool 'out'/],
    ] as const) {
      await h.call(
        op,
        'PATCH',
        '/api/v1/config/nat',
        {
          pools: [
            { name: 'out', range: '10.1.2.100-10.1.2.103' },
            { name: 'more', range: second },
          ],
        },
        mp,
      );
      const c = await h.call(op, 'POST', '/api/v1/config/commit');
      expect(c.status).toBe(400);
      expect(c.headers['content-type']).toMatch(/^application\/problem\+json/);
      expect(c.body).toMatchObject({ status: 400, tier: 'semantic' });
      expect(c.body.errors).toContainEqual(
        expect.objectContaining({
          pointer: '/nat/pools/1/range',
          message: expect.stringMatching(rule),
        }),
      );
      await h.call(op, 'POST', '/api/v1/config/discard');
    }
  });

  it('pages sessions server-side: pageSize=100 never returns more than 100', async () => {
    const st = natFake(h.fake);
    st.sessions = [];
    for (let u = 0; u < 21; u++)
      for (let i = 0; i < 100; i++)
        st.sessions.push(
          fakeSession({
            insideAddress: `10.1.1.${10 + u}`,
            insidePort: 10000 + i,
            outsideAddress: '10.1.2.100',
            outsidePort: 20000 + u * 100 + i,
            externalAddress: '10.1.2.2',
            externalPort: 80,
          }),
        );
    st.sessions.push(fakeSession({ protocol: 'udp', insideAddress: '10.1.1.5', externalPort: 53 }));
    const r = await h.call(ro, 'GET', '/api/v1/state/nat/sessions?pageSize=100&page=3');
    expect(r.status).toBe(200);
    expect(r.body).toMatchObject({
      page: 3,
      pageSize: 100,
      total: 2101,
      totalUsers: 22,
      truncated: false,
    });
    expect(r.body.items).toHaveLength(100);
    expect(r.body.items[0]).toMatchObject({
      insideAddress: '10.1.1.11',
      protocol: 'tcp',
      bytes: 120,
      packets: 2,
    });
    const req = h.fake.calls.filter((x) => x.method === 'NatSessions').at(-1)?.request as {
      offset: number;
      limit: number;
      owner: string;
    };
    expect(req).toMatchObject({ offset: 200, limit: 100, owner: h.prefix });

    const f = await h.call(ro, 'GET', '/api/v1/state/nat/sessions?protocol=UDP&external=10.1.2.2');
    expect(f.body.total).toBe(1);
    expect(f.body.items[0]).toMatchObject({ protocol: 'udp', externalPort: 53 });
    const last = await h.call(ro, 'GET', '/api/v1/state/nat/sessions?pageSize=250&page=9');
    expect(last.body.items).toHaveLength(101);
    for (const bad of ['pageSize=257', 'inside=10.1.1', 'port=70000', 'vrf=a%20b']) {
      const b = await h.call(ro, 'GET', `/api/v1/state/nat/sessions?${bad}`);
      expect(b.status, bad).toBe(400);
      expect(b.headers['content-type']).toMatch(/^application\/problem\+json/);
    }
  });

  it('summarises per pool with the running pool names', async () => {
    const s = await h.call(ro, 'GET', '/api/v1/state/nat/summary');
    expect(s.status).toBe(200);
    expect(s.body).toMatchObject({
      enabled: true,
      totalSessions: 2101,
      totalUsers: 22,
      byProtocol: { tcp: 2100, udp: 1 },
    });
    expect(s.body.pools).toContainEqual(
      expect.objectContaining({
        name: 'out',
        kind: 'range',
        range: '10.1.2.100-10.1.2.103',
        addresses: 4,
        sessions: 2101,
        applied: true,
        configured: true,
      }),
    );
    expect(s.body.pools).toContainEqual(
      expect.objectContaining({ name: 'wan', kind: 'interface', interface: 'host-w1w0' }),
    );
  });

  it('kills a session through the agent Action (5-tuple), audited; RBAC and a bad body', async () => {
    const body = {
      protocol: 'tcp',
      insideAddress: '10.1.1.10',
      insidePort: 10005,
      externalAddress: '10.1.2.2',
      externalPort: 80,
    };
    expect((await h.call(ro, 'POST', '/api/v1/actions/nat/sessions/kill', body)).status).toBe(403);
    const bad = await h.call(op, 'POST', '/api/v1/actions/nat/sessions/kill', {
      ...body,
      insidePort: 70000,
    });
    expect(bad.status).toBe(400);
    expect(bad.body.errors).toContainEqual(expect.objectContaining({ pointer: '/insidePort' }));

    // the fake agent's generic Action answers UNIMPLEMENTED (its A4 dispatch is F-vrf-static-ecmp's): the request
    // reaches it with the 5-tuple, and the API maps the gRPC status (501)
    const k = await h.call(op, 'POST', '/api/v1/actions/nat/sessions/kill', body);
    expect(k.status).toBe(501);
    const sent = h.fake.calls.filter((x) => x.method === 'Action').at(-1)?.request as {
      natSessionKill?: Record<string, unknown>;
    };
    expect(sent.natSessionKill).toEqual({ ...body, vrf: '' });

    const audit = await h.db.execute(
      sql`select action, resource, result, status, username, before from audit_log where action like '%/actions/nat/sessions/kill' order by id`,
    );
    const rows = (audit.rows as Record<string, unknown>[]).filter((r) => r['username'] === 'op1');
    expect(rows.map((r) => [r['result'], r['status']])).toEqual([
      ['failure', 400],
      ['failure', 501],
    ]);
    expect(rows.at(-1)).toMatchObject({
      action: 'POST /api/v1/actions/nat/sessions/kill',
      resource: 'nat/sessions/tcp/10.1.1.10:10005/10.1.2.2:80/default',
      before: { ...body, vrf: 'default' },
    });
  });
});
