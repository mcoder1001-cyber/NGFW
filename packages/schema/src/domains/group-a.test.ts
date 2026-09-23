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
  bgpCommunity,
  BgpNeighborSchema,
  BgpPeerGroupSchema,
  BfdSessionSchema,
  isisNet,
  NextHopSchema,
  ospfAreaNumber,
  OspfSchema,
  PrefixListRuleSchema,
  PrefixListSchema,
  RouteMapSchema,
  StaticRouteSchema,
} from './routing.js';
import { SystemSchema } from './system.js';
import { VrfSchema, vrfExists } from './vrfs.js';

/** First failing path of a parse, as a plain array — the assertion target for refinements. */
const failPath = (schema: z.ZodType, value: unknown): unknown[] | undefined =>
  schema.safeParse(value).error?.issues[0]?.path.map(String);

describe('system', () => {
  it('fills defaults for the empty section', () => {
    expect(SystemSchema.parse({})).toEqual({
      hostname: 'vrx',
      timezone: 'UTC',
      banner: {},
      ntp: { enabled: true, servers: [], vrf: 'default' },
      dns: { servers: [], searchDomains: [], vrf: 'default' },
    });
  });
  it('rejects unknown keys and bad values with the offending path', () => {
    expect(failPath(SystemSchema, { hostnme: 'x' })).toEqual([]);
    expect(failPath(SystemSchema, { timezone: 'Mars/Olympus' })).toEqual(['timezone']);
    expect(failPath(SystemSchema, { ntp: { servers: [{ address: 'bad host!' }] } })).toEqual([
      'ntp',
      'servers',
      '0',
      'address',
    ]);
    expect(failPath(SystemSchema, { banner: { login: 'x'.repeat(4097) } })).toEqual([
      'banner',
      'login',
    ]);
  });
});

describe('dataplane', () => {
  it('accepts the empty section and enforces bounds', () => {
    expect(DataplaneSchema.parse({})).toEqual({ pciWhitelist: [] });
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
    expect(failPath(StaticRouteSchema, { prefix: '10.0.0.0/8', nextHops: [] })).toEqual([
      'nextHops',
    ]);
  });

  it('prefix lists: unique sequences, family-consistent prefixes, ge ≤ le', () => {
    expect(PrefixListSchema.parse({})).toEqual({ family: 'ipv4', rules: [] });
    expect(
      PrefixListRuleSchema.safeParse({
        seq: 1,
        action: 'permit',
        prefix: '10.0.0.0/8',
        ge: 16,
        le: 24,
      }).success,
    ).toBe(true);
    expect(
      failPath(PrefixListRuleSchema, {
        seq: 1,
        action: 'permit',
        prefix: '10.0.0.0/8',
        ge: 24,
        le: 16,
      }),
    ).toEqual(['le']);
    expect(
      failPath(PrefixListRuleSchema, { seq: 0, action: 'permit', prefix: '10.0.0.0/8' }),
    ).toEqual(['seq']);
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

  it('route maps: unique sequences, empty match/set allowed', () => {
    expect(RouteMapSchema.parse({ entries: [{ seq: 10, action: 'permit' }] })).toEqual({
      entries: [{ seq: 10, action: 'permit', match: {}, set: { communityAdditive: false } }],
    });
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

  it('bgp: hold time above keepalive, remoteAs or peer group', () => {
    expect(
      BgpNeighborSchema.safeParse({
        address: '10.0.0.1',
        remoteAs: 65001,
        keepaliveSec: 30,
        holdTimeSec: 90,
      }).success,
    ).toBe(true);
    expect(
      failPath(BgpNeighborSchema, {
        address: '10.0.0.1',
        remoteAs: 65001,
        keepaliveSec: 90,
        holdTimeSec: 90,
      }),
    ).toEqual(['holdTimeSec']);
    expect(failPath(BgpNeighborSchema, { address: '10.0.0.1' })).toEqual(['remoteAs']);
    expect(BgpNeighborSchema.safeParse({ address: '10.0.0.1', peerGroup: 'g' }).success).toBe(true);
    expect(BgpPeerGroupSchema.safeParse({ keepaliveSec: 10, holdTimeSec: 30 }).success).toBe(true);
    expect(failPath(BgpPeerGroupSchema, { keepaliveSec: 30, holdTimeSec: 10 })).toEqual([
      'holdTimeSec',
    ]);
    expect(failPath(BgpNeighborSchema, { address: '10.0.0.1', remoteAs: 0 })).toEqual(['remoteAs']);
  });

  it('ospf: area ids normalise and must be unique; isis NET format', () => {
    expect(ospfAreaNumber(51)).toBe(51);
    expect(ospfAreaNumber('0.0.0.51')).toBe(51);
    expect(ospfAreaNumber('1.0.0.0')).toBe(16777216);
    expect(OspfSchema.safeParse({ areas: [{ id: 0 }, { id: '0.0.0.1' }] }).success).toBe(true);
    expect(failPath(OspfSchema, { areas: [{ id: 0 }, { id: '0.0.0.0' }] })).toEqual(['areas']);
    expect(failPath(OspfSchema, { areas: [{ id: 'x' }] })).toEqual(['areas', '0', 'id']);
    for (const ok of ['49.0001.1921.6800.1001.00', '49.0001.0000.0000.0001.00'])
      expect(isisNet.safeParse(ok).success).toBe(true);
    for (const bad of ['49.0001.1921.6800.1001', '49.1.2.3.00', '', 'gg.0001.1921.6800.1001.00'])
      expect(isisNet.safeParse(bad).success).toBe(false);
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
        radius: { servers: [{ address: '10.0.0.1', secretRef: 'aaa/radius/1' }] },
        tacacs: { servers: [{ address: '10.0.0.2', secretRef: 'aaa/tacacs/1', port: 4949 }] },
      }).success,
    ).toBe(true);
  });

  it('tls: certificate and key come together', () => {
    expect(TlsSchema.parse({})).toEqual({ minVersion: '1.2' });
    expect(
      TlsSchema.safeParse({
        certificateRef: 'tls/api/cert',
        privateKeyRef: 'tls/api/key',
        minVersion: '1.3',
      }).success,
    ).toBe(true);
    expect(failPath(TlsSchema, { certificateRef: 'tls/api/cert' })).toEqual(['privateKeyRef']);
    expect(failPath(TlsSchema, { privateKeyRef: 'tls/api/key' })).toEqual(['privateKeyRef']);
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
