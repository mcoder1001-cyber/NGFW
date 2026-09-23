import { describe, expect, it } from 'vitest';
import type { z } from 'zod';
import { RootConfig } from '../index.js';
import { DataplaneSchema } from './dataplane.js';
import {
  InterfaceSchema,
  InterfacesSchema,
  interfaceNames,
  parentInterfaceName,
  SubinterfaceSchema,
  subInterfaceId,
  subInterfaceName,
} from './interfaces.js';
import { AaaSchema, sshPublicKey, TlsSchema, UserSchema } from './management.js';
import {
  bgpAsPathRegex,
  bgpCommunity,
  BgpNeighborSchema,
  BgpPeerGroupSchema,
  BgpSchema,
  BfdSessionSchema,
  IsisSchema,
  isisNet,
  NextHopSchema,
  ospfAreaId,
  ospfAreaNumber,
  OspfSchema,
  PrefixListRuleSchema,
  PrefixListSchema,
  RipSchema,
  RouteMapSchema,
  RoutingSchema,
  StaticRouteSchema,
} from './routing.js';
import { SystemSchema } from './system.js';
import { VrfSchema, vrfExists } from './vrfs.js';

/** First failing path of a parse, as a plain array — the assertion target for refinements. */
const NET = '49.0001.1921.6800.1001.00';

const failPath = (schema: z.ZodType, value: unknown): unknown[] | undefined =>
  schema.safeParse(value).error?.issues[0]?.path.map(String);

describe('system', () => {
  it('fills defaults for the empty section', () => {
    expect(SystemSchema.parse({})).toEqual({
      hostname: 'vrx',
      timezone: 'UTC',
      banner: {},
      dns: { servers: [], searchDomains: [], vrf: 'default' },
    });
  });
  it('rejects unknown keys and bad values with the offending path', () => {
    expect(failPath(SystemSchema, { hostnme: 'x' })).toEqual([]);
    expect(failPath(SystemSchema, { timezone: 'Mars/Olympus' })).toEqual(['timezone']);
    expect(failPath(SystemSchema, { ntp: {} })).toEqual([]); // D-050: NTP lives in services.ntp
    expect(failPath(SystemSchema, { dns: { servers: ['dns.example'] } })).toEqual([
      'dns',
      'servers',
      '0',
    ]);
    expect(failPath(SystemSchema, { banner: { login: 'x'.repeat(4097) } })).toEqual([
      'banner',
      'login',
    ]);
  });

  it('banners are multi-line but reject terminal control sequences (review M1)', () => {
    expect(SystemSchema.safeParse({ banner: { login: 'Authorised\n\tonly' } }).success).toBe(true);
    for (const bad of ['a\r\nb', 'x\u001b[2J', 'bell\u0007', 'c1\u009b'])
      expect(failPath(SystemSchema, { banner: { motd: bad } })).toEqual(['banner', 'motd']);
  });
});

describe('dataplane', () => {
  it('accepts the empty section and enforces bounds', () => {
    expect(DataplaneSchema.parse({})).toEqual({
      pciWhitelist: [],
      managementPci: [],
      devices: {},
    });
    expect(
      DataplaneSchema.safeParse({
        workers: 4,
        corelist: [2, 3, 4, 5],
        mainCore: 1,
        rxQueues: 2,
        txQueues: 2,
        hugepagesGb: 4,
      }).success,
    ).toBe(true);
    expect(failPath(DataplaneSchema, { workers: 256 })).toEqual(['workers']);
    expect(failPath(DataplaneSchema, { corelist: [1024] })).toEqual(['corelist', '0']);
    expect(failPath(DataplaneSchema, { hugepagesGb: 0 })).toEqual(['hugepagesGb']);
    expect(failPath(DataplaneSchema, { pciWhitelist: ['0b:00.0'] })).toEqual(['pciWhitelist', '0']);
  });
});

describe('vrfs', () => {
  it('requires an id and treats default as implicit', () => {
    expect(VrfSchema.safeParse({ id: 4294967295 }).success).toBe(true);
    expect(failPath(VrfSchema, {})).toEqual(['id']);
    expect(failPath(VrfSchema, { id: -1 })).toEqual(['id']);
    expect(failPath(VrfSchema, { id: 1.5 })).toEqual(['id']);
    expect(vrfExists({}, 'default')).toBe(true);
    expect(vrfExists({ a: { id: 1 } }, 'a')).toBe(true);
    expect(vrfExists({ a: { id: 1 } }, 'b')).toBe(false);
  });
});

