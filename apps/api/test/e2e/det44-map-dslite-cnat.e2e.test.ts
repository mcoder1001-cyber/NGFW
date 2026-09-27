import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { det44MapFake } from '../../src/features/det44-map-dslite-cnat/fake.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { op: runSecret(), ro: runSecret() };

/**
 * F-det44-map-dslite-cnat on the host PostgreSQL with the fake agent: DET44 / CNAT / PNAT configuration through the
 * generic pointer routes (a CNAT SNAT policy without addresses and an lw4o6 domain without rules → 400 problem+json
 * with the pointer), the DET44 session browser + lookup + close (RBAC, audit), the CNAT session browser and the
 * admin-only purge. Slot 8 names and addresses (host-w8l0 / host-w8w0, 10.8.0.0/16).
 */
describe('det44-map-dslite-cnat e2e (PostgreSQL + fake agent)', () => {
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
        'host-w8l0': { enabled: true, ipv4: ['10.8.1.1/24'] },
        'host-w8w0': { enabled: true, ipv4: ['10.8.2.1/24'] },
      },
      mp,
    );
    expect(ifs.status).toBe(200);
  });
  afterAll(async () => h?.close());

  it('commits DET44, CNAT and PNAT through the generic nat routes', async () => {
    const p = await h.call(
      op,
      'PATCH',
      '/api/v1/config/nat',
      {
        det44: {
          enabled: true,
          inside: ['host-w8l0'],
          outside: ['host-w8w0'],
          mappings: [{ inside: '10.8.1.0/24', outside: '10.8.2.200/30' }],
        },
        cnat: {
          translations: [
            {
              name: 'web',
              protocol: 'tcp',
              vip: { ip: '10.8.2.100', port: 80 },
              backends: [
                { ip: '10.8.1.2', port: 8080 },
                { ip: '10.8.1.3', port: 8080 },
              ],
            },
          ],
          snat: { addresses: { ipv4: '10.8.2.1' } },
        },
        pnat: {
          bindings: [
            {
              name: 'dns',
              match: { proto: 'udp', dst: '10.8.2.53', dport: 53 },
              rewrite: { dst: '10.8.1.53' },
            },
          ],
          attachments: [{ binding: 'dns', interface: 'host-w8w0', point: 'input' }],
        },
      },
      mp,
    );
    expect(p.status).toBe(200);
    const c = await h.call(op, 'POST', '/api/v1/config/commit?comment=cgnat');
    expect(c.status).toBe(200);
    expect(c.body.status).toBe('applied');
  });

  it('CNAT SNAT policy without addresses and lw4o6 without rules → 400 problem+json with pointers', async () => {
    await h.call(
      op,
      'PATCH',
      '/api/v1/config/nat',
      {
        cnat: { snat: { policy: 'interface', addresses: null } },
        map: {
          domains: [
            {
              name: 'lw',
              mode: 'lw4o6',
              ipv4Prefix: '10.8.65.0/24',
              ipv6Prefix: 'fd00:8:65::/64',
              ipv6Source: 'fd00:8::1/128',
            },
          ],
        },
      },
      mp,
    );
    const c = await h.call(op, 'POST', '/api/v1/config/commit');
    expect(c.status).toBe(400);
    expect(c.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect(c.body).toMatchObject({ status: 400, tier: 'semantic' });
    const pointers = (c.body.errors as { pointer: string }[]).map((e) => e.pointer);
    expect(pointers).toContain('/nat/cnat/snat/addresses');
    expect(pointers).toContain('/nat/map/domains/0/rules');
    await h.call(op, 'POST', '/api/v1/config/discard');
  });

  it('DET44 sessions (paged), lookup and close; RBAC and audit', async () => {
    det44MapFake(h.fake).det44Sessions.set(
      '10.8.1.5',
      [0, 1, 2].map((i) => ({
        insidePort: 40000 + i,
        outsidePort: 6064 + i,
        externalAddress: '10.8.2.2',
        externalPort: 80,
        state: 'tcp-established',
        expire: 100,
      })),
    );
    const s = await h.call(ro, 'GET', '/api/v1/state/nat/det44/sessions?user=10.8.1.5&pageSize=2');
    expect(s.status).toBe(200);
    expect(s.body).toMatchObject({
      outsideAddress: '10.8.2.200',
      portLo: 6064,
      portHi: 7071,
      total: 3,
    });
    expect(s.body.items).toHaveLength(2);
    expect((await h.call(ro, 'GET', '/api/v1/state/nat/det44/sessions?user=nope')).status).toBe(
      400,
    );

    const f = await h.call(op, 'POST', '/api/v1/actions/nat/det44/lookup', { inside: '10.8.1.70' });
    expect(f.status).toBe(200);
    expect(f.body).toEqual({
      inside: '10.8.1.70',
      outside: '10.8.2.201',
      portLo: 7072,
      portHi: 8079,
    });
    const r = await h.call(op, 'POST', '/api/v1/actions/nat/det44/lookup', {
      outside: '10.8.2.201',
      port: 7100,
    });
    expect(r.body).toMatchObject({ inside: '10.8.1.70' });

    const body = {
      direction: 'in',
      address: '10.8.1.5',
      port: 40000,
      externalAddress: '10.8.2.2',
      externalPort: 80,
    };
    expect(
      (await h.call(ro, 'POST', '/api/v1/actions/nat/det44/sessions/close', body)).status,
    ).toBe(403);
    expect(
      (await h.call(op, 'POST', '/api/v1/actions/nat/det44/sessions/close', body)).status,
    ).toBe(200);
    expect(
      (await h.call(op, 'POST', '/api/v1/actions/nat/det44/sessions/close', body)).status,
    ).toBe(404);
    const audit = await h.db.execute(
      sql`select resource, result, status from audit_log where action = 'POST /api/v1/actions/nat/det44/sessions/close' and username = 'op1' order by id`,
    );
    expect(
      (audit.rows as Record<string, unknown>[]).map((x) => [x['result'], x['status']]),
    ).toEqual([
      ['success', 200],
      ['failure', 404],
    ]);
  });

  it('CNAT sessions; the purge is admin-only and refused by a slot agent', async () => {
    det44MapFake(h.fake).cnatSessions = [
      {
        dstAddress: '10.8.2.100',
        dstPort: 80,
        srcAddress: '10.8.1.10',
        srcPort: 40000,
        protocol: 'tcp',
        translationIndex: 0,
        flags: 0,
      },
    ];
    const s = await h.call(ro, 'GET', '/api/v1/state/nat/cnat/sessions');
    expect(s.status).toBe(200);
    expect(s.body).toMatchObject({
      total: 1,
      items: [{ dstAddress: '10.8.2.100', srcAddress: '10.8.1.10' }],
    });
    expect((await h.call(op, 'POST', '/api/v1/actions/nat/cnat/sessions/purge')).status).toBe(403);
    expect((await h.call(admin, 'POST', '/api/v1/actions/nat/cnat/sessions/purge')).status).toBe(
      403,
    ); // slot agent (D-071)
  });
});
