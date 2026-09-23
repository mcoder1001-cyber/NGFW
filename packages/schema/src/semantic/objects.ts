import type { RootConfig } from '../index.js';
import { isPlainObject } from '../json.js';
import { jsonPointer } from '../pointer.js';
import { parsePortRange, type ObjectsConfig, type ServiceSpec } from '../domains/objects.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `objects` (tier b) plus the cross-domain lookup helpers that `semantic/nat.ts` and
 * `semantic/acl.ts` share (interfaces, VRFs, IP arithmetic, service specs). Every validator is a pure function of
 * the schema-valid document and returns `{ pointer, message }[]` built with `jsonPointer()`.
 *
 * Rules (name → what it rejects):
 *   objects.names-disjoint          the same name in addresses+addressGroups or services+serviceGroups (ACL rules
 *                                   reference either through one `name`, so a clash would be ambiguous)
 *   objects.tags-exist              a `tags[]` entry that is not a key of `objects.tags`
 *   objects.address-range-valid     `range` objects with mixed families or end < start
 *   objects.address-group-members   members that do not exist, are listed twice, or form a cycle
 *   objects.service-group-members   same for service groups
 *   objects.service-valid           ICMP code without type, TCP flag bits outside the mask, overlapping port ranges
 *   objects.schedule-valid          end ≤ start (recurring or once), a weekday listed twice
 *   objects.zone-interfaces         zone members that do not exist, are listed twice, or belong to another zone
 *
 * Only P02b edits this file.
 */

// ---------------------------------------------------------------------------------------------------------------
// Shared helpers (exported for semantic/nat.ts and semantic/acl.ts)
// ---------------------------------------------------------------------------------------------------------------

/** Own-property lookup that ignores the prototype (`constructor`, `toString` … are valid object names). */
export function own<T>(record: Record<string, T>, key: string): T | undefined {
  return Object.hasOwn(record, key) ? record[key] : undefined;
}

/** `4` for dotted-quad text, `6` for anything containing `:` (schema-valid input assumed). */
export function ipFamily(address: string): 4 | 6 {
  return address.includes(':') ? 6 : 4;
}

/** Dotted quad → unsigned 32-bit number (schema-valid input assumed). */
export function ipv4ToNumber(address: string): number {
  return address.split('.').reduce((acc, octet) => acc * 256 + Number(octet), 0);
}

/**
 * RFC 4291 text (compressed `::`, optional embedded IPv4 tail such as `::ffff:10.0.0.1`) → 128-bit BigInt.
 * Schema-valid input assumed (Zod's `z.ipv6()` has already rejected zone ids and malformed groups).
 */
export function ipv6ToBigInt(address: string): bigint {
  let text = address;
  const lastColon = text.lastIndexOf(':');
  const tail = text.slice(lastColon + 1);
  if (tail.includes('.')) {
    const v4 = ipv4ToNumber(tail);
    text = `${text.slice(0, lastColon + 1)}${(v4 >>> 16).toString(16)}:${(v4 & 0xffff).toString(16)}`;
  }
  const [head = '', rest] = text.split('::');
  const headGroups = head === '' ? [] : head.split(':');
  const tailGroups = rest === undefined || rest === '' ? [] : rest.split(':');
  const groups = [
    ...headGroups,
    ...Array<string>(8 - headGroups.length - tailGroups.length).fill('0'),
    ...tailGroups,
  ];
  return groups.reduce((acc, g) => (acc << 16n) | BigInt(parseInt(g, 16)), 0n);
}

/** Any IP address → BigInt in its family's space. */
export function addressToBigInt(address: string): bigint {
  return ipFamily(address) === 4 ? BigInt(ipv4ToNumber(address)) : ipv6ToBigInt(address);
}

/** An inclusive, family-tagged address interval. */
export interface AddressRange {
  family: 4 | 6;
  start: bigint;
  end: bigint;
}

/** Prefix length of a CIDR string (`10.0.0.0/24` → 24). */
export function prefixLength(cidr: string): number {
  return Number(cidr.slice(cidr.indexOf('/') + 1));
}