describe('interfaces', () => {
  it('keys are parent names; sub-interface ids are decimal u32', () => {
    expect(parentInterfaceName.safeParse('TenGigabitEthernet0/0/0').success).toBe(true);
    expect(parentInterfaceName.safeParse('vmxnet3-0/b/0/0').success).toBe(true);
    expect(parentInterfaceName.safeParse('TenGigabitEthernet0/0/0.100').success).toBe(false);
    expect(failPath(InterfacesSchema, { 'TenGigabitEthernet0/0/0.100': {} })).toEqual([
      'TenGigabitEthernet0/0/0.100',
    ]);
    for (const ok of ['0', '100', '4294967295'])
      expect(subInterfaceId.safeParse(ok).success).toBe(true);
    for (const bad of ['', '01', '-1', '1.5', '4294967296', '10000000000', 'a'])
      expect(subInterfaceId.safeParse(bad).success).toBe(false);
  });

  it('applies defaults and rejects unknown keys', () => {
    expect(InterfaceSchema.parse({})).toEqual({
      enabled: false,
      promiscuous: false,
      ipv4: [],
      ipv6: [],
      vrf: 'default',
      subinterfaces: {},
    });
    expect(SubinterfaceSchema.parse({ vlanId: 5 })).toEqual({
      vlanId: 5,
      dot1ad: false,
      enabled: false,
      ipv4: [],
      ipv6: [],
      vrf: 'default',
    });
    expect(failPath(InterfaceSchema, { mtuu: 1500 })).toEqual([]);
    expect(failPath(InterfaceSchema, { mtu: 67 })).toEqual(['mtu']);
    expect(failPath(InterfaceSchema, { rxMode: 'fast' })).toEqual(['rxMode']);
    expect(failPath(InterfaceSchema, { ipv4: ['10.0.0.1'] })).toEqual(['ipv4', '0']);
    expect(failPath(InterfaceSchema, { ipv6: ['10.0.0.1/24'] })).toEqual(['ipv6', '0']);
    expect(failPath(SubinterfaceSchema, {})).toEqual(['vlanId']);
    expect(failPath(SubinterfaceSchema, { vlanId: 4095 })).toEqual(['vlanId']);
  });

  it('dhcpClient: present = enabled, defaults, exclusive with unnumbered (D-050)', () => {
    expect(InterfaceSchema.parse({ dhcpClient: {} }).dhcpClient).toEqual({
      setBroadcastFlag: false,
    });
    expect(
      SubinterfaceSchema.safeParse({
        vlanId: 1,
        dhcpClient: { hostname: 'vrx-a', clientId: 'vrx-a-wan', setBroadcastFlag: true },
      }).success,
    ).toBe(true);
    expect(failPath(InterfaceSchema, { dhcpClient: { clientId: 'has space' } })).toEqual([
      'dhcpClient',
      'clientId',
    ]);
    expect(failPath(InterfaceSchema, { dhcpClient: { bogus: 1 } })).toEqual(['dhcpClient']);
    expect(failPath(InterfaceSchema, { unnumbered: 'loop0', dhcpClient: {} })).toEqual([
      'unnumbered',
    ]);
  });

  it('forbids own addresses on unnumbered (sub-)interfaces', () => {
    expect(InterfaceSchema.safeParse({ unnumbered: 'loop0' }).success).toBe(true);
    expect(failPath(InterfaceSchema, { unnumbered: 'loop0', ipv4: ['10.0.0.1/24'] })).toEqual([
      'unnumbered',
    ]);
    expect(
      failPath(SubinterfaceSchema, { vlanId: 1, unnumbered: 'loop0', ipv6: ['2001:db8::1/64'] }),
    ).toEqual(['unnumbered']);
  });

  it('enumerates parent and sub-interface names', () => {
    const parsed = InterfacesSchema.parse({
      'TenGigabitEthernet0/0/0': { subinterfaces: { '100': { vlanId: 100 } } },
      loop0: {},
    });
    expect([...interfaceNames(parsed)].sort()).toEqual([
      'TenGigabitEthernet0/0/0',
      'TenGigabitEthernet0/0/0.100',
      'loop0',
    ]);
    expect(subInterfaceName('loop0', 5)).toBe('loop0.5');
  });
});

