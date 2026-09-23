import { describe, expect, it } from 'vitest';
import type { z } from 'zod';
import { GreTunnelSchema, IpipTunnelSchema, TunnelsSchema, VxlanTunnelSchema } from './tunnels.js';

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
    expect(TunnelsSchema.parse({})).toEqual({ gre: {}, vxlan: {}, ipip: {} });
    expect(TunnelsSchema.parse({ gre: {} })).toEqual({ gre: {}, vxlan: {}, ipip: {} });
  });
  it('rejects unknown kinds, bad names and non-object entries', () => {
    expect(ok(TunnelsSchema, { gtpu: {} })).toBe(false);
    expect(ok(TunnelsSchema, { gre: { 'bad name': { src: '10.0.0.1', dst: '10.0.0.2' } } })).toBe(
      false,
    );
    expect(ok(TunnelsSchema, { gre: { a: null } })).toBe(false);
    expect(ok(TunnelsSchema, { gre: [] })).toBe(false);
  });
});
