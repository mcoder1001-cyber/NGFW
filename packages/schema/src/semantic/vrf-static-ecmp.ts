import { interfaceNames } from '../domains/interfaces.js';
import { vrfExists } from '../domains/vrfs.js';
import { prefixKey } from '../ip.js';
import { jsonPointer } from '../pointer.js';
import type { RootConfig } from '../index.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { duplicateIssues } from './unique.js';

/**
 * F-vrf-static-ecmp rules (wave-A-hotspots C2; ids `vrfs.vrf-static-ecmp-…` / `routing.vrf-static-ecmp-…`). They cover the
 * fields this feature adds (`vrfs.<name>.sourceSelect`, `routing.static[].nextHops[].vrf`) and the ECMP weight. The rules
 * that already exist are reused, not repeated: `vrfs.id-unique`, `vrfs.default-is-table-zero`, `routing.vrf-exists`,
 * `routing.static-nexthop-interface-exists`, `routing.static-unique`.
 */

/** Every `sourceSelect` entry of every VRF with its pointer segments. */
function sourceSelects(config: RootConfig) {
  return Object.entries(config.vrfs).flatMap(([vrf, v]) =>
    (v.sourceSelect ?? []).map((entry, k) => ({
      vrf,
      entry,
      path: ['vrfs', vrf, 'sourceSelect', k] as const,
    })),
  );
}

export const vrfStaticEcmpValidators: readonly ValidatorDefinition[] = [
  {
    // the next hop is resolved in another VRF (VPP path table_id): that VRF must exist, differ from the route's own VRF
    // (canonical form: omit it for the same VRF) and the hop must be given by address only
    name: 'routing.vrf-static-ecmp-nexthop-vrf',
    domains: ['routing', 'vrfs'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const [i, route] of config.routing.static.entries()) {
        for (const [j, hop] of route.nextHops.entries()) {
          if (hop.vrf === undefined) continue;
          const pointer = jsonPointer('routing', 'static', i, 'nextHops', j, 'vrf');
          if (!vrfExists(config.vrfs, hop.vrf)) {
            issues.push({ pointer, message: `VRF '${hop.vrf}' does not exist` });
          } else if (hop.vrf === route.vrf) {
            issues.push({
              pointer,
              message: `'${hop.vrf}' is the route's own VRF; omit the next-hop VRF`,
            });
          } else if (hop.interface !== undefined) {
            issues.push({
              pointer,
              message:
                'a next hop with an egress interface is resolved on that interface; a next-hop VRF applies only to a next hop given by address',
            });
          }
        }
      }
      return issues;
    },
  },
  {
    // weights split traffic between the paths of one route; on a single path they do nothing (ECMP needs ≥ 2 paths)
    name: 'routing.vrf-static-ecmp-single-path-weight',
    domains: ['routing'],
    validate: (config) =>
      config.routing.static.flatMap((route, i) =>
        route.nextHops.length === 1 && route.nextHops[0]!.weight !== 1
          ? [
              {
                pointer: jsonPointer('routing', 'static', i, 'nextHops', 0, 'weight'),
                message: `weight ${route.nextHops[0]!.weight} has no effect on a route with one next hop; weights split traffic between two or more next hops (ECMP)`,
              },
            ]
          : [],
      ),
  },
  {
    name: 'vrfs.vrf-static-ecmp-source-select-interface-exists',
    domains: ['vrfs', 'interfaces'],
    validate: (config) => {
      const names = interfaceNames(config.interfaces);
      return sourceSelects(config)
        .filter(({ entry }) => !names.has(entry.interface))
        .map(({ entry, path }) => ({
          pointer: jsonPointer(...path, 'interface'),
          message: `interface '${entry.interface}' does not exist`,
        }));
    },
  },
  {
    // one (ingress interface, source prefix) selects exactly one VRF: VPP keeps one source-VRF-select table per interface
    name: 'vrfs.vrf-static-ecmp-source-select-unique',
    domains: ['vrfs'],
    validate: (config) =>
      duplicateIssues(
        sourceSelects(config),
        ({ entry }) => `${entry.interface} ${prefixKey(entry.prefix)}`,
        ({ path }) => [...path, 'prefix'],
        ({ entry }) =>
          `source prefix ${entry.prefix} on interface '${entry.interface}' already selects a VRF`,
      ),
  },
];
