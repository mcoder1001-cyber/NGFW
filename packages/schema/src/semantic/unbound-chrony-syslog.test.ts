import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { SyslogServerSchema } from '../domains/management.js';
import { validateSemantics } from './index.js';
import { sortIssues } from './registry.js';
import { unboundChronySyslogValidators } from './unbound-chrony-syslog.js';

/** F-unbound-chrony-syslog: the D-086 syslog keys and the three cross-field rules. */

const run = (doc: unknown) =>
  sortIssues(unboundChronySyslogValidators.flatMap((v) => v.validate(RootConfig.parse(doc))));

const BASE = {
  vrfs: { default: { id: 0 } },
  interfaces: { loop1053: { ipv4: ['192.0.2.53/32'], ipv6: ['2001:db8:53::1/128'] } },
};

const resolver = (extra: Record<string, unknown> = {}) => ({
  listen: [{ address: '192.0.2.53', port: 53 }],
  ...extra,
});

const withDns = (dns: Record<string, unknown>) => ({ ...BASE, services: { dns } });

describe('F-unbound-chrony-syslog fixture', () => {
  it('the proto drift fixture is schema-valid and passes every semantic rule', () => {
    const doc: unknown = JSON.parse(
      readFileSync(
        new URL('../../../proto/test/fixtures/unbound-chrony-syslog-full.json', import.meta.url),
        'utf8',
      ),
    );
    const parsed = RootConfig.safeParse(doc);
    expect(parsed.error?.issues).toBeUndefined();
    expect(validateSemantics(parsed.data!)).toEqual([]);
    expect(parsed.data!.management.syslog[0]).toMatchObject({
      facilities: ['auth', 'authpriv', 'local7'],
      format: 'rfc5424',
      queueSize: 50000,
      tls: { caRef: 'cert/syslog-ca', authMode: 'x509/name' },
    });
  });

  it('validator names are prefixed by their domain and unique', () => {
    for (const v of unboundChronySyslogValidators) {
      expect(v.name).toMatch(/^(services|management)\.unbound-chrony-syslog-/);
      expect(v.domains.some((d) => v.name.startsWith(`${d}.`))).toBe(true);
    }
    expect(new Set(unboundChronySyslogValidators.map((v) => v.name)).size).toBe(
      unboundChronySyslogValidators.length,
    );
  });
});

describe('management.syslog[i] feature keys (D-086)', () => {
  const base = { address: '192.0.2.10' };
  it('absent keys parse to exactly the pre-D-086 value', () => {
    expect(SyslogServerSchema.parse(base)).toEqual({
      address: '192.0.2.10',
      port: 514,
      protocol: 'udp',
      severity: 'info',
      vrf: 'default',
    });
  });
  it.each([
    [{ facilities: ['kernel'] }, ['facilities', 0]],
    [{ format: 'json' }, ['format']],
    [{ queueSize: 99 }, ['queueSize']],
    [{ queueSize: 1000001 }, ['queueSize']],
    [{ tls: {} }, ['tls', 'caRef']],
    [{ tls: { caRef: 'key/ca' } }, ['tls', 'caRef']],
    [{ tls: { caRef: 'cert/ca', certRef: 'cert/c' } }, ['tls', 'keyRef']],
    [{ tls: { caRef: 'cert/ca', authMode: 'anon' } }, ['tls', 'authMode']],
    [{ tls: { caRef: 'cert/ca', pem: 'x' } }, ['tls']],
  ])('%j is rejected at %j', (extra, path) => {
    const r = SyslogServerSchema.safeParse({ ...base, protocol: 'tls', ...extra });
    expect(r.success).toBe(false);
    expect(r.error!.issues.map((i) => i.path)).toContainEqual(path);
  });
});

describe('services.unbound-chrony-syslog-vpp-cache-port', () => {
  it('an enabled VPP cache conflicts with every enabled port-53 listener', () => {
    const doc = withDns({
      resolvers: {
        a: resolver({ listen: [{ address: '192.0.2.53' }, { address: '192.0.2.53', port: 5353 }] }),
        off: resolver({ enabled: false }),
      },
      vppCache: { enabled: true, upstreams: ['9.9.9.9'] },
    });
    expect(run(doc)).toEqual([
      {
        pointer: '/services/dns/resolvers/a/listen/0',
        message:
          "resolver 'a' cannot listen on 192.0.2.53:53 while the VPP DNS cache (services.dns.vppCache) is enabled: VPP answers UDP port 53 on its addresses",
      },
    ]);
  });
  it('a disabled VPP cache does not', () => {
    expect(
      run(
        withDns({
          resolvers: { a: resolver() },
          vppCache: { enabled: false, upstreams: ['9.9.9.9'] },
        }),
      ),
    ).toEqual([]);
  });
});

