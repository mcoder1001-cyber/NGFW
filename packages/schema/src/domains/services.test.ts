import { describe, expect, it } from 'vitest';
import type { z } from 'zod';
import {
  DhcpPoolSchema,
  DhcpRelaySchema,
  DhcpServerSchema,
  DhcpSubnetSchema,
  DnsRecordSchema,
  DnsResolverSchema,
  DnsUpstreamSchema,
  IpfixExporterSchema,
  IpfixFlowprobeSchema,
  LldpSchema,
  QosInterfaceSchema,
  QosMapSchema,
  QosPolicerSchema,
  QosSchema,
  QosShaperSchema,
  ServicesNtpSchema,
  ServicesSchema,
  SflowSchema,
  SnmpSchema,
} from './services.js';
import { dnsName } from './_shared/primitives.js';

const NtpSchema = ServicesNtpSchema;

const ok = (schema: z.ZodType, value: unknown): boolean => schema.safeParse(value).success;
/** Paths of all issues; Zod 4 reports unknown keys with an empty path and the keys in `issue.keys`. */
const errorPaths = (schema: z.ZodType, value: unknown): string[] => {
  const r = schema.safeParse(value);
  if (r.success) return [];
  return r.error.issues.flatMap((i) =>
    i.code === 'unrecognized_keys'
      ? i.keys.map((k) => [...i.path, k].join('.'))
      : [i.path.join('.')],
  );
};
const NUL = String.fromCharCode(0);
const LF = String.fromCharCode(10);

const subnet = (extra: Record<string, unknown> = {}): Record<string, unknown> => ({
  subnet: '192.168.10.0/24',
  pools: [{ start: '192.168.10.100', end: '192.168.10.199' }],
  ...extra,
});
const v6subnet = (extra: Record<string, unknown> = {}): Record<string, unknown> => ({
  subnet: '2001:db8:10::/64',
  pools: [{ start: '2001:db8:10::1000', end: '2001:db8:10::1fff' }],
  ...extra,
});

