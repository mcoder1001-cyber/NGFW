import { z } from 'zod';
import {
  hostname,
  ipAddress,
  ipv4Cidr,
  ipv6Cidr,
  objectName,
  vppInterfaceName,
} from '../primitives.js';
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

/**
 * `objects` — reusable firewall objects referenced by `acl` (and, later, NAT policies): addresses, address groups,
 * services, service groups, schedules, zones and tags (WBS D5.1; TNSR has no object model, this is our superset).
 *
 * Shape: every kind is a record keyed by `objectName`; the key is the name, so names are unique within their kind
 * only (vdom.md guardrail #2) and API paths stay `/config/objects/<kind>/<name>`.
 *
 * Tier (a) here = shape and single-field formats. Cross-object rules (members exist, no cycles, zones reference
 * existing interfaces, tags exist, ranges ordered) are semantic validators in `../semantic/objects.ts`.
 *
 * Only P02b edits this file.
 */

// ---------------------------------------------------------------------------------------------------------------
// Local primitives — P02a owns `primitives.ts`; these are candidates to move there (docs/status/tasks/P02b-questions.md).
// Names are deliberately specific (`l4PortRange`, `ipPrefix`) so a later `portRange`/`ipCidr` in primitives.ts
// cannot collide with the `export *` in index.ts.
// ---------------------------------------------------------------------------------------------------------------

/** IPv4 or IPv6 prefix in CIDR notation (`10.0.0.0/24`, `2001:db8::/32`). */
export const ipPrefix = withUi(z.union([ipv4Cidr, ipv6Cidr]), {
  title: 'IP prefix',
  widget: 'cidr',
  help: 'e.g. 10.0.0.0/24 or 2001:db8::/32',
});

/** A single TCP/UDP/SCTP port, 1–65535. */
export const l4PortNumber = withUi(z.number().int().min(1).max(65535), {
  title: 'Port',
  widget: 'number',
});

const PORT_RANGE_RE = /^(\d{1,5})(?:-(\d{1,5}))?$/;

/** Parse `"443"` or `"8000-8080"` → `{ from, to }`; `undefined` when the text is not a port range at all. */
export function parsePortRange(text: string): { from: number; to: number } | undefined {
  const m = PORT_RANGE_RE.exec(text);
  if (m === null) return undefined;
  const from = Number(m[1]);
  const to = m[2] === undefined ? from : Number(m[2]);
  return { from, to };
}

/** A port (`443`) or an inclusive port range (`8000-8080`); both ends 1–65535, from ≤ to. */
export const l4PortRange = withUi(
  z
    .string()
    .regex(PORT_RANGE_RE, 'expected a port like 443 or a range like 8000-8080')
    .refine(
      (s) => {
        const r = parsePortRange(s);
        return r !== undefined && r.from >= 1 && r.to <= 65535 && r.from <= r.to;
      },
      { message: 'ports must be 1–65535 and the range start must not exceed its end' },
    ),
  { title: 'Port range', widget: 'port-range', help: '443 or 8000-8080' },
);

/** Local time of day `HH:MM` (24 h). */
export const timeOfDay = withUi(
  z.string().regex(/^(?:[01]\d|2[0-3]):[0-5]\d$/, 'expected HH:MM (00:00–23:59)'),
  { title: 'Time', widget: 'time' },
);

