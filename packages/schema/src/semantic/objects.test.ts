import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import {
  addressObjectNames,
  addressToBigInt,
  interfaceExists,
  interfaceVrf,
  ipFamily,
  ipv4ToNumber,
  ipv6ToBigInt,
  lookupInterface,
  objectsValidators,
  own,
  portBounds,
  prefixLength,
  prefixToRange,
  rangesOverlap,
  serviceObjectNames,
  vrfExists,
} from './objects.js';
import { sortIssues, type SemanticIssue } from './registry.js';

function run(name: string, doc: unknown): SemanticIssue[] {
  const v = objectsValidators.find((x) => x.name === name);
  if (v === undefined) throw new Error(`no validator '${name}'`);
  return sortIssues(v.validate(RootConfig.parse(doc)));
}

const host = (address: string, extra: Record<string, unknown> = {}) => ({
  type: 'host',
  address,
  ...extra,
});
const tcp = (ports: string[], extra: Record<string, unknown> = {}) => ({
  protocol: 'tcp',
  destinationPorts: ports,
  ...extra,
});

describe('IP helpers', () => {
  it('ipFamily / ipv4ToNumber / ipv6ToBigInt', () => {
    expect(ipFamily('10.0.0.1')).toBe(4);
    expect(ipFamily('::1')).toBe(6);
    expect(ipv4ToNumber('10.0.0.1')).toBe(167772161);
    expect(ipv4ToNumber('255.255.255.255')).toBe(4294967295);
    expect(ipv6ToBigInt('::')).toBe(0n);
    expect(ipv6ToBigInt('::1')).toBe(1n);
    expect(ipv6ToBigInt('1::')).toBe(1n << 112n);
    expect(ipv6ToBigInt('2001:db8::1')).toBe(
      ipv6ToBigInt('2001:0db8:0000:0000:0000:0000:0000:0001'),
    );
    expect(ipv6ToBigInt('1:2:3:4:5:6:7:8')).toBe(0x0001_0002_0003_0004_0005_0006_0007_0008n);
    expect(ipv6ToBigInt('::ffff:10.0.0.1')).toBe(0xffff_0a00_0001n);
    expect(ipv6ToBigInt('64:ff9b::192.0.2.33')).toBe(
      (0x64ff9bn << 96n) | BigInt(ipv4ToNumber('192.0.2.33')),
    );
    expect(addressToBigInt('10.0.0.1')).toBe(167772161n);
    expect(addressToBigInt('::1')).toBe(1n);
  });

  it('prefixToRange ignores host bits and rangesOverlap is family-aware', () => {
    expect(prefixToRange('10.0.0.77/24')).toEqual({
      family: 4,
      start: BigInt(ipv4ToNumber('10.0.0.0')),
      end: BigInt(ipv4ToNumber('10.0.0.255')),
    });
    expect(prefixToRange('0.0.0.0/0')).toEqual({ family: 4, start: 0n, end: (1n << 32n) - 1n });
    expect(prefixToRange('10.0.0.7/32')).toEqual({
      family: 4,
      start: 167772167n,
      end: 167772167n,
    });
    expect(prefixToRange('2001:db8::/32')).toEqual({
      family: 6,
      start: 0x20010db8n << 96n,
      end: (0x20010db9n << 96n) - 1n,
    });
    expect(prefixLength('10.0.0.0/24')).toBe(24);
    expect(prefixLength('::/0')).toBe(0);
    expect(rangesOverlap(prefixToRange('10.0.0.0/24'), prefixToRange('10.0.0.128/25'))).toBe(true);
    expect(rangesOverlap(prefixToRange('10.0.0.0/24'), prefixToRange('10.0.1.0/24'))).toBe(false);
    expect(rangesOverlap(prefixToRange('0.0.0.0/0'), prefixToRange('::/0'))).toBe(false);
  });

  it('portBounds parses a valid range and throws on garbage', () => {
    expect(portBounds('80')).toEqual({ from: 80, to: 80 });
    expect(portBounds('80-90')).toEqual({ from: 80, to: 90 });
    expect(() => portBounds('http')).toThrow(/invalid port range/);
  });

  it('own() ignores prototype members', () => {
    const record: Record<string, number> = { constructor: 1 };
    expect(own(record, 'constructor')).toBe(1);
    expect(own({}, 'constructor')).toBeUndefined();
    expect(own({}, 'toString')).toBeUndefined();
  });
});