describe('DHCP', () => {
  it('pools: same family, start <= end', () => {
    expect(ok(DhcpPoolSchema, { start: '10.0.0.1', end: '10.0.0.1' })).toBe(true);
    expect(errorPaths(DhcpPoolSchema, { start: '10.0.0.10', end: '10.0.0.1' })).toEqual(['end']);
    expect(errorPaths(DhcpPoolSchema, { start: '10.0.0.10', end: '2001:db8::1' })).toEqual(['end']);
  });
  it('subnet defaults and a valid DHCPv6 subnet with a DUID reservation', () => {
    expect(DhcpSubnetSchema.parse(subnet())).toMatchObject({
      dnsServers: [],
      ntpServers: [],
      domainSearch: [],
      options: [],
      reservations: {},
    });
    expect(
      ok(
        DhcpSubnetSchema,
        v6subnet({ reservations: { nas: { duid: '00:01:00:01:aa:bb', ip: '2001:db8:10::20' } } }),
      ),
    ).toBe(true);
    expect(
      ok(
        DhcpSubnetSchema,
        v6subnet({ reservations: { nas: { mac: 'aa:bb:cc:dd:ee:ff', ip: '2001:db8:10::20' } } }),
      ),
    ).toBe(true);
  });
  it.each([
    [
      'pool start outside the subnet',
      subnet({ pools: [{ start: '192.168.11.1', end: '192.168.11.9' }] }),
      'pools.0.start',
    ],
    [
      'pool end outside the subnet',
      subnet({ pools: [{ start: '192.168.10.250', end: '192.168.11.9' }] }),
      'pools.0.end',
    ],
    [
      'pool of the wrong family',
      subnet({ pools: [{ start: '2001:db8::1', end: '2001:db8::2' }] }),
      'pools.0.start',
    ],
    [
      'overlapping pools',
      subnet({
        pools: [
          { start: '192.168.10.10', end: '192.168.10.50' },
          { start: '192.168.10.50', end: '192.168.10.60' },
        ],
      }),
      'pools.1',
    ],
    ['no pool', subnet({ pools: [] }), 'pools'],
    ['gateway outside the subnet', subnet({ gateway: '192.168.11.1' }), 'gateway'],
    ['gateway on DHCPv6', v6subnet({ gateway: '2001:db8:10::1' }), 'gateway'],
    ['DNS server of the wrong family', subnet({ dnsServers: ['2001:db8::53'] }), 'dnsServers.0'],
    [
      'duplicate DNS server',
      subnet({ dnsServers: ['192.168.10.1', '192.168.10.1'] }),
      'dnsServers.1',
    ],
    ['NTP server of the wrong family', subnet({ ntpServers: ['2001:db8::123'] }), 'ntpServers.0'],
    [
      'five DNS servers',
      subnet({ dnsServers: ['10.0.0.1', '10.0.0.2', '10.0.0.3', '10.0.0.4', '10.0.0.5'] }),
      'dnsServers',
    ],
    ['DHCPv4 option code 300', subnet({ options: [{ code: 300, data: 'x' }] }), 'options.0.code'],
    [
      'duplicate option code',
      subnet({
        options: [
          { code: 42, data: 'a' },
          { code: 42, data: 'b' },
        ],
      }),
      'options.1',
    ],
    [
      'option data with control characters',
      subnet({ options: [{ code: 42, data: `a${LF}b` }] }),
      'options.0.data',
    ],
    [
      'reservation outside the subnet',
      subnet({ reservations: { r: { mac: 'aa:bb:cc:dd:ee:01', ip: '192.168.11.20' } } }),
      'reservations.r.ip',
    ],
    [
      'reservation of the wrong family',
      subnet({ reservations: { r: { mac: 'aa:bb:cc:dd:ee:01', ip: '2001:db8::20' } } }),
      'reservations.r.ip',
    ],
    [
      'same address reserved twice',
      subnet({
        reservations: {
          a: { mac: 'aa:bb:cc:dd:ee:01', ip: '192.168.10.20' },
          b: { mac: 'aa:bb:cc:dd:ee:02', ip: '192.168.10.20' },
        },
      }),
      'reservations.b.ip',
    ],
    [
      'same client reserved twice (case/separator-insensitive)',
      subnet({
        reservations: {
          a: { mac: 'aa:bb:cc:dd:ee:01', ip: '192.168.10.20' },
          b: { mac: 'AA-BB-CC-DD-EE-01', ip: '192.168.10.21' },
        },
      }),
      'reservations.b',
    ],
    [
      'DUID reservation on DHCPv4',
      subnet({ reservations: { r: { duid: '00:01:00:01', ip: '192.168.10.20' } } }),
      'reservations.r.duid',
    ],
    [
      'reservation with mac and duid',
      subnet({
        reservations: { r: { mac: 'aa:bb:cc:dd:ee:01', duid: '00:01', ip: '192.168.10.20' } },
      }),
      'reservations.r.mac',
    ],
    [
      'reservation without identifier',
      subnet({ reservations: { r: { ip: '192.168.10.20' } } }),
      'reservations.r.mac',
    ],
    [
      'bad DUID',
      v6subnet({ reservations: { r: { duid: '0001', ip: '2001:db8:10::20' } } }),
      'reservations.r.duid',
    ],
    ['lease below 60 s', subnet({ leaseTimeSec: 59 }), 'leaseTimeSec'],
    ['subnet without prefix length', subnet({ subnet: '192.168.10.0' }), 'subnet'],
  ])('subnet rejects %s', (_label, value, path) =>
    expect(errorPaths(DhcpSubnetSchema, value)).toContain(path),
  );

  const server = (extra: Record<string, unknown> = {}): Record<string, unknown> => ({
    interfaces: ['TenGigabitEthernet0/0/1'],
    subnets: { lan: subnet() },
    ...extra,
  });
  it('server defaults', () => {
    expect(DhcpServerSchema.parse(server())).toMatchObject({
      enabled: true,
      family: 'ipv4',
      vrf: 'default',
      leaseTimeSec: 3600,
      authoritative: true,
      options: [],
    });
  });
  it.each([
    ['no interfaces', server({ interfaces: [] }), 'interfaces'],
    ['duplicate interface', server({ interfaces: ['loop0', 'loop0'] }), 'interfaces.1'],
    ['renew timer >= lease', server({ leaseTimeSec: 3600, renewTimerSec: 3600 }), 'renewTimerSec'],
    [
      'rebind timer >= lease',
      server({ leaseTimeSec: 3600, rebindTimerSec: 4000 }),
      'rebindTimerSec',
    ],
    [
      'renew >= rebind',
      server({ leaseTimeSec: 3600, renewTimerSec: 3000, rebindTimerSec: 2000 }),
      'renewTimerSec',
    ],
    ['IPv6 subnet on an IPv4 server', server({ subnets: { a: v6subnet() } }), 'subnets.a.subnet'],
    [
      'IPv4 subnet on an IPv6 server',
      server({ family: 'ipv6', subnets: { a: subnet() } }),
      'subnets.a.subnet',
    ],
    [
      'overlapping subnets',
      server({
        subnets: {
          a: subnet(),
          b: subnet({
            subnet: '192.168.10.128/25',
            pools: [{ start: '192.168.10.130', end: '192.168.10.140' }],
          }),
        },
      }),
      'subnets.b.subnet',
    ],
    [
      'global DHCPv4 option code 255',
      server({ options: [{ code: 255, data: 'x' }] }),
      'options.0.code',
    ],
    [
      'unknown key (Kea passthrough is not allowed)',
      server({ 'hooks-libraries': [] }),
      'hooks-libraries',
    ],
  ])('server rejects %s', (_label, value, path) =>
    expect(errorPaths(DhcpServerSchema, value)).toContain(path),
  );
  it('DHCPv6 server accepts option codes above 254', () => {
    expect(
      ok(
        DhcpServerSchema,
        server({ family: 'ipv6', subnets: { a: v6subnet() }, options: [{ code: 300, data: 'x' }] }),
      ),
    ).toBe(true);
  });

  const relay = (extra: Record<string, unknown> = {}): Record<string, unknown> => ({
    interfaces: ['TenGigabitEthernet0/0/2'],
    servers: ['192.168.10.5'],
    sourceAddress: '10.20.0.1',
    ...extra,
  });
  it('relay: families, duplicates', () => {
    expect(DhcpRelaySchema.parse(relay())).toMatchObject({ family: 'ipv4', vrf: 'default' });
    expect(errorPaths(DhcpRelaySchema, relay({ servers: ['2001:db8::5'] }))).toEqual(['servers.0']);
    expect(errorPaths(DhcpRelaySchema, relay({ sourceAddress: '2001:db8::1' }))).toEqual([
      'sourceAddress',
    ]);
    expect(errorPaths(DhcpRelaySchema, relay({ servers: ['10.0.0.1', '10.0.0.1'] }))).toEqual([
      'servers.1',
    ]);
    expect(errorPaths(DhcpRelaySchema, relay({ interfaces: ['a', 'a'] }))).toEqual([
      'interfaces.1',
    ]);
    expect(
      ok(
        DhcpRelaySchema,
        relay({ family: 'ipv6', servers: ['2001:db8::5'], sourceAddress: '2001:db8:10::1' }),
      ),
    ).toBe(true);
    expect(ok(DhcpRelaySchema, relay({ servers: [] }))).toBe(false);
  });
});

