import { describe, expect, it } from 'vitest';
import type { z } from 'zod';
import {
  GreTunnelSchema,
  GtpuTunnelSchema,
  IpipTunnelSchema,
  L2tpv3TunnelSchema,
  PppoeSessionSchema,
  TUNNEL_KINDS,
  TunnelsSchema,
  VxlanGpeTunnelSchema,
  VxlanTunnelSchema,
} from './tunnels.js';

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

const gre = (extra: Record<string, unknown> = {}): Record<string, unknown> => ({
  src: '198.51.100.2',
  dst: '203.0.113.30',
  ...extra,
});

describe('GreTunnelSchema', () => {
  it('fills defaults', () => {
    expect(GreTunnelSchema.parse(gre())).toEqual({
      enabled: true,
      src: '198.51.100.2',
      dst: '203.0.113.30',
      type: 'l3',
      underlayVrf: 'default',
      vrf: 'default',
      ipv4: [],
      ipv6: [],
    });
  });
  it('accepts IPv6 endpoints, ERSPAN and TEB in a bridge domain', () => {
    expect(
      ok(GreTunnelSchema, gre({ src: '2001:db8::1', dst: '2001:db8::2', ipv4: ['10.0.0.1/30'] })),
    ).toBe(true);
    expect(ok(GreTunnelSchema, gre({ type: 'erspan', sessionId: 1023 }))).toBe(true);
    expect(ok(GreTunnelSchema, gre({ type: 'teb', bridgeDomain: 10 }))).toBe(true);
  });
  it.each([
    ['dst equals src', gre({ dst: '198.51.100.2' }), 'dst'],
    ['mixed families', gre({ dst: '2001:db8::1' }), 'dst'],
    ['multicast destination', gre({ dst: '239.1.1.1' }), 'dst'],
    ['unspecified source', gre({ src: '0.0.0.0' }), 'src'],
    ['multicast source', gre({ src: 'ff02::1', dst: '2001:db8::1' }), 'src'],
    ['unspecified destination', gre({ dst: '::', src: '2001:db8::1' }), 'dst'],
    ['ERSPAN without session id', gre({ type: 'erspan' }), 'sessionId'],
    ['session id on L3', gre({ sessionId: 1 }), 'sessionId'],
    ['session id 1024', gre({ type: 'erspan', sessionId: 1024 }), 'sessionId'],
    ['addresses on a TEB tunnel', gre({ type: 'teb', ipv4: ['10.0.0.1/30'] }), 'ipv4'],
    ['bridge domain on an L3 tunnel', gre({ bridgeDomain: 1 }), 'bridgeDomain'],
    ['duplicate address', gre({ ipv4: ['10.0.0.1/30', '10.0.0.1/30'] }), 'ipv4.1'],
    ['CIDR without length', gre({ ipv4: ['10.0.0.1'] }), 'ipv4.0'],
    ['mtu 67', gre({ mtu: 67 }), 'mtu'],
    ['instance 2^32', gre({ instance: 4294967296 }), 'instance'],
    ['unknown key', gre({ key: 1 }), 'key'],
    ['hostname as endpoint', gre({ dst: 'peer.example' }), 'dst'],
  ])('rejects %s', (_label, value, path) =>
    expect(errorPaths(GreTunnelSchema, value)).toContain(path),
  );
});

describe('IpipTunnelSchema', () => {
  it('p2p needs dst, p2mp forbids it', () => {
    expect(ok(IpipTunnelSchema, { src: '198.51.100.2', dst: '203.0.113.10' })).toBe(true);
    expect(errorPaths(IpipTunnelSchema, { src: '198.51.100.2' })).toEqual(['dst']);
    expect(ok(IpipTunnelSchema, { src: '198.51.100.2', mode: 'p2mp' })).toBe(true);
    expect(
      errorPaths(IpipTunnelSchema, { src: '198.51.100.2', mode: 'p2mp', dst: '203.0.113.10' }),
    ).toEqual(['dst']);
  });
  it('validates dscp, endpoints and addresses', () => {
    expect(
      errorPaths(IpipTunnelSchema, { src: '198.51.100.2', dst: '203.0.113.10', dscp: 64 }),
    ).toEqual(['dscp']);
    expect(errorPaths(IpipTunnelSchema, { src: '198.51.100.2', dst: '198.51.100.2' })).toEqual([
      'dst',
    ]);
    expect(
      errorPaths(IpipTunnelSchema, { src: '198.51.100.2', dst: '203.0.113.10', bridgeDomain: 1 }),
    ).toEqual(['bridgeDomain']);
    expect(
      ok(IpipTunnelSchema, {
        src: '198.51.100.2',
        dst: '203.0.113.10',
        ipv6: ['fd00::1/64'],
        dscp: 46,
      }),
    ).toBe(true);
  });
});

