import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { routingValidators } from './routing.js';

const run = (name: string, doc: RootConfigInput) =>
  routingValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

const IFACES: RootConfigInput['interfaces'] = {
  'TenGigabitEthernet0/0/0': { subinterfaces: { '100': { vlanId: 100 } } },
  loop0: {},
};

describe('routing.vrf-exists', () => {
  it('accepts default / declared VRFs on static routes and protocol instances, and absent protocols', () => {
    expect(
      run('routing.vrf-exists', {
        vrfs: { a: { id: 1 } },
        routing: {
          static: [{ prefix: '10.0.0.0/8', nextHops: [{ interface: 'loop0' }], vrf: 'a' }],
          bgp: { asn: 65000, vrf: 'a' },
          ospf: {},
        },
      }),
    ).toEqual([]);
  });
  it('reports unknown VRFs everywhere they may appear', () => {
    expect(
      run('routing.vrf-exists', {
        routing: {
          static: [{ prefix: '10.0.0.0/8', nextHops: [{ interface: 'loop0' }], vrf: 'x' }],
          bgp: { asn: 65000, vrf: 'b' },
          ospf: { vrf: 'o' },
          isis: { net: '49.0001.1921.6800.1001.00', vrf: 'i' },
          rip: { vrf: 'r' },
        },
      }),
    ).toEqual([
      { pointer: '/routing/static/0/vrf', message: "VRF 'x' does not exist" },
      { pointer: '/routing/bgp/vrf', message: "VRF 'b' does not exist" },
      { pointer: '/routing/ospf/vrf', message: "VRF 'o' does not exist" },
      { pointer: '/routing/isis/vrf', message: "VRF 'i' does not exist" },
      { pointer: '/routing/rip/vrf', message: "VRF 'r' does not exist" },
    ]);
  });
});

describe('routing.static-nexthop-interface-exists', () => {
  it('accepts address-only hops, parents and sub-interfaces', () => {
    expect(
      run('routing.static-nexthop-interface-exists', {
        interfaces: IFACES,
        routing: {
          static: [
            { prefix: '0.0.0.0/0', nextHops: [{ address: '10.0.0.1' }, { interface: 'loop0' }, { interface: 'TenGigabitEthernet0/0/0.100' }] },
          ],
        },
      }),
    ).toEqual([]);
  });
  it('reports an unknown egress interface', () => {
    expect(
      run('routing.static-nexthop-interface-exists', {
        routing: { static: [{ prefix: '0.0.0.0/0', nextHops: [{ address: '10.0.0.1' }, { address: '10.0.0.2', interface: 'loop9' }] }] },
      }),
    ).toEqual([{ pointer: '/routing/static/0/nextHops/1/interface', message: "interface 'loop9' does not exist" }]);
  });
});

describe('routing.static-unique', () => {
  it('allows the same prefix in different VRFs but not twice in one', () => {
    expect(
      run('routing.static-unique', {
        vrfs: { a: { id: 1 } },
        routing: {
          static: [
            { prefix: '0.0.0.0/0', nextHops: [{ address: '10.0.0.1' }] },
            { prefix: '0.0.0.0/0', nextHops: [{ address: '10.1.0.1' }], vrf: 'a' },
            { prefix: '2001:DB8::/32', nextHops: [{ address: '2001:db8::1' }] },
            { prefix: '2001:db8::/32', nextHops: [{ address: '2001:db8::2' }] },
          ],
        },
      }),
    ).toEqual([
      {
        pointer: '/routing/static/3/prefix',
        message: "route 2001:db8::/32 in VRF 'default' is already defined at /routing/static/2; add next hops there instead",
      },
    ]);
  });
});

