import { interfaceNames } from '../domains/interfaces.js';
import {
  ospfAreaNumber,
  type BgpNeighborConfig,
  type BgpPeerGroupConfig,
} from '../domains/routing.js';
import { vrfExists } from '../domains/vrfs.js';
import { ipFamily } from '../ip.js';
import { jsonPointer } from '../pointer.js';
import type { RootConfig } from '../index.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `routing`: VRFs, interfaces, prefix-lists, route-maps, peer groups and OSPF areas that
 * are referenced must exist; static routes are unique per (vrf, prefix). Single-object consistency (address
 * families, unique sequence numbers, hold > keepalive) is enforced by `.refine()` in `domains/routing.ts`.
 */

type Path = readonly (string | number)[];
type Protocol = 'bgp' | 'ospf' | 'isis' | 'rip';
const PROTOCOLS: readonly Protocol[] = ['bgp', 'ospf', 'isis', 'rip'];

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

/** Every `redistribute[].routeMap` of every enabled protocol. */
function checkRedistributeRouteMaps(config: RootConfig, refs: Refs): void {
  for (const protocol of PROTOCOLS) {
    const instance = config.routing[protocol];
    if (instance === undefined) continue;
    for (const [i, r] of instance.redistribute.entries()) {
      refs.check(r.routeMap, 'routing', protocol, 'redistribute', i, 'routeMap');
    }
  }
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
    name: 'routing.static-unique',
    domains: ['routing'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const seen = new Map<string, number>();
      for (const [i, route] of config.routing.static.entries()) {
        const key = `${route.vrf} ${route.prefix.toLowerCase()}`;
        const first = seen.get(key);
        if (first === undefined) seen.set(key, i);
        else {
          issues.push({
            pointer: jsonPointer('routing', 'static', i, 'prefix'),
            message: `route ${route.prefix} in VRF '${route.vrf}' is already defined at ${jsonPointer('routing', 'static', first)}; add next hops there instead`,
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'routing.prefix-list-exists',
    domains: ['routing'],
    validate: (config) => {
      const { prefixLists, routeMaps } = config.routing;
      const refs = new Refs('prefix list', (name) => Object.hasOwn(prefixLists, name));
      for (const [name, map] of Object.entries(routeMaps)) {
        for (const [i, entry] of map.entries.entries()) {
          refs.check(
            entry.match.prefixList,
            'routing',
            'routeMaps',
            name,
            'entries',
            i,
            'match',
            'prefixList',
          );
          refs.check(
            entry.match.nextHopPrefixList,
            'routing',
            'routeMaps',
            name,
            'entries',
            i,
            'match',
            'nextHopPrefixList',
          );
        }
      }
      for (const [path, peer] of bgpPeers(config)) {
        for (const af of ['ipv4Unicast', 'ipv6Unicast'] as const) {
          refs.check(peer[af]?.prefixListIn, ...path, af, 'prefixListIn');
          refs.check(peer[af]?.prefixListOut, ...path, af, 'prefixListOut');
        }
      }
      return refs.issues;
    },
  },
  {
    name: 'routing.route-map-exists',
    domains: ['routing'],
    validate: (config) => {
      const { routeMaps, bgp } = config.routing;
      const refs = new Refs('route map', (name) => Object.hasOwn(routeMaps, name));
      for (const [path, peer] of bgpPeers(config)) {
        for (const af of ['ipv4Unicast', 'ipv6Unicast'] as const) {
          refs.check(peer[af]?.routeMapIn, ...path, af, 'routeMapIn');
          refs.check(peer[af]?.routeMapOut, ...path, af, 'routeMapOut');
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
      for (const [i, n] of bgp.neighbors.entries())
        refs.check(n.peerGroup, 'routing', 'bgp', 'neighbors', i, 'peerGroup');
      return refs.issues;
    },
  },
  {
    name: 'routing.interface-exists',
    domains: ['routing', 'interfaces'],
    validate: (config) => {
      const names = interfaceNames(config.interfaces);
      const refs = new Refs('interface', (name) => names.has(name));
      const { routeMaps, ospf, isis, rip, bfd } = config.routing;
      for (const [name, map] of Object.entries(routeMaps)) {
        for (const [i, entry] of map.entries.entries()) {
          refs.check(
            entry.match.interface,
            'routing',
            'routeMaps',
            name,
            'entries',
            i,
            'match',
            'interface',
          );
        }
      }
      for (const [path, peer] of bgpPeers(config)) {
        // updateSource is an address or an interface name; only names are checked
        const source = peer.updateSource;
        if (source !== undefined && ipFamily(source) === undefined)
          refs.check(source, ...path, 'updateSource');
      }
      for (const [i, iface] of (ospf?.interfaces ?? []).entries())
        refs.check(iface.name, 'routing', 'ospf', 'interfaces', i, 'name');
      for (const [i, iface] of (isis?.interfaces ?? []).entries())
        refs.check(iface.name, 'routing', 'isis', 'interfaces', i, 'name');
      for (const [i, iface] of (rip?.interfaces ?? []).entries())
        refs.check(iface.name, 'routing', 'rip', 'interfaces', i, 'name');
      for (const [i, s] of (bfd?.sessions ?? []).entries())
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
      const areas = new Set(ospf.areas.map((a) => ospfAreaNumber(a.id)));
      return ospf.interfaces
        .map((iface, i) => ({ iface, i }))
        .filter(({ iface }) => !areas.has(ospfAreaNumber(iface.area)))
        .map(({ iface, i }) => ({
          pointer: jsonPointer('routing', 'ospf', 'interfaces', i, 'area'),
          message: `OSPF area ${iface.area} is not defined under /routing/ospf/areas`,
        }));
    },
  },
];

type BgpPeer = BgpNeighborConfig | BgpPeerGroupConfig;

/** Every BGP neighbour and peer group with the pointer segments that lead to it. */
function bgpPeers(config: RootConfig): [Path, BgpPeer][] {
  const bgp = config.routing.bgp;
  if (bgp === undefined) return [];
  const out: [Path, BgpPeer][] = [];
  for (const [name, group] of Object.entries(bgp.peerGroups))
    out.push([['routing', 'bgp', 'peerGroups', name], group]);
  for (const [i, neighbor] of bgp.neighbors.entries())
    out.push([['routing', 'bgp', 'neighbors', i], neighbor]);
  return out;
}
