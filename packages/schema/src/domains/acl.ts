import { z } from 'zod';
import { macPattern, objectName, vppInterfaceName } from '../primitives.js';
import { type UiHints, type UiMeta, withUi as setUi, X_VRX_UI } from '../ui.js';

/**
 * `withUi` that MERGES `x-vrx-ui` with the hints already on `schema` (review H1): Zod 4 merges registry meta
 * shallowly, so re-wrapping a hinted primitive (`withUi(ipAddress, { title })`) with `ui.ts`'s `withUi` would drop
 * its `widget`/`help`. Private on purpose (not re-exported; D-054 pattern) — becomes a plain alias once `ui.ts`
 * merges itself (D-043).
 */
function withUi<T extends z.ZodType>(schema: T, meta: UiMeta): T {
  const inherited = (schema.meta()?.[X_VRX_UI] ?? {}) as UiHints;
  const { title, description, ...hints } = meta;
  return setUi(schema, {
    ...(title !== undefined ? { title } : {}),
    ...(description !== undefined ? { description } : {}),
    ...inherited,
    ...hints,
  });
}
import { ipPrefix, ServiceSpecSchema } from './objects.js';

/**
 * `acl` — access control (WBS D5.2, D5.3; TNSR "ACL", "MACIP ACL" and "host ACL" are the reference).
 *
 *   lists              record<name, L3/L4 ACL>   → VPP `acl` plugin (stateless permit/deny, stateful `reflect`)
 *   macip              record<name, L2 ACL>      → VPP `macip` ACL (source MAC + mask + source prefix, input only)
 *   host               record<name, host ACL>    → nftables on the management plane (local-in/out protection)
 *   attachments[]      { list, target: interface|zone, direction, sequence, vrf? }
 *   macipAttachments[] { list, interface, vrf? }   (VPP: one MACIP ACL per interface, input only)
 *   hostAttachments[]  { list, chain, priority }
 *
 * Rules match on objects from `objects` (address/service/schedule by name) or inline prefixes/services, so the same
 * object edit updates every rule (renderer expands objects into VPP rules). Rule order = `sequence` (unique per
 * list — semantic); VPP hit counters are state, not config.
 *
 * Guardrails (vdom.md #1, #2): attachments carry `vrf` (must equal the interface's VRF when both are set);
 * names are unique within `acl.lists` / `acl.macip` / `acl.host` (the record key), never globally.
 *
 * Only P02b edits this file.
 */

const description = withUi(z.string().max(255), { title: 'Description', widget: 'textarea' });
const tags = withUi(z.array(objectName).max(32).default([]), {
  title: 'Tags',
  widget: 'tag-picker',
  help: 'Names of entries in objects.tags',
});

/** Rule sequence — the order rules are evaluated in; unique within a list (semantic). */
export const ruleSequence = withUi(z.number().int().min(1).max(2_147_483_647), {
  title: 'Sequence',
  widget: 'number',
});

/** Linux interface name (IFNAMSIZ 15) for host ACL rules. */
export const linuxInterfaceName = withUi(
  z.string().regex(/^[A-Za-z0-9_.-]{1,15}$/, 'expected a Linux interface name (max 15 chars)'),
  { title: 'Host interface', widget: 'host-interface-picker' },
);

// ---------------------------------------------------------------------------------------------------------------
// Match fields
// ---------------------------------------------------------------------------------------------------------------

/** Source/destination match: anything, a literal prefix, or an address object / address group by name. */
export const AddressMatchSchema = withUi(
  z.discriminatedUnion('kind', [
    z.strictObject({ kind: z.literal('any') }),
    z.strictObject({ kind: z.literal('prefix'), prefix: ipPrefix }),
    z.strictObject({
      kind: z.literal('object'),
      name: withUi(objectName, {
        title: 'Object',
        widget: 'object-picker',
        help: 'objects.addresses or objects.addressGroups',
      }),
    }),
  ]),
  { title: 'Address match' },
);

/** Service match: anything, a service / service group by name, or an inline protocol+ports spec. */
export const ServiceMatchSchema = withUi(
  z.discriminatedUnion('kind', [
    z.strictObject({ kind: z.literal('any') }),
    z.strictObject({
      kind: z.literal('object'),
      name: withUi(objectName, {
        title: 'Object',
        widget: 'object-picker',
        help: 'objects.services or objects.serviceGroups',
      }),
    }),
    z.strictObject({ kind: z.literal('inline'), spec: ServiceSpecSchema }),
  ]),
  { title: 'Service match' },
);

