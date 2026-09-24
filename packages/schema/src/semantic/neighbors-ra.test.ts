import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { validateSemantics } from './index.js';
import { isDefaultIpv6Ra, neighborsRaValidators } from './neighbors-ra.js';

const run = (name: string, doc: RootConfigInput) =>
  neighborsRaValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

const LAN = 'host-w9l0';
const lan = (extra: Record<string, unknown> = {}) => ({
  [LAN]: { ipv4: ['10.9.1.1/24'], ipv6: ['2001:db8:9:1::1/64'], ...extra },
});

describe('F-neighbors-ra schema', () => {
  it('accepts the full feature document and fills the defaults', () => {
    const doc = RootConfig.parse({
      interfaces: {
        ...lan({
          ipv6Ra: { suppress: false, prefixes: { '2001:db8:9:1::/64': {} } },
          proxyArp: true,
          proxyNd: ['2001:db8:9:1::99'],
          subinterfaces: {
            '100': { vlanId: 100, ipv6: ['2001:db8:9:100::1/64'], ipv6Ra: { suppress: false } },
          },
        }),
      },
      vrfs: { w9red: { id: 9001, proxyArpRanges: [{ low: '10.9.3.10', high: '10.9.3.20' }] } },
      routing: {
        neighbors: {
          static: [{ interface: LAN, ip: '10.9.1.50', mac: '02:00:00:00:09:50' }],
          dad: {},
        },
      },
    });
    expect(doc.interfaces[LAN]!.ipv6Ra).toEqual({
      suppress: false,
      managed: false,
      other: false,
      lifetimeSec: 600,
      maxIntervalSec: 200,
      minIntervalSec: 150,
      prefixes: {
        '2001:db8:9:1::/64': {
          validSec: 2592000,
          preferredSec: 604800,
          offLink: false,
          noAutoconfig: false,
        },
      },
    });
    expect(doc.routing.neighbors).toEqual({
      static: [{ interface: LAN, ip: '10.9.1.50', mac: '02:00:00:00:09:50', noFibEntry: false }],
      dad: { transmits: 1, delayMs: 1000 },
    });
    expect(validateSemantics(doc)).toEqual([]);
  });

  it('adds no key to documents that do not use the feature (absent = off)', () => {
    const doc = RootConfig.parse({ interfaces: lan(), vrfs: { a: { id: 9002 } } });
    expect(Object.keys(doc.interfaces[LAN]!)).not.toContain('ipv6Ra');
    expect(Object.keys(doc.interfaces[LAN]!)).not.toContain('proxyArp');
    expect(Object.keys(doc.interfaces[LAN]!)).not.toContain('proxyNd');
    expect(Object.keys(doc.vrfs['a']!)).not.toContain('proxyArpRanges');
    expect(Object.keys(doc.routing)).not.toContain('neighbors');
  });

  it('rejects RA interval and lifetime combinations VPP refuses, with pointers', () => {
    const bad = (ipv6Ra: Record<string, unknown>) =>
      RootConfig.safeParse({ interfaces: lan({ ipv6Ra }) }).error?.issues.map((i) =>
        i.path.join('/'),
      );
    expect(bad({ maxIntervalSec: 100, minIntervalSec: 90 })).toEqual([
      `interfaces/${LAN}/ipv6Ra/minIntervalSec`,
    ]);
    expect(bad({ maxIntervalSec: 1801, lifetimeSec: 0 })).toEqual([
      `interfaces/${LAN}/ipv6Ra/maxIntervalSec`,
    ]);
    expect(bad({ minIntervalSec: 2 })).toEqual([`interfaces/${LAN}/ipv6Ra/minIntervalSec`]);
    expect(bad({ lifetimeSec: 200 })).toEqual([`interfaces/${LAN}/ipv6Ra/lifetimeSec`]); // = max interval
    expect(bad({ lifetimeSec: 9001 })).toEqual([`interfaces/${LAN}/ipv6Ra/lifetimeSec`]);
    expect(bad({ lifetimeSec: 0 })).toBeUndefined(); // not a default router
    expect(bad({ maxIntervalSec: 1800, minIntervalSec: 1350, lifetimeSec: 9000 })).toBeUndefined();
  });

  it('rejects prefixes with host bits, preferred > valid, and non-unicast MACs', () => {
    const r = RootConfig.safeParse({
      interfaces: lan({
        ipv6Ra: {
          prefixes: {
            '2001:db8:9:1::1/64': {},
            '2001:db8:9:2::/64': { validSec: 10, preferredSec: 20 },
          },
        },
      }),
      routing: {
        neighbors: { static: [{ interface: LAN, ip: '10.9.1.5', mac: '01:00:5e:00:00:01' }] },
      },
    });
    expect(r.success).toBe(false);
    const paths = r.error!.issues.map((i) => i.path.join('/'));
    expect(paths).toContain(`interfaces/${LAN}/ipv6Ra/prefixes/2001:db8:9:1::1/64`);
    expect(paths).toContain(`interfaces/${LAN}/ipv6Ra/prefixes/2001:db8:9:2::/64/preferredSec`);
    expect(paths).toContain('routing/neighbors/static/0/mac');
  });

  it('proxy-ARP ranges are IPv4 only and ordered low ≤ high', () => {
    const parse = (r: Record<string, unknown>) =>
      RootConfig.safeParse({ vrfs: { a: { id: 9003, proxyArpRanges: [r] } } });
    expect(parse({ low: '10.9.0.1', high: '10.9.0.1' }).success).toBe(true);
    expect(parse({ low: '10.9.0.9', high: '10.9.0.1' }).error?.issues[0]?.path).toEqual([
      'vrfs',
      'a',
      'proxyArpRanges',
      0,
      'high',
    ]);
    expect(parse({ low: '2001:db8::1', high: '2001:db8::9' }).success).toBe(false);
  });

  it('bounds DAD to what VPP accepts (1–10 probes, 100–10000 ms)', () => {
    const dad = (d: Record<string, unknown>) =>
      RootConfig.safeParse({ routing: { neighbors: { dad: d } } }).success;
    expect(dad({ transmits: 10, delayMs: 100 })).toBe(true);
    expect(dad({ transmits: 11 })).toBe(false);
    expect(dad({ delayMs: 99 })).toBe(false);
    expect(dad({ delayMs: 10001 })).toBe(false);
  });
});

