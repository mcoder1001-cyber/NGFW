import { z } from 'zod';
import { ipFamily, parseCidr } from '../ip.js';
import {
  asNumber,
  descriptionText,
  ipAddress,
  ipNetwork,
  ipv4Network,
  objectName,
  routerId,
  secretRefOf,
  uint32,
  vppInterfaceName,
  vrfName,
} from '../primitives.js';
import { withUi } from '../ui.js';
import { DEFAULT_VRF } from './vrfs.js';
import { routingL2Field } from './ext/bridge-l2.js'; // wave-A: F-bridge-l2

/**
 * `routing` — static routes, the routing-policy skeleton (prefix-lists, route-maps) and the dynamic protocols
 * (docs/04-api-datamodel.md; WBS D2.2, D3.x). Static routes and BFD are programmed in VPP by vrx-agent; bgp / ospf /
 * isis / rip are rendered into FRR configuration. Each protocol is a typed object that is *absent* when the
 * protocol is not enabled — `{}` is a valid, empty routing section.
 *
 * Shapes follow the FRR renderer contract of P12 §2 (D-045): collections with a natural key are records
 * (`bgp.neighbors` by address, `*.interfaces` by interface name, `ospf.areas` by area id, `redistribute` by source
 * protocol) so PATCH pointers are stable and duplicates are impossible; ordered lists (`static`, prefix-list rules,
 * route-map entries, `bgp.networks`, `bfd.sessions`) stay arrays and their keys are unique by refine / semantic rule.
 *
 * Guardrail (vdom.md #1): every route and protocol instance carries `vrf` (default `default`).
 * Cross-object rules live in `../semantic/routing.ts` (next-hop interface exists, VRF exists, referenced
 * prefix-lists / route-maps exist …); single-object consistency (address families, unique sequence numbers)
 * is enforced here with `.refine()`.
 */

/* ------------------------------------------------------------------------------------------- small helpers */

export const RouteAction = z.enum(['permit', 'deny']);
export type RouteAction = z.infer<typeof RouteAction>;

/** Sequence number of a prefix-list rule or route-map entry (FRR: 1–4294967295). */
const sequence = withUi(uint32.min(1), { title: 'Sequence', widget: 'number' });

/** Administrative distance / preference (1–255). */
const distance = withUi(z.number().int().min(1).max(255), { title: 'Distance', widget: 'number' });

/** Route metric as used by FRR `metric` / `set metric` (0–4294967295). */
const metric = withUi(uint32, { title: 'Metric', widget: 'number' });

/** Protocols whose routes can be redistributed into another protocol. */
export const RedistributeSource = z.enum(['connected', 'static', 'bgp', 'ospf', 'isis', 'rip']);
export type RedistributeSource = z.infer<typeof RedistributeSource>;

/** Options of one redistributed source; the key being present means "redistribute it". */
export const RedistributeOptionsSchema = z.strictObject({
  metric: withUi(metric.optional(), { order: 1 }),
  routeMap: withUi(objectName.optional(), { title: 'Route map', order: 2 }),
});

/**
 * `redistribute{ connected?, static?, … }` — a record keyed by source protocol (P12 §2, D-045), so a source can be
 * listed only once. A protocol cannot redistribute into itself: its own key is not accepted.
 */
function redistributeInto<S extends RedistributeSource>(self: S) {
  type Option = z.ZodOptional<typeof RedistributeOptionsSchema>;
  const shape = {} as Record<Exclude<RedistributeSource, S>, Option>;
  for (const [i, source] of RedistributeSource.options.entries()) {
    if (source === self) continue;
    shape[source as Exclude<RedistributeSource, S>] = withUi(RedistributeOptionsSchema.optional(), {
      title: `Redistribute ${source}`,
      order: i + 1,
    });
  }
  const sources = z.strictObject(shape);
  // every member is optional, so `{}` is a valid input (the cast only helps TS with the generic mapped type)
  return withUi(sources.prefault({} as z.input<typeof sources>), {
    title: 'Redistribute',
    help: 'routes of these sources are announced; presence of a key enables it',
  });
}

