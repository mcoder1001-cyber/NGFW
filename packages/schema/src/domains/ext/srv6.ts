import { z } from 'zod';
import { ipAddress, ipNetwork, ipv6Address, vppInterfaceName, vrfName } from '../../primitives.js';
import { withUi } from '../../ui.js';
import { DEFAULT_VRF } from '../vrfs.js';

/**
 * F-srv6 (WBS D6.7): `routing.srv6` — SRv6 local SIDs (network programming), SR policies with weighted segment lists,
 * L2/L3 steering into a policy's binding SID, and the two VPP-wide encapsulation settings. vrx-agent programs it with
 * DF-6's `sr.*` descriptors (VPP core `sr` binary API); `docs/user/vpn/srv6.md` is the user guide.
 *
 * - `localSids.<sid>`: one local SID per IPv6 address (`sr_localsid_add_del`). Behaviours END, END.X, END.T, END.DX2,
 *   END.DX4, END.DX6, END.DT4, END.DT6. The service-chaining proxies END.AD / END.AM / END.AS exist in VPP 26.06 only
 *   as CLI (no binary API: docs/vpp-code-track.md, V-new F-srv6) and are not offered; neither are uSID behaviours or
 *   SRv6-mobile (D-074, D-085).
 * - `policies.<bsid>`: one SR policy per binding SID (`sr_policy_add_v2` + `sr_policy_mod_v2`): `encap` = H.Encaps
 *   (outer IPv6 header + SRH), otherwise H.Insert (SRH inserted into IPv6 traffic). Every encapsulating policy carries
 *   its own outer source address (D-074: VPP's global default cannot be read back): the policy's `encapSource`, else
 *   `routing.srv6.encapSource`.
 * - `steering[]`: traffic steered into a policy — `l3` = a prefix of a VRF (IPv4 or IPv6), `l2` = everything received
 *   on an interface (the interface is switched to L2 cross-connect mode by VPP).
 * - `encapSource` / `encapHopLimit`: VPP-wide settings (`sr_set_encap_source`, `sr_set_encap_hop_limit`), applied only by
 *   the globals owner (the product agent on a real box, D-071). They cannot be read back (write-only, D-063), so a
 *   Retrieve never reports them.
 *
 * Record keys are written in canonical form (lower case, shortest `::` form) — `routing.srv6-canonical` — so the keys
 * the agent reports back compare equal. Cross-object rules (behaviour fields, VRFs and interfaces exist, encap source,
 * steering BSIDs and uniqueness) live in `../../semantic/srv6.ts`.
 */

/** UI group of the feature (x-vrx-ui group = task slug, wave-A-hotspots C1). */
export const SRV6_GROUP = 'srv6';

/** Size of `srv6_sid_list.sids` in the VPP 26.06 binary API. */
export const SRV6_MAX_SIDS = 16;

/** VPP's segment-list weight when none is given (SR_SEGMENT_LIST_WEIGHT_DEFAULT). */
export const SRV6_DEFAULT_WEIGHT = 1;

/** Local SID behaviours VPP 26.06 offers through the binary API (sr_types.sr_behavior, classic SIDs). */
export const Srv6Behavior = withUi(
  z.enum(['end', 'end.x', 'end.t', 'end.dx2', 'end.dx4', 'end.dx6', 'end.dt4', 'end.dt6']),
  {
    title: 'Behavior',
    help: 'end: endpoint · end.x: endpoint with L3 cross-connect · end.t: endpoint with table lookup · end.dx2/dx4/dx6: decapsulate and cross-connect · end.dt4/dt6: decapsulate and look up in a VRF',
  },
);
export type Srv6Behavior = z.infer<typeof Srv6Behavior>;