describe('DNS', () => {
  it.each([
    'example.com',
    'example.com.',
    '_sip._tcp.example.com',
    '*.example.com',
    '.',
    'a',
    'xn--80ak6aa92e.com',
  ])('dnsName accepts %s', (v) => expect(ok(dnsName, v)).toBe(true));
  it.each([
    '',
    'exa mple.com',
    'a..b',
    `${'a'.repeat(64)}.com`,
    'ex@mple.com',
    '*.',
    '**.example.com',
    'a'.repeat(254),
  ])('dnsName rejects %j', (v) => expect(ok(dnsName, v)).toBe(false));
  it('records are typed', () => {
    const rec = (type: string, data: string) => ({ name: 'h.example.com', type, data });
    expect(ok(DnsRecordSchema, rec('A', '192.0.2.1'))).toBe(true);
    expect(errorPaths(DnsRecordSchema, rec('A', '2001:db8::1'))).toEqual(['data']);
    expect(ok(DnsRecordSchema, rec('AAAA', '2001:db8::1'))).toBe(true);
    expect(errorPaths(DnsRecordSchema, rec('AAAA', '192.0.2.1'))).toEqual(['data']);
    expect(ok(DnsRecordSchema, rec('CNAME', 'target.example.com.'))).toBe(true);
    expect(errorPaths(DnsRecordSchema, rec('PTR', 'not a name'))).toEqual(['data']);
    expect(ok(DnsRecordSchema, rec('NS', 'ns1.example.com'))).toBe(true);
    expect(ok(DnsRecordSchema, rec('MX', '10 mail.example.com'))).toBe(true);
    expect(errorPaths(DnsRecordSchema, rec('MX', 'mail.example.com'))).toEqual(['data']);
    expect(ok(DnsRecordSchema, rec('SRV', '10 60 5060 sip.example.com'))).toBe(true);
    expect(errorPaths(DnsRecordSchema, rec('SRV', '10 5060 sip.example.com'))).toEqual(['data']);
    expect(ok(DnsRecordSchema, rec('TXT', 'v=spf1 -all'))).toBe(true);
    expect(ok(DnsRecordSchema, rec('TXT', `a${NUL}b`))).toBe(false);
    expect(ok(DnsRecordSchema, rec('HINFO', 'x'))).toBe(false);
    expect(DnsRecordSchema.parse(rec('A', '192.0.2.1')).ttlSec).toBe(3600);
  });
  it('upstreams: tlsServerName needs tls', () => {
    expect(
      errorPaths(DnsUpstreamSchema, { address: '9.9.9.9', tlsServerName: 'dns.quad9.net' }),
    ).toEqual(['tlsServerName']);
    expect(DnsUpstreamSchema.parse({ address: '9.9.9.9' })).toEqual({
      address: '9.9.9.9',
      port: 53,
      tls: false,
    });
  });
  const resolver = (extra: Record<string, unknown> = {}): Record<string, unknown> => ({
    listen: [{ address: '192.168.10.1' }],
    ...extra,
  });
  it('resolver defaults', () => {
    expect(DnsResolverSchema.parse(resolver())).toMatchObject({
      enabled: true,
      vrf: 'default',
      listen: [{ address: '192.168.10.1', port: 53 }],
      dnssec: { enabled: true, trustAnchorAuto: true },
      cache: { minTtlSec: 0, maxTtlSec: 86400, prefetch: false, msgCacheMb: 4, rrsetCacheMb: 8 },
      threads: 1,
      qnameMinimisation: true,
      hideIdentity: true,
      hideVersion: true,
      logQueries: false,
    });
  });
  it.each([
    ['no listen address', resolver({ listen: [] }), 'listen'],
    [
      'duplicate listen socket',
      resolver({ listen: [{ address: '192.168.10.1' }, { address: '192.168.10.1', port: 53 }] }),
      'listen.1',
    ],
    [
      'forwarder equal to a listen address (loop)',
      resolver({ forwarders: [{ address: '192.168.10.1' }] }),
      'forwarders.0',
    ],
    [
      'duplicate forwarder',
      resolver({ forwarders: [{ address: '9.9.9.9' }, { address: '9.9.9.9' }] }),
      'forwarders.1',
    ],
    [
      'duplicate forward zone (case-insensitive)',
      resolver({
        forwardZones: [
          { zone: 'Corp.Example', forwarders: [{ address: '10.0.0.53' }] },
          { zone: 'corp.example', forwarders: [{ address: '10.0.0.54' }] },
        ],
      }),
      'forwardZones.1',
    ],
    [
      'forward zone without forwarders',
      resolver({ forwardZones: [{ zone: 'corp.example', forwarders: [] }] }),
      'forwardZones.0.forwarders',
    ],
    [
      'duplicate local zone',
      resolver({ localZones: [{ zone: 'lab.example' }, { zone: 'lab.example' }] }),
      'localZones.1',
    ],
    [
      'duplicate record',
      resolver({
        localZones: [
          {
            zone: 'lab.example',
            records: [
              { name: 'a.lab.example', type: 'A', data: '10.0.0.1' },
              { name: 'A.lab.example', type: 'A', data: '10.0.0.1' },
            ],
          },
        ],
      }),
      'localZones.0.records.1',
    ],
    [
      'duplicate access-control prefix',
      resolver({
        accessControl: [{ prefix: '10.0.0.0/8' }, { prefix: '10.0.0.0/8', action: 'deny' }],
      }),
      'accessControl.1',
    ],
    [
      'cache min TTL above max TTL',
      resolver({ cache: { minTtlSec: 100, maxTtlSec: 50 } }),
      'cache.minTtlSec',
    ],
    ['65 threads', resolver({ threads: 65 }), 'threads'],
    ['unknown Unbound option passthrough', resolver({ 'server:': {} }), 'server:'],
  ])('resolver rejects %s', (_label, value, path) =>
    expect(errorPaths(DnsResolverSchema, value)).toContain(path),
  );
});

