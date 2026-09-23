import { interfaceNames } from '../domains/interfaces.js';
import {
  BGP_AFIS,
  ospfAreaNumber,
  type BgpNeighborConfig,
  type BgpPeerGroupConfig,
} from '../domains/routing.js';
import { vrfExists } from '../domains/vrfs.js';
import { canonicalIp, canonicalPrefix, ipFamily } from '../ip.js';
import { jsonPointer } from '../pointer.js';
import type { RootConfig } from '../index.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { duplicateIssues } from './unique.js';

/**
 * Semantic validators for `routing`: VRFs, interfaces, prefix-lists, route-maps, peer groups and OSPF areas that
 * are referenced must exist; static routes, BGP neighbours, originated networks and BFD sessions are unique by
 * their canonical key (D-049, review M2/M3). Single-object consistency (address families, unique sequence numbers,
 * hold > keepalive, prefix-length ranges) is enforced by `.refine()` in `domains/routing.ts`.
 */

type Path = readonly (string | number)[];
type Protocol = 'bgp' | 'ospf' | 'isis' | 'rip';
const PROTOCOLS: readonly Protocol[] = ['bgp', 'ospf', 'isis', 'rip'];
const IGPS = ['ospf', 'isis', 'rip'] as const;

/** Collects `{ pointer, message }` for references that do not resolve. */
class Refs {
  readonly issues: SemanticIssue[] = [];
  constructor(
    private readonly kind: string,
    private readonly exists: (name: string) => boolean,
  ) {}
  check(name: string | undefined, ...path: Path): void {
    if (name !== undefined && !this.exists(name)) {
      this.issues.push({
        pointer: jsonPointer(...path),
        message: `${this.kind} '${name}' does not exist`,
      });
    }
  }
}

/** Every `redistribute.<source>.routeMap` of every enabled protocol. */
function checkRedistributeRouteMaps(config: RootConfig, refs: Refs): void {
  for (const protocol of PROTOCOLS) {
    const instance = config.routing[protocol];
    if (instance === undefined) continue;
    for (const [source, options] of Object.entries(instance.redistribute)) {
      refs.check(options?.routeMap, 'routing', protocol, 'redistribute', source, 'routeMap');
    }
  }
}

type BgpPeer = BgpNeighborConfig | BgpPeerGroupConfig;

/** Every BGP neighbour and peer group with the pointer segments that lead to it. */
function bgpPeers(config: RootConfig): [Path, BgpPeer][] {
  const bgp = config.routing.bgp;
  if (bgp === undefined) return [];
  const out: [Path, BgpPeer][] = [];
  for (const [name, group] of Object.entries(bgp.peerGroups))
    out.push([['routing', 'bgp', 'peerGroups', name], group]);
  for (const [address, neighbor] of Object.entries(bgp.neighbors))
    out.push([['routing', 'bgp', 'neighbors', address], neighbor]);
  return out;
}

/** Every route-map entry with its pointer segments. */
function routeMapEntries(config: RootConfig) {
  return Object.entries(config.routing.policy.routeMaps).flatMap(([name, map]) =>
    map.entries.map((entry, i) => ({
      entry,
      path: ['routing', 'policy', 'routeMaps', name, 'entries', i] as Path,
    })),
  );
}