/** `#rrggbb` colour for tags. */
export const hexColor = withUi(
  z.string().regex(/^#[0-9a-fA-F]{6}$/, 'expected a colour like #1e88e5'),
  { title: 'Colour', widget: 'color' },
);

/** ICMP type or code, 0–255. */
const icmpNumber = z.number().int().min(0).max(255);

// ---------------------------------------------------------------------------------------------------------------
// Fields shared by every object kind
// ---------------------------------------------------------------------------------------------------------------

const description = withUi(z.string().max(255), { title: 'Description', widget: 'textarea' });

/** Tag names; each must exist in `objects.tags` (semantic `objects.tags-exist`). */
const tags = withUi(z.array(objectName).max(32).default([]), {
  title: 'Tags',
  widget: 'tag-picker',
  help: 'Names of entries in objects.tags',
});

const common = { description: description.optional(), tags };

// ---------------------------------------------------------------------------------------------------------------
// Addresses
// ---------------------------------------------------------------------------------------------------------------

/**
 * One address object. `fqdn` objects are resolved by the agent (DNS) when rendered into ACLs — VPP ACLs match
 * prefixes only, so an FQDN that resolves to nothing matches nothing.
 */
export const AddressObjectSchema = withUi(
  z.discriminatedUnion('type', [
    z.strictObject({
      type: z.literal('host'),
      address: withUi(ipAddress, { title: 'Address' }),
      ...common,
    }),
    z.strictObject({
      type: z.literal('network'),
      prefix: withUi(ipPrefix, { title: 'Prefix' }),
      ...common,
    }),
    z.strictObject({
      type: z.literal('range'),
      start: withUi(ipAddress, { title: 'First address' }),
      end: withUi(ipAddress, { title: 'Last address' }),
      ...common,
    }),
    z.strictObject({
      type: z.literal('fqdn'),
      fqdn: withUi(hostname, {
        title: 'FQDN',
        help: 'Resolved by the agent; IPv4 and IPv6 answers are used',
      }),
      ...common,
    }),
  ]),
  { title: 'Address object' },
);

/** Address group: members are address objects or other address groups (no cycles — semantic). */
export const AddressGroupSchema = withUi(
  z.strictObject({
    ...common,
    members: withUi(z.array(objectName).max(4096).default([]), {
      title: 'Members',
      widget: 'object-picker',
      help: 'Names from objects.addresses or objects.addressGroups; may be empty while the group is being built, but an ACL rule may not reference an empty group',
    }),
  }),
  { title: 'Address group' },
);

// ---------------------------------------------------------------------------------------------------------------
// Services
// ---------------------------------------------------------------------------------------------------------------

/** TCP flag match as VPP ACL takes it: `(flags & mask) == value`. */
export const TcpFlagsSchema = withUi(
  z.strictObject({
    mask: withUi(z.number().int().min(0).max(255), { title: 'Mask', help: 'bitmask over FIN…CWR' }),
    value: withUi(z.number().int().min(0).max(255), { title: 'Value' }),
  }),
  { title: 'TCP flags' },
);

const portFields = {
  destinationPorts: withUi(z.array(l4PortRange).max(64).default([]), {
    title: 'Destination ports',
    help: 'empty = any port',
  }),
  sourcePorts: withUi(z.array(l4PortRange).max(64).default([]), {
    title: 'Source ports',
    help: 'empty = any port',
  }),
};

/**
 * The protocol/port part of a service. Built twice (with and without `description`/`tags`) so ACL rules can carry
 * an inline service without object bookkeeping: `serviceVariants({})` vs `serviceVariants(common)`.
 */
function serviceVariants<T extends z.ZodRawShape>(extra: T) {
  return [
    z.strictObject({
      protocol: withUi(z.enum(['tcp', 'tcp-udp']), { title: 'Protocol' }),
      ...portFields,
      tcpFlags: TcpFlagsSchema.optional(),
      ...extra,
    }),
    z.strictObject({
      protocol: withUi(z.enum(['udp', 'sctp']), { title: 'Protocol' }),
      ...portFields,
      ...extra,
    }),
    z.strictObject({
      protocol: withUi(z.enum(['icmp', 'icmp6']), { title: 'Protocol' }),
      type: withUi(icmpNumber, { title: 'ICMP type', help: 'omit = any type' }).optional(),
      code: withUi(icmpNumber, {
        title: 'ICMP code',
        help: 'requires type; omit = any code',
      }).optional(),
      ...extra,
    }),
    z.strictObject({ protocol: withUi(z.literal('any'), { title: 'Protocol' }), ...extra }),
    z.strictObject({
      protocol: withUi(z.literal('other'), { title: 'Protocol' }),
      number: withUi(z.number().int().min(0).max(255), {
        title: 'IP protocol number',
        help: 'e.g. 47 = GRE, 50 = ESP, 89 = OSPF',
      }),
      ...extra,
    }),
  ] as const;
}

/** Protocol + ports only — used inline in ACL rules. */
export const ServiceSpecSchema = withUi(
  z.discriminatedUnion('protocol', [...serviceVariants({})]),
  { title: 'Service' },
);

/** A named service object. */
export const ServiceObjectSchema = withUi(
  z.discriminatedUnion('protocol', [...serviceVariants(common)]),
  { title: 'Service object' },
);

/** Service group: members are services or other service groups (no cycles — semantic). */
export const ServiceGroupSchema = withUi(
  z.strictObject({
    ...common,
    members: withUi(z.array(objectName).max(4096).default([]), {
      title: 'Members',
      widget: 'object-picker',
      help: 'Names from objects.services or objects.serviceGroups; may be empty while the group is being built, but an ACL rule may not reference an empty group',
    }),
  }),
  { title: 'Service group' },
);

// ---------------------------------------------------------------------------------------------------------------
// Schedules, zones, tags
// ---------------------------------------------------------------------------------------------------------------

export const WEEKDAYS = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'] as const;

/**
 * Schedule: `recurring` = weekdays + daily window (start < end, same day; an overnight window is two schedules),
 * `once` = absolute window in RFC 3339 with offset. Ordering is checked by `objects.schedule-time-range`.
 */
export const ScheduleSchema = withUi(
  z.discriminatedUnion('type', [
    z.strictObject({
      type: z.literal('recurring'),
      days: withUi(z.array(z.enum(WEEKDAYS)).min(1).max(7), { title: 'Days' }),
      start: withUi(timeOfDay, { title: 'Start' }),
      end: withUi(timeOfDay, { title: 'End' }),
      ...common,
    }),
    z.strictObject({
      type: z.literal('once'),
      start: withUi(z.iso.datetime({ offset: true }), { title: 'Start', widget: 'datetime' }),
      end: withUi(z.iso.datetime({ offset: true }), { title: 'End', widget: 'datetime' }),
      ...common,
    }),
  ]),
  { title: 'Schedule' },
);

/** Zone = named set of interfaces; an ACL attachment may target a zone instead of one interface. */
export const ZoneSchema = withUi(
  z.strictObject({
    ...common,
    interfaces: withUi(z.array(vppInterfaceName).max(1024).default([]), {
      title: 'Interfaces',
      widget: 'interface-picker',
    }),
  }),
  { title: 'Zone' },
);

export const TagSchema = withUi(
  z.strictObject({
    description: description.optional(),
    color: hexColor.optional(),
  }),
  { title: 'Tag' },
);

// ---------------------------------------------------------------------------------------------------------------
// Domain
// ---------------------------------------------------------------------------------------------------------------

const record = <T extends z.ZodType>(value: T) => z.record(objectName, value).default({});

export const ObjectsSchema = withUi(
  z.strictObject({
    addresses: withUi(record(AddressObjectSchema), {
      title: 'Addresses',
      group: 'Addresses',
      order: 1,
    }),
    addressGroups: withUi(record(AddressGroupSchema), {
      title: 'Address groups',
      group: 'Addresses',
      order: 2,
    }),
    services: withUi(record(ServiceObjectSchema), {
      title: 'Services',
      group: 'Services',
      order: 3,
    }),
    serviceGroups: withUi(record(ServiceGroupSchema), {
      title: 'Service groups',
      group: 'Services',
      order: 4,
    }),
    schedules: withUi(record(ScheduleSchema), {
      title: 'Schedules',
      group: 'Schedules',
      order: 5,
    }),
    zones: withUi(record(ZoneSchema), { title: 'Zones', group: 'Zones', order: 6 }),
    tags: withUi(record(TagSchema), { title: 'Tags', group: 'Tags', order: 7 }),
  }),
  {
    title: 'Objects',
    description:
      'Reusable address, address-group, service, service-group, schedule, zone and tag objects referenced by ACL and NAT.',
    order: 70,
  },
);

export type ObjectsConfig = z.infer<typeof ObjectsSchema>;
export type AddressObject = z.infer<typeof AddressObjectSchema>;
export type AddressGroup = z.infer<typeof AddressGroupSchema>;
export type ServiceSpec = z.infer<typeof ServiceSpecSchema>;
export type ServiceObject = z.infer<typeof ServiceObjectSchema>;
export type ServiceGroup = z.infer<typeof ServiceGroupSchema>;
export type Schedule = z.infer<typeof ScheduleSchema>;
export type Zone = z.infer<typeof ZoneSchema>;
export type Tag = z.infer<typeof TagSchema>;