/** BGP standard community `AS:NN` or a well-known name. */
export const bgpCommunity = withUi(
  z
    .string()
    .regex(
      /^(?:(?:0|[1-9][0-9]{0,4}):(?:0|[1-9][0-9]{0,4})|internet|no-export|no-advertise|local-AS)$/,
      'expected a community like 65000:100 or a well-known name (no-export …)',
    )
    .refine((c) => c.split(':').every((part) => !/^[0-9]+$/.test(part) || Number(part) <= 65535), {
      message: 'community numbers must be 0–65535',
    }),
  { title: 'Community' },
);

/** IS-IS network entity title: `AFI.AreaID.SystemID.SEL`, e.g. `49.0001.1921.6800.1001.00`. */
export const isisNet = withUi(
  z
    .string()
    .regex(
      /^[0-9a-fA-F]{2}(?:\.[0-9a-fA-F]{4}){3,9}\.00$/,
      'expected a NET like 49.0001.1921.6800.1001.00',
    ),
  { title: 'NET', help: 'network entity title, e.g. 49.0001.1921.6800.1001.00' },
);

/**
 * OSPF area id — decimal (`"0"`, `"51"`) or dotted quad (`"0.0.0.51"`), always a JSON string so the record key
 * (`/routing/ospf/areas/<id>`) and the references to it have one type (a number|string union has no protobuf
 * projection). `ospfAreaNumber()` normalises both spellings; `"0"` and `"0.0.0.0"` are the same area.
 */
export const ospfAreaId = withUi(
  z.union([
    z
      .string()
      .regex(/^(?:0|[1-9][0-9]{0,9})$/, 'expected a decimal area id or a dotted quad')
      .refine((id) => Number(id) <= 4294967295, 'area id must fit in 32 bits'),
    z.ipv4(),
  ]),
  { title: 'Area', help: 'decimal or dotted quad, e.g. 0 or 0.0.0.51' },
);

/** `'51'` and `'0.0.0.51'` → 51. */
export function ospfAreaNumber(id: string): number {
  if (!id.includes('.')) return Number(id);
  return id.split('.').reduce((acc, octet) => acc * 256 + Number(octet), 0);
}

const uniqueSequences = (items: readonly { seq: number }[]): boolean =>
  new Set(items.map((i) => i.seq)).size === items.length;
const SEQ_UNIQUE = 'sequence numbers must be unique';

/* ------------------------------------------------------------------------------------------- static routes */

export const NextHopSchema = z
  .strictObject({
    address: withUi(ipAddress.optional(), {
      title: 'Next-hop address',
      help: 'gateway address; may be omitted for a directly connected (interface) route',
      order: 1,
    }),
    interface: withUi(vppInterfaceName.optional(), {
      title: 'Interface',
      help: 'egress interface; required when no address is given',
      order: 2,
    }),
    weight: withUi(z.number().int().min(1).max(255).default(1), {
      title: 'Weight',
      help: 'relative share for equal-cost multipath',
      order: 3,
    }),
    // Feature keys (sub-schema in domains/ext/<slug>.ts): one key line under the feature's anchor.
    // wave-A: F-vrf-static-ecmp
  })
  .refine((hop) => hop.address !== undefined || hop.interface !== undefined, {
    message: 'a next hop needs an address, an interface or both',
    path: ['address'],
  });
export type NextHopConfig = z.infer<typeof NextHopSchema>;

export const StaticRouteSchema = z
  .strictObject({
    prefix: withUi(ipNetwork, {
      title: 'Destination',
      help: 'network prefix, e.g. 0.0.0.0/0 or 2001:db8::/32',
      order: 1,
    }),
    vrf: withUi(vrfName.default(DEFAULT_VRF), { title: 'VRF', order: 2 }),
    nextHops: withUi(z.array(NextHopSchema).max(32).default([]), {
      title: 'Next hops',
      help: 'more than one = equal-cost multipath; empty only for a blackhole route',
      order: 3,
    }),
    blackhole: withUi(z.boolean().default(false), {
      title: 'Blackhole',
      help: 'silently drop matching traffic (null route); requires an empty next-hop list',
      order: 4,
    }),
    distance: withUi(distance.default(1), {
      title: 'Distance',
      help: 'administrative distance (1–255)',
      order: 5,
    }),
    description: withUi(descriptionText.optional(), { title: 'Description', order: 6 }),
    // Feature keys (sub-schema in domains/ext/<slug>.ts): one key line under the feature's anchor.
    // wave-A: F-vrf-static-ecmp
    // wave-A: P12
  })
  .refine((route) => route.blackhole === (route.nextHops.length === 0), {
    message: 'a route needs at least one next hop, unless it is a blackhole route (then none)',
    path: ['nextHops'],
  })
  .refine(
    (route) => {
      const family = parseCidr(route.prefix)?.family;
      return route.nextHops.every(
        (hop) => hop.address === undefined || ipFamily(hop.address) === family,
      );
    },
    {
      message: 'next-hop addresses must be in the same address family as the prefix',
      path: ['nextHops'],
    },
  );
