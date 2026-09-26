import { z } from 'zod';
import { ipAddress, macAddress, vlanId, vppInterfaceName, vrfName } from '../../primitives.js';
import { withUi } from '../../ui.js';
import { WEEKDAYS } from '../objects.js';
import { DEFAULT_VRF } from '../vrfs.js';

/**
 * L2 switching (F-bridge-l2, WBS D1.6; placement D-109 (c)): VPP bridge domains, L2 cross-connects, L3
 * cross-connects, VLAN tag rewrite on L2 ports and the `mactime` time-range MAC filter.
 *
 * The model has two halves (never a new root key — D-109 c):
 * - **per-port leaves** `interfaces.<if>.l2` / `interfaces.<if>.subinterfaces.<id>.l2` ({@link BridgeL2PortSchema}):
 *   which bridge domain the (sub-)interface is a member of, its split-horizon group, BVI / uu-fwd role, its VLAN
 *   tag rewrite and whether the MAC filter runs on it;
 * - **the container** `routing.l2` ({@link BridgeL2Schema}): the bridge-domain records (keyed by name, each with
 *   the numeric VPP id that `tunnels.*.bridgeDomain` references), the L2 and L3 cross-connects (keyed by the
 *   receiving interface) and the MAC-filter devices (keyed by name).
 *
 * Cross-field rules (one membership per port, no L3 on a bridged port except the BVI, one loopback BVI per bridge
 * domain, xconnect rx ≠ tx, tag rewrite only on an L2 port, …) are semantic validators in
 * `../../semantic/bridge-l2.ts` (`routing.bridge-l2-*` / `interfaces.bridge-l2-*`).
 */

const UI_GROUP = 'bridge-l2';

/** VPP bridge-domain id: 1–16777215 (`L2_BD_ID_MAX`; 0 is VPP's default domain, ~0 is reserved). */
export const bridgeDomainId = withUi(z.number().int().min(1).max(16777215), {
  title: 'Bridge-domain ID',
  help: 'VPP bridge-domain id (1–16777215); tunnels reference the bridge domain by this number',
  widget: 'number',
});

/**
 * Name of a bridge-domain record (the key of `routing.l2.bridgeDomains`). At most 32 characters: the agent stores
 * `<owner>:<id>/<name>` in the 63-byte VPP bridge-domain tag, so Retrieve can name the domain again.
 */
export const bridgeDomainName = withUi(
  z
    .string()
    .min(1)
    .max(32)
    .regex(/^[A-Za-z0-9][A-Za-z0-9_.-]*$/, 'letters, digits, `_`, `.` and `-` only'),
  { title: 'Bridge domain', widget: 'bridge-domain-picker' },
);

/** Name of a MAC-filter device (the key of `routing.l2.macFilters`); VPP stores `<owner>:<name>` (≤ 63 bytes). */
export const macFilterName = withUi(
  z
    .string()
    .min(1)
    .max(48)
    .regex(/^[A-Za-z0-9][A-Za-z0-9_.-]*$/, 'letters, digits, `_`, `.` and `-` only'),
  { title: 'Device name' },
);

/** VLAN tag rewrite operations (VPP `l2_interface_vlan_tag_rewrite`, `l2_vtr_op_t`). */
export const VlanTagRewriteOp = z.enum([
  'push-1',
  'push-2',
  'pop-1',
  'pop-2',
  'translate-1-1',
  'translate-1-2',
  'translate-2-1',
  'translate-2-2',
]);
export type VlanTagRewriteOp = z.infer<typeof VlanTagRewriteOp>;

/** How many tags an operation writes (tag1 / tag1 + tag2); pop operations write none. */
export function tagRewriteTagCount(op: VlanTagRewriteOp): 0 | 1 | 2 {
  switch (op) {
    case 'pop-1':
    case 'pop-2':
      return 0;
    case 'push-1':
    case 'translate-1-1':
    case 'translate-2-1':
      return 1;
    default:
      return 2;
  }
}

/**
 * VLAN tag rewrite on an L2 port (a bridge member or an L2 cross-connect rx), applied on ingress and reversed on
 * egress. Typical: a VLAN sub-interface bridged with `pop-1` strips its tag so that it meets untagged ports.
 */