describe('SNMP', () => {
  const snmp = (extra: Record<string, unknown> = {}): Record<string, unknown> => ({
    enabled: true,
    communities: { mon: { secretRef: 'password/snmp-mon' } },
    v3Users: { noc: { authRef: 'password/snmp-noc-auth', privRef: 'password/snmp-noc-priv' } },
    ...extra,
  });
  it('defaults', () => {
    expect(SnmpSchema.parse(snmp())).toMatchObject({
      vrf: 'default',
      listen: [],
      communities: { mon: { access: 'ro', sources: [] } },
      v3Users: {
        noc: { securityLevel: 'authPriv', authProtocol: 'sha', privProtocol: 'aes', access: 'ro' },
      },
      trapReceivers: [],
    });
    expect(SnmpSchema.parse({})).toMatchObject({ enabled: false });
  });
  it.each([
    [
      'inline community string',
      snmp({ communities: { pub: { community: 'public' } } }),
      'communities.pub.community',
    ],
    [
      'community string pasted as the reference',
      snmp({ communities: { pub: { secretRef: 'public' } } }),
      'communities.pub.secretRef',
    ],
    [
      'community reference of the wrong kind',
      snmp({ communities: { pub: { secretRef: 'key/public' } } }),
      'communities.pub.secretRef',
    ],
    [
      'community without secretRef',
      snmp({ communities: { pub: { access: 'ro' } } }),
      'communities.pub.secretRef',
    ],
    [
      'authPriv without privRef',
      snmp({ v3Users: { u: { authRef: 'password/u-auth' } } }),
      'v3Users.u.privRef',
    ],
    [
      'authNoPriv with privRef',
      snmp({
        v3Users: {
          u: {
            securityLevel: 'authNoPriv',
            authRef: 'password/u-auth',
            privRef: 'password/u-priv',
          },
        },
      }),
      'v3Users.u.privRef',
    ],
    [
      'authNoPriv without authRef',
      snmp({ v3Users: { u: { securityLevel: 'authNoPriv' } } }),
      'v3Users.u.authRef',
    ],
    [
      'noAuthNoPriv with authRef',
      snmp({ v3Users: { u: { securityLevel: 'noAuthNoPriv', authRef: 'password/u-auth' } } }),
      'v3Users.u.authRef',
    ],
    [
      'inline auth password',
      snmp({ v3Users: { u: { authPassword: 'x', privRef: 'password/u-priv' } } }),
      'v3Users.u.authPassword',
    ],
    [
      'v2c trap without community',
      snmp({ trapReceivers: [{ address: '10.0.0.5' }] }),
      'trapReceivers.0.community',
    ],
    [
      'v2c trap with a user',
      snmp({ trapReceivers: [{ address: '10.0.0.5', community: 'mon', user: 'noc' }] }),
      'trapReceivers.0.user',
    ],
    [
      'v3 trap without user',
      snmp({ trapReceivers: [{ address: '10.0.0.5', version: 'v3' }] }),
      'trapReceivers.0.user',
    ],
    [
      'v3 trap with a community',
      snmp({
        trapReceivers: [{ address: '10.0.0.5', version: 'v3', user: 'noc', community: 'mon' }],
      }),
      'trapReceivers.0.community',
    ],
    [
      'trap referencing an undefined community',
      snmp({ trapReceivers: [{ address: '10.0.0.5', community: 'nope' }] }),
      'trapReceivers.0.community',
    ],
    [
      'trap referencing an undefined user',
      snmp({ trapReceivers: [{ address: '10.0.0.5', version: 'v3', user: 'nope' }] }),
      'trapReceivers.0.user',
    ],
    [
      'duplicate trap receiver',
      snmp({
        trapReceivers: [
          { address: '10.0.0.5', community: 'mon' },
          { address: '10.0.0.5', community: 'mon' },
        ],
      }),
      'trapReceivers.1',
    ],
    [
      'duplicate listen socket',
      snmp({ listen: [{ address: '10.0.0.1' }, { address: '10.0.0.1' }] }),
      'listen.1',
    ],
    ['odd-length engine id', snmp({ engineId: '80000' }), 'engineId'],
    ['short engine id', snmp({ engineId: '8000' }), 'engineId'],
  ])('rejects %s', (_label, value, path) => expect(errorPaths(SnmpSchema, value)).toContain(path));
  it('accepts a valid v3 trap receiver and engine id', () => {
    expect(
      ok(
        SnmpSchema,
        snmp({
          engineId: '8000000001020304',
          trapReceivers: [{ address: 'nms.example', version: 'v3', user: 'noc', inform: true }],
        }),
      ),
    ).toBe(true);
  });
});