export type StaticRouteConfig = z.infer<typeof StaticRouteSchema>;

/* ---------------------------------------------------------------------------------------- routing policy */

const MAX_LENGTH = { 4: 32, 6: 128 } as const;

/**
 * One prefix-list rule. Length ranges follow FRR (`lib/plist.c`): `len < ge`, `len ≤ le ≤ max`, `ge ≤ le`, with
 * `max` = 32 for IPv4 and 128 for IPv6 — anything else is rejected here instead of by `vtysh -C` (review L5).
 */
export const PrefixListRuleSchema = z
  .strictObject({
    seq: withUi(sequence, { order: 1 }),
    action: withUi(RouteAction, { title: 'Action', widget: 'select', order: 2 }),
    prefix: withUi(ipNetwork, { title: 'Prefix', order: 3 }),
    ge: withUi(z.number().int().min(1).max(128).optional(), {
      title: 'Minimum length (ge)',
      help: 'match prefixes at least this long (longer than the prefix itself)',
      order: 4,
    }),
    le: withUi(z.number().int().min(0).max(128).optional(), {
      title: 'Maximum length (le)',
      help: 'match prefixes at most this long',
      order: 5,
    }),
  })
  .superRefine((rule, ctx) => {
    const prefix = parseCidr(rule.prefix);
    if (prefix === undefined) return; // reported by `prefix`
    const max = MAX_LENGTH[prefix.family];
    const fail = (path: 'ge' | 'le', message: string) =>
      ctx.addIssue({ code: 'custom', path: [path], message });
    if (rule.ge !== undefined && (rule.ge <= prefix.length || rule.ge > max))
      fail('ge', `ge must be greater than ${prefix.length} and at most ${max}`);
    if (rule.le !== undefined && (rule.le < prefix.length || rule.le > max))
      fail('le', `le must be between ${prefix.length} and ${max}`);
    if (rule.ge !== undefined && rule.le !== undefined && rule.ge > rule.le)
      fail('le', 'ge must not exceed le');
  });
export type PrefixListRuleConfig = z.infer<typeof PrefixListRuleSchema>;

export const PrefixFamily = z.enum(['ipv4', 'ipv6']);

/**
 * `routing.policy.prefixLists.<name>` — the ordered rules of one list (P12 §2) plus its family and description.
 * An object rather than a bare array (P12's sketch) because the document is projected 1:1 onto protobuf, whose
 * map values cannot be `repeated` (D-042/D-061; logged in P02a-contract.md).
 */
export const PrefixListSchema = z
  .strictObject({
    description: withUi(descriptionText.optional(), { title: 'Description', order: 1 }),
    family: withUi(PrefixFamily.default('ipv4'), {
      title: 'Address family',
      help: 'ip (IPv4) or ipv6 prefix-list',
      widget: 'select',
      order: 2,
    }),
    rules: withUi(z.array(PrefixListRuleSchema).max(1024).default([]), {
      title: 'Rules',
      itemKey: ['seq'],
      order: 3,
    }),
  })
  .refine((list) => uniqueSequences(list.rules), { message: SEQ_UNIQUE, path: ['rules'] })
  .refine(
    (list) =>
      list.rules.every(
        (rule) => parseCidr(rule.prefix)?.family === (list.family === 'ipv4' ? 4 : 6),
      ),
    { message: 'every rule prefix must match the address family of the list', path: ['rules'] },
  );