export const VlanTagRewriteSchema = z
  .strictObject({
    op: withUi(VlanTagRewriteOp, {
      title: 'Operation',
      help: 'push-1/2 add tags, pop-1/2 remove tags, translate-N-M replace N tags by M tags',
      widget: 'select',
      order: 1,
    }),
    tag1: withUi(vlanId.optional(), {
      title: 'Tag 1',
      help: 'outer VLAN id written by push / translate',
      order: 2,
    }),
    tag2: withUi(vlanId.optional(), {
      title: 'Tag 2',
      help: 'inner VLAN id written by push-2 / translate-1-2 / translate-2-2',
      order: 3,
    }),
    dot1ad: withUi(z.boolean().default(false), {
      title: '802.1ad outer tag',
      help: 'pushed outer tag uses 0x88a8 instead of 0x8100 (push and translate only)',
      order: 4,
    }),
  })
  .superRefine((v, ctx) => {
    const n = tagRewriteTagCount(v.op);
    if (n >= 1 && v.tag1 === undefined) {
      ctx.addIssue({ code: 'custom', path: ['tag1'], message: `${v.op} needs tag1` });
    }
    if (n === 2 && v.tag2 === undefined) {
      ctx.addIssue({ code: 'custom', path: ['tag2'], message: `${v.op} needs tag2` });
    }
    if (n < 1 && v.tag1 !== undefined) {
      ctx.addIssue({ code: 'custom', path: ['tag1'], message: `${v.op} writes no tag` });
    }
    if (n < 2 && v.tag2 !== undefined) {
      ctx.addIssue({ code: 'custom', path: ['tag2'], message: `${v.op} writes at most one tag` });
    }
    if (n === 0 && v.dot1ad) {
      ctx.addIssue({ code: 'custom', path: ['dot1ad'], message: `${v.op} pushes no tag` });
    }
  });
export type VlanTagRewriteConfig = z.infer<typeof VlanTagRewriteSchema>;

/**
 * `interfaces.<if>.l2` and `interfaces.<if>.subinterfaces.<id>.l2`: the L2 role of one (sub-)interface.
 * Present with `bridgeDomain` = a bridge member; `tagRewrite` also applies to an L2 cross-connect rx
 * (`routing.l2.xconnects.<if>`); `macFilter` runs the mactime filter on a parent interface.
 */
export const BridgeL2PortSchema = z.strictObject({
  bridgeDomain: withUi(bridgeDomainName.optional(), {
    title: 'Bridge domain',
    help: 'name of the bridge domain (routing.l2.bridgeDomains) this interface is a member of; absent = not bridged',
    group: UI_GROUP,
    order: 1,
  }),
  shg: withUi(z.number().int().min(0).max(255).default(0), {
    title: 'Split-horizon group',
    help: '0 = none; members of the same group never forward to each other',
    group: UI_GROUP,
    order: 2,
  }),
  bvi: withUi(z.boolean().default(false), {
    title: 'BVI',
    help: 'bridge virtual interface: the routed (L3) port of the bridge domain; a loopback, one per domain',
    group: UI_GROUP,
    order: 3,
  }),
  uuFwd: withUi(z.boolean().default(false), {
    title: 'Unknown-unicast forwarder',
    help: 'send unknown-unicast frames to this member only (uu-fwd port)',
    group: UI_GROUP,
    order: 4,
  }),
  tagRewrite: withUi(VlanTagRewriteSchema.optional(), {
    title: 'VLAN tag rewrite',
    help: 'on a bridge member or an L2 cross-connect rx',
    group: UI_GROUP,
    order: 5,
  }),
  macFilter: withUi(z.boolean().default(false), {
    title: 'Time-range MAC filter',
    help: 'run the mactime filter (routing.l2.macFilters) on frames received here; parent interfaces only',
    group: UI_GROUP,
    order: 6,
  }),
});
export type BridgeL2PortConfig = z.infer<typeof BridgeL2PortSchema>;

/** A static L2 FIB entry of a bridge domain (`l2fib_add_del`, static). */
export const BridgeStaticMacSchema = z.strictObject({
  mac: withUi(macAddress, { title: 'MAC address', order: 1 }),
  interface: withUi(vppInterfaceName, {
    title: 'Interface',
    help: 'member of this bridge domain the MAC is reached through',
    order: 2,
  }),
});
export type BridgeStaticMacConfig = z.infer<typeof BridgeStaticMacSchema>;

