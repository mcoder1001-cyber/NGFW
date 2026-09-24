import { describe, expect, it } from 'vitest';
import type { z } from 'zod';
import { HaClusterSchema, HaSchema, VrrpInstanceSchema } from './ha.js';

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

const vr = (extra: Record<string, unknown> = {}): Record<string, unknown> => ({
  interface: 'TenGigabitEthernet0/0/1',
  vrId: 10,
  addresses: ['192.168.10.254'],
  ...extra,
});

describe('VrrpInstanceSchema', () => {
  it('fills defaults', () => {
    expect(VrrpInstanceSchema.parse(vr())).toEqual({
      enabled: true,
      interface: 'TenGigabitEthernet0/0/1',
      vrId: 10,
      addressFamily: 'ipv4',
      priority: 100,
      advertisementIntervalMs: 1000,
      preempt: true,
      acceptMode: false,
      addresses: ['192.168.10.254'],
      vrf: 'default',
      engine: 'vpp',
      track: [],
    });
  });
  it('accepts the edges of every range', () => {
    expect(ok(VrrpInstanceSchema, vr({ vrId: 1, priority: 1, advertisementIntervalMs: 10 }))).toBe(
      true,
    );
    expect(
      ok(VrrpInstanceSchema, vr({ vrId: 255, priority: 255, advertisementIntervalMs: 40950 })),
    ).toBe(true);
    expect(
      ok(
        VrrpInstanceSchema,
        vr({ addressFamily: 'ipv6', addresses: ['2001:db8:10::fe', 'fe80::fe'] }),
      ),
    ).toBe(true);
    expect(
      ok(VrrpInstanceSchema, vr({ unicast: { peers: ['192.168.10.3'] }, engine: 'keepalived' })),
    ).toBe(true);
  });
  it.each([
    ['vrId 0', vr({ vrId: 0 }), 'vrId'],
    ['vrId 256', vr({ vrId: 256 }), 'vrId'],
    ['priority 0', vr({ priority: 0 }), 'priority'],
    ['priority 256', vr({ priority: 256 }), 'priority'],
    [
      'interval not a multiple of 10 ms',
      vr({ advertisementIntervalMs: 15 }),
      'advertisementIntervalMs',
    ],
    ['interval above 40950 ms', vr({ advertisementIntervalMs: 40960 }), 'advertisementIntervalMs'],
    ['IPv6 address on an IPv4 router', vr({ addresses: ['2001:db8::1'] }), 'addresses.0'],
    [
      'IPv4 address on an IPv6 router',
      vr({ addressFamily: 'ipv6', addresses: ['192.168.10.1'] }),
      'addresses.0',
    ],
    [
      'duplicate virtual address',
      vr({ addresses: ['192.168.10.254', '192.168.10.254'] }),
      'addresses.1',
    ],
    ['no virtual address', vr({ addresses: [] }), 'addresses'],
    ['address with prefix length', vr({ addresses: ['192.168.10.254/24'] }), 'addresses.0'],
    [
      'unicast peer of the wrong family',
      vr({ unicast: { peers: ['2001:db8::3'] } }),
      'unicast.peers.0',
    ],
    [
      'duplicate unicast peer',
      vr({ unicast: { peers: ['192.168.10.3', '192.168.10.3'] } }),
      'unicast.peers.1',
    ],
    ['empty unicast peers', vr({ unicast: { peers: [] } }), 'unicast.peers'],
    [
      'tracking its own interface',
      vr({ track: [{ interface: 'TenGigabitEthernet0/0/1' }] }),
      'track.0.interface',
    ],
    [
      'tracking an interface twice',
      vr({ track: [{ interface: 'loop0' }, { interface: 'loop0' }] }),
      'track.1.interface',
    ],
    [
      'priority decrement 254',
      vr({ track: [{ interface: 'loop0', priorityDecrement: 254 }] }),
      'track.0.priorityDecrement',
    ],
    ['bad interface name', vr({ interface: 'Ten Gig' }), 'interface'],
    ['authentication (not in VRRPv3)', vr({ authPassword: 'x' }), 'authPassword'],
  ])('rejects %s', (_label, value, path) =>
    expect(errorPaths(VrrpInstanceSchema, value)).toContain(path),
  );
});

const cluster = (extra: Record<string, unknown> = {}): Record<string, unknown> => ({
  nodeName: 'vrx-a',
  peers: [{ name: 'vrx-b', address: '192.168.10.3' }],
  secretRef: 'key/cluster',
  ...extra,
});

describe('HaClusterSchema', () => {
  it('fills defaults', () => {
    expect(HaClusterSchema.parse(cluster())).toMatchObject({
      enabled: false,
      port: 4370,
      vrf: 'default',
      configSync: true,
      stateSync: { nat: false, ipsec: false, acl: false },
    });
  });
  it.each([
    [
      'peer named like this node',
      cluster({ peers: [{ name: 'vrx-a', address: '192.168.10.3' }] }),
      'peers.0.name',
    ],
    [
      'duplicate peer name',
      cluster({
        peers: [
          { name: 'b', address: '10.0.0.1' },
          { name: 'b', address: '10.0.0.2' },
        ],
      }),
      'peers.1.name',
    ],
    [
      'duplicate peer address',
      cluster({
        peers: [
          { name: 'b', address: '10.0.0.1' },
          { name: 'c', address: '10.0.0.1' },
        ],
      }),
      'peers.1.address',
    ],
    ['no peers', cluster({ peers: [] }), 'peers'],
    ['inline secret', cluster({ secret: 'hunter2' }), 'secret'],
    ['secret pasted into secretRef', cluster({ secretRef: 'hunter2hunter2' }), 'secretRef'],
    ['secretRef of the wrong kind', cluster({ secretRef: 'psk/cluster' }), 'secretRef'],
    [
      'missing secretRef',
      { nodeName: 'vrx-a', peers: [{ name: 'vrx-b', address: '10.0.0.1' }] },
      'secretRef',
    ],
    ['bad port', cluster({ port: 70000 }), 'port'],
  ])('rejects %s', (_label, value, path) =>
    expect(errorPaths(HaClusterSchema, value)).toContain(path),
  );
});

describe('HaSchema root', () => {
  it('parses {} to a populated root; vrrp is a record keyed by name (D-053)', () => {
    expect(HaSchema.parse({})).toEqual({ vrrp: {} });
    const parsed = HaSchema.parse({
      vrrp: { 'lan-v4': { interface: 'loop0', vrId: 1, addresses: ['192.168.10.1'] } },
    });
    expect(Object.keys(parsed.vrrp)).toEqual(['lan-v4']);
    expect(parsed.cluster).toBeUndefined();
  });
  it('rejects unknown keys, array-shaped vrrp and bad record keys', () => {
    expect(ok(HaSchema, { carp: {} })).toBe(false);
    expect(ok(HaSchema, { vrrp: [] })).toBe(false);
    expect(
      ok(HaSchema, {
        vrrp: { 'bad name': { interface: 'loop0', vrId: 1, addresses: ['192.168.10.1'] } },
      }),
    ).toBe(false);
  });
});