export type PrefixListConfig = z.infer<typeof PrefixListSchema>;

/**
 * FRR AS-path regular expression: digits and the regex/FRR metacharacters only (`^$_.*+?()[]|{},-` and space).
 * Rendered verbatim into `bgp as-path access-list … permit <regex>`, so CR/LF and every other character that
 * could end the line or start a new FRR statement are rejected (review M1, D-049).
 */
export const bgpAsPathRegex = withUi(
  z
    .string()
    .min(1)
    .max(255)
    .regex(
      /^[0-9_.*+?^$()|[\]{}, -]+$/,
      'digits and ^ $ _ . * + ? ( ) [ ] { } | , - and space only',
    ),
  { title: 'AS path regex', help: 'FRR as-path regular expression, e.g. ^65000_' },
);

export const RouteMapMatchSchema = z.strictObject({
  prefixList: withUi(objectName.optional(), {
    title: 'Prefix list',
    help: 'match the destination prefix against a prefix list',
    order: 1,
  }),
  nextHopPrefixList: withUi(objectName.optional(), {
    title: 'Next-hop prefix list',
    help: 'match the next hop against a prefix list',
    order: 2,
  }),
  interface: withUi(vppInterfaceName.optional(), { title: 'Interface', order: 3 }),
  community: withUi(bgpCommunity.optional(), { title: 'Community', order: 4 }),
  asPath: withUi(bgpAsPathRegex.optional(), { order: 5 }),
  metric: withUi(metric.optional(), { order: 6 }),
  tag: withUi(uint32.optional(), { title: 'Tag', order: 7 }),
});

/** `set` clauses; names follow P12 §2 (`localPref`, `med`, `community`, `nextHop`) plus FRR extras. */
export const RouteMapSetSchema = z.strictObject({
  localPref: withUi(uint32.optional(), { title: 'Local preference', order: 1 }),
  med: withUi(metric.optional(), {
    title: 'MED / metric',
    help: 'BGP multi-exit discriminator (FRR `set metric`)',
    order: 2,
  }),
  weight: withUi(z.number().int().min(0).max(65535).optional(), { title: 'Weight', order: 3 }),
  nextHop: withUi(ipAddress.optional(), { title: 'Next hop', order: 4 }),
  community: withUi(z.array(bgpCommunity).max(32).optional(), { title: 'Communities', order: 5 }),
  communityAdditive: withUi(z.boolean().default(false), {
    title: 'Additive',
    help: 'append the communities instead of replacing them',
    order: 6,
  }),
  asPathPrepend: withUi(z.array(asNumber).max(16).optional(), {
    title: 'AS path prepend',
    order: 7,
  }),
  tag: withUi(uint32.optional(), { title: 'Tag', order: 8 }),
});

export const RouteMapEntrySchema = z.strictObject({
  seq: withUi(sequence, { order: 1 }),
  action: withUi(RouteAction, { title: 'Action', widget: 'select', order: 2 }),
  description: withUi(descriptionText.optional(), { title: 'Description', order: 3 }),
  match: withUi(RouteMapMatchSchema.prefault({}), {
    title: 'Match',
    help: 'all given conditions must match; an empty match matches everything',
    order: 4,
  }),
  set: withUi(RouteMapSetSchema.prefault({}), { title: 'Set', order: 5 }),
});
export type RouteMapEntryConfig = z.infer<typeof RouteMapEntrySchema>;

/** `routing.policy.routeMaps.<name>` — ordered entries (P12 §2) plus a description; an object for the same reason. */
export const RouteMapSchema = z
  .strictObject({
    description: withUi(descriptionText.optional(), { title: 'Description', order: 1 }),
    entries: withUi(z.array(RouteMapEntrySchema).max(1024).default([]), {
      title: 'Entries',
      itemKey: ['seq'],
      order: 2,
    }),
  })
  .refine((map) => uniqueSequences(map.entries), { message: SEQ_UNIQUE, path: ['entries'] });
export type RouteMapConfig = z.infer<typeof RouteMapSchema>;

