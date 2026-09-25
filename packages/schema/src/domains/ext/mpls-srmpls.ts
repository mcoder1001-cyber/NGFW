import { z } from 'zod';
import { ipFamily } from '../../ip.js';
import { ipAddress, ipNetwork, vppInterfaceName, vrfName } from '../../primitives.js';
import { withUi } from '../../ui.js';

/**
 * F-mpls-srmpls sub-schemas (wave-A-hotspots C1; WBS D2.8): the `routing` domain object carries one key line under its
 * `wave-BC: F-mpls-srmpls` anchor (`mpls`), the shapes live here. Static MPLS (MPLS-enabled interfaces, MPLS tables,
 * static label routes / LSPs, label ↔ IP-prefix bindings, MPLS tunnels) and SR-MPLS (policies by binding SID, steering
 * by BSID). LDP is F-mpls-ldp's: it adds `routing.mpls.ldp` below the `wave-BC: F-mpls-ldp` anchor in `MplsSchema`.
 * Cross-object rules are in `../../semantic/mpls-srmpls.ts` (rule ids `routing.mpls-srmpls-…`).
 *
 * Tables: MPLS table 0 is the default MPLS FIB (VPP-global, D-071) and is never listed; `tables` are additional MPLS
 * FIBs (ids 1–4294967295). Labels are 16–1048575 (RFC 3032: 0–15 are reserved); an out-label stack has at most 16
 * labels (VPP `fib_path.label_stack[16]`).
 */

/** `x-vrx-ui` group of every field this feature adds (group = task slug, C1). */
const GROUP = 'mpls-srmpls';

/** Smallest unreserved MPLS label (RFC 3032: 0–15 are reserved). */
export const MPLS_MIN_LABEL = 16;
/** Largest MPLS label (20 bits). */
export const MPLS_MAX_LABEL = 1048575;
/** VPP's out-label stack depth (`fib_path.label_stack[16]`) and the SR-MPLS segment-list limit. */
export const MPLS_MAX_LABEL_STACK = 16;

/** An MPLS label an operator may configure: 16–1048575. */
export const mplsLabel = withUi(z.number().int().min(MPLS_MIN_LABEL).max(MPLS_MAX_LABEL), {
  title: 'Label',
  help: 'MPLS label 16–1048575 (0–15 are reserved)',
  widget: 'number',
});

/** Record key of `tables`: an MPLS table id 1–4294967295 in decimal (table 0 is the implicit default table). */
export const mplsTableKey = z
  .string()
  .regex(
    /^[1-9][0-9]{0,9}$/,
    'an MPLS table id 1–4294967295 in decimal (table 0 is the default table)',
  )
  .refine((k) => Number(k) <= 4294967295, 'an MPLS table id is at most 4294967295');

/** Record key of `sr.policies`: the binding SID (a label 16–1048575) in decimal. */
export const mplsBsidKey = z
  .string()
  .regex(/^[1-9][0-9]{1,6}$/, 'a binding SID: a label 16–1048575 in decimal')
  .refine(
    (k) => Number(k) >= MPLS_MIN_LABEL && Number(k) <= MPLS_MAX_LABEL,
    'a binding SID is a label 16–1048575',
  );

/**
 * Record key of `tunnels`: the tunnel's name. It becomes the logical name of the VPP MPLS tunnel interface (D-069),
 * so it shares the interface name space: it must not also be an `interfaces` key (semantic rule).
 */
export const mplsTunnelName = withUi(
  z
    .string()
    .min(1)
    .max(32)
    .regex(/^[A-Za-z][A-Za-z0-9_-]*$/, 'letters, digits, `_` and `-`, starting with a letter'),
  { title: 'Tunnel name' },
);

/** EOS payload protocols of an end-of-stack label route (what is under the popped label). */
export const MplsPayload = z.enum(['ip4', 'ip6', 'ethernet']);
export type MplsPayload = z.infer<typeof MplsPayload>;

/**
 * One path of a label route or tunnel: forward to `nextHop` (on `interface`) pushing `outLabels` (swap / push; an empty
 * stack pops), or — for an end-of-stack route — pop and look the payload up in `vrf`'s IP table (`vrf` alone).
 * `interface` may name an `interfaces` key or an MPLS tunnel of `tunnels`.
 */