/** Behaviours that cross-connect to an interface (need `interface`). */
export const SRV6_INTERFACE_BEHAVIORS: readonly Srv6Behavior[] = [
  'end.x',
  'end.dx2',
  'end.dx4',
  'end.dx6',
];
/** Behaviours that forward to a next hop (need `nextHop`; IPv4 for end.dx4, IPv6 otherwise). */
export const SRV6_NEXT_HOP_BEHAVIORS: readonly Srv6Behavior[] = ['end.x', 'end.dx4', 'end.dx6'];
/** Behaviours that look the inner packet up in a VRF (need `lookupVrf`). */
export const SRV6_LOOKUP_BEHAVIORS: readonly Srv6Behavior[] = ['end.t', 'end.dt4', 'end.dt6'];
/** Behaviours that may pop the SRH at the penultimate segment. */
export const SRV6_PSP_BEHAVIORS: readonly Srv6Behavior[] = ['end', 'end.x', 'end.t'];

/** One local SID (`routing.srv6.localSids.<sid>`); which fields apply depends on `behavior`. */
export const Srv6LocalSidSchema = z.strictObject({
  behavior: withUi(Srv6Behavior, { order: 1 }),
  psp: withUi(z.boolean().default(false), {
    title: 'Penultimate segment pop (PSP)',
    help: 'end, end.x and end.t only: remove the SRH when the last segment is reached',
    order: 2,
  }),
  vrf: withUi(vrfName.default(DEFAULT_VRF), {
    title: 'VRF',
    help: 'the IPv6 table the SID is installed in',
    order: 3,
  }),
  interface: withUi(vppInterfaceName.optional(), {
    title: 'Interface',
    help: 'end.x, end.dx2, end.dx4, end.dx6: the interface to cross-connect to',
    order: 4,
  }),
  nextHop: withUi(ipAddress.optional(), {
    title: 'Next hop',
    help: 'end.x and end.dx6: an IPv6 next hop; end.dx4: an IPv4 next hop',
    order: 5,
  }),
  lookupVrf: withUi(vrfName.optional(), {
    title: 'Lookup VRF',
    help: 'end.t and end.dt6: the IPv6 table; end.dt4: the IPv4 table the inner packet is looked up in',
    order: 6,
  }),
});
export type Srv6LocalSidConfig = z.infer<typeof Srv6LocalSidSchema>;

/** One segment list of a policy: up to 16 SIDs, first to last, and a load-balancing weight. */
export const Srv6SidListSchema = z.strictObject({
  sids: withUi(z.array(ipv6Address).min(1).max(SRV6_MAX_SIDS), {
    title: 'Segments',
    help: 'segment IDs in order, the first is visited first (at most 16)',
    order: 1,
  }),
  weight: withUi(z.number().int().min(1).max(65535).default(SRV6_DEFAULT_WEIGHT), {
    title: 'Weight',
    help: 'share of the traffic this list carries (weighted ECMP across the lists of a policy)',
    widget: 'number',
    order: 2,
  }),
});
export type Srv6SidListConfig = z.infer<typeof Srv6SidListSchema>;

/** Policy types (sr.sr_policy_type): default = weighted ECMP over the lists, spray = a copy on every list. */
export const Srv6PolicyType = withUi(z.enum(['default', 'spray', 'tef']), {
  title: 'Type',
  help: 'default: load-balance over the segment lists · spray: send a copy on every list · tef: traffic engineering (timestamps)',
});

/** One SR policy (`routing.srv6.policies.<bsid>`). */
export const Srv6PolicySchema = z.strictObject({
  type: withUi(Srv6PolicyType.default('default'), { order: 1 }),
  encap: withUi(z.boolean().default(true), {
    title: 'Encapsulate (H.Encaps)',
    help: 'on: add an outer IPv6 header with the SRH (IPv4, IPv6 and L2 traffic) · off: insert the SRH into IPv6 traffic (H.Insert)',
    order: 2,
  }),
  vrf: withUi(vrfName.default(DEFAULT_VRF), {
    title: 'VRF',
    help: 'the IPv6 table the binding SID is installed in',
    order: 3,
  }),
  encapSource: withUi(ipv6Address.optional(), {
    title: 'Encapsulation source',
    help: 'outer source address of this policy (encapsulating policies only); empty = routing.srv6.encapSource',
    order: 4,
  }),
  sidLists: withUi(z.array(Srv6SidListSchema).min(1).max(64), {
    title: 'Segment lists',
    help: 'at least one list; traffic is shared by weight (default) or copied to every list (spray)',
    order: 5,
  }),
});
export type Srv6PolicyConfig = z.infer<typeof Srv6PolicySchema>;