describe('cross-domain lookups', () => {
  const config = RootConfig.parse({
    interfaces: {
      'Gig0/0/0': { vrf: 'default' },
      'Gig0/0/1': { subinterfaces: { '100': { vrf: 'cust' } } },
      'Gig0/0/2.200': {},
      weird: 5,
    },
    vrfs: { cust: {} },
  });

  it('interfaceExists handles top-level, nested and own-key sub-interfaces', () => {
    expect(interfaceExists(config, 'Gig0/0/0')).toBe(true);
    expect(interfaceExists(config, 'Gig0/0/1.100')).toBe(true);
    expect(interfaceExists(config, 'Gig0/0/2.200')).toBe(true);
    expect(interfaceExists(config, 'weird')).toBe(true);
    expect(interfaceExists(config, 'Gig0/0/1.200')).toBe(false);
    expect(interfaceExists(config, 'Gig0/0/0.5')).toBe(false);
    expect(interfaceExists(config, 'Gig0/0/9.1')).toBe(false);
    expect(interfaceExists(config, 'weird.1')).toBe(false);
    expect(interfaceExists(config, 'nope')).toBe(false);
    expect(interfaceExists(config, 'constructor')).toBe(false);
    expect(interfaceExists(config, 'constructor.1')).toBe(false);
    expect(lookupInterface(config, 'Gig0/0/1.100')).toEqual({ vrf: 'cust' });
  });

  it('interfaceVrf reads vrf only from object entries', () => {
    expect(interfaceVrf(config, 'Gig0/0/0')).toBe('default');
    expect(interfaceVrf(config, 'Gig0/0/1.100')).toBe('cust');
    expect(interfaceVrf(config, 'Gig0/0/2.200')).toBeUndefined();
    expect(interfaceVrf(config, 'weird')).toBeUndefined();
    expect(interfaceVrf(config, 'nope')).toBeUndefined();
  });

  it('vrfExists implies default and ignores the prototype', () => {
    expect(vrfExists(config, 'default')).toBe(true);
    expect(vrfExists(RootConfig.parse({}), 'default')).toBe(true);
    expect(vrfExists(config, 'cust')).toBe(true);
    expect(vrfExists(config, 'nope')).toBe(false);
    expect(vrfExists(config, 'constructor')).toBe(false);
  });

  it('addressObjectNames / serviceObjectNames union objects and groups', () => {
    const { objects } = RootConfig.parse({
      objects: {
        addresses: { a: host('10.0.0.1') },
        addressGroups: { g: { members: ['a'] } },
        services: { s: tcp(['80']) },
        serviceGroups: { sg: { members: ['s'] } },
      },
    });
    expect([...addressObjectNames(objects)].sort()).toEqual(['a', 'g']);
    expect([...serviceObjectNames(objects)].sort()).toEqual(['s', 'sg']);
  });
});

describe('objects.names-disjoint', () => {
  it('rejects a group named like an object of the same family', () => {
    expect(
      run('objects.names-disjoint', {
        objects: {
          addresses: { web: host('10.0.0.1') },
          addressGroups: { web: { members: ['web'] } },
          services: { http: tcp(['80']) },
          serviceGroups: { http: { members: ['http'] } },
        },
      }).map((i) => i.pointer),
    ).toEqual(['/objects/addressGroups/web', '/objects/serviceGroups/http']);
  });

  it('is prototype-safe and accepts disjoint names', () => {
    expect(
      run('objects.names-disjoint', {
        objects: {
          addresses: { a: host('10.0.0.1') },
          addressGroups: { constructor: { members: ['a'] }, toString: { members: ['a'] } },
          services: { s: tcp(['1']) },
          serviceGroups: { hasOwnProperty: { members: ['s'] } },
        },
      }),
    ).toEqual([]);
  });
});