export const MplsPathSchema = z
  .strictObject({
    nextHop: withUi(ipAddress.optional(), {
      title: 'Next hop',
      help: 'next-hop address; may be omitted on a point-to-point interface',
      order: 1,
    }),
    interface: withUi(vppInterfaceName.optional(), {
      title: 'Interface',
      help: 'egress interface or MPLS tunnel',
      order: 2,
    }),
    outLabels: withUi(z.array(mplsLabel).max(MPLS_MAX_LABEL_STACK).default([]), {
      title: 'Out labels',
      help: 'label stack pushed on the path, outermost first (swap/push); empty = pop',
      order: 3,
    }),
    weight: withUi(z.number().int().min(1).max(255).default(1), {
      title: 'Weight',
      help: 'relative share when a route has several paths',
      order: 4,
    }),
    vrf: withUi(vrfName.optional(), {
      title: 'Lookup VRF',
      help: 'end-of-stack routes only: pop the label and look the IP payload up in this VRF (no next hop, no interface)',
      order: 5,
    }),
  })
  .refine((p) => p.nextHop !== undefined || p.interface !== undefined || p.vrf !== undefined, {
    message: 'a path needs a next hop, an interface or a lookup VRF',
    path: ['nextHop'],
  })
  .refine(
    (p) =>
      p.vrf === undefined ||
      (p.nextHop === undefined && p.interface === undefined && p.outLabels.length === 0),
    {
      message: 'a lookup-VRF path has no next hop, no interface and no out labels',
      path: ['vrf'],
    },
  );
export type MplsPathConfig = z.infer<typeof MplsPathSchema>;

/**
 * `routing.mpls.labelRoutes[]` — a static label route (LSP hop): packets arriving with `label` (as the bottom of the
 * stack when `eos`, else with more labels under it) in MPLS table `table` are forwarded on `paths`. The key
 * (table, label, eos) is unique (semantic rule).
 */
export const MplsLabelRouteSchema = z
  .strictObject({
    table: withUi(z.number().int().min(0).max(4294967295).default(0), {
      title: 'MPLS table',
      help: '0 = the default MPLS table (where labelled packets arrive); another id must be listed under tables',
      widget: 'number',
      order: 1,
    }),
    label: withUi(mplsLabel, { title: 'Local label', order: 2 }),
    eos: withUi(z.boolean().default(true), {
      title: 'End of stack',
      help: 'the route matches the label as the bottom of the stack (EOS); off = non-EOS (more labels follow)',
      order: 3,
    }),
    payload: withUi(MplsPayload.optional(), {
      title: 'Payload',
      help: 'end-of-stack routes: what is under the label — ip4, ip6 or ethernet; unset = ip6 when a next hop is IPv6, else ip4',
      order: 4,
    }),
    paths: withUi(z.array(MplsPathSchema).min(1).max(32), {
      title: 'Paths',
      help: 'more than one = load balancing by weight',
      order: 5,
    }),
  })
  .refine((r) => r.eos || r.payload === undefined, {
    message: 'payload applies to end-of-stack routes only',
    path: ['payload'],
  })
  .refine((r) => r.eos || r.paths.every((p) => p.vrf === undefined), {
    message: 'a lookup-VRF path (pop and IP lookup) applies to end-of-stack routes only',
    path: ['paths'],
  })
  .refine((r) => r.payload !== 'ethernet' || r.paths.every((p) => p.vrf === undefined), {
    message: 'an ethernet payload cannot be looked up in an IP table',
    path: ['payload'],
  })
  .refine(
    (r) => {
      const families = new Set(
        r.paths.flatMap((p) => (p.nextHop === undefined ? [] : [ipFamily(p.nextHop)])),
      );
      return families.size <= 1;
    },
    { message: 'the next hops of one label route must be of one address family', path: ['paths'] },
  );
export type MplsLabelRouteConfig = z.infer<typeof MplsLabelRouteSchema>;

/**
 * `routing.mpls.ipBindings[]` — binds local label `label` to IP prefix `prefix` of VRF `vrf` (VPP mpls_ip_bind_unbind):
 * VPP installs the label's entries in the default MPLS table (0). One label per prefix (semantic rule).
 */
export const MplsIpBindingSchema = z.strictObject({
  label: withUi(mplsLabel, { title: 'Local label', order: 1 }),
  vrf: withUi(vrfName.default('default'), { title: 'VRF', order: 2 }),
  prefix: withUi(ipNetwork, { title: 'Prefix', help: 'IP prefix the label is bound to', order: 3 }),
});
export type MplsIpBindingConfig = z.infer<typeof MplsIpBindingSchema>;

/** `routing.mpls.tunnels.<name>` — an MPLS tunnel interface (a head-end LSP) with its paths and out labels. */
export const MplsTunnelSchema = z.strictObject({
  paths: withUi(z.array(MplsPathSchema).min(1).max(32), {
    title: 'Paths',
    help: 'where the tunnel sends its packets and with which out labels',
    order: 1,
  }),
  l2Only: withUi(z.boolean().default(false), {
    title: 'L2 only',
    help: 'a layer-2 tunnel (pseudowire head end, no IP)',
    order: 2,
  }),
});
export type MplsTunnelConfig = z.infer<typeof MplsTunnelSchema>;