/** `routing.policy` — the filters referenced by BGP/OSPF/… (P12 §2). */
export const RoutingPolicySchema = z.strictObject({
  prefixLists: withUi(z.record(objectName, PrefixListSchema).default({}), {
    title: 'Prefix lists',
    order: 1,
  }),
  routeMaps: withUi(z.record(objectName, RouteMapSchema).default({}), {
    title: 'Route maps',
    order: 2,
  }),
});
export type RoutingPolicyConfig = z.infer<typeof RoutingPolicySchema>;

/* -------------------------------------------------------------------------------------------------- BGP */

/** Per-peer settings of one address family (P12 §2 `afi.<family>`). */
export const BgpAddressFamilySchema = z.strictObject({
  enabled: withUi(z.boolean().default(true), { title: 'Enabled', order: 1 }),
  routeMapIn: withUi(objectName.optional(), { title: 'Route map in', order: 2 }),
  routeMapOut: withUi(objectName.optional(), { title: 'Route map out', order: 3 }),
  prefixListIn: withUi(objectName.optional(), { title: 'Prefix list in', order: 4 }),
  prefixListOut: withUi(objectName.optional(), { title: 'Prefix list out', order: 5 }),
  nextHopSelf: withUi(z.boolean().default(false), { title: 'Next-hop self', order: 6 }),
  softReconfig: withUi(z.boolean().default(false), {
    title: 'Soft reconfiguration inbound',
    help: 'keep an unmodified copy of received routes so policy changes apply without a session reset',
    order: 7,
  }),
  maximumPrefixes: withUi(uint32.min(1).optional(), {
    title: 'Maximum prefixes',
    help: 'tear the session down above this many prefixes',
    order: 8,
  }),
  defaultOriginate: withUi(z.boolean().default(false), { title: 'Default originate', order: 9 }),
});
export type BgpAddressFamilyConfig = z.infer<typeof BgpAddressFamilySchema>;

export const BGP_AFIS = ['ipv4Unicast', 'ipv6Unicast'] as const;
export type BgpAfi = (typeof BGP_AFIS)[number];

/** `afi{ ipv4Unicast?, ipv6Unicast? }` — an absent family is not activated for the peer. */
export const BgpAfiSchema = z.strictObject({
  ipv4Unicast: withUi(BgpAddressFamilySchema.optional(), { title: 'IPv4 unicast', order: 1 }),
  ipv6Unicast: withUi(BgpAddressFamilySchema.optional(), { title: 'IPv6 unicast', order: 2 }),
});

/** Fields shared by neighbours and peer groups (a neighbour inherits what its peer group sets). */
const bgpPeerFields = {
  remoteAs: withUi(asNumber.optional(), { title: 'Remote AS', order: 2 }),
  description: withUi(descriptionText.optional(), { title: 'Description', order: 3 }),
  updateSource: withUi(z.union([ipAddress, vppInterfaceName]).optional(), {
    title: 'Update source',
    help: 'local address or interface for the session',
    order: 4,
  }),
  ebgpMultihop: withUi(z.number().int().min(1).max(255).optional(), {
    title: 'eBGP multihop',
    help: 'TTL for eBGP sessions that are not directly connected',
    order: 5,
  }),
  passwordRef: withUi(secretRefOf('password').optional(), {
    title: 'MD5 password',
    help: 'TCP MD5 session password in the secret store, e.g. password/bgp-upstream',
    order: 6,
  }),
  keepaliveSec: withUi(z.number().int().min(1).max(65535).optional(), {
    title: 'Keepalive (s)',
    order: 7,
  }),
  holdTimeSec: withUi(z.number().int().min(3).max(65535).optional(), {
    title: 'Hold time (s)',
    order: 8,
  }),
  bfd: withUi(z.boolean().default(false), {
    title: 'BFD',
    help: 'fast failure detection',
    order: 9,
  }),
  afi: withUi(BgpAfiSchema.prefault({}), { title: 'Address families', order: 10 }),
} as const;

const holdAboveKeepalive = (peer: {
  keepaliveSec?: number | undefined;
  holdTimeSec?: number | undefined;
}) =>
  peer.keepaliveSec === undefined ||
  peer.holdTimeSec === undefined ||
  peer.holdTimeSec > peer.keepaliveSec;
