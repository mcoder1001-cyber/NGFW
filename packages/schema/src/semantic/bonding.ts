import type { InterfaceConfig } from '../domains/interfaces.js';
import {
  BOND_HASH_MODES,
  bondIdOf,
  NON_ETHERNET_INTERFACE_RE,
  type BondConfig,
  type BondMemberConfig,
} from '../domains/ext/bonding.js';
import { DEFAULT_VRF } from '../domains/vrfs.js';
import { jsonPointer } from '../pointer.js';
import type { RootConfig } from '../index.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * Semantic validators of `interfaces.<name>.bond` (F-bonding). Rule names `interfaces.bonding-*`; pointers are built with
 * `jsonPointer()` because interface names contain `/`.
 */

interface BondEntry {
  name: string;
  id: number;
  bond: BondConfig;
}

/** Every configured bond, ordered by bond id then name — "the second membership" of a member is well defined. */
function bonds(config: RootConfig): BondEntry[] {
  const out: BondEntry[] = [];
  for (const [name, itf] of Object.entries(config.interfaces)) {
    if (itf.bond === undefined) continue;
    out.push({
      name,
      id: itf.bond.id ?? bondIdOf(name) ?? Number.MAX_SAFE_INTEGER,
      bond: itf.bond,
    });
  }
  return out.sort((a, b) => a.id - b.id || a.name.localeCompare(b.name));
}

function memberPtr(bond: string, member: string, ...rest: string[]): string {
  return jsonPointer('interfaces', bond, 'bond', 'members', member, ...rest);
}

/** Is this interface (by name and configuration) a bond, a loopback or another non-Ethernet interface — never a member? */
function notMemberKind(name: string, itf: InterfaceConfig | undefined): string | undefined {
  if (itf?.bond !== undefined || bondIdOf(name) !== undefined) return 'a bond';
  if (/^loop[0-9]+$/.test(name)) return 'a loopback';
  if (NON_ETHERNET_INTERFACE_RE.test(name))
    return 'not an Ethernet interface (a tunnel or virtual L3 interface)';
  return undefined;
}

/** The first L3 leaf a member carries, as [pointer segment, description]. */
function l3Leaf(itf: InterfaceConfig): [string, string] | undefined {
  if (itf.ipv4.length > 0) return ['ipv4', 'IPv4 addresses'];
  if (itf.ipv6.length > 0) return ['ipv6', 'IPv6 addresses'];
  if (itf.dhcpClient !== undefined) return ['dhcpClient', 'a DHCP client'];
  if (itf.unnumbered !== undefined) return ['unnumbered', 'IP unnumbered'];
  if (itf.vrf !== DEFAULT_VRF) return ['vrf', `VRF '${itf.vrf}'`];
  if (Object.keys(itf.subinterfaces).length > 0) return ['subinterfaces', 'sub-interfaces'];
  return undefined;
}

function eachMember(
  config: RootConfig,
  fn: (b: BondEntry, member: string, m: BondMemberConfig) => SemanticIssue | undefined,
): SemanticIssue[] {
  const issues: SemanticIssue[] = [];
  for (const b of bonds(config)) {
    for (const [member, m] of Object.entries(b.bond.members)) {
      const issue = fn(b, member, m);
      if (issue !== undefined) issues.push(issue);
    }
  }
  return issues;
}

