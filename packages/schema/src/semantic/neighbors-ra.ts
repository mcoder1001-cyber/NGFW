import { RA_DEFAULTS, type Ipv6RaConfig } from '../domains/ext/neighbors-ra.js';
import {
  interfaceNames,
  type InterfaceConfig,
  type SubinterfaceConfig,
} from '../domains/interfaces.js';
import { ipKey, parseCidr, parseIpv4, parseIpv6, prefixContains, type IpPrefix } from '../ip.js';
import { jsonPointer } from '../pointer.js';
import type { RootConfig } from '../index.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * Semantic rules of F-neighbors-ra (rule ids `interfaces.neighbors-ra-…`, `vrfs.neighbors-ra-…`, `routing.neighbors-ra-…`):
 * RA and proxy-ND need IPv6 on the interface, SLAAC prefixes are /64, proxy-ND addresses and proxy-ARP ranges are unique,
 * a static neighbour names a configured interface and lies in one of its connected subnets. Single-object rules (RA
 * interval ordering, proxy-ARP range order, lifetimes) are refinements in `../domains/ext/neighbors-ra.ts`.
 */

/** One (sub-)interface with the pointer segments that lead to it. */
interface Node {
  name: string;
  path: readonly (string | number)[];
  value: InterfaceConfig | SubinterfaceConfig;
}

function nodes(config: RootConfig): Node[] {
  const out: Node[] = [];
  for (const [parent, iface] of Object.entries(config.interfaces)) {
    out.push({ name: parent, path: ['interfaces', parent], value: iface });
    for (const [id, sub] of Object.entries(iface.subinterfaces)) {
      out.push({
        name: `${parent}.${id}`,
        path: ['interfaces', parent, 'subinterfaces', id],
        value: sub,
      });
    }
  }
  return out;
}

/**
 * True when `ra` changes nothing on VPP: every field at its default and no prefix. Such an object is "off" (the
 * interface drawer writes defaults back) and needs no IPv6 on the interface.
 */
export function isDefaultIpv6Ra(ra: Ipv6RaConfig | undefined): boolean {
  if (ra === undefined) return true;
  return (
    ra.suppress === RA_DEFAULTS.suppress &&
    ra.managed === RA_DEFAULTS.managed &&
    ra.other === RA_DEFAULTS.other &&
    ra.lifetimeSec === RA_DEFAULTS.lifetimeSec &&
    ra.maxIntervalSec === RA_DEFAULTS.maxIntervalSec &&
    ra.minIntervalSec === RA_DEFAULTS.minIntervalSec &&
    Object.keys(ra.prefixes).length === 0
  );
}

/** Connected IPv4/IPv6 prefixes of a node: its own addresses, or those of its `unnumbered` source. */
function connectedPrefixes(node: Node, byName: Map<string, Node>): IpPrefix[] {
  const source = node.value.unnumbered !== undefined ? byName.get(node.value.unnumbered) : node;
  if (source === undefined) return [];
  const out: IpPrefix[] = [];
  for (const text of [...source.value.ipv4, ...source.value.ipv6]) {
    const p = parseCidr(text);
    if (p !== undefined) out.push(p);
  }
  return out;
}

const LINK_LOCAL: IpPrefix = { family: 6, address: 0xfe80n << 112n, length: 10 };