const HOLD_ABOVE_KEEPALIVE = 'hold time must be greater than the keepalive interval';

export const BgpPeerGroupSchema = z
  .strictObject(bgpPeerFields)
  .refine(holdAboveKeepalive, { message: HOLD_ABOVE_KEEPALIVE, path: ['holdTimeSec'] });
export type BgpPeerGroupConfig = z.infer<typeof BgpPeerGroupSchema>;

/** A neighbour; its address is the record key (`/routing/bgp/neighbors/10.0.0.1`, P12 §2, D-045). */
export const BgpNeighborSchema = z
  .strictObject({
    peerGroup: withUi(objectName.optional(), {
      title: 'Peer group',
      help: 'inherit settings from this peer group',
      order: 1,
    }),
    shutdown: withUi(z.boolean().default(false), {
      title: 'Shutdown',
      help: 'keep the configuration but do not bring the session up',
      order: 11,
    }),
    ...bgpPeerFields,
  })
  .refine(holdAboveKeepalive, { message: HOLD_ABOVE_KEEPALIVE, path: ['holdTimeSec'] })
  .refine((n) => n.remoteAs !== undefined || n.peerGroup !== undefined, {
    message: 'a neighbour needs remoteAs unless it inherits one from a peer group',
    path: ['remoteAs'],
  });
export type BgpNeighborConfig = z.infer<typeof BgpNeighborSchema>;

export const BgpNetworkSchema = z.strictObject({
  prefix: withUi(ipNetwork, { title: 'Prefix', order: 1 }),
  routeMap: withUi(objectName.optional(), { title: 'Route map', order: 2 }),
});

export const BgpSchema = z.strictObject({
  asn: withUi(asNumber, { title: 'Local AS', order: 1 }),
  routerId: withUi(routerId.optional(), { title: 'Router ID', order: 2 }),
  vrf: withUi(vrfName.default(DEFAULT_VRF), { title: 'VRF', order: 3 }),
  peerGroups: withUi(z.record(objectName, BgpPeerGroupSchema).default({}), {
    title: 'Peer groups',
    order: 4,
  }),
  neighbors: withUi(
    z.record(withUi(ipAddress, { title: 'Neighbour address' }), BgpNeighborSchema).default({}),
    {
      title: 'Neighbours',
      help: 'keyed by neighbour address, e.g. 10.0.0.1 or 2001:db8::1',
      order: 5,
    },
  ),
  networks: withUi(z.array(BgpNetworkSchema).max(4096).default([]), {
    title: 'Networks',
    help: 'prefixes originated by this router',
    itemKey: ['prefix'],
    order: 6,
  }),
  redistribute: withUi(redistributeInto('bgp'), { order: 7 }),
  gracefulRestart: withUi(z.boolean().default(false), { title: 'Graceful restart', order: 8 }),
  ebgpRequiresPolicy: withUi(z.boolean().default(true), {
    title: 'eBGP requires policy',
    help: 'RFC 8212: eBGP sessions without an in/out policy accept and announce nothing',
    order: 9,
  }),
});
export type BgpConfig = z.infer<typeof BgpSchema>;

/* -------------------------------------------------------------------------------------------------- OSPF */

/** An OSPF area; its id is the record key (`/routing/ospf/areas/0.0.0.51`). */
export const OspfAreaSchema = z.strictObject({
  type: withUi(z.enum(['normal', 'stub', 'nssa']).default('normal'), {
    title: 'Area type',
    widget: 'select',
    order: 1,
  }),
  noSummary: withUi(z.boolean().default(false), {
    title: 'No summary',
    help: 'totally stubby / totally NSSA: do not inject inter-area routes',
    order: 2,
  }),
});