export const bondingValidators: readonly ValidatorDefinition[] = [
  {
    name: 'interfaces.bonding-name',
    domains: ['interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const [name, itf] of Object.entries(config.interfaces)) {
        if (itf.bond === undefined) continue;
        const id = bondIdOf(name);
        if (id === undefined) {
          issues.push({
            pointer: jsonPointer('interfaces', name, 'bond'),
            message: `a bond interface must be named BondEthernet<id> (VPP's name), not '${name}'`,
          });
        } else if (itf.bond.id !== undefined && itf.bond.id !== id) {
          issues.push({
            pointer: jsonPointer('interfaces', name, 'bond', 'id'),
            message: `bond id ${itf.bond.id} does not match the interface name ${name} (id ${id})`,
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'interfaces.bonding-member-exists',
    domains: ['interfaces'],
    validate: (config) =>
      eachMember(config, (b, member) =>
        member in config.interfaces
          ? undefined
          : {
              pointer: memberPtr(b.name, member),
              message: `member interface '${member}' is not configured (add interfaces.${member} with enabled: true)`,
            },
      ),
  },
  {
    name: 'interfaces.bonding-member-kind',
    domains: ['interfaces'],
    validate: (config) =>
      eachMember(config, (b, member) => {
        if (member === b.name) {
          return {
            pointer: memberPtr(b.name, member),
            message: 'a bond cannot be a member of itself',
          };
        }
        const kind = notMemberKind(member, config.interfaces[member]);
        return kind === undefined
          ? undefined
          : {
              pointer: memberPtr(b.name, member),
              message: `'${member}' is ${kind}; bond members must be physical interfaces`,
            };
      }),
  },
  {
    name: 'interfaces.bonding-member-unique',
    domains: ['interfaces'],
    validate: (config) => {
      const first = new Map<string, string>();
      return eachMember(config, (b, member) => {
        const owner = first.get(member);
        if (owner === undefined) {
          first.set(member, b.name);
          return undefined;
        }
        return {
          pointer: memberPtr(b.name, member),
          message: `'${member}' is already a member of ${owner} (${memberPtr(owner, member)}); an interface belongs to at most one bond`,
        };
      });
    },
  },
  {
    name: 'interfaces.bonding-member-l3',
    domains: ['interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const seen = new Set<string>();
      for (const b of bonds(config)) {
        for (const member of Object.keys(b.bond.members)) {
          const itf = config.interfaces[member];
          if (itf === undefined || seen.has(member)) continue;
          seen.add(member);
          const leaf = l3Leaf(itf);
          if (leaf !== undefined) {
            issues.push({
              pointer: jsonPointer('interfaces', member, leaf[0]),
              message: `${member} is a member of ${b.name}: configure ${leaf[1]} on the bond, not on a member`,
            });
          }
        }
      }
      return issues;
    },
  },
  {
    // F-bonding review F5: bond_add_member gives the members the bond's MAC (and restores theirs on detach); a member MAC of
    // its own would fight it (permanent drift, and a re-apply would take the member off the bond's MAC).
    name: 'interfaces.bonding-member-mac',
    domains: ['interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const seen = new Set<string>();
      for (const b of bonds(config)) {
        for (const member of Object.keys(b.bond.members)) {
          const itf = config.interfaces[member];
          if (itf?.mac === undefined || seen.has(member)) continue;
          seen.add(member);
          issues.push({
            pointer: jsonPointer('interfaces', member, 'mac'),
            message: `${member} is a member of ${b.name}: members take the bond's MAC address; set mac on the bond instead`,
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'interfaces.bonding-load-balance',
    domains: ['interfaces'],
    validate: (config) =>
      bonds(config)
        .filter((b) => b.bond.loadBalance !== undefined && !BOND_HASH_MODES.includes(b.bond.mode))
        .map((b) => ({
          pointer: jsonPointer('interfaces', b.name, 'bond', 'loadBalance'),
          message: `loadBalance applies to xor and lacp bonds only; VPP forces the algorithm of a ${b.bond.mode} bond`,
        })),
  },
  {
    name: 'interfaces.bonding-lacp-options',
    domains: ['interfaces'],
    validate: (config) =>
      eachMember(config, (b, member, m) => {
        if (b.bond.mode === 'lacp') return undefined;
        const leaf = m.passive ? 'passive' : m.longTimeout ? 'longTimeout' : undefined;
        return leaf === undefined
          ? undefined
          : {
              pointer: memberPtr(b.name, member, leaf),
              message: `${leaf} is an LACP option; ${b.name} is a ${b.bond.mode} bond`,
            };
      }),
  },
  {
    name: 'interfaces.bonding-weight',
    domains: ['interfaces'],
    validate: (config) =>
      eachMember(config, (b, member, m) =>
        m.weight === undefined || b.bond.mode === 'active-backup'
          ? undefined
          : {
              pointer: memberPtr(b.name, member, 'weight'),
              message: `weight applies to active-backup bonds only; ${b.name} is a ${b.bond.mode} bond`,
            },
      ),
  },
];