export const neighborsRaValidators: readonly ValidatorDefinition[] = [
  {
    name: 'interfaces.neighbors-ra-ipv6-required',
    domains: ['interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const node of nodes(config)) {
        if (node.value.ipv6.length > 0) continue;
        if (!isDefaultIpv6Ra(node.value.ipv6Ra)) {
          issues.push({
            pointer: jsonPointer(...node.path, 'ipv6Ra'),
            message: `router advertisements need IPv6 on ${node.name}: add an IPv6 address to the interface`,
          });
        }
        if ((node.value.proxyNd ?? []).length > 0) {
          issues.push({
            pointer: jsonPointer(...node.path, 'proxyNd'),
            message: `proxy ND needs IPv6 on ${node.name}: add an IPv6 address to the interface`,
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'interfaces.neighbors-ra-slaac-prefix-length',
    domains: ['interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const node of nodes(config)) {
        for (const [prefix, opts] of Object.entries(node.value.ipv6Ra?.prefixes ?? {})) {
          const p = parseCidr(prefix);
          if (p === undefined || opts.noAutoconfig || p.length === 64) continue;
          issues.push({
            pointer: jsonPointer(...node.path, 'ipv6Ra', 'prefixes', prefix),
            message: `${prefix} is a /${p.length}: SLAAC (autonomous flag) needs a /64 — use a /64 or set noAutoconfig`,
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'interfaces.neighbors-ra-proxy-nd-unique',
    domains: ['interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const node of nodes(config)) {
        const seen = new Set<string>();
        for (const [i, addr] of (node.value.proxyNd ?? []).entries()) {
          const k = ipKey(addr);
          if (seen.has(k)) {
            issues.push({
              pointer: jsonPointer(...node.path, 'proxyNd', i),
              message: `proxy ND address ${addr} is listed twice on ${node.name}`,
            });
          }
          seen.add(k);
        }
      }
      return issues;
    },
  },
  {
    name: 'vrfs.neighbors-ra-proxy-arp-range-unique',
    domains: ['vrfs'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const [name, vrf] of Object.entries(config.vrfs)) {
        const seen = new Map<string, number>();
        for (const [i, r] of (vrf.proxyArpRanges ?? []).entries()) {
          const k = `${parseIpv4(r.low) ?? r.low}-${parseIpv4(r.high) ?? r.high}`;
          const first = seen.get(k);
          if (first !== undefined) {
            issues.push({
              pointer: jsonPointer('vrfs', name, 'proxyArpRanges', i),
              message: `proxy-ARP range ${r.low}–${r.high} is already listed at index ${first} of VRF '${name}'`,
            });
          } else seen.set(k, i);
        }
      }
      return issues;
    },
  },
  {
    name: 'routing.neighbors-ra-static-interface-exists',
    domains: ['routing', 'interfaces'],
    validate: (config) => {
      const names = interfaceNames(config.interfaces);
      return (config.routing.neighbors?.static ?? []).flatMap((n, i) =>
        names.has(n.interface)
          ? []
          : [
              {
                pointer: jsonPointer('routing', 'neighbors', 'static', i, 'interface'),
                message: `interface '${n.interface}' does not exist`,
              },
            ],
      );
    },
  },
  {
    name: 'routing.neighbors-ra-static-connected',
    domains: ['routing', 'interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const byName = new Map(nodes(config).map((n) => [n.name, n]));
      for (const [i, n] of (config.routing.neighbors?.static ?? []).entries()) {
        const node = byName.get(n.interface);
        if (node === undefined) continue; // routing.neighbors-ra-static-interface-exists reports it
        const v4 = parseIpv4(n.ip);
        const family = v4 !== undefined ? 4 : 6;
        const address = v4 ?? parseIpv6(n.ip);
        if (address === undefined) continue; // cannot happen after schema validation
        const connected = connectedPrefixes(node, byName);
        const pointer = jsonPointer('routing', 'neighbors', 'static', i, 'ip');
        if (connected.some((p) => p.family === family && p.address === address)) {
          issues.push({
            pointer,
            message: `${n.ip} is an address of ${n.interface} itself, not a neighbour`,
          });
          continue;
        }
        const linkLocal =
          family === 6 &&
          prefixContains(LINK_LOCAL, 6, address) &&
          connected.some((p) => p.family === 6);
        if (!linkLocal && !connected.some((p) => prefixContains(p, family, address))) {
          const hasFamily = connected.some((p) => p.family === family);
          issues.push({
            pointer,
            message: hasFamily
              ? `${n.ip} is outside every connected subnet of ${n.interface}`
              : `${n.ip} is outside every connected subnet of ${n.interface}: it has no IPv${family} address`,
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'routing.neighbors-ra-static-unique',
    domains: ['routing'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const seen = new Map<string, number>();
      for (const [i, n] of (config.routing.neighbors?.static ?? []).entries()) {
        const k = `${n.interface}|${ipKey(n.ip)}`;
        const first = seen.get(k);
        if (first !== undefined) {
          issues.push({
            pointer: jsonPointer('routing', 'neighbors', 'static', i, 'ip'),
            message: `static neighbour ${n.ip} on ${n.interface} is already defined at index ${first}`,
          });
        } else seen.set(k, i);
      }
      return issues;
    },
  },
];