describe('LLDP, IPFIX, sFlow', () => {
  it('LLDP', () => {
    expect(LldpSchema.parse({})).toEqual({
      enabled: false,
      txHold: 4,
      txIntervalSec: 30,
      interfaces: [],
    });
    expect(
      errorPaths(LldpSchema, { interfaces: [{ interface: 'loop0' }, { interface: 'loop0' }] }),
    ).toEqual(['interfaces.1']);
    expect(
      errorPaths(LldpSchema, { interfaces: [{ interface: 'loop0', mgmtOid: 'abc' }] }),
    ).toEqual(['interfaces.0.mgmtOid']);
    expect(
      errorPaths(LldpSchema, { interfaces: [{ interface: 'loop0', mgmtIpv4: '2001:db8::1' }] }),
    ).toEqual(['interfaces.0.mgmtIpv4']);
    expect(errorPaths(LldpSchema, { txHold: 11 })).toEqual(['txHold']);
    expect(
      ok(LldpSchema, {
        enabled: true,
        interfaces: [{ interface: 'loop0', mgmtIpv6: '2001:db8::1', mgmtOid: '1.3.6.1' }],
      }),
    ).toBe(true);
  });
  it('IPFIX exporter', () => {
    const e = { collector: { address: '192.168.10.6' }, sourceAddress: '192.168.10.1' };
    expect(IpfixExporterSchema.parse(e)).toMatchObject({
      collector: { port: 4739 },
      pathMtu: 512,
      templateIntervalSec: 20,
      udpChecksum: false,
      vrf: 'default',
    });
    expect(errorPaths(IpfixExporterSchema, { ...e, sourceAddress: '2001:db8::1' })).toEqual([
      'sourceAddress',
    ]);
    expect(errorPaths(IpfixExporterSchema, { ...e, sourceAddress: '192.168.10.6' })).toEqual([
      'collector.address',
    ]);
    expect(errorPaths(IpfixExporterSchema, { ...e, pathMtu: 1451 })).toEqual(['pathMtu']);
  });
  it('flowprobe', () => {
    expect(IpfixFlowprobeSchema.parse({})).toMatchObject({
      activeTimerSec: 15,
      passiveTimerSec: 120,
      recordL2: false,
      recordL3: true,
      recordL4: true,
      interfaces: [],
    });
    expect(errorPaths(IpfixFlowprobeSchema, { activeTimerSec: 100, passiveTimerSec: 50 })).toEqual([
      'passiveTimerSec',
    ]);
    expect(
      errorPaths(IpfixFlowprobeSchema, {
        interfaces: [{ interface: 'loop0', l2: false, ip4: false, ip6: false }],
      }),
    ).toEqual(['interfaces.0.ip4']);
    expect(
      errorPaths(IpfixFlowprobeSchema, {
        interfaces: [{ interface: 'loop0' }, { interface: 'loop0' }],
      }),
    ).toEqual(['interfaces.1']);
    expect(
      errorPaths(IpfixFlowprobeSchema, {
        recordL3: false,
        recordL4: false,
        interfaces: [{ interface: 'loop0' }],
      }),
    ).toEqual(['recordL3']);
    expect(ok(IpfixFlowprobeSchema, { recordL3: false, recordL4: false })).toBe(true);
  });
  it('sFlow', () => {
    const s = { collectors: [{ address: '192.168.10.6' }] };
    expect(SflowSchema.parse(s)).toMatchObject({
      enabled: false,
      samplingN: 10000,
      pollingIntervalSec: 20,
      headerBytes: 128,
      collectors: [{ port: 6343 }],
      interfaces: [],
    });
    expect(errorPaths(SflowSchema, { collectors: [] })).toEqual(['collectors']);
    expect(errorPaths(SflowSchema, { ...s, interfaces: ['loop0', 'loop0'] })).toEqual([
      'interfaces.1',
    ]);
    expect(
      errorPaths(SflowSchema, { collectors: [{ address: '10.0.0.1' }, { address: '10.0.0.1' }] }),
    ).toEqual(['collectors.1']);
    expect(errorPaths(SflowSchema, { ...s, samplingN: 0 })).toEqual(['samplingN']);
  });
});