const anyAddress = { kind: 'any' } as const;
const anyService = { kind: 'any' } as const;

// ---------------------------------------------------------------------------------------------------------------
// L3/L4 lists (VPP acl plugin)
// ---------------------------------------------------------------------------------------------------------------

/**
 * One L3/L4 rule. `reflect` = permit and create a reflexive (stateful) entry for the return direction.
 * `ipVersion: any` renders as one IPv4 and one IPv6 VPP rule when nothing else pins the family.
 */
export const AclRuleSchema = withUi(
  z.strictObject({
    sequence: ruleSequence,
    description: description.optional(),
    enabled: withUi(z.boolean().default(true), { title: 'Enabled' }),
    action: withUi(z.enum(['permit', 'deny', 'reflect']), { title: 'Action' }),
    ipVersion: withUi(z.enum(['ipv4', 'ipv6', 'any']).default('any'), {
      title: 'IP version',
      help: 'any = both families (must agree with prefixes and ICMP protocol)',
    }),
    source: AddressMatchSchema.default(anyAddress),
    destination: AddressMatchSchema.default(anyAddress),
    service: ServiceMatchSchema.default(anyService),
    schedule: withUi(objectName, {
      title: 'Schedule',
      widget: 'object-picker',
      help: 'objects.schedules; omit = always',
    }).optional(),
    log: withUi(z.boolean().default(false), { title: 'Log' }),
  }),
  { title: 'ACL rule' },
);

export const AclListSchema = withUi(
  z.strictObject({
    description: description.optional(),
    tags,
    rules: withUi(z.array(AclRuleSchema).default([]), { title: 'Rules', widget: 'rule-editor' }),
  }),
  { title: 'Access list' },
);

// ---------------------------------------------------------------------------------------------------------------
// MACIP lists (VPP macip acl)
// ---------------------------------------------------------------------------------------------------------------

export const MacipRuleSchema = withUi(
  z.strictObject({
    sequence: ruleSequence,
    description: description.optional(),
    action: withUi(z.enum(['permit', 'deny']), { title: 'Action' }),
    // macPattern, not macAddress: match values and masks may be all-zero / have the group bit (P02a merge)
    sourceMac: withUi(macPattern, { title: 'Source MAC' }),
    sourceMacMask: withUi(macPattern.default('ff:ff:ff:ff:ff:ff'), {
      title: 'Source MAC mask',
      widget: 'mac',
      help: 'ff:ff:ff:ff:ff:ff = exact match',
    }),
    sourcePrefix: withUi(ipPrefix, {
      title: 'Source prefix',
      help: 'omit = any address',
    }).optional(),
  }),
  { title: 'MACIP rule' },
);

export const MacipListSchema = withUi(
  z.strictObject({
    description: description.optional(),
    tags,
    rules: withUi(z.array(MacipRuleSchema).default([]), { title: 'Rules', widget: 'rule-editor' }),
  }),
  { title: 'MACIP access list' },
);

// ---------------------------------------------------------------------------------------------------------------
// Host lists (management-plane nftables)
// ---------------------------------------------------------------------------------------------------------------

export const HostRuleSchema = withUi(
  z.strictObject({
    sequence: ruleSequence,
    description: description.optional(),
    enabled: withUi(z.boolean().default(true), { title: 'Enabled' }),
    action: withUi(z.enum(['accept', 'drop', 'reject']), { title: 'Action' }),
    ipVersion: withUi(z.enum(['ipv4', 'ipv6', 'any']).default('any'), { title: 'IP version' }),
    source: AddressMatchSchema.default(anyAddress),
    destination: AddressMatchSchema.default(anyAddress),
    service: ServiceMatchSchema.default(anyService),
    interface: linuxInterfaceName.optional(),
    log: withUi(z.boolean().default(false), { title: 'Log' }),
  }),
  { title: 'Host ACL rule' },
);

export const HostListSchema = withUi(
  z.strictObject({
    description: description.optional(),
    tags,
    rules: withUi(z.array(HostRuleSchema).default([]), { title: 'Rules', widget: 'rule-editor' }),
  }),
  { title: 'Host access list' },
);

// ---------------------------------------------------------------------------------------------------------------
// Attachments
// ---------------------------------------------------------------------------------------------------------------

