import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { routingValidators } from './routing.js';

const run = (name: string, doc: RootConfigInput) =>
  routingValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

const IFACES: RootConfigInput['interfaces'] = {
  'TenGigabitEthernet0/0/0': { subinterfaces: { '100': { vlanId: 100 } } },
  loop0: {},
};
const NET = '49.0001.1921.6800.1001.00';

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
          isis: { net: NET, vrf: 'i' },
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
  it('accepts address-only hops, parents, sub-interfaces and blackhole routes', () => {
    expect(
      run('routing.static-nexthop-interface-exists', {
        interfaces: IFACES,
        routing: {
          static: [
            {
              prefix: '0.0.0.0/0',
              nextHops: [
                { address: '10.0.0.1' },
                { interface: 'loop0' },
                { interface: 'TenGigabitEthernet0/0/0.100' },
              ],
            },
            { prefix: '192.0.2.0/24', blackhole: true },
          ],
        },
      }),
    ).toEqual([]);
  });
  it('reports an unknown egress interface', () => {
    expect(
      run('routing.static-nexthop-interface-exists', {
        routing: {
          static: [
            {
              prefix: '0.0.0.0/0',
              nextHops: [{ address: '10.0.0.1' }, { address: '10.0.0.2', interface: 'loop9' }],
            },
          ],
        },
      }),
    ).toEqual([
      {
        pointer: '/routing/static/0/nextHops/1/interface',
        message: "interface 'loop9' does not exist",
      },
    ]);
  });
});

describe('routing.static-unique (review M3)', () => {
  it('allows the same prefix in different VRFs but not twice in one, in any spelling', () => {
    expect(
      run('routing.static-unique', {
        vrfs: { a: { id: 1 } },
        routing: {
          static: [
            { prefix: '0.0.0.0/0', nextHops: [{ address: '10.0.0.1' }] },
            { prefix: '0.0.0.0/0', nextHops: [{ address: '10.1.0.1' }], vrf: 'a' },
            { prefix: '2001:DB8:0::/32', nextHops: [{ address: '2001:db8::1' }] },
            { prefix: '2001:db8::/32', nextHops: [{ address: '2001:db8::2' }] },
            { prefix: '2001:db8::/48', blackhole: true },
          ],
        },
      }),
    ).toEqual([
      {
        pointer: '/routing/static/3/prefix',
        message:
          "route 2001:db8::/32 in VRF 'default' is already defined; add next hops there instead (first defined at /routing/static/2/prefix)",
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
          policy: {
            prefixLists: { pl: {} },
            routeMaps: {
              rm: {
                entries: [
                  {
                    seq: 10,
                    action: 'permit',
                    match: { prefixList: 'pl', nextHopPrefixList: 'pl' },
                  },
                  { seq: 20, action: 'deny' },
                ],
              },
            },
          },
          bgp: {
            asn: 1,
            peerGroups: {
              g: { afi: { ipv4Unicast: { prefixListIn: 'pl', prefixListOut: 'pl' } } },
            },
            neighbors: {
              '10.0.0.1': { remoteAs: 2, afi: { ipv6Unicast: { prefixListIn: 'pl' } } },
              '10.0.0.2': { remoteAs: 2 },
            },
          },
        },
      }),
    ).toEqual([]);
  });
  it('reports every dangling prefix-list reference', () => {
    expect(
      run('routing.prefix-list-exists', {
        routing: {
          policy: {
            routeMaps: {
              rm: {
                entries: [
                  { seq: 10, action: 'permit', match: { prefixList: 'a', nextHopPrefixList: 'b' } },
                ],
              },
            },
          },
          bgp: {
            asn: 1,
            peerGroups: { g: { afi: { ipv4Unicast: { prefixListIn: 'c' } } } },
            neighbors: {
              '10.0.0.1': { remoteAs: 2, afi: { ipv6Unicast: { prefixListOut: 'd' } } },
            },
          },
        },
      }),
    ).toEqual([
      {
        pointer: '/routing/policy/routeMaps/rm/entries/0/match/prefixList',
        message: "prefix list 'a' does not exist",
      },
      {
        pointer: '/routing/policy/routeMaps/rm/entries/0/match/nextHopPrefixList',
        message: "prefix list 'b' does not exist",
      },
      {
        pointer: '/routing/bgp/peerGroups/g/afi/ipv4Unicast/prefixListIn',
        message: "prefix list 'c' does not exist",
      },
      {
        pointer: '/routing/bgp/neighbors/10.0.0.1/afi/ipv6Unicast/prefixListOut',
        message: "prefix list 'd' does not exist",
      },
    ]);
  });
});