/** OSPF on one interface; the interface name is the record key (`/routing/ospf/interfaces/loop0`). */
export const OspfInterfaceSchema = z.strictObject({
  area: withUi(ospfAreaId, { order: 1 }),
  cost: withUi(z.number().int().min(1).max(65535).optional(), { title: 'Cost', order: 2 }),
  passive: withUi(z.boolean().default(false), {
    title: 'Passive',
    help: 'advertise the subnet but form no adjacencies',
    order: 3,
  }),
  networkType: withUi(
    z.enum(['broadcast', 'point-to-point', 'non-broadcast', 'point-to-multipoint']).optional(),
    { title: 'Network type', widget: 'select', order: 4 },
  ),
  helloIntervalSec: withUi(z.number().int().min(1).max(65535).optional(), {
    title: 'Hello interval (s)',
    order: 5,
  }),
  deadIntervalSec: withUi(z.number().int().min(1).max(65535).optional(), {
    title: 'Dead interval (s)',
    order: 6,
  }),
  priority: withUi(z.number().int().min(0).max(255).optional(), { title: 'DR priority', order: 7 }),
  bfd: withUi(z.boolean().default(false), { title: 'BFD', order: 8 }),
  // wave-BC: F-ospf
  // wave-BC: F-bfd-redistribution
});

/** Record keyed by the interface an IGP runs on (parent or `<parent>.<id>`). */
const igpInterfaces = <T extends z.ZodType>(item: T) =>
  withUi(z.record(vppInterfaceName, item).default({}), {
    title: 'Interfaces',
    help: 'keyed by interface name',
  });

export const OspfSchema = z
  .strictObject({
    routerId: withUi(routerId.optional(), { title: 'Router ID', order: 1 }),
    vrf: withUi(vrfName.default(DEFAULT_VRF), { title: 'VRF', order: 2 }),
    areas: withUi(z.record(ospfAreaId, OspfAreaSchema).default({}), {
      title: 'Areas',
      help: 'keyed by area id, e.g. 0 or 0.0.0.51',
      order: 3,
    }),
    interfaces: withUi(igpInterfaces(OspfInterfaceSchema), { order: 4 }),
    redistribute: withUi(redistributeInto('ospf'), { order: 5 }),
    defaultInformationOriginate: withUi(z.enum(['off', 'on', 'always']).default('off'), {
      title: 'Default route origination',
      widget: 'select',
      order: 6,
    }),
  })
  .refine(
    (ospf) =>
      new Set(Object.keys(ospf.areas).map(ospfAreaNumber)).size === Object.keys(ospf.areas).length,
    {
      message: 'area ids must be unique (0 and 0.0.0.0 are the same area)',
      path: ['areas'],
    },
  );
export type OspfConfig = z.infer<typeof OspfSchema>;

/* ------------------------------------------------------------------------------------------------- IS-IS */

export const IsisLevel = z.enum(['level-1', 'level-2', 'level-1-2']);

export const IsisInterfaceSchema = z.strictObject({
  passive: withUi(z.boolean().default(false), { title: 'Passive', order: 2 }),
  metric: withUi(z.number().int().min(1).max(16777215).optional(), { title: 'Metric', order: 3 }),
  circuitType: withUi(IsisLevel.optional(), { title: 'Circuit type', widget: 'select', order: 4 }),
  networkType: withUi(z.enum(['broadcast', 'point-to-point']).optional(), {
    title: 'Network type',
    widget: 'select',
    order: 5,
  }),
  bfd: withUi(z.boolean().default(false), { title: 'BFD', order: 6 }),
  // wave-BC: F-isis-rip
  // wave-BC: F-bfd-redistribution
});

export const IsisSchema = z.strictObject({
  net: withUi(isisNet, { order: 1 }),
  level: withUi(IsisLevel.default('level-1-2'), { title: 'IS type', widget: 'select', order: 2 }),
  vrf: withUi(vrfName.default(DEFAULT_VRF), { title: 'VRF', order: 3 }),
  interfaces: withUi(igpInterfaces(IsisInterfaceSchema), { order: 4 }),
  redistribute: withUi(redistributeInto('isis'), { order: 5 }),
  // wave-BC: F-isis-rip
});
export type IsisConfig = z.infer<typeof IsisSchema>;

/* --------------------------------------------------------------------------------------------------- RIP */

export const RipInterfaceSchema = z.strictObject({
  passive: withUi(z.boolean().default(false), { title: 'Passive', order: 2 }),
  // wave-BC: F-isis-rip
});

