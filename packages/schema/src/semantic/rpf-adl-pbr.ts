import { interfaceNames } from '../domains/interfaces.js';
import { vrfExists } from '../domains/vrfs.js';
import type { PbrConfig, PbrFamily } from '../domains/ext/rpf-adl-pbr.js';
import { ipFamily } from '../ip.js';
import { jsonPointer } from '../pointer.js';
import type { RootConfig } from '../index.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { duplicateIssues } from './unique.js';

/**
 * Semantic validators of F-rpf-adl-pbr (uRPF, ADL, ABF policy-based routing, Auto-SDL). Every reference must
 * resolve — a PBR policy's ACL (`acl.lists`), path VRFs and interfaces, an attachment's policy and interface, the
 * ADL allow-list VRF — attachments are unique per (policy, interface, family), and an attachment's family must
 * match the next hops of its policy. Rule names carry the owning domain and the task slug (wave-A-hotspots rule 5).
 *
 * Not here: "attachment interface is not bridged" needs F-bridge-l2's `interfaces.<if>.l2` (not on main yet,
 * F-rpf-adl-pbr-questions); "strict uRPF on an ECMP egress" is a warning, which tier (b) cannot express — the
 * agent reports it (DryRun, `interfaces.rpf-adl-pbr-urpf-strict-ecmp`).
 */

type Path = readonly (string | number)[];

const pbrOf = (config: RootConfig): PbrConfig | undefined => config.routing.pbr;

const FAMILY_NAME: Record<PbrFamily, string> = { ipv4: 'IPv4', ipv6: 'IPv6' };

/** The address family of a policy's next-hop addresses; undefined when it has none, 'mixed' when both occur. */
function pathFamily(config: RootConfig, policy: string): PbrFamily | 'mixed' | undefined {
  const paths = pbrOf(config)?.policies[policy]?.paths ?? [];
  const families = new Set<PbrFamily>();
  for (const p of paths) {
    if (p.address !== undefined) families.add(ipFamily(p.address) === 6 ? 'ipv6' : 'ipv4');
  }
  if (families.size > 1) return 'mixed';
  return [...families][0];
}

function missing(kind: string, name: string, path: Path): SemanticIssue {
  return { pointer: jsonPointer(...path), message: `${kind} '${name}' does not exist` };
}

export const rpfAdlPbrValidators: readonly ValidatorDefinition[] = [
  {
    name: 'routing.rpf-adl-pbr-acl-exists',
    domains: ['routing', 'acl'],
    validate: (config) =>
      Object.entries(pbrOf(config)?.policies ?? {})
        .filter(([, p]) => !Object.hasOwn(config.acl.lists, p.acl))
        .map(([name, p]) => missing('ACL', p.acl, ['routing', 'pbr', 'policies', name, 'acl'])),
  },
  {
    name: 'routing.rpf-adl-pbr-path-refs',
    domains: ['routing', 'interfaces', 'vrfs'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const ifs = interfaceNames(config.interfaces);
      for (const [name, policy] of Object.entries(pbrOf(config)?.policies ?? {})) {
        for (const [i, p] of policy.paths.entries()) {
          const at = ['routing', 'pbr', 'policies', name, 'paths', i];
          if (!vrfExists(config.vrfs, p.vrf)) issues.push(missing('VRF', p.vrf, [...at, 'vrf']));
          if (p.interface !== undefined && !ifs.has(p.interface)) {
            issues.push(missing('interface', p.interface, [...at, 'interface']));
          }
        }
      }
      return issues;
    },
  },
  {
    name: 'routing.rpf-adl-pbr-path-family',
    domains: ['routing'],
    validate: (config) =>
      Object.keys(pbrOf(config)?.policies ?? {})
        .filter((name) => pathFamily(config, name) === 'mixed')
        .map((name) => ({
          pointer: jsonPointer('routing', 'pbr', 'policies', name, 'paths'),
          message: `policy '${name}' mixes IPv4 and IPv6 next hops; use one policy per address family`,
        })),
  },
  {
    name: 'routing.rpf-adl-pbr-attachment-refs',
    domains: ['routing', 'interfaces'],
    validate: (config) => {
      const pbr = pbrOf(config);
      if (pbr === undefined) return [];
      const issues: SemanticIssue[] = [];
      const ifs = interfaceNames(config.interfaces);
      for (const [i, a] of pbr.attachments.entries()) {
        const at = ['routing', 'pbr', 'attachments', i];
        if (!Object.hasOwn(pbr.policies, a.policy)) {
          issues.push(missing('PBR policy', a.policy, [...at, 'policy']));
          continue;
        }
        if (!ifs.has(a.interface))
          issues.push(missing('interface', a.interface, [...at, 'interface']));
        const fam = pathFamily(config, a.policy);
        if (fam !== undefined && fam !== 'mixed' && fam !== a.family) {
          issues.push({
            pointer: jsonPointer(...at, 'family'),
            message: `policy '${a.policy}' forwards to ${FAMILY_NAME[fam]} next hops; it cannot be attached for ${FAMILY_NAME[a.family]}`,
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'routing.rpf-adl-pbr-attachment-unique',
    domains: ['routing'],
    validate: (config) =>
      duplicateIssues(
        pbrOf(config)?.attachments ?? [],
        (a) => `${a.policy}\u0000${a.interface}\u0000${a.family}`,
        (_, i) => ['routing', 'pbr', 'attachments', i],
        (a) => `policy '${a.policy}' is attached to ${a.interface} (${a.family}) twice`,
      ),
  },
  {
    name: 'interfaces.rpf-adl-pbr-adl-vrf-exists',
    domains: ['interfaces', 'vrfs'],
    validate: (config) =>
      Object.entries(config.interfaces)
        .filter(
          ([, itf]) => itf.adl?.allowVrf !== undefined && !vrfExists(config.vrfs, itf.adl.allowVrf),
        )
        .map(([name, itf]) =>
          missing('VRF', itf.adl?.allowVrf ?? '', ['interfaces', name, 'adl', 'allowVrf']),
        ),
  },
];
