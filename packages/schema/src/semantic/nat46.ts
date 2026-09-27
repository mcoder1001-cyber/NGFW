import { NAT46_PREFIX } from '../domains/ext/nat46.js';
import { jsonPointer } from '../pointer.js';
import { interfaceExists } from './objects.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * F-nat46 rules (tier b), mirroring the agent's refusals (`descriptors/nat46.Validate`, `desired/nat46.go`) so the
 * API answers 400 with a pointer before a DryRun reaches the agent. Intra-`nat46` rules are refinements in
 * `domains/ext/nat46.ts`.
 *
 *   nat.nat46-interfaces      a NAT46 interface that does not exist
 *   nat.nat46-map-overlap     a NAT46 service address inside a `nat.map` domain's IPv4 prefix (both are MAP domains
 *                             in one VPP table: the longest match would silently steal traffic), or a MAP domain
 *                             named like a NAT46 domain (the reverse of `nat.det44-map-dslite-cnat-map-domain-name`)
 *   nat.nat46-map-interface   a NAT46 interface that `nat.map` binds in map-e (encapsulation) mode — one interface is
 *                             either translation or encapsulation; map-t is shared and fine
 */

function ipv4ToInt(a: string): number {
  return a.split('.').reduce((n, o) => n * 256 + Number(o), 0);
}

function ipv4PrefixContains(prefix: string, addr: string): boolean {
  const [net, len] = prefix.split('/');
  const bits = Number(len);
  if (net === undefined || !Number.isInteger(bits)) return false;
  const size = 2 ** (32 - bits);
  const lo = Math.floor(ipv4ToInt(net) / size) * size;
  const a = ipv4ToInt(addr);
  return a >= lo && a < lo + size;
}

export const nat46Validators: readonly ValidatorDefinition[] = [
  {
    name: 'nat.nat46-interfaces',
    domains: ['nat', 'interfaces'],
    validate: (config) =>
      (config.nat.nat46?.interfaces ?? []).flatMap((n, i) =>
        interfaceExists(config, n)
          ? []
          : [{ pointer: jsonPointer('nat', 'nat46', 'interfaces', i), message: `interface '${n}' does not exist` }],
      ),
  },
  {
    name: 'nat.nat46-map-overlap',
    domains: ['nat'],
    validate: ({ nat }) => {
      const issues: SemanticIssue[] = [];
      const mappings = nat.nat46?.mappings ?? [];
      mappings.forEach((m, i) => {
        const d = nat.map.domains.find((x) => ipv4PrefixContains(x.ipv4Prefix, m.ipv4));
        if (d !== undefined) {
          issues.push({
            pointer: jsonPointer('nat', 'nat46', 'mappings', i, 'ipv4'),
            message: `${m.ipv4} is inside MAP domain '${d.name}' (${d.ipv4Prefix}); NAT46 and MAP share VPP's domain table`,
          });
        }
        const clash = nat.map.domains.findIndex((x) => x.name === NAT46_PREFIX + m.name);
        if (clash >= 0) {
          issues.push({
            pointer: jsonPointer('nat', 'nat46', 'mappings', i, 'name'),
            message: `MAP domain '${NAT46_PREFIX + m.name}' would collide with this NAT46 mapping`,
          });
        }
      });
      return issues;
    },
  },
  {
    name: 'nat.nat46-map-interface',
    domains: ['nat'],
    validate: ({ nat }) =>
      (nat.nat46?.interfaces ?? []).flatMap((n, i) =>
        nat.map.interfaces.some((b) => b.interface === n && b.mode === 'map-e')
          ? [
              {
                pointer: jsonPointer('nat', 'nat46', 'interfaces', i),
                message: `interface '${n}' is a MAP-E interface in nat.map; NAT46 needs translation (map-t) there`,
              },
            ]
          : [],
      ),
  },
];