describe('objects.tags-exist', () => {
  it('reports an unknown tag on every object kind', () => {
    const issues = run('objects.tags-exist', {
      objects: {
        tags: { prod: {} },
        addresses: { a: host('10.0.0.1', { tags: ['prod', 'x'] }) },
        addressGroups: { g: { members: ['a'], tags: ['x'] } },
        services: { s: tcp(['80'], { tags: ['x'] }) },
        serviceGroups: { sg: { members: ['s'], tags: ['x'] } },
        schedules: {
          sch: { type: 'recurring', days: ['mon'], start: '08:00', end: '17:00', tags: ['x'] },
        },
        zones: { z: { interfaces: [], tags: ['constructor'] } },
      },
    });
    expect(issues.map((i) => i.pointer)).toEqual([
      '/objects/addresses/a/tags/1',
      '/objects/addressGroups/g/tags/0',
      '/objects/schedules/sch/tags/0',
      '/objects/serviceGroups/sg/tags/0',
      '/objects/services/s/tags/0',
      '/objects/zones/z/tags/0',
    ]);
    expect(issues[0]?.message).toMatch(/tag 'x' does not exist/);
  });
});

describe('objects.address-range-valid', () => {
  it('rejects reversed and mixed-family ranges, accepts ordered ones', () => {
    const issues = run('objects.address-range-valid', {
      objects: {
        addresses: {
          reversed: { type: 'range', start: '10.0.0.10', end: '10.0.0.1' },
          mixed: { type: 'range', start: '10.0.0.1', end: '::1' },
          ok: { type: 'range', start: '10.0.0.1', end: '10.0.0.1' },
          ok6: { type: 'range', start: '2001:db8::1', end: '2001:db8::ffff' },
          h: host('10.0.0.1'),
        },
      },
    });
    expect(issues).toEqual([
      {
        pointer: '/objects/addresses/mixed/end',
        message: expect.stringMatching(/same address family/),
      },
      { pointer: '/objects/addresses/reversed/end', message: expect.stringMatching(/lower than/) },
    ]);
  });
});

describe('objects.address-group-members / objects.service-group-members', () => {
  it('rejects unknown and duplicate members', () => {
    const issues = run('objects.address-group-members', {
      objects: {
        addresses: { a: host('10.0.0.1') },
        addressGroups: { g: { members: ['a', 'nope', 'a', 'constructor'] } },
      },
    });
    expect(issues).toEqual([
      {
        pointer: '/objects/addressGroups/g/members/1',
        message: expect.stringMatching(/'nope' is not an entry/),
      },
      {
        pointer: '/objects/addressGroups/g/members/2',
        message: expect.stringMatching(/listed twice/),
      },
      {
        pointer: '/objects/addressGroups/g/members/3',
        message: expect.stringMatching(/'constructor' is not/),
      },
    ]);
  });

  it('detects self-reference and longer cycles once, at the closing member', () => {
    expect(
      run('objects.address-group-members', {
        objects: {
          addresses: { x: host('10.0.0.1') },
          addressGroups: {
            self: { members: ['self'] },
            a: { members: ['b', 'x'] },
            b: { members: ['c'] },
            c: { members: ['a'] },
          },
        },
      }),
    ).toEqual([
      {
        pointer: '/objects/addressGroups/c/members/0',
        message: 'group membership cycle: a → b → c → a',
      },
      {
        pointer: '/objects/addressGroups/self/members/0',
        message: 'group membership cycle: self → self',
      },
    ]);
  });

  it('accepts nested groups and diamonds', () => {
    expect(
      run('objects.address-group-members', {
        objects: {
          addresses: { x: host('10.0.0.1') },
          addressGroups: {
            a: { members: ['b', 'c'] },
            b: { members: ['c'] },
            c: { members: ['x'] },
          },
        },
      }),
    ).toEqual([]);
  });

  it('applies the same rules to service groups', () => {
    expect(
      run('objects.service-group-members', {
        objects: {
          services: { s: tcp(['80']) },
          serviceGroups: { g: { members: ['s', 'nope', 'g'] } },
        },
      }).map((i) => i.pointer),
    ).toEqual(['/objects/serviceGroups/g/members/1', '/objects/serviceGroups/g/members/2']);
  });
});