/** `routing.mpls.tables.<id>` — an additional MPLS FIB (no settings of its own yet). */
export const MplsTableSchema = z.strictObject({});
export type MplsTableConfig = z.infer<typeof MplsTableSchema>;

/** One segment list of an SR-MPLS policy: `labels[0]` is the first segment (resolved in MPLS table 0). */
export const MplsSrSegmentListSchema = z.strictObject({
  labels: withUi(z.array(mplsLabel).min(1).max(MPLS_MAX_LABEL_STACK), {
    title: 'Segments',
    help: 'segment labels, first segment first (1–16)',
    order: 1,
  }),
  weight: withUi(z.number().int().min(1).max(255).default(1), {
    title: 'Weight',
    help: 'share of this list when the policy load-balances',
    order: 2,
  }),
});
export type MplsSrSegmentListConfig = z.infer<typeof MplsSrSegmentListSchema>;

/** `routing.mpls.sr.policies.<bsid>` — an SR-MPLS policy identified by its binding SID. */
export const MplsSrPolicySchema = z.strictObject({
  segmentLists: withUi(z.array(MplsSrSegmentListSchema).min(1).max(32), {
    title: 'Segment lists',
    order: 1,
  }),
  spray: withUi(z.boolean().default(false), {
    title: 'Spray',
    help: 'replicate on every segment list instead of load-balancing',
    order: 2,
  }),
});
export type MplsSrPolicyConfig = z.infer<typeof MplsSrPolicySchema>;

/** `routing.mpls.sr.steering[]` — steer IP prefix `prefix` of VRF `vrf` into the policy with binding SID `bsid`. */
export const MplsSrSteeringSchema = z.strictObject({
  vrf: withUi(vrfName.default('default'), { title: 'VRF', order: 1 }),
  prefix: withUi(ipNetwork, {
    title: 'Prefix',
    help: 'traffic to this prefix enters the policy',
    order: 2,
  }),
  bsid: withUi(mplsLabel, {
    title: 'Binding SID',
    help: 'the policy (a key of sr.policies)',
    order: 3,
  }),
  vpnLabel: withUi(mplsLabel.optional(), {
    title: 'VPN label',
    help: 'optional label pushed under the segments',
    order: 4,
  }),
});
export type MplsSrSteeringConfig = z.infer<typeof MplsSrSteeringSchema>;

/** `routing.mpls.sr` — SR-MPLS. */
export const MplsSrSchema = z.strictObject({
  policies: withUi(z.record(mplsBsidKey, MplsSrPolicySchema).default({}), {
    title: 'SR policies',
    help: 'by binding SID',
    order: 1,
  }),
  steering: withUi(z.array(MplsSrSteeringSchema).max(65536).default([]), {
    title: 'Steering',
    itemKey: ['vrf', 'prefix'],
    order: 2,
  }),
});
export type MplsSrConfig = z.infer<typeof MplsSrSchema>;

/**
 * `routing.mpls` — static MPLS and SR-MPLS. A plain object: F-mpls-ldp adds `ldp` below its anchor (additive), so this
 * stays a `z.strictObject` of named keys and never becomes a union or a record.
 */
export const MplsSchema = z.strictObject({
  interfaces: withUi(z.array(vppInterfaceName).max(4096).default([]), {
    title: 'MPLS interfaces',
    help: 'interfaces that accept labelled packets (MPLS enabled)',
    widget: 'interface-picker',
    order: 1,
  }),
  tables: withUi(z.record(mplsTableKey, MplsTableSchema).default({}), {
    title: 'MPLS tables',
    help: 'additional MPLS FIBs by id (table 0 is the default table and is not listed)',
    order: 2,
  }),
  labelRoutes: withUi(z.array(MplsLabelRouteSchema).max(65536).default([]), {
    title: 'Label routes',
    help: 'static label switching: local label → paths with out labels',
    itemKey: ['table', 'label', 'eos'],
    order: 3,
  }),
  ipBindings: withUi(z.array(MplsIpBindingSchema).max(65536).default([]), {
    title: 'Label bindings',
    help: 'local label ↔ IP prefix',
    itemKey: ['vrf', 'prefix'],
    order: 4,
  }),
  tunnels: withUi(z.record(mplsTunnelName, MplsTunnelSchema).default({}), {
    title: 'MPLS tunnels',
    order: 5,
  }),
  sr: withUi(MplsSrSchema.prefault({}), { title: 'SR-MPLS', order: 6 }),
  // Keys added by later tasks (additive, one key line each):
  // wave-BC: F-mpls-ldp
});
export type MplsConfig = z.infer<typeof MplsSchema>;

/** The `mpls` member of `RoutingSchema` (optional: absent = no MPLS configured). */
export const routingMpls = withUi(MplsSchema.optional(), {
  title: 'MPLS',
  help: 'static MPLS label switching, MPLS tunnels and SR-MPLS',
  group: GROUP,
  order: 20,
});
