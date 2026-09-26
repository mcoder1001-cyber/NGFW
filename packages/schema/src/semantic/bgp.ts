import { interfaceNames } from '../domains/interfaces.js';
import { lcpHostIfName } from '../domains/ext/frr-linuxcp.js';
import { ipFamily } from '../ip.js';
import { jsonPointer } from '../pointer.js';
import type { RootConfig } from '../index.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { duplicateIssues } from './unique.js';

/**
 * P12 rules (wave-A-hotspots C2; ids `routing.bgp-…`). They cover what P12 adds (`interfaces.<n>.lcp`,
 * `routing.static[].tag`) and what FRR needs from the rest of the document: every interface FRR must name (a BGP
 * update-source, a route-map `match interface`, the egress interface of a `viaFrr` route) needs a linux-cp pair, because
 * FRR sees only Linux interfaces. The reference rules that already exist are reused, not repeated
 * (`routing.interface-exists`, `routing.route-map-exists`, `routing.prefix-list-exists`, `routing.bgp-peer-group-exists`).
 */

/** The Linux name a linux-cp pair gets: `lcp.hostIfName`, else the VPP name itself. */
export function lcpHostName(vppName: string, hostIfName: string | undefined): string {
  return hostIfName ?? vppName;
}

/** Interfaces with a linux-cp pair: VPP name → Linux name. */
export function lcpInterfaces(config: RootConfig): Map<string, string> {
  const out = new Map<string, string>();
  for (const [name, iface] of Object.entries(config.interfaces)) {
    if (iface.lcp !== undefined) out.set(name, lcpHostName(name, iface.lcp.hostIfName));
  }
  return out;
}

/** FRR's description rule (the agent's `frr.Description`): what a LINE token in frr.conf can carry and frr-reload.py can
 * compare. '' when `d` is fine, else the reason. Review M2: the schema and the agent must agree. */
export function frrDescriptionProblem(d: string): string {
  if (d.length > 80) return 'at most 80 characters (FRR)';
  if (!/^[\x20-\x7e]*$/.test(d)) return 'printable ASCII only: FRR stores it verbatim in frr.conf';
  if (d.trim() !== d) return 'no leading or trailing blanks';
  if (d.startsWith('!') || d.startsWith('#'))
    return 'must not start with ! or # (FRR reads a comment)';
  if (d.includes('|')) return "no '|' (FRR's CLI pipe cuts the line there)";
  if (d.includes('  ')) return 'no double blanks (FRR joins the words with one)';
  return '';
}

/** Every description P12 renders into FRR, with its path. Interface descriptions are not rendered (not FRR's). */
function frrDescriptions(config: RootConfig): [string, (string | number)[]][] {
  const out: [string, (string | number)[]][] = [];
  const bgp = config.routing.bgp;
  for (const [name, g] of Object.entries(bgp?.peerGroups ?? {}))
    if (g.description !== undefined)
      out.push([g.description, ['routing', 'bgp', 'peerGroups', name, 'description']]);
  for (const [addr, n] of Object.entries(bgp?.neighbors ?? {}))
    if (n.description !== undefined)
      out.push([n.description, ['routing', 'bgp', 'neighbors', addr, 'description']]);
  for (const [name, pl] of Object.entries(config.routing.policy.prefixLists))
    if (pl.description !== undefined)
      out.push([pl.description, ['routing', 'policy', 'prefixLists', name, 'description']]);
  for (const [name, rm] of Object.entries(config.routing.policy.routeMaps))
    for (const [i, e] of rm.entries.entries())
      if (e.description !== undefined)
        out.push([
          e.description,
          ['routing', 'policy', 'routeMaps', name, 'entries', i, 'description'],
        ]);
  return out;
}

