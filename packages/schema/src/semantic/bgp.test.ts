import { describe, expect, it } from 'vitest';
import { InterfaceSchema, RootConfig, StaticRouteSchema, type RootConfigInput } from '../index.js';
import { bgpValidators, lcpInterfaces } from './bgp.js';
import { validateSemantics } from './index.js';

const run = (name: string, doc: RootConfigInput) =>
  bgpValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

describe('schema: the fields P12 adds', () => {
  it('lcp is optional; present = a pair with tap by default; names are Linux names', () => {
    expect(InterfaceSchema.parse({})).not.toHaveProperty('lcp');
    expect(InterfaceSchema.parse({ lcp: {} }).lcp).toEqual({ hostIfType: 'tap' });
    expect(
      InterfaceSchema.parse({ lcp: { hostIfName: 'w8-l0', netns: 'ns-w8-frr', hostIfType: 'tun' } })
        .lcp,
    ).toEqual({
      hostIfName: 'w8-l0',
      netns: 'ns-w8-frr',
      hostIfType: 'tun',
    });
    for (const bad of [
      { hostIfName: 'a-name-longer-than-15' },
      { hostIfName: 'has space' },
      { hostIfName: 'x/y' },
      { hostIfName: '10.0.0.1' },
      { hostIfName: '..' },
      { hostIfType: 'veth' },
      { netns: 'a/b' },
      { extra: 1 },
    ]) {
      expect(InterfaceSchema.safeParse({ lcp: bad }).success, JSON.stringify(bad)).toBe(false);
    }
  });

  it('static[].tag is optional, a positive uint32', () => {
    expect(StaticRouteSchema.parse({ prefix: '10.0.0.0/8', blackhole: true })).not.toHaveProperty(
      'tag',
    );
    expect(StaticRouteSchema.parse({ prefix: '10.0.0.0/8', blackhole: true, tag: 7 }).tag).toBe(7);
    for (const bad of [0, -1, 4294967296, 1.5]) {
      expect(
        StaticRouteSchema.safeParse({ prefix: '10.0.0.0/8', blackhole: true, tag: bad }).success,
      ).toBe(false);
    }
  });
});

describe('semantic rules routing.bgp-*', () => {
  it('lcp-host-name: a VPP name that is no Linux name needs hostIfName', () => {
    const doc: RootConfigInput = {
      interfaces: { 'TenGigabitEthernet0/0/0': { lcp: {} }, loop0: { lcp: {} } },
    };
    expect(run('routing.bgp-lcp-host-name', doc)).toEqual([
      {
        pointer: '/interfaces/TenGigabitEthernet0~10~10/lcp/hostIfName',
        message: expect.stringContaining('set hostIfName') as string,
      },
    ]);
    expect(
      run('routing.bgp-lcp-host-name', {
        interfaces: { 'TenGigabitEthernet0/0/0': { lcp: { hostIfName: 'te0' } } },
      }),
    ).toEqual([]);
  });

  it('lcp-host-name-unique: two pairs cannot share a Linux name (explicit or derived)', () => {
    const issues = run('routing.bgp-lcp-host-name-unique', {
      interfaces: {
        loop0: { lcp: {} },
        loop1: { lcp: { hostIfName: 'loop0' } },
        loop2: { lcp: { hostIfName: 'l2' } },
      },
    });
    expect(issues).toHaveLength(1);
    expect(issues[0]!.message).toContain("'loop0'");
  });

  it('interface-has-lcp: update-source, match interface and viaFrr egress interfaces need a pair', () => {
    const doc: RootConfigInput = {
      interfaces: { lan: {}, wan: { lcp: {} } },
      routing: {
        static: [
          { prefix: '10.9.0.0/16', viaFrr: true, nextHops: [{ interface: 'lan' }] },
          { prefix: '10.10.0.0/16', nextHops: [{ interface: 'lan' }] },
          { prefix: '10.11.0.0/16', viaFrr: true, nextHops: [{ interface: 'wan' }] },
        ],
        policy: {
          routeMaps: {
            rm: { entries: [{ seq: 10, action: 'permit', match: { interface: 'lan' } }] },
          },
        },
        bgp: {
          asn: 65000,
          ebgpRequiresPolicy: false,
          neighbors: {
            '10.0.0.1': { remoteAs: 65001, updateSource: 'lan' },
            '10.0.0.2': { remoteAs: 65001, updateSource: 'wan' },
            '10.0.0.3': { remoteAs: 65001, updateSource: '10.0.0.9' },
          },
        },
      },
    };
    expect(
      run('routing.bgp-interface-has-lcp', doc)
        .map((i) => i.pointer)
        .sort(),
    ).toEqual([
      '/routing/bgp/neighbors/10.0.0.1/updateSource',
      '/routing/policy/routeMaps/rm/entries/0/match/interface',
      '/routing/static/0/nextHops/0/interface',
    ]);
  });

  it('static-tag-via-frr: a tag needs viaFrr', () => {
    expect(
      run('routing.bgp-static-tag-via-frr', {
        routing: {
          static: [
            { prefix: '10.1.0.0/16', blackhole: true, tag: 5 },
            { prefix: '10.2.0.0/16', blackhole: true, tag: 5, viaFrr: true },
          ],
        },
      }),
    ).toEqual([
      { pointer: '/routing/static/0/tag', message: expect.stringContaining('viaFrr') as string },
    ]);
  });

  it('lcpInterfaces maps VPP names to Linux names', () => {
    const c = RootConfig.parse({
      interfaces: { loop0: { lcp: {} }, 'Gi0/1/0': { lcp: { hostIfName: 'gi0' } }, x: {} },
    });
    expect([...lcpInterfaces(c)]).toEqual([
      ['loop0', 'loop0'],
      ['Gi0/1/0', 'gi0'],
    ]);
  });

  it('are registered and run through validateSemantics', () => {
    const doc = RootConfig.parse({ interfaces: { 'Gi0/1/0': { lcp: {} } } });
    expect(validateSemantics(doc).map((i) => i.pointer)).toContain(
      '/interfaces/Gi0~11~10/lcp/hostIfName',
    );
  });
});
