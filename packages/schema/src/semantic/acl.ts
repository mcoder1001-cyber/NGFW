import type { RootConfig } from '../index.js';
import { jsonPointer } from '../pointer.js';
import type { AclConfig, AclRule, HostRule } from '../domains/acl.js';
import { duplicates } from './nat.js';
import {
  addressObjectNames,
  ipFamily,
  interfaceExists,
  interfaceVrf,
  own,
  serviceObjectNames,
  serviceSpecIssues,
  vrfExists,
} from './objects.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `acl` (tier b). Pure functions of the schema-valid document, `{ pointer, message }[]`.
 *
 * Rules (name → what it rejects):
 *   acl.rule-sequences-unique  two rules with the same `sequence` inside one list (lists, macip, host)
 *   acl.rule-references        a rule whose source/destination/service `name` or `schedule` is not in `objects`
 *   acl.rule-consistency       prefix family ≠ `ipVersion`, mixed source/destination families, icmp vs icmp6 in
 *                              the wrong family, and inline service specs failing `serviceSpecIssues`
 *   acl.tags-exist             a list `tags[]` entry that is not a key of `objects.tags`
 *   acl.macip-rules            a source MAC with bits outside its mask
 *   acl.attachments            unknown list / interface / zone / VRF, a VRF that differs from the target
 *                              interface's VRF, the same list attached twice to one target+direction, two
 *                              attachments sharing a sequence on one target+direction
 *   acl.macip-attachments      unknown list / interface / VRF, VRF mismatch, more than one MACIP ACL per interface
 *   acl.host-attachments       unknown list, the same list attached twice to one chain
 *
 * Only P02b edits this file.
 */

type Segment = string | number;
type RuleKind = 'lists' | 'host';

/** The three list records with the fields every list kind shares (typed once so `Object.entries` stays typed). */
function listKinds(
  acl: AclConfig,
): [string, Record<string, { rules: { sequence: number }[]; tags: string[] }>][] {
  return [
    ['lists', acl.lists],
    ['macip', acl.macip],
    ['host', acl.host],
  ];
}

/** Effective address family of a rule: `ipVersion`, else the family of its first literal prefix, else `undefined`. */
function ruleFamily(rule: AclRule | HostRule): 4 | 6 | undefined {
  if (rule.ipVersion !== 'any') return rule.ipVersion === 'ipv4' ? 4 : 6;
  for (const side of [rule.source, rule.destination]) {
    if (side.kind === 'prefix') return ipFamily(side.prefix);
  }
  return undefined;
}

function ruleConsistencyIssues(
  rule: AclRule | HostRule,
  at: (...s: Segment[]) => string,
): SemanticIssue[] {
  const issues: SemanticIssue[] = [];
  const family = ruleFamily(rule);
  for (const sideName of ['source', 'destination'] as const) {
    const side = rule[sideName];
    if (side.kind === 'prefix' && family !== undefined && ipFamily(side.prefix) !== family) {
      issues.push({
        pointer: at(sideName, 'prefix'),
        message: `IPv${ipFamily(side.prefix)} prefix in an IPv${family} rule`,
      });
    }
  }
  if (rule.service.kind === 'inline') {
    const { spec } = rule.service;
    const icmpFamily = spec.protocol === 'icmp' ? 4 : spec.protocol === 'icmp6' ? 6 : undefined;
    if (icmpFamily !== undefined && family !== undefined && icmpFamily !== family) {
      issues.push({
        pointer: at('service', 'spec', 'protocol'),
        message: `${spec.protocol} cannot be matched in an IPv${family} rule`,
      });
    }
    issues.push(...serviceSpecIssues(spec, (...s) => at('service', 'spec', ...s)));
  }
  return issues;
}

/** 48-bit MAC → BigInt (`aa:bb:cc:dd:ee:ff` or `aa-bb-…`, any case; schema-valid input assumed). */
export function macToBigInt(mac: string): bigint {
  return BigInt(`0x${mac.replace(/[:-]/g, '')}`);
}

/** Attachment targets as a comparable key: `if:<name>` or `zone:<name>`. */
function targetKey(target: AclConfig['attachments'][number]['target']): string {
  return target.kind === 'interface' ? `if:${target.interface}` : `zone:${target.zone}`;
}