export const RipSchema = z.strictObject({
  vrf: withUi(vrfName.default(DEFAULT_VRF), { title: 'VRF', order: 1 }),
  networks: withUi(z.array(ipv4Network).max(256).default([]), {
    title: 'Networks',
    help: 'run RIP on interfaces inside these prefixes',
    order: 2,
  }),
  interfaces: withUi(igpInterfaces(RipInterfaceSchema), { order: 3 }),
  redistribute: withUi(redistributeInto('rip'), { order: 4 }),
  defaultMetric: withUi(z.number().int().min(1).max(16).default(1), {
    title: 'Default metric',
    order: 5,
  }),
  // wave-BC: F-isis-rip
});
export type RipConfig = z.infer<typeof RipSchema>;

/* --------------------------------------------------------------------------------------------------- BFD */

/** BFD interval in microseconds (VPP `bfd udp session add … desired-min-tx / required-min-rx`). */
const bfdIntervalUs = withUi(z.number().int().min(10000).max(60000000), {
  title: 'Interval (µs)',
  widget: 'number',
});

export const BfdSessionSchema = z
  .strictObject({
    interface: withUi(vppInterfaceName, { title: 'Interface', order: 1 }),
    localAddress: withUi(ipAddress, { title: 'Local address', order: 2 }),
    peerAddress: withUi(ipAddress, { title: 'Peer address', order: 3 }),
    desiredMinTxUs: withUi(bfdIntervalUs.default(300000), {
      title: 'Desired min TX (µs)',
      order: 4,
    }),
    requiredMinRxUs: withUi(bfdIntervalUs.default(300000), {
      title: 'Required min RX (µs)',
      order: 5,
    }),
    detectMultiplier: withUi(z.number().int().min(1).max(255).default(3), {
      title: 'Detect multiplier',
      order: 6,
    }),
    enabled: withUi(z.boolean().default(true), { title: 'Enabled', order: 7 }),
    // wave-BC: F-bfd-redistribution
  })
  .refine((s) => ipFamily(s.localAddress) === ipFamily(s.peerAddress), {
    message: 'local and peer address must be in the same address family',
    path: ['peerAddress'],
  });
export type BfdSessionConfig = z.infer<typeof BfdSessionSchema>;

export const BfdSchema = z.strictObject({
  sessions: withUi(z.array(BfdSessionSchema).max(1024).default([]), {
    title: 'Sessions',
    itemKey: ['interface', 'peerAddress'],
    order: 1,
  }),
  // wave-BC: F-bfd-redistribution
});
export type BfdConfig = z.infer<typeof BfdSchema>;

/* -------------------------------------------------------------------------------------------------- root */

export const RoutingSchema = withUi(
  z.strictObject({
    static: withUi(z.array(StaticRouteSchema).max(65536).default([]), {
      title: 'Static routes',
      itemKey: ['vrf', 'prefix'],
      group: 'static',
      order: 1,
    }),
    policy: withUi(RoutingPolicySchema.prefault({}), {
      title: 'Routing policy',
      help: 'prefix lists and route maps',
      group: 'policy',
      order: 2,
    }),
    bgp: withUi(BgpSchema.optional(), { title: 'BGP', group: 'dynamic', order: 3 }),
    ospf: withUi(OspfSchema.optional(), { title: 'OSPF', group: 'dynamic', order: 4 }),
    isis: withUi(IsisSchema.optional(), { title: 'IS-IS', group: 'dynamic', order: 5 }),
    rip: withUi(RipSchema.optional(), { title: 'RIP', group: 'dynamic', order: 6 }),
    bfd: withUi(BfdSchema.optional(), { title: 'BFD', group: 'dynamic', order: 7 }),
    // Feature keys (sub-schema in domains/ext/<slug>.ts): one key line under the feature's anchor.
    // wave-BC: F-ospf
    // wave-BC: F-isis-rip
    // wave-BC: F-mpls-srmpls
    // wave-BC: F-igmp-mfib
    // wave-BC: F-srv6
    // wave-A: F-bridge-l2
    l2: routingL2Field,
    // wave-A: F-neighbors-ra
    // wave-A: F-rpf-adl-pbr
  }),
  {
    title: 'Routing',
    description: 'Static routes and dynamic routing protocols (FRR).',
    order: 50,
  },
);

export type RoutingConfig = z.infer<typeof RoutingSchema>;