describe('objects.service-valid', () => {
  it('rejects ICMP code without type, flag bits outside the mask and overlapping ports', () => {
    const issues = run('objects.service-valid', {
      objects: {
        services: {
          icmpBad: { protocol: 'icmp', code: 0 },
          icmp6Bad: { protocol: 'icmp6', code: 1 },
          icmpOk: { protocol: 'icmp', type: 8, code: 0 },
          icmpAny: { protocol: 'icmp6' },
          flagsBad: tcp(['80'], { tcpFlags: { mask: 0x02, value: 0x12 } }),
          flagsOk: tcp(['80'], { tcpFlags: { mask: 0x12, value: 0x02 } }),
          overlap: tcp(['80', '80-90', '443']),
          udpOverlap: { protocol: 'udp', sourcePorts: ['1-100', '50'] },
          tcpUdp: { protocol: 'tcp-udp', destinationPorts: ['53', '53'] },
          sctp: { protocol: 'sctp', destinationPorts: ['3868'] },
          any: { protocol: 'any' },
          gre: { protocol: 'other', number: 47 },
        },
      },
    });
    expect(issues.map((i) => i.pointer)).toEqual([
      '/objects/services/flagsBad/tcpFlags/value',
      '/objects/services/icmp6Bad/code',
      '/objects/services/icmpBad/code',
      '/objects/services/overlap/destinationPorts/1',
      '/objects/services/tcpUdp/destinationPorts/1',
      '/objects/services/udpOverlap/sourcePorts/1',
    ]);
    expect(issues[3]?.message).toBe("port range '80-90' overlaps entry 0");
  });
});

describe('objects.schedule-valid', () => {
  it('rejects empty/overnight recurring windows, duplicate days and reversed one-time windows', () => {
    const issues = run('objects.schedule-valid', {
      objects: {
        schedules: {
          empty: { type: 'recurring', days: ['mon'], start: '09:00', end: '09:00' },
          overnight: {
            type: 'recurring',
            days: ['mon', 'tue', 'mon'],
            start: '22:00',
            end: '06:00',
          },
          okDay: { type: 'recurring', days: ['sat', 'sun'], start: '00:00', end: '23:59' },
          reversed: { type: 'once', start: '2026-10-01T00:00:00Z', end: '2026-09-30T23:00:00Z' },
          zero: { type: 'once', start: '2026-10-01T03:30:00+03:30', end: '2026-10-01T00:00:00Z' },
          okOnce: { type: 'once', start: '2026-10-01T00:00:00Z', end: '2026-10-02T00:00:00Z' },
        },
      },
    });
    expect(issues.map((i) => i.pointer)).toEqual([
      '/objects/schedules/empty/end',
      '/objects/schedules/overnight/days/2',
      '/objects/schedules/overnight/end',
      '/objects/schedules/reversed/end',
      '/objects/schedules/zero/end',
    ]);
    expect(issues[2]?.message).toMatch(/overnight window/);
  });
});

describe('objects.zone-interfaces', () => {
  const interfaces = {
    'Gig0/0/0': {},
    'Gig0/0/1': { subinterfaces: { '100': {} } },
  };

  it('rejects unknown, duplicate and already-zoned interfaces', () => {
    const issues = run('objects.zone-interfaces', {
      interfaces,
      objects: {
        zones: {
          lan: { interfaces: ['Gig0/0/0', 'Gig0/0/1.100', 'Gig0/0/0', 'nope'] },
          wan: { interfaces: ['Gig0/0/0'] },
        },
      },
    });
    expect(issues).toEqual([
      {
        pointer: '/objects/zones/lan/interfaces/2',
        message: "interface 'Gig0/0/0' is listed twice",
      },
      { pointer: '/objects/zones/lan/interfaces/3', message: "interface 'nope' does not exist" },
      {
        pointer: '/objects/zones/wan/interfaces/0',
        message: "interface 'Gig0/0/0' already belongs to zone 'lan'",
      },
    ]);
  });

  it('accepts disjoint zones', () => {
    expect(
      run('objects.zone-interfaces', {
        interfaces,
        objects: {
          zones: { lan: { interfaces: ['Gig0/0/0'] }, wan: { interfaces: ['Gig0/0/1.100'] } },
        },
      }),
    ).toEqual([]);
  });
});