describe('services.unbound-chrony-syslog-forwarder-loop', () => {
  it('reports forward-zone forwarders and forwarders pointing at another resolver', () => {
    const doc = withDns({
      resolvers: {
        a: resolver({
          forwardZones: [{ zone: 'corp.example', forwarders: [{ address: '192.0.2.53' }] }],
        }),
        b: {
          listen: [{ address: '2001:db8:53::1', port: 5353 }],
          forwarders: [{ address: '192.0.2.53' }],
        },
      },
    });
    expect(run(doc).map((i) => i.pointer)).toEqual([
      '/services/dns/resolvers/a/forwardZones/0/forwarders/0',
      '/services/dns/resolvers/b/forwarders/0',
    ]);
  });
  it('a wildcard listener covers every address of its family and port', () => {
    const doc = withDns({
      resolvers: {
        a: { listen: [{ address: '0.0.0.0', port: 53 }] },
        b: {
          listen: [{ address: '2001:db8:53::1', port: 5353 }],
          forwarders: [{ address: '203.0.113.9', port: 53 }],
        },
      },
    });
    expect(run(doc)).toEqual([
      {
        pointer: '/services/dns/resolvers/b/forwarders/0',
        message:
          "forwarder 203.0.113.9:53 is a listen address of resolver 'a' in VRF 'default' (one Unbound instance serves every resolver: the query would loop)",
      },
    ]);
  });
  it("leaves the resolver's own default forwarder to the schema (reported once, with a pointer)", () => {
    const doc = withDns({
      resolvers: { a: resolver({ forwarders: [{ address: '192.0.2.53' }] }) },
    });
    const r = RootConfig.safeParse(doc);
    expect(r.success).toBe(false);
    expect(r.error!.issues.map((i) => i.path.join('/'))).toEqual([
      'services/dns/resolvers/a/forwarders/0',
    ]);
  });
  it('another port or VRF is not a loop', () => {
    const doc = withDns({
      resolvers: {
        a: resolver({
          forwardZones: [
            { zone: 'x.example', forwarders: [{ address: '192.0.2.53', port: 5300 }] },
          ],
        }),
      },
    });
    expect(run(doc)).toEqual([]);
  });
});

describe('management.unbound-chrony-syslog-tls', () => {
  const doc = (syslog: unknown[]) => ({ ...BASE, management: { syslog } });
  it('protocol tls needs tls (and with it a CA reference)', () => {
    expect(run(doc([{ address: '192.0.2.10', protocol: 'tls' }]))).toEqual([
      {
        pointer: '/management/syslog/0/tls',
        message:
          'syslog export to 192.0.2.10:514 over TLS needs tls.caRef (the CA that verifies the collector)',
      },
    ]);
  });
  it('tls on a udp/tcp target is rejected, not ignored', () => {
    expect(
      run(doc([{ address: '192.0.2.10', protocol: 'tcp', tls: { caRef: 'cert/ca' } }])).map(
        (i) => i.pointer,
      ),
    ).toEqual(['/management/syslog/0/tls']);
  });
  it('a complete TLS target is clean', () => {
    expect(
      run(doc([{ address: '192.0.2.10', protocol: 'tls', tls: { caRef: 'cert/ca' } }])),
    ).toEqual([]);
  });
});

describe('services.dns.vppCache (D-137)', () => {
  it('an enabled cache needs at least one IPv4 upstream (400 at the upstreams pointer)', () => {
    const r = RootConfig.safeParse({
      services: {
        dns: { vppCache: { enabled: true, upstreams: ['2001:db8::53', '2001:db8::54'] } },
      },
    });
    expect(r.success).toBe(false);
    expect(r.error!.issues).toContainEqual(
      expect.objectContaining({
        path: ['services', 'dns', 'vppCache', 'upstreams'],
        message: 'VPP DNS cache needs at least one IPv4 upstream (VPP 26.06 defect, D-137)',
      }),
    );
  });
  it('one IPv4 upstream is enough; a disabled cache is not checked', () => {
    for (const vppCache of [
      { enabled: true, upstreams: ['2001:db8::53', '192.0.2.53'] },
      { enabled: false, upstreams: ['2001:db8::53'] },
    ]) {
      expect(RootConfig.safeParse({ services: { dns: { vppCache } } }).success).toBe(true);
    }
  });
});