describe('NTP (services.ntp is the only NTP model — D-050)', () => {
  it('defaults and a valid server list', () => {
    expect(NtpSchema.parse({})).toEqual({
      enabled: false,
      vrf: 'default',
      servers: [],
      pools: [],
      allow: [],
      listen: [],
      deny: [],
      port: 123,
      orphan: false,
      rtcSync: true,
      makestep: { thresholdSec: 1, limit: 3 },
    });
    expect(NtpSchema.parse({ servers: [{ address: 'time.example' }] }).servers[0]).toEqual({
      address: 'time.example',
      iburst: true,
      prefer: false,
      nts: false,
    });
    expect(ok(NtpSchema, { enabled: true, localStratum: 10 })).toBe(true);
    expect(ok(NtpSchema, { enabled: true, pools: ['pool.ntp.org'] })).toBe(true);
    expect(
      ok(NtpSchema, {
        enabled: true,
        servers: [{ address: '10.0.0.1', keyRef: 'key/ntp-1' }],
        allow: ['192.168.0.0/16'],
        deny: ['192.168.99.0/24'],
        rateLimit: {},
        localStratum: 8,
        orphan: true,
        ntsServer: { certificateRef: 'cert/ntp', keyRef: 'key/ntp' },
      }),
    ).toBe(true);
  });
  it.each([
    ['enabled without any source', { enabled: true }, 'servers'],
    ['invalid server name', { servers: [{ address: 'bad host' }] }, 'servers.0.address'],
    ['mistyped IPv4 server (F12)', { servers: [{ address: '10.0.0.300' }] }, 'servers.0.address'],
    ['all-numeric pool (F12)', { pools: ['10.0.0'] }, 'pools.0'],
    [
      'duplicate server (case-insensitive)',
      { servers: [{ address: 'Time.Example' }, { address: 'time.example' }] },
      'servers.1',
    ],
    ['duplicate pool', { pools: ['a.example', 'a.example'] }, 'pools.1'],
    [
      'minPoll above maxPoll',
      { servers: [{ address: '10.0.0.1', minPoll: 6, maxPoll: 4 }] },
      'servers.0.minPoll',
    ],
    [
      'NTS with a symmetric key',
      { servers: [{ address: '10.0.0.1', nts: true, keyRef: 'key/ntp-1' }] },
      'servers.0.keyRef',
    ],
    [
      'inline key',
      { servers: [{ address: '10.0.0.1', key: 'HMAC-SHA256 secret' }] },
      'servers.0.key',
    ],
    [
      'key pasted into keyRef',
      { servers: [{ address: '10.0.0.1', keyRef: 'HMACSHA256secret' }] },
      'servers.0.keyRef',
    ],
    ['duplicate allow prefix', { allow: ['10.0.0.0/8', '10.0.0.0/8'] }, 'allow.1'],
    ['duplicate deny prefix', { deny: ['10.0.0.0/8', '10.0.0.0/8'] }, 'deny.1'],
    ['allow without prefix length', { allow: ['10.0.0.0'] }, 'allow.0'],
    ['duplicate bind address', { listen: ['10.0.0.1', '10.0.0.1'] }, 'listen.1'],
    ['local stratum 16', { localStratum: 16 }, 'localStratum'],
    ['orphan without local stratum', { orphan: true }, 'orphan'],
    ['serving on port 0', { allow: ['10.0.0.0/8'], port: 0 }, 'port'],
    ['rate limit without server mode', { rateLimit: {} }, 'rateLimit'],
    [
      'NTS server with inline PEM',
      { ntsServer: { certificate: 'x', certificateRef: 'cert/a', keyRef: 'key/a' } },
      'ntsServer.certificate',
    ],
    ['makestep threshold 0', { makestep: { thresholdSec: 0 } }, 'makestep.thresholdSec'],
  ])('rejects %s', (_label, value, path) => expect(errorPaths(NtpSchema, value)).toContain(path));
});