const vxlan = (extra: Record<string, unknown> = {}): Record<string, unknown> => ({
  src: '192.168.10.1',
  dst: '192.168.10.50',
  vni: 100,
  ...extra,
});

describe('VxlanTunnelSchema', () => {
  it('fills defaults and accepts the VNI range 0..2^24-1', () => {
    expect(VxlanTunnelSchema.parse(vxlan())).toMatchObject({
      srcPort: 4789,
      dstPort: 4789,
      decap: 'l2',
    });
    expect(ok(VxlanTunnelSchema, vxlan({ vni: 0 }))).toBe(true);
    expect(ok(VxlanTunnelSchema, vxlan({ vni: 16777215 }))).toBe(true);
    expect(
      ok(VxlanTunnelSchema, vxlan({ dst: '239.1.1.1', mcastInterface: 'TenGigabitEthernet0/0/1' })),
    ).toBe(true);
    expect(ok(VxlanTunnelSchema, vxlan({ decap: 'ip4', ipv4: ['10.0.0.1/24'] }))).toBe(true);
  });
  it.each([
    ['vni 2^24', vxlan({ vni: 16777216 }), 'vni'],
    ['negative vni', vxlan({ vni: -1 }), 'vni'],
    ['fractional vni', vxlan({ vni: 1.5 }), 'vni'],
    ['multicast dst without mcastInterface', vxlan({ dst: '239.1.1.1' }), 'mcastInterface'],
    [
      'mcastInterface with unicast dst',
      vxlan({ mcastInterface: 'TenGigabitEthernet0/0/1' }),
      'mcastInterface',
    ],
    ['addresses with l2 decap', vxlan({ ipv4: ['10.0.0.1/24'] }), 'ipv4'],
    ['bridge domain with ip4 decap', vxlan({ decap: 'ip4', bridgeDomain: 1 }), 'bridgeDomain'],
    ['dst equals src', vxlan({ dst: '192.168.10.1' }), 'dst'],
    ['port 65536', vxlan({ dstPort: 65536 }), 'dstPort'],
    ['bad interface name', vxlan({ dst: '239.1.1.1', mcastInterface: 'eth 0' }), 'mcastInterface'],
  ])('rejects %s', (_label, value, path) =>
    expect(errorPaths(VxlanTunnelSchema, value)).toContain(path),
  );
});

describe('TunnelsSchema root', () => {
  it('parses {} to empty records (D-017/D-053) and keeps given records', () => {
    const empty = { gre: {}, vxlan: {}, ipip: {}, vxlanGpe: {}, gtpu: {}, l2tpv3: {}, pppoe: {} };
    expect(TunnelsSchema.parse({})).toEqual(empty);
    expect(TunnelsSchema.parse({ gre: {} })).toEqual(empty);
  });
  it('rejects unknown kinds, bad names and non-object entries', () => {
    expect(ok(TunnelsSchema, { sctp: {} })).toBe(false);
    expect(ok(TunnelsSchema, { gre: { 'bad name': { src: '10.0.0.1', dst: '10.0.0.2' } } })).toBe(
      false,
    );
    expect(ok(TunnelsSchema, { gre: { a: null } })).toBe(false);
    expect(ok(TunnelsSchema, { gre: [] })).toBe(false);
  });
});

// ---- S-tunnels-contract (T1) ------------------------------------------------------------------------------------

describe('IpipTunnelSchema.sixrd', () => {
  const rd = {
    src: '198.51.100.2',
    sixrd: { ip6Prefix: '2001:db8:6::/48', ip4Prefix: '198.51.0.0/16' },
  };
  it('accepts a 6RD tunnel without dst and fills the defaults', () => {
    const t = IpipTunnelSchema.parse(rd);
    expect(t.sixrd).toEqual({
      ip6Prefix: '2001:db8:6::/48',
      ip4Prefix: '198.51.0.0/16',
      securityCheck: false,
    });
    expect(t.mode).toBe('p2p');
  });
  it('rejects dst, dscp, p2mp and an IPv6 source with sixrd', () => {
    expect(ok(IpipTunnelSchema, { ...rd, dst: '203.0.113.1' })).toBe(false);
    expect(ok(IpipTunnelSchema, { ...rd, dscp: 1 })).toBe(false);
    expect(ok(IpipTunnelSchema, { ...rd, mode: 'p2mp' })).toBe(false);
    expect(ok(IpipTunnelSchema, { ...rd, src: '2001:db8::1' })).toBe(false);
  });
});