describe('routing.prefix-list-exists', () => {
  it('is silent without BGP and when every reference resolves', () => {
    expect(run('routing.prefix-list-exists', {})).toEqual([]);
    expect(
      run('routing.prefix-list-exists', {
        routing: {
          prefixLists: { pl: {} },
          routeMaps: { rm: { entries: [{ seq: 10, action: 'permit', match: { prefixList: 'pl', nextHopPrefixList: 'pl' } }, { seq: 20, action: 'deny' }] } },
          bgp: {
            asn: 1,
            peerGroups: { g: { ipv4Unicast: { prefixListIn: 'pl', prefixListOut: 'pl' } } },
            neighbors: [{ address: '10.0.0.1', remoteAs: 2, ipv6Unicast: { prefixListIn: 'pl' } }],
          },
        },
      }),
    ).toEqual([]);
  });
  it('reports every dangling prefix-list reference', () => {
    expect(
      run('routing.prefix-list-exists', {
        routing: {
          routeMaps: { rm: { entries: [{ seq: 10, action: 'permit', match: { prefixList: 'a', nextHopPrefixList: 'b' } }] } },
          bgp: {
            asn: 1,
            peerGroups: { g: { ipv4Unicast: { prefixListIn: 'c' } } },
            neighbors: [{ address: '10.0.0.1', remoteAs: 2, ipv6Unicast: { prefixListOut: 'd' } }],
          },
        },
      }),
    ).toEqual([
      { pointer: '/routing/routeMaps/rm/entries/0/match/prefixList', message: "prefix list 'a' does not exist" },
      { pointer: '/routing/routeMaps/rm/entries/0/match/nextHopPrefixList', message: "prefix list 'b' does not exist" },
      { pointer: '/routing/bgp/peerGroups/g/ipv4Unicast/prefixListIn', message: "prefix list 'c' does not exist" },
      { pointer: '/routing/bgp/neighbors/0/ipv6Unicast/prefixListOut', message: "prefix list 'd' does not exist" },
    ]);
  });
});

describe('routing.route-map-exists', () => {
  it('is silent without protocols and when every reference resolves', () => {
    expect(run('routing.route-map-exists', {})).toEqual([]);
    expect(
      run('routing.route-map-exists', {
        routing: {
          routeMaps: { rm: {} },
          bgp: {
            asn: 1,
            neighbors: [{ address: '10.0.0.1', remoteAs: 2, ipv4Unicast: { routeMapIn: 'rm', routeMapOut: 'rm' } }],
            networks: [{ prefix: '10.0.0.0/8', routeMap: 'rm' }, { prefix: '10.1.0.0/16' }],
            redistribute: [{ protocol: 'connected', routeMap: 'rm' }, { protocol: 'static' }],
          },
          ospf: { redistribute: [{ protocol: 'bgp', routeMap: 'rm' }] },
        },
      }),
    ).toEqual([]);
  });
  it('reports dangling route-map references in BGP policy, networks and every redistribute list', () => {
    expect(
      run('routing.route-map-exists', {
        routing: {
          bgp: {
            asn: 1,
            peerGroups: { g: { ipv6Unicast: { routeMapOut: 'a' } } },
            neighbors: [{ address: '10.0.0.1', remoteAs: 2, ipv4Unicast: { routeMapIn: 'b' } }],
            networks: [{ prefix: '10.0.0.0/8', routeMap: 'c' }],
            redistribute: [{ protocol: 'connected', routeMap: 'd' }],
          },
          ospf: { redistribute: [{ protocol: 'bgp', routeMap: 'e' }] },
          isis: { net: '49.0001.1921.6800.1001.00', redistribute: [{ protocol: 'static', routeMap: 'f' }] },
          rip: { redistribute: [{ protocol: 'ospf', routeMap: 'g' }] },
        },
      }),
    ).toEqual([
      { pointer: '/routing/bgp/peerGroups/g/ipv6Unicast/routeMapOut', message: "route map 'a' does not exist" },
      { pointer: '/routing/bgp/neighbors/0/ipv4Unicast/routeMapIn', message: "route map 'b' does not exist" },
      { pointer: '/routing/bgp/networks/0/routeMap', message: "route map 'c' does not exist" },
      { pointer: '/routing/bgp/redistribute/0/routeMap', message: "route map 'd' does not exist" },
      { pointer: '/routing/ospf/redistribute/0/routeMap', message: "route map 'e' does not exist" },
      { pointer: '/routing/isis/redistribute/0/routeMap', message: "route map 'f' does not exist" },
      { pointer: '/routing/rip/redistribute/0/routeMap', message: "route map 'g' does not exist" },
    ]);
  });
});