/** One bridge domain (`routing.l2.bridgeDomains.<name>`; VPP `bridge_domain_add_del_v2`). */
export const BridgeDomainSchema = z.strictObject({
  id: withUi(bridgeDomainId, { order: 1 }),
  flood: withUi(z.boolean().default(true), {
    title: 'Flood',
    help: 'flood broadcast / multicast frames to all members',
    order: 2,
  }),
  uuFlood: withUi(z.boolean().default(true), {
    title: 'Unknown-unicast flood',
    help: 'flood frames whose destination MAC is not learned',
    order: 3,
  }),
  forward: withUi(z.boolean().default(true), {
    title: 'Forward',
    help: 'forward by the L2 FIB',
    order: 4,
  }),
  learn: withUi(z.boolean().default(true), {
    title: 'Learn',
    help: 'learn source MAC addresses',
    order: 5,
  }),
  arpTerm: withUi(z.boolean().default(false), {
    title: 'ARP termination',
    help: 'answer ARP requests from the bridge ARP table',
    order: 6,
  }),
  macAgeMin: withUi(z.number().int().min(0).max(255).default(0), {
    title: 'MAC aging (minutes)',
    help: '0 = learned MACs never age out',
    order: 7,
  }),
  staticMacs: withUi(z.array(BridgeStaticMacSchema).max(1024).default([]), {
    title: 'Static MACs',
    itemKey: ['mac'],
    order: 8,
  }),
});
export type BridgeDomainConfig = z.infer<typeof BridgeDomainSchema>;

/** One direction of an L2 cross-connect (`routing.l2.xconnects.<rx>`; VPP `sw_interface_set_l2_xconnect`). */
export const L2XconnectSchema = z.strictObject({
  tx: withUi(vppInterfaceName, {
    title: 'Transmit interface',
    help: 'every frame received on the key interface is sent out here; a bidirectional cross-connect is two records',
    order: 1,
  }),
});
export type L2XconnectConfig = z.infer<typeof L2XconnectSchema>;

/** One path of an L3 cross-connect (a FIB path). */
export const L3xcPathSchema = z.strictObject({
  nextHop: withUi(ipAddress.optional(), {
    title: 'Next hop',
    help: 'next-hop address (same family as the list); absent = interface-only or table lookup',
    order: 1,
  }),
  interface: withUi(vppInterfaceName.optional(), {
    title: 'Interface',
    help: 'egress interface; absent = resolve the next hop (or look up) in the VRF',
    order: 2,
  }),
  vrf: withUi(vrfName.default(DEFAULT_VRF), {
    title: 'VRF',
    help: 'table the next hop is resolved in (or looked up in, without next hop and interface)',
    order: 3,
  }),
  weight: withUi(z.number().int().min(1).max(255).default(1), { title: 'Weight', order: 4 }),
  preference: withUi(z.number().int().min(0).max(255).default(0), {
    title: 'Preference',
    order: 5,
  }),
});
export type L3xcPathConfig = z.infer<typeof L3xcPathSchema>;

/** L3 cross-connect of one rx interface (`routing.l2.l3xc.<rx>`; VPP `l3xc_update`). */
export const L3xcSchema = z.strictObject({
  ipv4Paths: withUi(z.array(L3xcPathSchema).max(16).default([]), {
    title: 'IPv4 paths',
    help: 'every IPv4 packet received on the interface is forwarded via these paths, bypassing the FIB',
    order: 1,
  }),
  ipv6Paths: withUi(z.array(L3xcPathSchema).max(16).default([]), {
    title: 'IPv6 paths',
    order: 2,
  }),
});
export type L3xcConfig = z.infer<typeof L3xcSchema>;

/** Day of the week of a MAC-filter range (the spelling of `objects` schedules; VPP counts from Sunday). */
export const MacFilterDay = z.enum(WEEKDAYS);
export type MacFilterDay = z.infer<typeof MacFilterDay>;

/** `HH:MM`, 00:00–24:00 (24:00 = end of the day). */
export const macFilterTime = withUi(
  z.string().regex(/^(?:(?:[01][0-9]|2[0-3]):[0-5][0-9]|24:00)$/, 'expected HH:MM (00:00–24:00)'),
  { title: 'Time', widget: 'time' },
);

