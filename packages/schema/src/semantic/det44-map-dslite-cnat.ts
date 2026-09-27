import { isNat44Enabled } from '../domains/nat.js';
import { jsonPointer } from '../pointer.js';
import { interfaceExists } from './objects.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * F-det44-map-dslite-cnat rules (tier b), mirroring the agent's refusals in `apps/agent/internal/desired/{cnat,map}.go`
 * so the API answers 400 with a pointer before a DryRun ever reaches the agent:
 *
 *   nat.det44-map-dslite-cnat-cnat-snat-address    an SNAT policy, policy interfaces or excluded prefixes without SNAT
 *                                                  addresses (the default SNAT entry — V10: VPP would crash / never apply)
 *   nat.det44-map-dslite-cnat-cnat-nat44-interface a CNAT SNAT policy interface that is also a NAT44-ED interface
 *                                                  (no supported feature ordering in VPP 26.06; questions Q2)
 *   nat.det44-map-dslite-cnat-map-domain-name      a MAP domain name starting with `nat46-` (reserved for NAT46's
 *                                                  domains; an empty name is already a schema error)
 *   nat.det44-map-dslite-cnat-lw4o6-rules          an lw4o6 domain without per-PSID rules (it would read back as MAP-E)
 *   nat.det44-map-dslite-cnat-pnat-interfaces      a PNAT attachment on an interface that does not exist
 */

/** Domain-name prefix the agent reserves for NAT46 (`descriptors/nat46.DomainPrefix`). */
export const NAT46_DOMAIN_PREFIX = 'nat46-';

export const det44MapDsliteCnatValidators: readonly ValidatorDefinition[] = [
  {
    name: 'nat.det44-map-dslite-cnat-cnat-snat-address',
    domains: ['nat'],
    validate: ({ nat: { cnat } }) => {
      const { snat } = cnat;
      const a = snat.addresses;
      const hasAddr = a.ipv4 !== undefined || a.ipv6 !== undefined || a.interface !== undefined;
      const needs =
        snat.policy !== 'none' || snat.interfaces.length > 0 || snat.excludePrefixes.length > 0;
      if (!needs || hasAddr) return [];
      return [
        {
          pointer: jsonPointer('nat', 'cnat', 'snat', 'addresses'),
          message:
            'an SNAT policy, policy interfaces or excluded prefixes need SNAT addresses (ipv4/ipv6 or an interface): VPP needs the default SNAT entry first',
        },
      ];
    },
  },
  {
    name: 'nat.det44-map-dslite-cnat-cnat-nat44-interface',
    domains: ['nat'],
    validate: ({ nat }) => {
      if (!isNat44Enabled(nat) || nat.mode !== 'ed') return [];
      const ed = new Set([...nat.inside, ...nat.outside, ...nat.outputFeature]);
      const issues: SemanticIssue[] = [];
      nat.cnat.snat.interfaces.forEach((b, i) => {
        if (ed.has(b.interface)) {
          issues.push({
            pointer: jsonPointer('nat', 'cnat', 'snat', 'interfaces', i, 'interface'),
            message: `interface '${b.interface}' is also a NAT44-ED interface: CNAT and NAT44-ED on one interface are not supported`,
          });
        }
      });
      return issues;
    },
  },
  {
    name: 'nat.det44-map-dslite-cnat-map-domain-name',
    domains: ['nat'],
    validate: ({ nat }) =>
      nat.map.domains.flatMap((d, i) =>
        d.name.startsWith(NAT46_DOMAIN_PREFIX)
          ? [
              {
                pointer: jsonPointer('nat', 'map', 'domains', i, 'name'),
                message: `domain names starting with '${NAT46_DOMAIN_PREFIX}' are reserved for NAT46`,
              },
            ]
          : [],
      ),
  },
  {
    name: 'nat.det44-map-dslite-cnat-lw4o6-rules',
    domains: ['nat'],
    validate: ({ nat }) =>
      nat.map.domains.flatMap((d, i) =>
        d.mode === 'lw4o6' && d.rules.length === 0
          ? [
              {
                pointer: jsonPointer('nat', 'map', 'domains', i, 'rules'),
                message: 'an lw4o6 domain needs at least one per-PSID rule',
              },
            ]
          : [],
      ),
  },
  {
    name: 'nat.det44-map-dslite-cnat-pnat-interfaces',
    domains: ['nat', 'interfaces'],
    validate: (config) =>
      (config.nat.pnat?.attachments ?? []).flatMap((a, i) =>
        interfaceExists(config, a.interface)
          ? []
          : [
              {
                pointer: jsonPointer('nat', 'pnat', 'attachments', i, 'interface'),
                message: `interface '${a.interface}' does not exist`,
              },
            ],
      ),
  },
];