/** The address interval covered by a CIDR prefix (host bits are ignored: `10.0.0.1/24` → 10.0.0.0–10.0.0.255). */
export function prefixToRange(cidr: string): AddressRange {
  const slash = cidr.indexOf('/');
  const address = cidr.slice(0, slash);
  const family = ipFamily(address);
  const hostBits = BigInt((family === 4 ? 32 : 128) - Number(cidr.slice(slash + 1)));
  const start = (addressToBigInt(address) >> hostBits) << hostBits;
  return { family, start, end: start + (1n << hostBits) - 1n };
}

/** True when two intervals of the same family share at least one address. */
export function rangesOverlap(a: AddressRange, b: AddressRange): boolean {
  return a.family === b.family && a.start <= b.end && b.start <= a.end;
}

/** Bounds of a schema-valid `l4PortRange` string; throws on text the schema would have rejected. */
export function portBounds(text: string): { from: number; to: number } {
  const r = parsePortRange(text);
  if (r === undefined) throw new Error(`invalid port range '${text}'`);
  return r;
}

/**
 * Look up an interface entry by VPP name. `interfaces` is a record keyed by VPP interface name; a sub-interface
 * `parent.N` is either its own key or `interfaces[parent].subinterfaces[N]` (docs/04 shape). Tolerant of the
 * pre-P02a placeholder (values may be anything): returns `undefined` when the name is unknown.
 */
export function lookupInterface(config: RootConfig, name: string): unknown {
  const interfaces: Record<string, unknown> = config.interfaces;
  if (Object.hasOwn(interfaces, name)) return interfaces[name];
  const dot = name.lastIndexOf('.');
  if (dot < 0) return undefined;
  const parent = own(interfaces, name.slice(0, dot));
  if (!isPlainObject(parent) || !isPlainObject(parent.subinterfaces)) return undefined;
  return own(parent.subinterfaces, name.slice(dot + 1));
}

/** True when `name` is an interface or sub-interface of the document. */
export function interfaceExists(config: RootConfig, name: string): boolean {
  return lookupInterface(config, name) !== undefined;
}

/** The `vrf` of an interface when the entry declares one (string), else `undefined`. */
export function interfaceVrf(config: RootConfig, name: string): string | undefined {
  const entry = lookupInterface(config, name);
  return isPlainObject(entry) && typeof entry.vrf === 'string' ? entry.vrf : undefined;
}

/** True when `name` is a VRF of the document. `default` always exists (vdom.md #1: VRF 0 is named, never implied). */
export function vrfExists(config: RootConfig, name: string): boolean {
  return name === 'default' || Object.hasOwn(config.vrfs, name);
}

/** Names an ACL rule may use as an address match: `objects.addresses` ∪ `objects.addressGroups`. */
export function addressObjectNames(objects: ObjectsConfig): Set<string> {
  return new Set([...Object.keys(objects.addresses), ...Object.keys(objects.addressGroups)]);
}

/** Names an ACL rule may use as a service match: `objects.services` ∪ `objects.serviceGroups`. */
export function serviceObjectNames(objects: ObjectsConfig): Set<string> {
  return new Set([...Object.keys(objects.services), ...Object.keys(objects.serviceGroups)]);
}

/** Overlaps inside one port list: `["80", "80-90"]` → issue on index 1. */
function portListIssues(ports: readonly string[], pointer: (i: number) => string): SemanticIssue[] {
  const issues: SemanticIssue[] = [];
  const seen: { from: number; to: number; index: number }[] = [];
  ports.forEach((text, i) => {
    const r = portBounds(text);
    const hit = seen.find((s) => s.from <= r.to && r.from <= s.to);
    if (hit !== undefined) {
      issues.push({
        pointer: pointer(i),
        message: `port range '${text}' overlaps entry ${hit.index}`,
      });
    }
    seen.push({ ...r, index: i });
  });
  return issues;
}

/**
 * Cross-field checks of one protocol/port spec (used for `objects.services` entries and inline ACL services):
 * ICMP code requires a type; TCP flag `value` bits must lie inside `mask`; port lists must not overlap.
 * `base` builds the pointer of the spec itself.
 */