describe('QoS (services.qos — D-052)', () => {
  it('policer defaults and algorithm-specific fields', () => {
    expect(QosPolicerSchema.parse({ cir: 10000, cb: 12500 })).toEqual({
      type: '1r2c',
      rateUnit: 'kbps',
      cir: 10000,
      cb: 12500,
      round: 'closest',
      colorAware: false,
      conformAction: { action: 'transmit' },
      exceedAction: { action: 'drop' },
      violateAction: { action: 'drop' },
    });
    expect(
      ok(QosPolicerSchema, {
        type: '2r3c-rfc2698',
        cir: 1000,
        eir: 2000,
        cb: 1500,
        eb: 3000,
        colorAware: true,
        exceedAction: { action: 'mark-and-transmit', dscp: 10 },
      }),
    ).toBe(true);
    expect(ok(QosPolicerSchema, { type: '1r3c-rfc2697', cir: 1000, cb: 1, eb: 1 })).toBe(true);
  });
  it.each([
    ['cir 0', { cir: 0, cb: 1 }, 'cir'],
    ['two-rate without eir', { type: '2r3c-rfc4115', cir: 1, cb: 1, eb: 1 }, 'eir'],
    ['eir on a single-rate policer', { cir: 1, cb: 1, eir: 2 }, 'eir'],
    ['three-colour without eb', { type: '1r3c-rfc2697', cir: 1, cb: 1 }, 'eb'],
    ['eb on a two-colour policer', { cir: 1, cb: 1, eb: 1 }, 'eb'],
    [
      'RFC 2698 peak below cir',
      { type: '2r3c-rfc2698', cir: 2000, eir: 1000, cb: 1, eb: 1 },
      'eir',
    ],
    ['colour aware on 1r2c', { cir: 1, cb: 1, colorAware: true }, 'colorAware'],
    [
      'violate action on 1r2c',
      { cir: 1, cb: 1, violateAction: { action: 'transmit' } },
      'violateAction',
    ],
    [
      'mark without dscp',
      { cir: 1, cb: 1, conformAction: { action: 'mark-and-transmit' } },
      'conformAction.dscp',
    ],
    [
      'dscp without mark',
      { cir: 1, cb: 1, conformAction: { action: 'transmit', dscp: 4 } },
      'conformAction.dscp',
    ],
    [
      'dscp 64',
      { cir: 1, cb: 1, conformAction: { action: 'mark-and-transmit', dscp: 64 } },
      'conformAction.dscp',
    ],
  ])('policer rejects %s', (_label, value, path) =>
    expect(errorPaths(QosPolicerSchema, value)).toContain(path),
  );
  it('shapers and maps', () => {
    expect(ok(QosShaperSchema, { rateKbps: 100000 })).toBe(true);
    expect(ok(QosShaperSchema, { rateKbps: 0 })).toBe(false);
    expect(ok(QosShaperSchema, { rateKbps: 1, burstBytes: 63 })).toBe(false);
    expect(ok(QosMapSchema, { id: 1, rows: { ip: [{ from: 46, to: 5 }] } })).toBe(true);
    expect(errorPaths(QosMapSchema, {})).toEqual(['rows']);
    expect(errorPaths(QosMapSchema, { rows: { vlan: [{ from: 8, to: 0 }] } })).toEqual([
      'rows.vlan.0.from',
    ]);
    expect(
      errorPaths(QosMapSchema, {
        rows: {
          ip: [
            { from: 1, to: 1 },
            { from: 1, to: 2 },
          ],
        },
      }),
    ).toEqual(['rows.ip.1']);
  });
  it.each([
    ['empty attachment', {}, ''],
    ['empty policer block', { policer: {} }, 'policer'],
    ['shaper and output policer', { shaper: 's', policer: { output: 'p' } }, 'shaper'],
    ['store value beyond the source', { store: { source: 'vlan', value: 8 } }, 'store.value'],
    [
      'record and store on one slot',
      { record: 'ip', store: { source: 'ip', value: 1 } },
      'store.source',
    ],
    ['unknown mark header', { mark: { map: 'm', output: 'dscp' } }, 'mark.output'],
  ])('interface attachment rejects %s', (_label, value, path) =>
    expect(errorPaths(QosInterfaceSchema, value)).toContain(path),
  );
  it('accepts a complete attachment and defaults the root', () => {
    expect(
      ok(QosInterfaceSchema, {
        policer: { input: 'p' },
        shaper: 's',
        record: 'ip',
        store: { source: 'ext', value: 200 },
        mark: { map: 'm', output: 'vlan' },
      }),
    ).toBe(true);
    expect(QosSchema.parse({})).toEqual({ policers: {}, shapers: {}, maps: {}, interfaces: {} });
    expect(ok(QosSchema, { hqos: {} })).toBe(false);
  });
});