describe('routing.route-map-exists', () => {
  it('is silent without protocols and when every reference resolves', () => {
    expect(run('routing.route-map-exists', {})).toEqual([]);
    expect(
      run('routing.route-map-exists', {
        routing: {
          policy: { routeMaps: { rm: {} } },
          bgp: {
            asn: 1,
            neighbors: {
              '10.0.0.1': {
                remoteAs: 2,
                afi: { ipv4Unicast: { routeMapIn: 'rm', routeMapOut: 'rm' } },
              },
            },
            networks: [{ prefix: '10.0.0.0/8', routeMap: 'rm' }, { prefix: '10.1.0.0/16' }],
            redistribute: { connected: { routeMap: 'rm' }, static: {} },
          },
          ospf: { redistribute: { bgp: { routeMap: 'rm' } } },
        },
      }),
    ).toEqual([]);
  });
  it('reports dangling route-map references in BGP policy, networks and every redistribute record', () => {
    expect(
      run('routing.route-map-exists', {
        routing: {
          bgp: {
            asn: 1,
            peerGroups: { g: { afi: { ipv6Unicast: { routeMapOut: 'a' } } } },
            neighbors: { '10.0.0.1': { remoteAs: 2, afi: { ipv4Unicast: { routeMapIn: 'b' } } } },
            networks: [{ prefix: '10.0.0.0/8', routeMap: 'c' }],
            redistribute: { connected: { routeMap: 'd' } },
          },
          ospf: { redistribute: { bgp: { routeMap: 'e' } } },
          isis: { net: NET, redistribute: { static: { routeMap: 'f' } } },
          rip: { redistribute: { ospf: { routeMap: 'g' } } },
        },
      }),
    ).toEqual([
      {
        pointer: '/routing/bgp/peerGroups/g/afi/ipv6Unicast/routeMapOut',
        message: "route map 'a' does not exist",
      },
      {
        pointer: '/routing/bgp/neighbors/10.0.0.1/afi/ipv4Unicast/routeMapIn',
        message: "route map 'b' does not exist",
      },
      { pointer: '/routing/bgp/networks/0/routeMap', message: "route map 'c' does not exist" },
      {
        pointer: '/routing/bgp/redistribute/connected/routeMap',
        message: "route map 'd' does not exist",
      },
      {
        pointer: '/routing/ospf/redistribute/bgp/routeMap',
        message: "route map 'e' does not exist",
      },
      {
        pointer: '/routing/isis/redistribute/static/routeMap',
        message: "route map 'f' does not exist",
      },
      {
        pointer: '/routing/rip/redistribute/ospf/routeMap',
        message: "route map 'g' does not exist",
      },
    ]);
  });
});

describe('routing.bgp-peer-group-exists', () => {
  it('is silent without BGP, without peer groups, or when the group exists', () => {
    expect(run('routing.bgp-peer-group-exists', {})).toEqual([]);
    expect(
      run('routing.bgp-peer-group-exists', {
        routing: {
          bgp: {
            asn: 1,
            peerGroups: { g: { remoteAs: 2 } },
            neighbors: { '10.0.0.1': { peerGroup: 'g' }, '10.0.0.2': { remoteAs: 3 } },
          },
        },
      }),
    ).toEqual([]);
  });
  it('reports an unknown peer group', () => {
    expect(
      run('routing.bgp-peer-group-exists', {
        routing: { bgp: { asn: 1, neighbors: { '2001:db8::1': { peerGroup: 'nope' } } } },
      }),
    ).toEqual([
      {
        pointer: '/routing/bgp/neighbors/2001:db8::1/peerGroup',
        message: "peer group 'nope' does not exist",
      },
    ]);
  });
});

describe('routing.bgp-neighbor-unique (review M2)', () => {
  it('is silent without BGP and for distinct addresses', () => {
    expect(run('routing.bgp-neighbor-unique', {})).toEqual([]);
    expect(
      run('routing.bgp-neighbor-unique', {
        routing: {
          bgp: {
            asn: 1,
            neighbors: { '10.0.0.1': { remoteAs: 2 }, '2001:db8::1': { remoteAs: 2 } },
          },
        },
      }),
    ).toEqual([]);
  });
  it('reports two spellings of one address', () => {
    expect(
      run('routing.bgp-neighbor-unique', {
        routing: {
          bgp: {
            asn: 1,
            neighbors: { '2001:db8::1': { remoteAs: 2 }, '2001:DB8:0::1': { remoteAs: 3 } },
          },
        },
      }),
    ).toEqual([
      {
        pointer: '/routing/bgp/neighbors/2001:DB8:0::1',
        message:
          'neighbour 2001:DB8:0::1 is configured twice (addresses compare canonically) (first defined at /routing/bgp/neighbors/2001:db8::1)',
      },
    ]);
  });
});

describe('routing.network-unique (review M2)', () => {
  it('is silent without protocols and for distinct networks', () => {
    expect(run('routing.network-unique', {})).toEqual([]);
    expect(
      run('routing.network-unique', {
        routing: {
          bgp: { asn: 1, networks: [{ prefix: '10.0.0.0/8' }, { prefix: '10.0.0.0/16' }] },
          rip: { networks: ['10.0.0.0/8', '192.168.0.0/16'] },
        },
      }),
    ).toEqual([]);
  });
  it('reports repeated BGP and RIP networks', () => {
    expect(
      run('routing.network-unique', {
        routing: {
          bgp: { asn: 1, networks: [{ prefix: '2001:db8::/32' }, { prefix: '2001:DB8::/32' }] },
          rip: { networks: ['10.0.0.0/8', '10.0.0.0/8'] },
        },
      }),
    ).toEqual([
      {
        pointer: '/routing/bgp/networks/1/prefix',
        message:
          'network 2001:DB8::/32 is listed more than once (first defined at /routing/bgp/networks/0/prefix)',
      },
      {
        pointer: '/routing/rip/networks/1',
        message:
          'network 10.0.0.0/8 is listed more than once (first defined at /routing/rip/networks/0)',
      },
    ]);
  });
});