/** One weekly time range of a MAC-filter device: `start`–`end` on each of `days`. */
export const MacFilterRangeSchema = z
  .strictObject({
    days: withUi(z.array(MacFilterDay).min(1).max(7), { title: 'Days', order: 1 }),
    start: withUi(macFilterTime, { title: 'Start', order: 2 }),
    end: withUi(macFilterTime, { title: 'End', help: 'after start; 24:00 = midnight', order: 3 }),
  })
  .refine((r) => macFilterMinutes(r.end) > macFilterMinutes(r.start), {
    message: 'end must be after start (split ranges that cross midnight)',
    path: ['end'],
  });
export type MacFilterRangeConfig = z.infer<typeof MacFilterRangeSchema>;

/** Minutes since midnight of an `HH:MM` string (NaN when malformed). */
export function macFilterMinutes(hhmm: string): number {
  const m = /^(\d\d):(\d\d)$/.exec(hhmm);
  return m ? Number(m[1]) * 60 + Number(m[2]) : Number.NaN;
}

/**
 * One device of the time-range MAC filter (`routing.l2.macFilters.<name>`; VPP `mactime_add_del_range`).
 * Without ranges the action is permanent (static allow / drop); with ranges `allow` admits the device only inside
 * them and `drop` blocks it only inside them. Times are evaluated in VPP's mactime clock (start-up setting
 * `mactime { timezone_offset }`, default UTC−5 with US daylight saving).
 */
export const MacFilterSchema = z.strictObject({
  mac: withUi(macAddress, { title: 'MAC address', help: 'source MAC of the device', order: 1 }),
  action: withUi(z.enum(['allow', 'drop']).default('allow'), {
    title: 'Action',
    help: 'allow = admit only inside the ranges (always without ranges); drop = block inside the ranges (always without ranges)',
    widget: 'select',
    order: 2,
  }),
  ranges: withUi(z.array(MacFilterRangeSchema).max(28).default([]), {
    title: 'Weekly ranges',
    order: 3,
  }),
});
export type MacFilterConfig = z.infer<typeof MacFilterSchema>;

/** `routing.l2`: the bridge-domain records and the cross-connect / MAC-filter tables (D-109 c). */
export const BridgeL2Schema = z.strictObject({
  bridgeDomains: withUi(z.record(bridgeDomainName, BridgeDomainSchema).default({}), {
    title: 'Bridge domains',
    help: 'keyed by name; members join through interfaces.<if>.l2.bridgeDomain',
    order: 1,
  }),
  xconnects: withUi(z.record(vppInterfaceName, L2XconnectSchema).default({}), {
    title: 'L2 cross-connects',
    help: 'keyed by the receiving interface (one direction per record)',
    order: 2,
  }),
  l3xc: withUi(z.record(vppInterfaceName, L3xcSchema).default({}), {
    title: 'L3 cross-connects',
    help: 'keyed by the receiving interface',
    order: 3,
  }),
  macFilters: withUi(z.record(macFilterName, MacFilterSchema).default({}), {
    title: 'Time-range MAC filter',
    help: 'devices keyed by name; enable the filter per interface with interfaces.<if>.l2.macFilter',
    order: 4,
  }),
});
export type BridgeL2Config = z.infer<typeof BridgeL2Schema>;

/** The `interfaces.<if>.l2` key line (`Interface` field 14). */
export const interfaceL2Field = withUi(BridgeL2PortSchema.optional(), {
  title: 'L2 switching',
  help: 'bridge-domain membership, VLAN tag rewrite and MAC filter of this interface',
  group: UI_GROUP,
  order: 30,
});

/** The `interfaces.<if>.subinterfaces.<id>.l2` key line (`Subinterface` field 12). */
export const subinterfaceL2Field = withUi(BridgeL2PortSchema.optional(), {
  title: 'L2 switching',
  help: 'bridge-domain membership and VLAN tag rewrite of this sub-interface',
  group: UI_GROUP,
  order: 30,
});

/** The `routing.l2` key line (`RoutingConfig` field 20). */
export const routingL2Field = withUi(BridgeL2Schema.optional(), {
  title: 'L2 switching',
  description: 'Bridge domains, L2/L3 cross-connects and the time-range MAC filter.',
  group: UI_GROUP,
  order: 20,
});