describe('ServicesSchema root', () => {
  it('parses {} to a populated root (prefault, D-017/D-053)', () => {
    const parsed = ServicesSchema.parse({});
    expect(Object.keys(parsed)).toEqual(['dhcp', 'dns', 'snmp', 'lldp', 'ipfix', 'ntp', 'qos']);
    expect(parsed.dhcp).toEqual({ servers: {}, relays: {} });
    expect(parsed.dns).toEqual({ resolvers: {} });
    expect(parsed.snmp).toMatchObject({ enabled: false, communities: {}, v3Users: {} });
    expect(parsed.lldp).toMatchObject({ enabled: false, interfaces: [] });
    expect(parsed.ipfix).toMatchObject({ exporters: {}, flowprobe: { activeTimerSec: 15 } });
    expect(parsed.ipfix.sflow).toBeUndefined();
    expect(parsed.ntp).toMatchObject({ enabled: false, servers: [] });
    expect(parsed.qos).toEqual({ policers: {}, shapers: {}, maps: {}, interfaces: {} });
  });
  it('rejects unknown services and non-objects', () => {
    expect(ok(ServicesSchema, { tftp: {} })).toBe(false);
    expect(ok(ServicesSchema, { dhcp: [] })).toBe(false);
    expect(ok(ServicesSchema, { snmp: 'on' })).toBe(false);
  });
});