/** Where an ACL is applied: one VPP interface, or every interface of a zone (`objects.zones`). */
export const AttachmentTargetSchema = withUi(
  z.discriminatedUnion('kind', [
    z.strictObject({ kind: z.literal('interface'), interface: vppInterfaceName }),
    z.strictObject({
      kind: z.literal('zone'),
      zone: withUi(objectName, { title: 'Zone', widget: 'object-picker' }),
    }),
  ]),
  { title: 'Target' },
);

/** `vrf` is first-class (vdom.md #1); when set it must be the VRF of the target interface. */
const vrf = withUi(objectName, {
  title: 'VRF',
  widget: 'vrf-picker',
  help: 'Must match the VRF of the target interface',
}).optional();

export const AclAttachmentSchema = withUi(
  z.strictObject({
    list: withUi(objectName, { title: 'Access list', widget: 'object-picker' }),
    target: AttachmentTargetSchema,
    direction: withUi(z.enum(['in', 'out']).default('in'), { title: 'Direction' }),
    sequence: withUi(ruleSequence, {
      title: 'Sequence',
      help: 'Order among several lists on the same interface and direction',
    }),
    vrf,
    enabled: withUi(z.boolean().default(true), { title: 'Enabled' }),
    description: description.optional(),
  }),
  { title: 'ACL attachment' },
);

export const MacipAttachmentSchema = withUi(
  z.strictObject({
    list: withUi(objectName, { title: 'MACIP access list', widget: 'object-picker' }),
    interface: vppInterfaceName,
    vrf,
    enabled: withUi(z.boolean().default(true), { title: 'Enabled' }),
    description: description.optional(),
  }),
  { title: 'MACIP attachment' },
);

export const HostAttachmentSchema = withUi(
  z.strictObject({
    list: withUi(objectName, { title: 'Host access list', widget: 'object-picker' }),
    chain: withUi(z.enum(['input', 'output', 'forward']), { title: 'Chain' }),
    priority: withUi(z.number().int().min(-500).max(500).default(0), {
      title: 'Priority',
      help: 'nftables chain priority; lower runs first',
    }),
    enabled: withUi(z.boolean().default(true), { title: 'Enabled' }),
    description: description.optional(),
  }),
  { title: 'Host ACL attachment' },
);

// ---------------------------------------------------------------------------------------------------------------
// Domain
// ---------------------------------------------------------------------------------------------------------------

const record = <T extends z.ZodType>(value: T) => z.record(objectName, value).default({});

export const AclSchema = withUi(
  z.strictObject({
    lists: withUi(record(AclListSchema), { title: 'Access lists', group: 'Lists', order: 1 }),
    macip: withUi(record(MacipListSchema), { title: 'MACIP lists', group: 'Lists', order: 2 }),
    host: withUi(record(HostListSchema), { title: 'Host lists', group: 'Lists', order: 3 }),
    attachments: withUi(z.array(AclAttachmentSchema).default([]), {
      title: 'Attachments',
      group: 'Attachments',
      order: 4,
    }),
    macipAttachments: withUi(z.array(MacipAttachmentSchema).default([]), {
      title: 'MACIP attachments',
      group: 'Attachments',
      order: 5,
    }),
    hostAttachments: withUi(z.array(HostAttachmentSchema).default([]), {
      title: 'Host attachments',
      group: 'Attachments',
      order: 6,
    }),
  }),
  {
    title: 'ACL',
    description:
      'L3/L4, MACIP and host access-control lists and their attachments to interfaces, zones and host chains.',
    order: 80,
  },
);

export type AclConfig = z.infer<typeof AclSchema>;
export type AddressMatch = z.infer<typeof AddressMatchSchema>;
export type ServiceMatch = z.infer<typeof ServiceMatchSchema>;
export type AclRule = z.infer<typeof AclRuleSchema>;
export type AclList = z.infer<typeof AclListSchema>;
export type MacipRule = z.infer<typeof MacipRuleSchema>;
export type MacipList = z.infer<typeof MacipListSchema>;
export type HostRule = z.infer<typeof HostRuleSchema>;
export type HostList = z.infer<typeof HostListSchema>;
export type AclAttachment = z.infer<typeof AclAttachmentSchema>;
export type MacipAttachment = z.infer<typeof MacipAttachmentSchema>;
export type HostAttachment = z.infer<typeof HostAttachmentSchema>;