/** `vrf` of an attachment must exist and equal the VRF of every interface it targets (when the interface says). */
function vrfIssues(
  config: RootConfig,
  vrf: string | undefined,
  interfaces: readonly string[],
  pointer: string,
): SemanticIssue[] {
  if (vrf === undefined) return [];
  if (!vrfExists(config, vrf)) return [{ pointer, message: `VRF '${vrf}' does not exist` }];
  const issues: SemanticIssue[] = [];
  for (const iface of interfaces) {
    const actual = interfaceVrf(config, iface);
    if (actual !== undefined && actual !== vrf) {
      issues.push({ pointer, message: `interface '${iface}' is in VRF '${actual}', not '${vrf}'` });
    }
  }
  return issues;
}

export const aclValidators: readonly ValidatorDefinition[] = [
  {
    name: 'acl.rule-sequences-unique',
    domains: ['acl'],
    validate: ({ acl }) => {
      const issues: SemanticIssue[] = [];
      for (const [kind, record] of listKinds(acl)) {
        for (const [name, list] of Object.entries(record)) {
          for (const { index, first, item } of duplicates(list.rules, (r) => String(r.sequence))) {
            issues.push({
              pointer: jsonPointer('acl', kind, name, 'rules', index, 'sequence'),
              message: `sequence ${item.sequence} is already used by rule ${first}`,
            });
          }
        }
      }
      return issues;
    },
  },
  {
    name: 'acl.rule-references',
    domains: ['acl', 'objects'],
    validate: ({ acl, objects }) => {
      const issues: SemanticIssue[] = [];
      const addresses = addressObjectNames(objects);
      const services = serviceObjectNames(objects);
      const check = (kind: RuleKind, name: string, rule: AclRule | HostRule, i: number): void => {
        const at = (...s: Segment[]): string => jsonPointer('acl', kind, name, 'rules', i, ...s);
        for (const sideName of ['source', 'destination'] as const) {
          const side = rule[sideName];
          if (side.kind === 'object' && !addresses.has(side.name)) {
            issues.push({
              pointer: at(sideName, 'name'),
              message: `'${side.name}' is not an entry of objects.addresses or objects.addressGroups`,
            });
          }
        }
        if (rule.service.kind === 'object' && !services.has(rule.service.name)) {
          issues.push({
            pointer: at('service', 'name'),
            message: `'${rule.service.name}' is not an entry of objects.services or objects.serviceGroups`,
          });
        }
        if (
          'schedule' in rule &&
          rule.schedule !== undefined &&
          !Object.hasOwn(objects.schedules, rule.schedule)
        ) {
          issues.push({
            pointer: at('schedule'),
            message: `schedule '${rule.schedule}' does not exist in objects.schedules`,
          });
        }
      };
      for (const [name, list] of Object.entries(acl.lists)) {
        list.rules.forEach((rule, i) => check('lists', name, rule, i));
      }
      for (const [name, list] of Object.entries(acl.host)) {
        list.rules.forEach((rule, i) => check('host', name, rule, i));
      }
      return issues;
    },
  },
  {
    name: 'acl.rule-consistency',
    domains: ['acl'],
    validate: ({ acl }) => {
      const issues: SemanticIssue[] = [];
      const kinds: [RuleKind, Record<string, { rules: (AclRule | HostRule)[] }>][] = [
        ['lists', acl.lists],
        ['host', acl.host],
      ];
      for (const [kind, record] of kinds) {
        for (const [name, list] of Object.entries(record)) {
          list.rules.forEach((rule, i) => {
            issues.push(
              ...ruleConsistencyIssues(rule, (...s) =>
                jsonPointer('acl', kind, name, 'rules', i, ...s),
              ),
            );
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'acl.tags-exist',
    domains: ['acl', 'objects'],
    validate: ({ acl, objects }) => {
      const issues: SemanticIssue[] = [];
      for (const [kind, record] of listKinds(acl)) {
        for (const [name, list] of Object.entries(record)) {
          list.tags.forEach((tag, i) => {
            if (!Object.hasOwn(objects.tags, tag)) {
              issues.push({
                pointer: jsonPointer('acl', kind, name, 'tags', i),
                message: `tag '${tag}' does not exist in objects.tags`,
              });
            }
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'acl.macip-rules',
    domains: ['acl'],
    validate: ({ acl }) => {
      const issues: SemanticIssue[] = [];
      for (const [name, list] of Object.entries(acl.macip)) {
        list.rules.forEach((rule, i) => {
          if (
            (macToBigInt(rule.sourceMac) & ~macToBigInt(rule.sourceMacMask) & 0xffffffffffffn) !==
            0n
          ) {
            issues.push({
              pointer: jsonPointer('acl', 'macip', name, 'rules', i, 'sourceMac'),
              message: 'source MAC has bits set outside sourceMacMask',
            });
          }
        });
      }
      return issues;
    },
  },
  {
    name: 'acl.attachments',
    domains: ['acl', 'objects', 'interfaces', 'vrfs'],
    validate: (config) => {
      const { acl, objects } = config;
      const issues: SemanticIssue[] = [];
      acl.attachments.forEach((a, i) => {
        const at = (...s: Segment[]): string => jsonPointer('acl', 'attachments', i, ...s);
        if (!Object.hasOwn(acl.lists, a.list)) {
          issues.push({ pointer: at('list'), message: `access list '${a.list}' does not exist` });
        }
        let targetInterfaces: readonly string[] = [];
        if (a.target.kind === 'interface') {
          if (interfaceExists(config, a.target.interface)) targetInterfaces = [a.target.interface];
          else {
            issues.push({
              pointer: at('target', 'interface'),
              message: `interface '${a.target.interface}' does not exist`,
            });
          }
        } else {
          const zone = own(objects.zones, a.target.zone);
          if (zone === undefined) {
            issues.push({
              pointer: at('target', 'zone'),
              message: `zone '${a.target.zone}' does not exist`,
            });
          } else {
            targetInterfaces = zone.interfaces;
          }
        }
        issues.push(...vrfIssues(config, a.vrf, targetInterfaces, at('vrf')));
      });
      for (const { index, item } of duplicates(
        acl.attachments,
        (a) => `${a.list}|${targetKey(a.target)}|${a.direction}`,
      )) {
        issues.push({
          pointer: jsonPointer('acl', 'attachments', index, 'list'),
          message: `access list '${item.list}' is already attached to this target in direction '${item.direction}'`,
        });
      }
      for (const { index, first, item } of duplicates(
        acl.attachments,
        (a) => `${targetKey(a.target)}|${a.direction}|${a.sequence}`,
      )) {
        issues.push({
          pointer: jsonPointer('acl', 'attachments', index, 'sequence'),
          message: `sequence ${item.sequence} is already used by attachment ${first} on the same target and direction`,
        });
      }
      return issues;
    },
  },
  {
    name: 'acl.macip-attachments',
    domains: ['acl', 'interfaces', 'vrfs'],
    validate: (config) => {
      const { acl } = config;
      const issues: SemanticIssue[] = [];
      acl.macipAttachments.forEach((a, i) => {
        const at = (...s: Segment[]): string => jsonPointer('acl', 'macipAttachments', i, ...s);
        if (!Object.hasOwn(acl.macip, a.list)) {
          issues.push({
            pointer: at('list'),
            message: `MACIP access list '${a.list}' does not exist`,
          });
        }
        if (!interfaceExists(config, a.interface)) {
          issues.push({
            pointer: at('interface'),
            message: `interface '${a.interface}' does not exist`,
          });
          issues.push(...vrfIssues(config, a.vrf, [], at('vrf')));
        } else {
          issues.push(...vrfIssues(config, a.vrf, [a.interface], at('vrf')));
        }
      });
      for (const { index, item } of duplicates(acl.macipAttachments, (a) => a.interface)) {
        issues.push({
          pointer: jsonPointer('acl', 'macipAttachments', index, 'interface'),
          message: `interface '${item.interface}' already has a MACIP access list (VPP allows one per interface)`,
        });
      }
      return issues;
    },
  },
  {
    name: 'acl.host-attachments',
    domains: ['acl'],
    validate: ({ acl }) => {
      const issues: SemanticIssue[] = [];
      acl.hostAttachments.forEach((a, i) => {
        if (!Object.hasOwn(acl.host, a.list)) {
          issues.push({
            pointer: jsonPointer('acl', 'hostAttachments', i, 'list'),
            message: `host access list '${a.list}' does not exist`,
          });
        }
      });
      for (const { index, item } of duplicates(
        acl.hostAttachments,
        (a) => `${a.list}|${a.chain}`,
      )) {
        issues.push({
          pointer: jsonPointer('acl', 'hostAttachments', index, 'chain'),
          message: `host access list '${item.list}' is already attached to chain '${item.chain}'`,
        });
      }
      return issues;
    },
  },
];