describe('routing.bgp-peer-group-exists', () => {
  it('is silent without BGP, without peer groups, or when the group exists', () => {
    expect(run('routing.bgp-peer-group-exists', {})).toEqual([]);
    expect(
      run('routing.bgp-peer-group-exists', {
        routing: { bgp: { asn: 1, peerGroups: { g: { remoteAs: 2 } }, neighbors: [{ address: '10.0.0.1', peerGroup: 'g' }, { address: '10.0.0.2', remoteAs: 3 }] } },
      }),
    ).toEqual([]);
  });
  it('reports an unknown peer group', () => {
    expect(run('routing.bgp-peer-group-exists', { routing: { bgp: { asn: 1, neighbors: [{ address: '10.0.0.1', peerGroup: 'nope' }] } } })).toEqual([
      { pointer: '/routing/bgp/neighbors/0/peerGroup', message: "peer group 'nope' does not exist" },
    ]);
  });
});

describe('routing.interface-exists', () => {
  it('accepts known interfaces and address-typed update sources; skips absent protocols', () => {
    expect(run('routing.interface-exists', {})).toEqual([]);
    expect(
      run('routing.interface-exists', {
        interfaces: IFACES,
        routing: {
          routeMaps: { rm: { entries: [{ seq: 1, action: 'permit', match: { interface: 'loop0' } }, { seq: 2, action: 'permit' }] } },
          bgp: {
            asn: 1,
            peerGroups: { g: { updateSource: 'loop0' } },
            neighbors: [{ address: '10.0.0.1', remoteAs: 2, updateSource: '10.255.0.1' }, { address: '10.0.0.2', remoteAs: 2 }],
          },
          ospf: { areas: [{ id: 0 }], interfaces: [{ name: 'TenGigabitEthernet0/0/0.100', area: 0 }] },
          isis: { net: '49.0001.1921.6800.1001.00', interfaces: [{ name: 'loop0' }] },
          rip: { interfaces: [{ name: 'loop0' }] },
          bfd: { sessions: [{ interface: 'loop0', localAddress: '10.0.0.1', peerAddress: '10.0.0.2' }] },
        },
      }),
    ).toEqual([]);
  });
  it('reports unknown interfaces in route maps, BGP update sources and every protocol', () => {
    expect(
      run('routing.interface-exists', {
        routing: {
          routeMaps: { rm: { entries: [{ seq: 1, action: 'permit', match: { interface: 'a' } }] } },
          bgp: { asn: 1, neighbors: [{ address: '10.0.0.1', remoteAs: 2, updateSource: 'b' }] },
          ospf: { interfaces: [{ name: 'c', area: 0 }] },
          isis: { net: '49.0001.1921.6800.1001.00', interfaces: [{ name: 'd' }] },
          rip: { interfaces: [{ name: 'e' }] },
          bfd: { sessions: [{ interface: 'f', localAddress: '10.0.0.1', peerAddress: '10.0.0.2' }] },
        },
      }),
    ).toEqual([
      { pointer: '/routing/routeMaps/rm/entries/0/match/interface', message: "interface 'a' does not exist" },
      { pointer: '/routing/bgp/neighbors/0/updateSource', message: "interface 'b' does not exist" },
      { pointer: '/routing/ospf/interfaces/0/name', message: "interface 'c' does not exist" },
      { pointer: '/routing/isis/interfaces/0/name', message: "interface 'd' does not exist" },
      { pointer: '/routing/rip/interfaces/0/name', message: "interface 'e' does not exist" },
      { pointer: '/routing/bfd/sessions/0/interface', message: "interface 'f' does not exist" },
    ]);
  });
});

describe('routing.ospf-area-exists', () => {
  it('is silent without OSPF and treats 0 and 0.0.0.0 as the same area', () => {
    expect(run('routing.ospf-area-exists', {})).toEqual([]);
    expect(
      run('routing.ospf-area-exists', {
        routing: { ospf: { areas: [{ id: '0.0.0.0' }, { id: 51 }], interfaces: [{ name: 'loop0', area: 0 }, { name: 'loop1', area: '0.0.0.51' }] } },
      }),
    ).toEqual([]);
  });
  it('reports an interface in an undefined area', () => {
    expect(run('routing.ospf-area-exists', { routing: { ospf: { areas: [{ id: 0 }], interfaces: [{ name: 'loop0', area: '0.0.0.1' }] } } })).toEqual([
      { pointer: '/routing/ospf/interfaces/0/area', message: 'OSPF area 0.0.0.1 is not defined under /routing/ospf/areas' },
    ]);
  });
});