describe('interfaces.neighbors-ra-ipv6-required', () => {
  it('a default (off) RA object needs no IPv6; a real RA config or proxy ND does', () => {
    expect(
      run('interfaces.neighbors-ra-ipv6-required', { interfaces: { loop901: { ipv6Ra: {} } } }),
    ).toEqual([]);
    expect(
      run('interfaces.neighbors-ra-ipv6-required', {
        interfaces: {
          loop901: { ipv6Ra: { suppress: false }, proxyNd: ['2001:db8::5'] },
          loop902: { ipv6: ['2001:db8:2::1/64'], ipv6Ra: { suppress: false } },
        },
      }),
    ).toEqual([
      {
        pointer: '/interfaces/loop901/ipv6Ra',
        message: 'router advertisements need IPv6 on loop901: add an IPv6 address to the interface',
      },
      {
        pointer: '/interfaces/loop901/proxyNd',
        message: 'proxy ND needs IPv6 on loop901: add an IPv6 address to the interface',
      },
    ]);
  });
  it('covers sub-interfaces', () => {
    expect(
      run('interfaces.neighbors-ra-ipv6-required', {
        interfaces: {
          'GigabitEthernet0/8/0': {
            subinterfaces: { '7': { vlanId: 7, ipv6Ra: { managed: true } } },
          },
        },
      }),
    ).toEqual([
      {
        pointer: '/interfaces/GigabitEthernet0~18~10/subinterfaces/7/ipv6Ra',
        message:
          'router advertisements need IPv6 on GigabitEthernet0/8/0.7: add an IPv6 address to the interface',
      },
    ]);
  });
  it('isDefaultIpv6Ra: defaults only, no prefixes', () => {
    expect(isDefaultIpv6Ra(undefined)).toBe(true);
    const parsed = RootConfig.parse({ interfaces: { loop1: { ipv6Ra: {} } } }).interfaces['loop1']!
      .ipv6Ra;
    expect(isDefaultIpv6Ra(parsed)).toBe(true);
    expect(
      isDefaultIpv6Ra({
        ...parsed!,
        prefixes: {
          '2001:db8::/64': { validSec: 1, preferredSec: 1, offLink: false, noAutoconfig: false },
        },
      }),
    ).toBe(false);
    expect(isDefaultIpv6Ra({ ...parsed!, suppress: false })).toBe(false);
  });
});

describe('interfaces.neighbors-ra-slaac-prefix-length', () => {
  it('SLAAC prefixes must be /64 unless autoconfiguration is off', () => {
    expect(
      run('interfaces.neighbors-ra-slaac-prefix-length', {
        interfaces: lan({
          ipv6Ra: {
            prefixes: {
              '2001:db8:9::/48': {},
              '2001:db8:8::/48': { noAutoconfig: true },
              '2001:db8:9:1::/64': {},
            },
          },
        }),
      }),
    ).toEqual([
      {
        pointer: `/interfaces/${LAN}/ipv6Ra/prefixes/2001:db8:9::~148`,
        message:
          '2001:db8:9::/48 is a /48: SLAAC (autonomous flag) needs a /64 — use a /64 or set noAutoconfig',
      },
    ]);
  });
});