export const bgpValidators: readonly ValidatorDefinition[] = [
  {
    // review M2: descriptions FRR renders follow FRR's rule (the agent refuses the rest at render time)
    name: 'routing.bgp-frr-description',
    domains: ['routing'],
    validate: (config) =>
      frrDescriptions(config).flatMap(([d, path]) => {
        const why = frrDescriptionProblem(d);
        return why === ''
          ? []
          : [{ pointer: jsonPointer(...path), message: `FRR description: ${why}` }];
      }),
  },
  {
    // review M2: FRR's route-map sequence numbers are 1–65535 (the schema's seq is a uint32 shared with prefix lists)
    name: 'routing.bgp-route-map-seq',
    domains: ['routing'],
    validate: (config) =>
      Object.entries(config.routing.policy.routeMaps).flatMap(([name, rm]) =>
        rm.entries.flatMap((e, i) =>
          e.seq > 65535
            ? [
                {
                  pointer: jsonPointer('routing', 'policy', 'routeMaps', name, 'entries', i, 'seq'),
                  message: 'FRR route-map sequence numbers are 1–65535',
                },
              ]
            : [],
        ),
      ),
  },
  {
    // without hostIfName the VPP name becomes the Linux name, which must then be a valid Linux interface name
    name: 'routing.bgp-lcp-host-name',
    domains: ['routing', 'interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const [name, iface] of Object.entries(config.interfaces)) {
        if (iface.lcp === undefined || iface.lcp.hostIfName !== undefined) continue;
        if (!lcpHostIfName.safeParse(name).success) {
          issues.push({
            pointer: jsonPointer('interfaces', name, 'lcp', 'hostIfName'),
            message: `'${name}' is not a valid Linux interface name (max 15 of letters, digits, _ . -): set hostIfName`,
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'routing.bgp-lcp-host-name-unique',
    domains: ['routing', 'interfaces'],
    validate: (config) =>
      duplicateIssues(
        [...lcpInterfaces(config).entries()],
        ([, host]) => host,
        ([name]) => ['interfaces', name, 'lcp', 'hostIfName'],
        ([, host]) => `Linux interface name '${host}' is used by two linux-cp pairs`,
      ),
  },
  {
    // FRR names interfaces by their Linux name: an interface FRR must use needs a linux-cp pair
    name: 'routing.bgp-interface-has-lcp',
    domains: ['routing', 'interfaces'],
    validate: (config) => {
      const names = interfaceNames(config.interfaces);
      const paired = lcpInterfaces(config);
      const issues: SemanticIssue[] = [];
      const need = (name: string | undefined, what: string, ...path: (string | number)[]) => {
        if (name === undefined || !names.has(name) || paired.has(name)) return; // unknown names: routing.interface-exists
        issues.push({
          pointer: jsonPointer(...path),
          message: `${what} '${name}' has no Linux pair (interfaces.${name}.lcp): FRR sees only Linux interfaces`,
        });
      };
      const bgp = config.routing.bgp;
      if (bgp !== undefined) {
        const peers: [string, string, { updateSource?: string | undefined }][] = [
          ...Object.entries(bgp.peerGroups).map(
            ([k, p]) => ['peerGroups', k, p] as [string, string, typeof p],
          ),
          ...Object.entries(bgp.neighbors).map(
            ([k, p]) => ['neighbors', k, p] as [string, string, typeof p],
          ),
        ];
        for (const [kind, key, peer] of peers) {
          const src = peer.updateSource;
          if (src !== undefined && ipFamily(src) === undefined)
            need(src, 'update-source interface', 'routing', 'bgp', kind, key, 'updateSource');
        }
      }
      for (const [name, map] of Object.entries(config.routing.policy.routeMaps)) {
        for (const [i, entry] of map.entries.entries())
          need(
            entry.match.interface,
            'match interface',
            'routing',
            'policy',
            'routeMaps',
            name,
            'entries',
            i,
            'match',
            'interface',
          );
      }
      for (const [i, route] of config.routing.static.entries()) {
        if (route.viaFrr !== true) continue;
        for (const [j, hop] of route.nextHops.entries())
          need(
            hop.interface,
            'egress interface of an FRR route',
            'routing',
            'static',
            i,
            'nextHops',
            j,
            'interface',
          );
      }
      return issues;
    },
  },
  {
    // VPP has no route tag: a tag means something only on a route FRR programs (D-072 viaFrr)
    name: 'routing.bgp-static-tag-via-frr',
    domains: ['routing'],
    validate: (config) =>
      config.routing.static.flatMap((route, i) =>
        route.tag !== undefined && route.viaFrr !== true
          ? [
              {
                pointer: jsonPointer('routing', 'static', i, 'tag'),
                message:
                  'a route tag is carried only by routes programmed via FRR (set viaFrr); VPP has no route tag',
              },
            ]
          : [],
      ),
  },
];
