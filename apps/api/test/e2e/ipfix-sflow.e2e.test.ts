import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { startHarness, type Harness } from '../support/harness.js';

/**
 * F-ipfix-sflow over HTTP (host PostgreSQL + fake agent): `services.ipfix` through the pointer routes, the semantic
 * 400 problem+json with a `pointer`, and `GET /api/v1/state/ipfix`. Slot names: host-w1a/host-w1b, 10.1.0.0/16,
 * collector ports 3171 (IPFIX) / 3172 (sFlow).
 */
describe('ipfix-sflow e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  const mp = { 'content-type': 'application/merge-patch+json' };

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
  });
  afterAll(async () => h?.close());

  it('flowprobe without an enabled IPv4 exporter: 400 problem+json with a pointer', async () => {
    expect(
      (
        await h.call(
          admin,
          'PATCH',
          '/api/v1/config/interfaces',
          {
            'host-w1a': { enabled: true, ipv4: ['10.1.1.1/24'] },
            'host-w1b': { enabled: true, ipv4: ['10.1.2.1/24'] },
          },
          mp,
        )
      ).status,
    ).toBe(200);
    expect(
      (
        await h.call(
          admin,
          'PATCH',
          '/api/v1/config/services/ipfix/flowprobe',
          { interfaces: [{ interface: 'host-w1a', direction: 'rx', ip4: true, ip6: false }] },
          mp,
        )
      ).status,
    ).toBe(200);
    const r = await h.call(admin, 'POST', '/api/v1/config/commit?comment=ipfix-bad');
    expect(r.status).toBe(400);
    expect(r.headers['content-type']).toMatch(/application\/problem\+json/);
    expect(r.body.errors).toContainEqual(
      expect.objectContaining({ pointer: '/services/ipfix/flowprobe/interfaces' }),
    );
  });

  it('commits an exporter + sFlow and reports them in /state/ipfix', async () => {
    expect(
      (
        await h.call(
          admin,
          'PATCH',
          '/api/v1/config/services/ipfix',
          {
            exporters: {
              lan: { collector: { address: '10.1.1.9', port: 3171 }, sourceAddress: '10.1.1.1' },
            },
            sflow: {
              enabled: true,
              samplingN: 1000,
              collectors: [{ address: '10.1.1.9', port: 3172 }],
              interfaces: ['host-w1b'],
            },
          },
          mp,
        )
      ).status,
    ).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=ipfix')).status).toBe(200);
    const st = await h.call(admin, 'GET', '/api/v1/state/ipfix');
    expect(st.status).toBe(200);
    expect(st.body.exporters[0]).toMatchObject({
      name: 'lan',
      defaultExporter: true,
      collectorPort: 3171,
    });
    expect(st.body.flowprobe.interfaces).toEqual([
      { interface: 'host-w1a', which: 'ip4', direction: 'rx' },
    ]);
    expect(st.body.sflow).toMatchObject({
      interfaces: [{ interface: 'host-w1b' }],
      exportsToCollectors: false,
    });
  });
});