export const routingValidators: readonly ValidatorDefinition[] = [
  {
    name: 'routing.vrf-exists',
    domains: ['routing', 'vrfs'],
    validate: (config) => {
      const refs = new Refs('VRF', (name) => vrfExists(config.vrfs, name));
      for (const [i, route] of config.routing.static.entries())
        refs.check(route.vrf, 'routing', 'static', i, 'vrf');
      for (const protocol of PROTOCOLS)
        refs.check(config.routing[protocol]?.vrf, 'routing', protocol, 'vrf');
      return refs.issues;
    },
  },
  {
    name: 'routing.static-nexthop-interface-exists',
    domains: ['routing', 'interfaces'],
    validate: (config) => {
      const names = interfaceNames(config.interfaces);
      const refs = new Refs('interface', (name) => names.has(name));
      for (const [i, route] of config.routing.static.entries()) {
        for (const [j, hop] of route.nextHops.entries()) {
          refs.check(hop.interface, 'routing', 'static', i, 'nextHops', j, 'interface');
        }
      }
      return refs.issues;
    },
  },
  {
    // keyed on the parsed prefix, so `2001:DB8:0::/32` and `2001:db8::/32` are the same FIB entry (review M3)
    name: 'routing.static-unique',
    domains: ['routing'],
    validate: (config) =>
      duplicateIssues(
        config.routing.static,
        (route) => `${route.vrf} ${canonicalPrefix(route.prefix) ?? route.prefix}`,
        (_route, i) => ['routing', 'static', i, 'prefix'],
        (route) =>
          `route ${route.prefix} in VRF '${route.vrf}' is already defined; add next hops there instead`,
      ),
  },
  {
    name: 'routing.prefix-list-exists',
    domains: ['routing'],
    validate: (config) => {
      const refs = new Refs('prefix list', (name) =>
        Object.hasOwn(config.routing.policy.prefixLists, name),
      );
      for (const { entry, path } of routeMapEntries(config)) {
        refs.check(entry.match.prefixList, ...path, 'match', 'prefixList');
        refs.check(entry.match.nextHopPrefixList, ...path, 'match', 'nextHopPrefixList');
      }
      for (const [path, peer] of bgpPeers(config)) {
        for (const af of BGP_AFIS) {
          refs.check(peer.afi[af]?.prefixListIn, ...path, 'afi', af, 'prefixListIn');
          refs.check(peer.afi[af]?.prefixListOut, ...path, 'afi', af, 'prefixListOut');
        }
      }
      return refs.issues;
    },
  },
  {
    name: 'routing.route-map-exists',
    domains: ['routing'],
    validate: (config) => {
      const { policy, bgp } = config.routing;
      const refs = new Refs('route map', (name) => Object.hasOwn(policy.routeMaps, name));
      for (const [path, peer] of bgpPeers(config)) {
        for (const af of BGP_AFIS) {
          refs.check(peer.afi[af]?.routeMapIn, ...path, 'afi', af, 'routeMapIn');
          refs.check(peer.afi[af]?.routeMapOut, ...path, 'afi', af, 'routeMapOut');
        }
      }
      for (const [i, network] of (bgp?.networks ?? []).entries()) {
        refs.check(network.routeMap, 'routing', 'bgp', 'networks', i, 'routeMap');
      }
      checkRedistributeRouteMaps(config, refs);
      return refs.issues;
    },
  },
  {
    name: 'routing.bgp-peer-group-exists',
    domains: ['routing'],
    validate: (config) => {
      const bgp = config.routing.bgp;
      if (bgp === undefined) return [];
      const refs = new Refs('peer group', (name) => Object.hasOwn(bgp.peerGroups, name));
      for (const [address, n] of Object.entries(bgp.neighbors))
        refs.check(n.peerGroup, 'routing', 'bgp', 'neighbors', address, 'peerGroup');
      return refs.issues;
    },
  },
  {
    // record keys are unique as text; two spellings of one address (`2001:DB8::1`, `2001:db8::1`) are not
    name: 'routing.bgp-neighbor-unique',
    domains: ['routing'],
    validate: (config) =>
      duplicateIssues(
        Object.keys(config.routing.bgp?.neighbors ?? {}),
        (address) => canonicalIp(address) ?? address,
        (address) => ['routing', 'bgp', 'neighbors', address],
        (address) => `neighbour ${address} is configured twice (addresses compare canonically)`,
      ),
  },
  {
    name: 'routing.network-unique',
    domains: ['routing'],
    validate: (config) => {
      const { bgp, rip } = config.routing;
      return [
        ...duplicateIssues(
          bgp?.networks ?? [],
          (n) => canonicalPrefix(n.prefix) ?? n.prefix,
          (_n, i) => ['routing', 'bgp', 'networks', i, 'prefix'],
          (n) => `network ${n.prefix} is listed more than once`,
        ),
        ...duplicateIssues(
          rip?.networks ?? [],
          (n) => canonicalPrefix(n) ?? n,
          (_n, i) => ['routing', 'rip', 'networks', i],
          (n) => `network ${n} is listed more than once`,
        ),
      ];
    },
  },
  {
    name: 'routing.bfd-session-unique',
    domains: ['routing'],
    validate: (config) =>
      duplicateIssues(
        config.routing.bfd?.sessions ?? [],
        (s) => `${s.interface} ${canonicalIp(s.peerAddress) ?? s.peerAddress}`,
        (_s, i) => ['routing', 'bfd', 'sessions', i, 'peerAddress'],
        (s) => `a BFD session to ${s.peerAddress} on ${s.interface} already exists`,
      ),
  },
  {
    name: 'routing.interface-exists',
    domains: ['routing', 'interfaces'],
    validate: (config) => {
      const names = interfaceNames(config.interfaces);
      const refs = new Refs('interface', (name) => names.has(name));
      for (const { entry, path } of routeMapEntries(config))
        refs.check(entry.match.interface, ...path, 'match', 'interface');
      for (const [path, peer] of bgpPeers(config)) {
        // updateSource is an address or an interface name; only names are checked
        const source = peer.updateSource;
        if (source !== undefined && ipFamily(source) === undefined)
          refs.check(source, ...path, 'updateSource');
      }
      for (const protocol of IGPS) {
        for (const name of Object.keys(config.routing[protocol]?.interfaces ?? {}))
          refs.check(name, 'routing', protocol, 'interfaces', name);
      }
      for (const [i, s] of (config.routing.bfd?.sessions ?? []).entries())
        refs.check(s.interface, 'routing', 'bfd', 'sessions', i, 'interface');
      return refs.issues;
    },
  },
  {
    name: 'routing.ospf-area-exists',
    domains: ['routing'],
    validate: (config) => {
      const ospf = config.routing.ospf;
      if (ospf === undefined) return [];
      const areas = new Set(Object.keys(ospf.areas).map(ospfAreaNumber));
      return Object.entries(ospf.interfaces)
        .filter(([, iface]) => !areas.has(ospfAreaNumber(iface.area)))
        .map(([name, iface]) => ({
          pointer: jsonPointer('routing', 'ospf', 'interfaces', name, 'area'),
          message: `OSPF area ${iface.area} is not defined under /routing/ospf/areas`,
        }));
    },
  },
];