describe('routing', () => {
  it('next hops need an address or an interface; families must match the prefix', () => {
    expect(NextHopSchema.parse({ address: '10.0.0.1' })).toEqual({
      address: '10.0.0.1',
      weight: 1,
    });
    expect(NextHopSchema.safeParse({ interface: 'loop0' }).success).toBe(true);
    expect(failPath(NextHopSchema, {})).toEqual(['address']);
    expect(failPath(NextHopSchema, { address: '10.0.0.1', weight: 0 })).toEqual(['weight']);
    expect(
      StaticRouteSchema.safeParse({
        prefix: '10.0.0.0/8',
        nextHops: [{ address: '10.1.1.1' }, { interface: 'loop0' }],
      }).success,
    ).toBe(true);
    expect(
      StaticRouteSchema.safeParse({
        prefix: '::/0',
        nextHops: [{ address: 'fe80::1', interface: 'loop0' }],
      }).success,
    ).toBe(true);
    expect(
      failPath(StaticRouteSchema, { prefix: '10.0.0.0/8', nextHops: [{ address: '2001:db8::1' }] }),
    ).toEqual(['nextHops']);
    expect(
      failPath(StaticRouteSchema, { prefix: '10.0.0.1/8', nextHops: [{ address: '10.1.1.1' }] }),
    ).toEqual(['prefix']);
  });

  it('blackhole routes have no next hops; every other route has at least one (review M7, D-045)', () => {
    expect(StaticRouteSchema.parse({ prefix: '192.0.2.0/24', blackhole: true })).toEqual({
      prefix: '192.0.2.0/24',
      vrf: 'default',
      nextHops: [],
      blackhole: true,
      distance: 1,
    });
    expect(failPath(StaticRouteSchema, { prefix: '10.0.0.0/8', nextHops: [] })).toEqual([
      'nextHops',
    ]);
    expect(failPath(StaticRouteSchema, { prefix: '10.0.0.0/8' })).toEqual(['nextHops']);
    expect(
      failPath(StaticRouteSchema, {
        prefix: '10.0.0.0/8',
        blackhole: true,
        nextHops: [{ address: '10.0.0.1' }],
      }),
    ).toEqual(['nextHops']);
  });

  it('prefix-list rules follow FRR length rules: len < ge ≤ le ≤ max (review L5)', () => {
    const rule = (extra: object) =>
      PrefixListRuleSchema.safeParse({ seq: 1, action: 'permit', prefix: '10.0.0.0/8', ...extra });
    expect(rule({ ge: 16, le: 24 }).success).toBe(true);
    expect(rule({ le: 8 }).success).toBe(true);
    expect(rule({ ge: 9 }).success).toBe(true);
    expect(rule({ ge: 32, le: 32 }).success).toBe(true);
    const path = (extra: object) => rule(extra).error?.issues.map((i) => i.path.join('.'));
    expect(path({ ge: 8 })).toEqual(['ge']); // ge must exceed the prefix length
    expect(path({ ge: 33 })).toEqual(['ge']); // IPv4 max is 32
    expect(path({ le: 7 })).toEqual(['le']);
    expect(path({ le: 33 })).toEqual(['le']);
    expect(path({ ge: 24, le: 16 })).toEqual(['le']);
    expect(
      PrefixListRuleSchema.safeParse({
        seq: 1,
        action: 'deny',
        prefix: '2001:db8::/32',
        ge: 48,
        le: 128,
      }).success,
    ).toBe(true);
    expect(
      PrefixListRuleSchema.safeParse({ seq: 1, action: 'deny', prefix: '10.0.0.1/8' }).success,
    ).toBe(false);
    expect(
      failPath(PrefixListRuleSchema, { seq: 0, action: 'permit', prefix: '10.0.0.0/8' }),
    ).toEqual(['seq']);
  });

  it('prefix lists: unique sequences, family-consistent prefixes', () => {
    expect(PrefixListSchema.parse({})).toEqual({ family: 'ipv4', rules: [] });
    expect(
      failPath(PrefixListSchema, {
        rules: [
          { seq: 1, action: 'permit', prefix: '10.0.0.0/8' },
          { seq: 1, action: 'deny', prefix: '0.0.0.0/0' },
        ],
      }),
    ).toEqual(['rules']);
    expect(
      failPath(PrefixListSchema, {
        family: 'ipv6',
        rules: [{ seq: 1, action: 'permit', prefix: '10.0.0.0/8' }],
      }),
    ).toEqual(['rules']);
    expect(
      PrefixListSchema.safeParse({
        family: 'ipv6',
        rules: [{ seq: 1, action: 'permit', prefix: '2001:db8::/32' }],
      }).success,
    ).toBe(true);
  });

  it('route maps: unique sequences, empty match/set allowed, P12 set names', () => {
    expect(RouteMapSchema.parse({ entries: [{ seq: 10, action: 'permit' }] })).toEqual({
      entries: [{ seq: 10, action: 'permit', match: {}, set: { communityAdditive: false } }],
    });
    expect(
      RouteMapSchema.safeParse({
        entries: [{ seq: 10, action: 'permit', set: { localPref: 200, med: 10 } }],
      }).success,
    ).toBe(true);
    expect(
      failPath(RouteMapSchema, {
        entries: [{ seq: 10, action: 'permit', set: { localPreference: 200 } }],
      }),
    ).toEqual(['entries', '0', 'set']);
    expect(
      failPath(RouteMapSchema, {
        entries: [
          { seq: 10, action: 'permit' },
          { seq: 10, action: 'deny' },
        ],
      }),
    ).toEqual(['entries']);
    expect(
      failPath(RouteMapSchema, {
        entries: [{ seq: 10, action: 'permit', set: { community: ['65000:70000'] } }],
      }),
    ).toEqual(['entries', '0', 'set', 'community', '0']);
    for (const ok of ['65000:100', '0:0', 'no-export', 'internet'])
      expect(bgpCommunity.safeParse(ok).success).toBe(true);
    for (const bad of ['65000', '65000:', 'a:b', '65536:1x', 'NO-EXPORT'])
      expect(bgpCommunity.safeParse(bad).success).toBe(false);
  });

  it('as-path regexes cannot inject FRR configuration lines (review M1)', () => {
    for (const ok of ['^65000_', '_65001$', '^(65000|65001)_[0-9]+$', '_6500[0-9]{1,2}_', '.*'])
      expect(bgpAsPathRegex.safeParse(ok).success).toBe(true);
    for (const bad of [
      '^65000_\nno router bgp\r!',
      '^65000_\n',
      'x',
      '65000; exit',
      '\\',
      '',
      '1'.repeat(256),
    ])
      expect(bgpAsPathRegex.safeParse(bad).success).toBe(false);
    expect(
      failPath(RouteMapSchema, {
        entries: [{ seq: 1, action: 'permit', match: { asPath: '^1_\nline vty' } }],
      }),
    ).toEqual(['entries', '0', 'match', 'asPath']);
  });

  it('bgp: neighbours keyed by address, afi per P12, hold time above keepalive, remoteAs or peer group', () => {
    expect(BgpNeighborSchema.parse({ remoteAs: 65001 })).toEqual({
      remoteAs: 65001,
      shutdown: false,
      bfd: false,
      afi: {},
    });
    expect(
      BgpNeighborSchema.parse({ remoteAs: 1, afi: { ipv4Unicast: {} } }).afi.ipv4Unicast,
    ).toEqual({
      enabled: true,
      nextHopSelf: false,
      softReconfig: false,
      defaultOriginate: false,
    });
    expect(
      failPath(BgpNeighborSchema, { remoteAs: 65001, keepaliveSec: 90, holdTimeSec: 90 }),
    ).toEqual(['holdTimeSec']);
    expect(failPath(BgpNeighborSchema, {})).toEqual(['remoteAs']);
    expect(failPath(BgpNeighborSchema, { remoteAs: 1, address: '10.0.0.1' })).toEqual([]);
    expect(failPath(BgpNeighborSchema, { remoteAs: 1, ipv4Unicast: {} })).toEqual([]);
    expect(BgpNeighborSchema.safeParse({ peerGroup: 'g' }).success).toBe(true);
    expect(BgpPeerGroupSchema.safeParse({ keepaliveSec: 10, holdTimeSec: 30 }).success).toBe(true);
    expect(failPath(BgpPeerGroupSchema, { keepaliveSec: 30, holdTimeSec: 10 })).toEqual([
      'holdTimeSec',
    ]);
    expect(failPath(BgpNeighborSchema, { remoteAs: 0 })).toEqual(['remoteAs']);
    expect(failPath(BgpNeighborSchema, { remoteAs: 1, passwordRef: 'psk/x' })).toEqual([
      'passwordRef',
    ]);
    expect(BgpNeighborSchema.safeParse({ remoteAs: 1, passwordRef: 'password/x' }).success).toBe(
      true,
    );
    const bgp = BgpSchema.parse({
      asn: 65000,
      neighbors: { '10.0.0.1': { remoteAs: 1 }, '2001:db8::1': { remoteAs: 2 } },
    });
    expect(Object.keys(bgp.neighbors)).toEqual(['10.0.0.1', '2001:db8::1']);
    expect(bgp.redistribute).toEqual({});
    expect(failPath(BgpSchema, { asn: 1, neighbors: { 'not-an-ip': { remoteAs: 1 } } })).toEqual([
      'neighbors',
      'not-an-ip',
    ]);
  });

  it('redistribute is keyed by source protocol and excludes the protocol itself (review M2)', () => {
    expect(
      BgpSchema.parse({ asn: 1, redistribute: { connected: {}, static: { metric: 5 } } })
        .redistribute,
    ).toEqual({ connected: {}, static: { metric: 5 } });
    expect(failPath(BgpSchema, { asn: 1, redistribute: { bgp: {} } })).toEqual(['redistribute']);
    expect(failPath(OspfSchema, { redistribute: { ospf: {} } })).toEqual(['redistribute']);
    expect(failPath(IsisSchema, { net: NET, redistribute: { isis: {} } })).toEqual([
      'redistribute',
    ]);
    expect(failPath(RipSchema, { redistribute: { rip: {} } })).toEqual(['redistribute']);
    expect(RipSchema.safeParse({ redistribute: { bgp: { routeMap: 'rm' } } }).success).toBe(true);
    expect(failPath(OspfSchema, { redistribute: { static: { bogus: 1 } } })).toEqual([
      'redistribute',
      'static',
    ]);
  });

  it('ospf: areas and interfaces are records; area ids are strings that normalise', () => {
    expect(ospfAreaNumber('51')).toBe(51);
    expect(ospfAreaNumber('0.0.0.51')).toBe(51);
    expect(ospfAreaNumber('1.0.0.0')).toBe(16777216);
    for (const ok of ['0', '51', '4294967295', '0.0.0.0', '10.1.2.3'])
      expect(ospfAreaId.safeParse(ok).success).toBe(true);
    for (const bad of ['01', '-1', '4294967296', '1.2.3', 'x', 0])
      expect(ospfAreaId.safeParse(bad).success).toBe(false);
    expect(
      OspfSchema.safeParse({
        areas: { '0': {}, '0.0.0.1': { type: 'nssa' } },
        interfaces: { loop0: { area: '0', passive: true } },
      }).success,
    ).toBe(true);
    expect(failPath(OspfSchema, { areas: { '0': {}, '0.0.0.0': {} } })).toEqual(['areas']);
    expect(failPath(OspfSchema, { areas: { x: {} } })).toEqual(['areas', 'x']);
    expect(failPath(OspfSchema, { interfaces: { loop0: { area: 0 } } })).toEqual([
      'interfaces',
      'loop0',
      'area',
    ]);
    expect(failPath(OspfSchema, { interfaces: { 'bad name': { area: '0' } } })).toEqual([
      'interfaces',
      'bad name',
    ]);
  });

  it('isis / rip: NET format, interfaces keyed by name', () => {
    for (const ok of ['49.0001.1921.6800.1001.00', '49.0001.0000.0000.0001.00'])
      expect(isisNet.safeParse(ok).success).toBe(true);
    for (const bad of ['49.0001.1921.6800.1001', '49.1.2.3.00', '', 'gg.0001.1921.6800.1001.00'])
      expect(isisNet.safeParse(bad).success).toBe(false);
    expect(IsisSchema.parse({ net: NET, interfaces: { loop0: {} } }).interfaces).toEqual({
      loop0: { passive: false, bfd: false },
    });
    expect(RipSchema.parse({ interfaces: { loop0: { passive: true } } })).toEqual({
      vrf: 'default',
      networks: [],
      interfaces: { loop0: { passive: true } },
      redistribute: {},
      defaultMetric: 1,
    });
  });

  it('routing.policy nests prefix lists and route maps (D-045); the old top-level keys are rejected', () => {
    expect(RoutingSchema.parse({})).toEqual({
      static: [],
      policy: { prefixLists: {}, routeMaps: {} },
    });
    expect(failPath(RoutingSchema, { prefixLists: {} })).toEqual([]);
    expect(
      RoutingSchema.safeParse({
        policy: { prefixLists: { pl: {} }, routeMaps: { rm: { entries: [] } } },
      }).success,
    ).toBe(true);
  });

  it('bfd: local and peer address in one family, VPP-scale intervals', () => {
    expect(
      BfdSessionSchema.parse({
        interface: 'loop0',
        localAddress: '10.0.0.1',
        peerAddress: '10.0.0.2',
      }),
    ).toMatchObject({
      desiredMinTxUs: 300000,
      requiredMinRxUs: 300000,
      detectMultiplier: 3,
      enabled: true,
    });
    expect(
      failPath(BfdSessionSchema, {
        interface: 'loop0',
        localAddress: '10.0.0.1',
        peerAddress: '2001:db8::2',
      }),
    ).toEqual(['peerAddress']);
    expect(
      failPath(BfdSessionSchema, {
        interface: 'loop0',
        localAddress: '10.0.0.1',
        peerAddress: '10.0.0.2',
        desiredMinTxUs: 1,
      }),
    ).toEqual(['desiredMinTxUs']);
  });
});