export function serviceSpecIssues(
  spec: ServiceSpec,
  base: (...segments: readonly (string | number)[]) => string,
): SemanticIssue[] {
  const issues: SemanticIssue[] = [];
  switch (spec.protocol) {
    case 'icmp':
    case 'icmp6':
      if (spec.code !== undefined && spec.type === undefined) {
        issues.push({ pointer: base('code'), message: 'ICMP code requires an ICMP type' });
      }
      break;
    case 'tcp':
    case 'tcp-udp':
      if (spec.tcpFlags !== undefined && (spec.tcpFlags.value & ~spec.tcpFlags.mask & 0xff) !== 0) {
        issues.push({
          pointer: base('tcpFlags', 'value'),
          message: 'TCP flag bits set in value must also be set in mask',
        });
      }
    // falls through: ports apply to tcp, tcp-udp, udp and sctp
    case 'udp':
    case 'sctp':
      issues.push(
        ...portListIssues(spec.destinationPorts, (i) => base('destinationPorts', i)),
        ...portListIssues(spec.sourcePorts, (i) => base('sourcePorts', i)),
      );
      break;
    default:
      break;
  }
  return issues;
}

// ---------------------------------------------------------------------------------------------------------------
// objects.* validators
// ---------------------------------------------------------------------------------------------------------------

/** Every object kind carrying `tags[]` (typed once so `Object.entries` stays typed across the union). */
function taggedKinds(objects: ObjectsConfig): [string, Record<string, { tags: string[] }>][] {
  return [
    ['addresses', objects.addresses],
    ['addressGroups', objects.addressGroups],
    ['services', objects.services],
    ['serviceGroups', objects.serviceGroups],
    ['schedules', objects.schedules],
    ['zones', objects.zones],
  ];
}

interface Group {
  members: string[];
}

/** Existence, duplicates and cycles for one group kind (`addressGroups` over `addresses`, …). */
function groupIssues(
  kind: 'addressGroups' | 'serviceGroups',
  leafKind: 'addresses' | 'services',
  groups: Record<string, Group>,
  leaves: Record<string, unknown>,
): SemanticIssue[] {
  const issues: SemanticIssue[] = [];
  const groupMap = new Map(Object.entries(groups));
  for (const [name, group] of groupMap) {
    const seen = new Set<string>();
    group.members.forEach((member, i) => {
      const pointer = jsonPointer('objects', kind, name, 'members', i);
      if (seen.has(member)) {
        issues.push({ pointer, message: `member '${member}' is listed twice` });
      } else if (!Object.hasOwn(leaves, member) && !groupMap.has(member)) {
        issues.push({
          pointer,
          message: `'${member}' is not an entry of objects.${leafKind} or objects.${kind}`,
        });
      }
      seen.add(member);
    });
  }
  // Cycle detection: DFS over group→group edges; the member that closes a cycle is reported.
  const state = new Map<string, 'visiting' | 'done'>();
  const path: string[] = [];
  const visit = (name: string, group: Group): void => {
    state.set(name, 'visiting');
    path.push(name);
    group.members.forEach((member, i) => {
      const next = groupMap.get(member);
      if (next === undefined) return;
      const seen = state.get(member);
      if (seen === 'visiting') {
        const cycle = [...path.slice(path.indexOf(member)), member].join(' → ');
        issues.push({
          pointer: jsonPointer('objects', kind, name, 'members', i),
          message: `group membership cycle: ${cycle}`,
        });
      } else if (seen === undefined) {
        visit(member, next);
      }
    });
    path.pop();
    state.set(name, 'done');
  };
  for (const [name, group] of groupMap) if (!state.has(name)) visit(name, group);
  return issues;
}