describe('interfaces.neighbors-ra-proxy-nd-unique', () => {
  it('reports the second spelling of one address', () => {
    expect(
      run('interfaces.neighbors-ra-proxy-nd-unique', {
        interfaces: lan({ proxyNd: ['2001:db8::5', '2001:DB8:0::5'] }),
      }),
    ).toEqual([
      {
        pointer: `/interfaces/${LAN}/proxyNd/1`,
        message: `proxy ND address 2001:DB8:0::5 is listed twice on ${LAN}`,
      },
    ]);
  });
});

describe('vrfs.neighbors-ra-proxy-arp-range-unique', () => {
  it('reports a duplicate range of one VRF, not the same range in two VRFs', () => {
    const r = { low: '10.9.3.1', high: '10.9.3.9' };
    expect(
      run('vrfs.neighbors-ra-proxy-arp-range-unique', {
        vrfs: { a: { id: 9004, proxyArpRanges: [r, r] }, b: { id: 9005, proxyArpRanges: [r] } },
      }),
    ).toEqual([
      {
        pointer: '/vrfs/a/proxyArpRanges/1',
        message: "proxy-ARP range 10.9.3.1–10.9.3.9 is already listed at index 0 of VRF 'a'",
      },
    ]);
  });
});

describe('routing.neighbors-ra-static-*', () => {
  const neighbors = (...st: { interface: string; ip: string; mac: string }[]) => ({
    neighbors: { static: st },
  });
  it('the interface must exist (parents and sub-interfaces)', () => {
    expect(
      run('routing.neighbors-ra-static-interface-exists', {
        interfaces: { ...lan({ subinterfaces: { '5': { vlanId: 5, ipv4: ['10.9.5.1/24'] } } }) },
        routing: neighbors(
          { interface: `${LAN}.5`, ip: '10.9.5.2', mac: '02:00:00:00:00:02' },
          { interface: 'host-w9x9', ip: '10.9.5.3', mac: '02:00:00:00:00:03' },
        ),
      }),
    ).toEqual([
      {
        pointer: '/routing/neighbors/static/1/interface',
        message: "interface 'host-w9x9' does not exist",
      },
    ]);
  });

  it('the address must lie in a connected subnet (acceptance: outside every subnet → pointer at ip)', () => {
    const doc = {
      interfaces: {
        ...lan(),
        'host-w9w0': { ipv4: ['10.9.2.1/24'] },
        loop905: { unnumbered: LAN },
      },
      routing: neighbors(
        { interface: LAN, ip: '10.9.1.77', mac: '02:00:00:00:00:01' },
        { interface: LAN, ip: '2001:db8:9:1::77', mac: '02:00:00:00:00:02' },
        { interface: LAN, ip: 'fe80::77', mac: '02:00:00:00:00:03' },
        { interface: LAN, ip: '10.9.2.77', mac: '02:00:00:00:00:04' },
        { interface: 'host-w9w0', ip: '2001:db8::1', mac: '02:00:00:00:00:05' },
        { interface: LAN, ip: '10.9.1.1', mac: '02:00:00:00:00:06' },
        { interface: 'loop905', ip: '10.9.1.99', mac: '02:00:00:00:00:07' },
        { interface: 'host-w9w0', ip: 'fe80::1', mac: '02:00:00:00:00:08' },
      ),
    };
    expect(run('routing.neighbors-ra-static-connected', doc)).toEqual([
      {
        pointer: '/routing/neighbors/static/3/ip',
        message: `10.9.2.77 is outside every connected subnet of ${LAN}`,
      },
      {
        pointer: '/routing/neighbors/static/4/ip',
        message:
          '2001:db8::1 is outside every connected subnet of host-w9w0: it has no IPv6 address',
      },
      {
        pointer: '/routing/neighbors/static/5/ip',
        message: `10.9.1.1 is an address of ${LAN} itself, not a neighbour`,
      },
      {
        pointer: '/routing/neighbors/static/7/ip',
        message: 'fe80::1 is outside every connected subnet of host-w9w0: it has no IPv6 address',
      },
    ]);
    // the same finding through the full validator run (what the API turns into a 400 problem+json)
    expect(validateSemantics(RootConfig.parse(doc)).map((i) => i.pointer)).toContain(
      '/routing/neighbors/static/3/ip',
    );
  });

  it('one entry per (interface, address), whatever the spelling', () => {
    expect(
      run('routing.neighbors-ra-static-unique', {
        interfaces: lan(),
        routing: neighbors(
          { interface: LAN, ip: '2001:db8:9:1::5', mac: '02:00:00:00:00:01' },
          { interface: LAN, ip: '2001:DB8:9:1:0::5', mac: '02:00:00:00:00:02' },
        ),
      }),
    ).toEqual([
      {
        pointer: '/routing/neighbors/static/1/ip',
        message: `static neighbour 2001:DB8:9:1:0::5 on ${LAN} is already defined at index 0`,
      },
    ]);
  });
});