describe('management', () => {
  it('users: scope is always *, hash write-only, keys validated', () => {
    expect(UserSchema.parse({ username: 'admin', role: 'admin' })).toEqual({
      username: 'admin',
      role: 'admin',
      scope: '*',
      sshKeys: [],
      disabled: false,
    });
    expect(failPath(UserSchema, { username: 'admin', role: 'admin', scope: 'tenant-a' })).toEqual([
      'scope',
    ]);
    expect(failPath(UserSchema, { username: 'Admin', role: 'admin' })).toEqual(['username']);
    expect(failPath(UserSchema, { username: 'admin', role: 'root' })).toEqual(['role']);
    expect(
      failPath(UserSchema, { username: 'admin', role: 'admin', passwordHash: 'plaintext' }),
    ).toEqual(['passwordHash']);
    expect(
      sshPublicKey.safeParse(
        'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFakeKeyForFixturesOnly0000000000000000000000 a@b',
      ).success,
    ).toBe(true);
    expect(sshPublicKey.safeParse('ecdsa-sha2-nistp256 AAAA==').success).toBe(true);
    for (const bad of ['ssh-dss AAAA', 'ssh-ed25519', 'ssh-ed25519 AAAA\nssh-rsa BBBB', 'AAAA'])
      expect(sshPublicKey.safeParse(bad).success).toBe(false);
  });

  it('aaa: unique order, servers present for listed methods', () => {
    expect(AaaSchema.parse({})).toEqual({
      order: ['local'],
      radius: { servers: [] },
      tacacs: { servers: [] },
    });
    expect(failPath(AaaSchema, { order: ['local', 'local'] })).toEqual(['order']);
    expect(failPath(AaaSchema, { order: ['radius', 'local'] })).toEqual(['radius', 'servers']);
    expect(failPath(AaaSchema, { order: ['tacacs'] })).toEqual(['tacacs', 'servers']);
    expect(failPath(AaaSchema, { order: [] })).toEqual(['order']);
    expect(
      AaaSchema.safeParse({
        order: ['tacacs', 'radius', 'local'],
        radius: { servers: [{ address: '10.0.0.1', secretRef: 'psk/radius-1' }] },
        tacacs: { servers: [{ address: '10.0.0.2', secretRef: 'psk/tacacs-1', port: 4949 }] },
      }).success,
    ).toBe(true);
  });

  it('tls: certificate and key come together', () => {
    expect(TlsSchema.parse({})).toEqual({ minVersion: '1.2' });
    expect(
      TlsSchema.safeParse({
        certificateRef: 'cert/api',
        privateKeyRef: 'key/api',
        minVersion: '1.3',
      }).success,
    ).toBe(true);
    expect(failPath(TlsSchema, { certificateRef: 'cert/api' })).toEqual(['privateKeyRef']);
    expect(failPath(TlsSchema, { certificateRef: 'key/api', privateKeyRef: 'key/api' })).toEqual([
      'certificateRef',
    ]);
    expect(failPath(TlsSchema, { privateKeyRef: 'key/api' })).toEqual(['privateKeyRef']);
    expect(failPath(TlsSchema, { minVersion: '1.1' })).toEqual(['minVersion']);
  });
});

describe('RootConfig with group (a) domains', () => {
  it('rejects a secret written inline where a secretRef is expected and reports the pointer path', () => {
    const r = RootConfig.safeParse({
      management: {
        aaa: { radius: { servers: [{ address: '10.0.0.1', secretRef: 'hunter2 plain' }] } },
      },
    });
    expect(r.error?.issues[0]?.path).toEqual([
      'management',
      'aaa',
      'radius',
      'servers',
      0,
      'secretRef',
    ]);
  });
});