/** L3 steering: a prefix of a VRF, IPv4 or IPv6, is routed into the policy's binding SID. */
export const Srv6SteeringL3Schema = z.strictObject({
  type: withUi(z.literal('l3'), { title: 'Match', help: 'l3: a prefix of a VRF', order: 1 }),
  prefix: withUi(ipNetwork, {
    title: 'Prefix',
    help: 'IPv4 prefixes need an encapsulating policy',
    order: 2,
  }),
  vrf: withUi(vrfName.default(DEFAULT_VRF), { title: 'VRF', order: 3 }),
  bsid: withUi(ipv6Address, {
    title: 'Binding SID',
    help: 'the policy to steer into (routing.srv6.policies.<bsid>)',
    order: 4,
  }),
});

/** L2 steering: every frame received on an interface is encapsulated into the policy (encapsulating policies only). */
export const Srv6SteeringL2Schema = z.strictObject({
  type: withUi(z.literal('l2'), {
    title: 'Match',
    help: 'l2: every frame received on an interface',
    order: 1,
  }),
  interface: withUi(vppInterfaceName, {
    title: 'Interface',
    help: 'VPP switches it to L2 cross-connect mode: it must not carry IP addresses',
    order: 2,
  }),
  bsid: withUi(ipv6Address, {
    title: 'Binding SID',
    help: 'the policy to steer into (routing.srv6.policies.<bsid>); it must encapsulate',
    order: 3,
  }),
});

/** One steering entry (`routing.srv6.steering[]`), unique by (vrf, prefix) or by interface. */
export const Srv6SteeringSchema = withUi(
  z.discriminatedUnion('type', [Srv6SteeringL3Schema, Srv6SteeringL2Schema]),
  {
    title: 'Steering',
  },
);
export type Srv6SteeringConfig = z.infer<typeof Srv6SteeringSchema>;

/** `routing.srv6` — absent = SRv6 not configured. */
export const Srv6Schema = withUi(
  z.strictObject({
    encapSource: withUi(ipv6Address.optional(), {
      title: 'Encapsulation source',
      help: 'outer source address of encapsulating policies that set none; also VPP’s global default (applied by the globals owner only)',
      group: SRV6_GROUP,
      order: 1,
    }),
    encapHopLimit: withUi(z.number().int().min(1).max(255).optional(), {
      title: 'Encapsulation hop limit',
      help: 'hop limit of the outer IPv6 header, VPP-wide (globals owner only; VPP default 64)',
      widget: 'number',
      group: SRV6_GROUP,
      order: 2,
    }),
    localSids: withUi(z.record(ipv6Address, Srv6LocalSidSchema).default({}), {
      title: 'Local SIDs',
      help: 'SRv6 network programming: one entry per local SID (IPv6 address, canonical form)',
      group: SRV6_GROUP,
      order: 3,
    }),
    policies: withUi(z.record(ipv6Address, Srv6PolicySchema).default({}), {
      title: 'Policies',
      help: 'SR policies keyed by binding SID (IPv6 address, canonical form)',
      group: SRV6_GROUP,
      order: 4,
    }),
    steering: withUi(z.array(Srv6SteeringSchema).max(65536).default([]), {
      title: 'Steering',
      help: 'traffic steered into a policy: a prefix of a VRF (l3) or an interface (l2)',
      itemKey: ['type', 'vrf', 'prefix', 'interface'],
      group: SRV6_GROUP,
      order: 5,
    }),
  }),
  {
    title: 'SRv6',
    description: 'Segment Routing over IPv6: local SIDs, policies and steering (VPP sr).',
  },
);
export type Srv6Config = z.infer<typeof Srv6Schema>;

/** The `routing.srv6` key (one key line under the F-srv6 anchor in `RoutingSchema`). */
export const srv6Field = withUi(Srv6Schema.optional(), {
  title: 'SRv6',
  group: SRV6_GROUP,
  order: 20,
});
