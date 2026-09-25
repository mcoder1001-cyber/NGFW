import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

const PW = { ro: runSecret(), op: runSecret() };

/**
 * F-unbound-chrony-syslog on the host PostgreSQL with the fake agent: configuration through the generic pointer routes
 * (services.dns, services.ntp, management.syslog with the D-086 keys), the DNS/NTP/syslog state routes, the log
 * explorer (paged, filtered, bounded), the DNS lookup action (the fake resolves the configured local records — the
 * slot's real VPP has no enabled dns plugin, D-071) and the validation failures with pointers.
 */
describe('unbound-chrony-syslog e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let ro: string;
  let op: string;
  const LOOP = 'loop1053';

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

  const resolver = {
    listen: [{ address: '10.10.53.1', port: 5353 }],
    forwardZones: [{ zone: 'corp.example.', forwarders: [{ address: '10.10.99.53' }] }],
    localZones: [
      {
        zone: 'lab.example.',
        records: [
          { name: 'gw.lab.example.', type: 'A', data: '10.10.53.1' },
          { name: 'gw.lab.example.', type: 'AAAA', data: '2001:db8:53::1' },
        ],
      },
    ],
  };

  it('commits DNS, NTP and syslog through the pointer routes and shows their live state', async () => {
    expect(
      (
        await h.call(op, 'PUT', `/api/v1/config/interfaces/${LOOP}`, {
          enabled: true,
          ipv4: ['10.10.53.1/24'],
        })
      ).status,
    ).toBe(200);
    expect(
      (await h.call(op, 'PUT', '/api/v1/config/services/dns/resolvers/lan', resolver)).status,
    ).toBe(200);
    expect(
      (
        await h.call(op, 'PUT', '/api/v1/config/services/ntp', {
          enabled: true,
          servers: [{ address: '10.10.123.1' }, { address: 'ntp.example.net' }],
          port: 0,
        })
      ).status,
    ).toBe(200);
    const syslog = [
      {
        address: '10.10.14.16',
        port: 31016,
        protocol: 'tcp',
        severity: 'notice',
        facilities: ['local7', 'daemon'],
        format: 'rfc5424',
        queueSize: 5000,
      },
    ];
    expect((await h.call(op, 'PUT', '/api/v1/config/management/syslog', syslog)).status).toBe(200);
    const c = await h.call(op, 'POST', '/api/v1/config/commit?comment=ucs');
    expect(c.status).toBe(200);
    const sent = h.fake.current as {
      services: { dns: { resolvers: Record<string, unknown> }; ntp: { enabled: boolean } };
      management: { syslog: Record<string, unknown>[] };
    };
    expect(sent.management.syslog[0]).toMatchObject({
      facilities: ['local7', 'daemon'],
      queueSize: 5000,
    });

    const dns = await h.call(ro, 'GET', '/api/v1/state/dns');
    expect(dns.status).toBe(200);
    expect(dns.body).toMatchObject({
      running: true,
      forwards: [{ zone: 'corp.example.', kind: 'forward', addresses: ['10.10.99.53'] }],
      localZones: [{ zone: 'lab.example.', type: 'static' }],
      pendingActions: [],
      vppCache: { configured: false, live: false },
    });
    expect(dns.body.localData).toContain('gw.lab.example. 3600 IN A 10.10.53.1');

    const ntp = await h.call(ro, 'GET', '/api/v1/state/ntp');
    expect(ntp.status).toBe(200);
    expect(ntp.body.sources.map((s: { name: string }) => s.name)).toEqual([
      '10.10.123.1',
      'ntp.example.net',
    ]);
    expect(ntp.body.tracking).toMatchObject({ stratum: 3, leap: 'Normal' });

    const sys = await h.call(ro, 'GET', '/api/v1/state/syslog');
    expect(sys.status).toBe(200);
    expect(sys.body.targets).toEqual([
      expect.objectContaining({
        index: 0,
        target: '10.10.14.16:31016',
        protocol: 'tcp',
        reported: true,
        processed: 7,
      }),
    ]);
    expect(sys.body.inputs).toEqual({ imuxsock: 9 });
  });

  it('log explorer: severity/facility/text filters, paging, bounded query validation', async () => {
    const all = await h.call(ro, 'GET', '/api/v1/state/logs');
    expect(all.status).toBe(200);
    expect(all.body).toMatchObject({
      total: 5,
      page: 1,
      pageSize: 100,
      source: 'journald',
      truncated: false,
    });
    const warn = await h.call(ro, 'GET', '/api/v1/state/logs?severity=warning');
    expect(warn.body.items.map((e: { identifier: string }) => e.identifier)).toEqual([
      'chronyd',
      'vrx-test',
    ]);
    const fac = await h.call(ro, 'GET', '/api/v1/state/logs?facility=local7&q=FORWARDED');
    expect(fac.body.items).toEqual([
      expect.objectContaining({ message: 'forwarded test line', severity: 'error' }),
    ]);
    const page2 = await h.call(ro, 'GET', '/api/v1/state/logs?page=2&pageSize=2');
    expect(page2.body).toMatchObject({ total: 5, page: 2, pageSize: 2 });
    expect(page2.body.items).toHaveLength(2);
    for (const [q, pointer] of [
      ['severity=loud', '/severity'],
      ['facility=kernel', '/facility'],
      ['pageSize=501', '/pageSize'],
      ['q=a%0Ab', '/q'],
      ['since=yesterday', '/since'],
    ] as const) {
      const bad = await h.call(ro, 'GET', `/api/v1/state/logs?${q}`);
      expect(bad.status, q).toBe(400);
      expect(bad.body.errors[0].pointer, q).toBe(pointer);
    }
  });

  it('dns-lookup resolves through the (fake) VPP cache; refused without it; operators only; name validated', async () => {
    // without the VPP cache applied the agent refuses (VPP 26.06 crashes on dns_resolve_name without a name server)
    const refused = await h.call(op, 'POST', '/api/v1/actions/dns-lookup', {
      name: 'gw.lab.example',
    });
    expect(refused.status).toBe(409);
    expect(refused.body.detail).toMatch(/VPP DNS cache/);
    expect(
      (
        await h.call(op, 'PUT', '/api/v1/config/services/dns/vppCache', {
          enabled: true,
          upstreams: ['10.10.99.53'],
        })
      ).status,
    ).toBe(200);
    expect((await h.call(op, 'POST', '/api/v1/config/commit?comment=vpp-cache')).status).toBe(200);
    const ok = await h.call(op, 'POST', '/api/v1/actions/dns-lookup', { name: 'gw.lab.example' });
    expect(ok.status).toBe(200);
    expect(ok.body).toEqual({
      name: 'gw.lab.example',
      ok: true,
      addresses: [
        { type: 'A', address: '10.10.53.1' },
        { type: 'AAAA', address: '2001:db8:53::1' },
      ],
      summary: 'gw.lab.example: 2 address(es)',
    });
    const miss = await h.call(op, 'POST', '/api/v1/actions/dns-lookup', {
      name: 'nope.example',
      timeoutMs: 1000,
    });
    expect(miss.body).toMatchObject({ ok: false, addresses: [] });
    const bad = await h.call(op, 'POST', '/api/v1/actions/dns-lookup', { name: 'x; rm -rf /' });
    expect(bad.status).toBe(400);
    expect(bad.body.errors[0].pointer).toBe('/name');
    expect(
      (await h.call(ro, 'POST', '/api/v1/actions/dns-lookup', { name: 'gw.lab.example' })).status,
    ).toBe(403);
    // the generic `/actions/:action` route is still 501 for everything else (the static route wins for dns-lookup)
    expect((await h.call(op, 'POST', '/api/v1/actions/ping')).status).toBe(501);
  });

  it('forwarder equal to a listen address → 400 problem+json with the pointer', async () => {
    const loop = await h.call(op, 'PUT', '/api/v1/config/services/dns/resolvers/lan', {
      ...resolver,
      forwarders: [{ address: '10.10.53.1', port: 5353 }],
    });
    expect(loop.status).toBe(400);
    expect(loop.headers['content-type']).toMatch(/^application\/problem\+json/);
    expect(loop.body.errors).toContainEqual(
      expect.objectContaining({
        pointer: '/services/dns/resolvers/lan/forwarders/0',
        message: expect.stringMatching(/loop/),
      }),
    );
    // a forward zone pointing at the instance itself is caught by the semantic rule at commit time
    expect(
      (
        await h.call(op, 'PUT', '/api/v1/config/services/dns/resolvers/lan', {
          ...resolver,
          forwardZones: [
            { zone: 'corp.example.', forwarders: [{ address: '10.10.53.1', port: 5353 }] },
          ],
        })
      ).status,
    ).toBe(200);
    const c = await h.call(op, 'POST', '/api/v1/config/commit');
    expect(c.status).toBe(400);
    expect(c.body).toMatchObject({ tier: 'semantic' });
    expect(c.body.errors).toContainEqual(
      expect.objectContaining({
        pointer: '/services/dns/resolvers/lan/forwardZones/0/forwarders/0',
      }),
    );
    // syslog over TLS without a CA → semantic 400 with the pointer
    await h.call(op, 'POST', '/api/v1/config/discard');
    await h.call(op, 'PUT', '/api/v1/config/management/syslog', [
      { address: '10.10.14.16', protocol: 'tls' },
    ]);
    const t = await h.call(op, 'POST', '/api/v1/config/commit');
    expect(t.status).toBe(400);
    expect(t.body.errors).toContainEqual(
      expect.objectContaining({ pointer: '/management/syslog/0/tls' }),
    );
    await h.call(op, 'POST', '/api/v1/config/discard');
  });
});
