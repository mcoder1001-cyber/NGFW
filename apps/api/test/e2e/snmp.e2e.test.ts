import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { startHarness, type Harness } from '../support/harness.js';

/**
 * F-snmp: `services.snmp` through the generic pointer routes, the 400 problem+json of a trap receiver naming an
 * unknown community, secrets in by value and never out, `GET /api/v1/state/snmp` from the fake agent's SnmpState.
 * Test literals only (`VRX_TEST_PSK_F-snmp_*`).
 */
describe('snmp e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  const community = `VRX_TEST_PSK_F-snmp_ro${Date.now()}`;

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
  });
  afterAll(async () => h?.close());

  it('trap receiver naming an unknown community → 400 problem+json with a pointer', async () => {
    const r = await h.call(admin, 'PATCH', '/api/v1/config/services/snmp', {
      enabled: true,
      communities: { ro: { secretRef: 'password/snmp-ro' } },
      trapReceivers: [{ address: '127.0.0.1', port: 3962, version: 'v2c', community: 'nope' }],
    });
    expect(r.status).toBe(400);
    expect(String(r.headers['content-type'])).toContain('application/problem+json');
    expect(r.body.errors).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ pointer: '/services/snmp/trapReceivers/0/community' }),
      ]),
    );
    await h.call(admin, 'POST', '/api/v1/config/discard');
  });

  it('commits services.snmp, never returns the community, reports state', async () => {
    const s = await h.call(admin, 'POST', '/api/v1/secrets', {
      kind: 'password',
      name: 'snmp-ro',
      value: community,
    });
    expect([200, 201]).toContain(s.status);
    const p = await h.call(admin, 'PATCH', '/api/v1/config/services/snmp', {
      enabled: true,
      sysName: 'vrx-w1',
      communities: { ro: { secretRef: 'password/snmp-ro', sources: ['127.0.0.0/8'] } },
      views: { vrx: { include: ['system', '.1.3.6.1.4.1.8072.9999.9999'] } },
      trapReceivers: [{ address: '127.0.0.1', port: 3962, version: 'v2c', community: 'ro' }],
    });
    expect(p.status).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=snmp')).status).toBe(200);
    const cfg = await h.call(admin, 'GET', '/api/v1/config/services/snmp');
    expect(cfg.raw).not.toContain(community);
    const st = await h.call(admin, 'GET', '/api/v1/state/snmp');
    expect(st.status).toBe(200);
    expect(st.body).toMatchObject({
      configured: true,
      daemon: { reachable: true, sysName: 'vrx-w1', credential: 'v2c community' },
      subagent: { registered: true },
    });
    expect(st.raw).not.toContain(community);
  });
});