export const objectsValidators: readonly ValidatorDefinition[] = [
  {
    name: 'objects.names-disjoint',
    domains: ['objects'],
    validate: ({ objects }) => {
      const issues: SemanticIssue[] = [];
      for (const name of Object.keys(objects.addressGroups)) {
        if (Object.hasOwn(objects.addresses, name)) {
          issues.push({
            pointer: jsonPointer('objects', 'addressGroups', name),
            message: `'${name}' is also an address object; addresses and address groups share one namespace`,
          });
        }
      }
      for (const name of Object.keys(objects.serviceGroups)) {
        if (Object.hasOwn(objects.services, name)) {
          issues.push({
            pointer: jsonPointer('objects', 'serviceGroups', name),
            message: `'${name}' is also a service object; services and service groups share one namespace`,
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'objects.tags-exist',
    domains: ['objects'],
    validate: ({ objects }) => {
      const issues: SemanticIssue[] = [];
      for (const [kind, record] of taggedKinds(objects)) {
        for (const [name, entry] of Object.entries(record)) {
          entry.tags.forEach((tag, i) => {
            if (!Object.hasOwn(objects.tags, tag)) {
              issues.push({
                pointer: jsonPointer('objects', kind, name, 'tags', i),
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
    name: 'objects.address-range-valid',
    domains: ['objects'],
    validate: ({ objects }) => {
      const issues: SemanticIssue[] = [];
      for (const [name, address] of Object.entries(objects.addresses)) {
        if (address.type !== 'range') continue;
        const pointer = jsonPointer('objects', 'addresses', name, 'end');
        if (ipFamily(address.start) !== ipFamily(address.end)) {
          issues.push({ pointer, message: 'range start and end must be the same address family' });
        } else if (addressToBigInt(address.start) > addressToBigInt(address.end)) {
          issues.push({ pointer, message: 'range end must not be lower than its start' });
        }
      }
      return issues;
    },
  },
  {
    name: 'objects.address-group-members',
    domains: ['objects'],
    validate: ({ objects }) =>
      groupIssues('addressGroups', 'addresses', objects.addressGroups, objects.addresses),
  },
  {
    name: 'objects.service-group-members',
    domains: ['objects'],
    validate: ({ objects }) =>
      groupIssues('serviceGroups', 'services', objects.serviceGroups, objects.services),
  },
  {
    name: 'objects.service-valid',
    domains: ['objects'],
    validate: ({ objects }) => {
      const issues: SemanticIssue[] = [];
      for (const [name, service] of Object.entries(objects.services)) {
        issues.push(
          ...serviceSpecIssues(service, (...s) => jsonPointer('objects', 'services', name, ...s)),
        );
      }
      return issues;
    },
  },
  {
    name: 'objects.schedule-valid',
    domains: ['objects'],
    validate: ({ objects }) => {
      const issues: SemanticIssue[] = [];
      for (const [name, schedule] of Object.entries(objects.schedules)) {
        const endPointer = jsonPointer('objects', 'schedules', name, 'end');
        if (schedule.type === 'recurring') {
          if (schedule.start >= schedule.end) {
            issues.push({
              pointer: endPointer,
              message: 'end must be later than start (model an overnight window as two schedules)',
            });
          }
          const seen = new Set<string>();
          schedule.days.forEach((day, i) => {
            if (seen.has(day)) {
              issues.push({
                pointer: jsonPointer('objects', 'schedules', name, 'days', i),
                message: `weekday '${day}' is listed twice`,
              });
            }
            seen.add(day);
          });
        } else if (Date.parse(schedule.start) >= Date.parse(schedule.end)) {
          issues.push({ pointer: endPointer, message: 'end must be later than start' });
        }
      }
      return issues;
    },
  },
  {
    name: 'objects.zone-interfaces',
    domains: ['objects', 'interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const owner = new Map<string, string>();
      for (const [name, zone] of Object.entries(config.objects.zones)) {
        zone.interfaces.forEach((iface, i) => {
          const pointer = jsonPointer('objects', 'zones', name, 'interfaces', i);
          const zoneOfIface = owner.get(iface);
          if (zoneOfIface === name) {
            issues.push({ pointer, message: `interface '${iface}' is listed twice` });
          } else if (zoneOfIface !== undefined) {
            issues.push({
              pointer,
              message: `interface '${iface}' already belongs to zone '${zoneOfIface}'`,
            });
          } else if (!interfaceExists(config, iface)) {
            issues.push({ pointer, message: `interface '${iface}' does not exist` });
          }
          if (zoneOfIface === undefined) owner.set(iface, name);
        });
      }
      return issues;
    },
  },
];