describe('VxlanGpeTunnelSchema / GtpuTunnelSchema / L2tpv3TunnelSchema / PppoeSessionSchema', () => {
  const gpe = { src: '198.51.100.2', dst: '203.0.113.40', vni: 300 };
  it('VXLAN-GPE defaults to ethernet on 4790; addresses only with ip4/ip6; nsh is not bridged', () => {
    const t = VxlanGpeTunnelSchema.parse(gpe);
    expect([t.protocol, t.srcPort, t.dstPort]).toEqual(['ethernet', 4790, 4790]);
    expect(ok(VxlanGpeTunnelSchema, { ...gpe, ipv4: ['10.0.0.1/30'] })).toBe(false);
    expect(ok(VxlanGpeTunnelSchema, { ...gpe, protocol: 'ip4', ipv4: ['10.0.0.1/30'] })).toBe(true);
    expect(ok(VxlanGpeTunnelSchema, { ...gpe, protocol: 'nsh', bridgeDomain: 1 })).toBe(false);
    expect(ok(VxlanGpeTunnelSchema, { ...gpe, dst: '239.1.1.1' })).toBe(false); // mcastInterface missing
    expect(ok(VxlanGpeTunnelSchema, { ...gpe, instance: 1 })).toBe(false); // no instance on this kind
  });
  const gtpu = { src: '198.51.100.2', dst: '203.0.113.50', teid: 7 };
  it('GTP-U defaults to decap ip4; qfi needs pduExtension; l2 takes a bridge domain', () => {
    expect(GtpuTunnelSchema.parse(gtpu).decap).toBe('ip4');
    expect(ok(GtpuTunnelSchema, { ...gtpu, qfi: 5 })).toBe(false);
    expect(ok(GtpuTunnelSchema, { ...gtpu, pduExtension: true, qfi: 5 })).toBe(true);
    expect(ok(GtpuTunnelSchema, { ...gtpu, decap: 'l2', bridgeDomain: 7 })).toBe(true);
    expect(ok(GtpuTunnelSchema, { ...gtpu, decap: 'drop', bridgeDomain: 7 })).toBe(false);
  });
  const l2 = {
    src: '2001:db8:0:1::2',
    dst: '2001:db8:ffff::9',
    localSessionId: 1,
    remoteSessionId: 2,
  };
  it('L2TPv3 is IPv6 only, L2 only, default VRF underlay', () => {
    expect(L2tpv3TunnelSchema.parse(l2).localCookie).toBe(0);
    expect(ok(L2tpv3TunnelSchema, { ...l2, src: '10.0.0.1', dst: '10.0.0.2' })).toBe(false);
    expect(ok(L2tpv3TunnelSchema, { ...l2, ipv6: ['2001:db8::1/64'] })).toBe(false);
    expect(ok(L2tpv3TunnelSchema, { ...l2, underlayVrf: 'red' })).toBe(false);
    expect(ok(L2tpv3TunnelSchema, { ...l2, bridgeDomain: 100 })).toBe(true);
  });
  it('PPPoE sessions: id 1–65535, a MAC and a unicast client address', () => {
    const p = { sessionId: 1, clientMac: '02:00:00:00:00:01', clientIp: '10.99.0.2' };
    expect(PppoeSessionSchema.parse(p).vrf).toBe('default');
    expect(ok(PppoeSessionSchema, { ...p, sessionId: 0 })).toBe(false);
    expect(ok(PppoeSessionSchema, { ...p, clientIp: '224.0.0.1' })).toBe(false);
    expect(ok(PppoeSessionSchema, { ...p, src: '10.0.0.1' })).toBe(false);
  });
  it('TUNNEL_KINDS carries the instance / advanced flags the rules and the UI read', () => {
    expect(TUNNEL_KINDS.map((k) => [k.key, k.instance, k.advanced])).toEqual([
      ['gre', true, false],
      ['vxlan', true, false],
      ['ipip', true, false],
      ['vxlanGpe', false, false],
      ['gtpu', false, true],
      ['l2tpv3', false, true],
      ['pppoe', false, true],
    ]);
  });
});