describe('routing.bfd-session-unique (review M2)', () => {
  const session = (iface: string, peer: string) => ({
    interface: iface,
    localAddress: peer.includes(':') ? '2001:db8::1' : '10.0.0.1',
    peerAddress: peer,
  });
  it('is silent without BFD and for distinct (interface, peer) pairs', () => {
    expect(run('routing.bfd-session-unique', {})).toEqual([]);
    expect(
      run('routing.bfd-session-unique', {
        routing: {
          bfd: { sessions: [session('loop0', '10.0.0.2'), session('loop1', '10.0.0.2')] },
        },
      }),
    ).toEqual([]);
  });
  it('reports a second session to the same peer on the same interface', () => {
    expect(
      run('routing.bfd-session-unique', {
        routing: {
          bfd: { sessions: [session('loop0', '2001:db8::2'), session('loop0', '2001:DB8::2')] },
        },
      }),
    ).toEqual([
      {
        pointer: '/routing/bfd/sessions/1/peerAddress',
        message:
          'a BFD session to 2001:DB8::2 on loop0 already exists (first defined at /routing/bfd/sessions/0/peerAddress)',
      },
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
          policy: {
            routeMaps: {
              rm: {
                entries: [
                  { seq: 1, action: 'permit', match: { interface: 'loop0' } },
                  { seq: 2, action: 'permit' },
                ],
              },
            },
          },
          bgp: {
            asn: 1,
            peerGroups: { g: { updateSource: 'loop0' } },
            neighbors: {
              '10.0.0.1': { remoteAs: 2, updateSource: '10.255.0.1' },
              '10.0.0.2': { remoteAs: 2 },
            },
          },
          ospf: {
            areas: { '0': {} },
            interfaces: { 'TenGigabitEthernet0/0/0.100': { area: '0' } },
          },
          isis: { net: NET, interfaces: { loop0: {} } },
          rip: { interfaces: { loop0: {} } },
          bfd: {
            sessions: [{ interface: 'loop0', localAddress: '10.0.0.1', peerAddress: '10.0.0.2' }],
          },
        },
      }),
    ).toEqual([]);
  });
  it('reports unknown interfaces in route maps, BGP update sources and every protocol', () => {
    expect(
      run('routing.interface-exists', {
        routing: {
          policy: {
            routeMaps: {
              rm: { entries: [{ seq: 1, action: 'permit', match: { interface: 'a' } }] },
            },
          },
          bgp: { asn: 1, neighbors: { '10.0.0.1': { remoteAs: 2, updateSource: 'b' } } },
          ospf: { interfaces: { c: { area: '0' } } },
          isis: { net: NET, interfaces: { d: {} } },
          rip: { interfaces: { e: {} } },
          bfd: {
            sessions: [{ interface: 'f', localAddress: '10.0.0.1', peerAddress: '10.0.0.2' }],
          },
        },
      }),
    ).toEqual([
      {
        pointer: '/routing/policy/routeMaps/rm/entries/0/match/interface',
        message: "interface 'a' does not exist",
      },
      {
        pointer: '/routing/bgp/neighbors/10.0.0.1/updateSource',
        message: "interface 'b' does not exist",
      },
      { pointer: '/routing/ospf/interfaces/c', message: "interface 'c' does not exist" },
      { pointer: '/routing/isis/interfaces/d', message: "interface 'd' does not exist" },
      { pointer: '/routing/rip/interfaces/e', message: "interface 'e' does not exist" },
      { pointer: '/routing/bfd/sessions/0/interface', message: "interface 'f' does not exist" },
    ]);
  });
});

describe('routing.ospf-area-exists', () => {
  it('is silent without OSPF and treats 0 and 0.0.0.0 as the same area', () => {
    expect(run('routing.ospf-area-exists', {})).toEqual([]);
    expect(
      run('routing.ospf-area-exists', {
        routing: {
          ospf: {
            areas: { '0.0.0.0': {}, '51': {} },
            interfaces: { loop0: { area: '0' }, loop1: { area: '0.0.0.51' } },
          },
        },
      }),
    ).toEqual([]);
  });
  it('reports an interface in an undefined area', () => {
    expect(
      run('routing.ospf-area-exists', {
        routing: { ospf: { areas: { '0': {} }, interfaces: { loop0: { area: '0.0.0.1' } } } },
      }),
    ).toEqual([
      {
        pointer: '/routing/ospf/interfaces/loop0/area',
        message: 'OSPF area 0.0.0.1 is not defined under /routing/ospf/areas',
      },
    ]);
  });
});
