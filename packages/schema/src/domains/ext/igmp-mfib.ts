import { z } from 'zod';
import { ipv4Address, ipv4Cidr, vppInterfaceName, vrfName } from '../../primitives.js';
import { withUi } from '../../ui.js';

/**
 * F-igmp-mfib: `routing.multicast` — IPv4 multicast in FAST MODE. IGMPv3 host/router interfaces and proxies, static
 * multicast routes (mFIB), and PIM (FRR pimd builds the state, the agent programs VPP mFIB). VPP 26.06's IGMP plugin
 * is INCLUDE-only (no EXCLUDE), so every join carries at least one source; groups are in 224.0.0.0/4 but never the
 * 224.0.0.0/24 link-local control block. The data-plane/FRR wiring is the agent's; this is the contract.
 */

const GROUP = 'multicast';

/** An IPv4 multicast group (224.0.0.0/4), excluding the 224.0.0.0/24 link-local control block (checked in semantics). */
export const multicastGroup = withUi(ipv4Address, {
  title: 'Group',
  help: 'IPv4 multicast group in 224.0.0.0/4 (not the 224.0.0.0/24 link-local range)',
  widget: 'ip',
});

/** One IGMP join: a group and the sources to include (VPP is INCLUDE-only, so at least one source is required). */
export const IgmpJoinSchema = z.strictObject({
  group: withUi(multicastGroup, { order: 1 }),
  sources: withUi(z.array(ipv4Address).min(1).max(64), {
    title: 'Sources',
    help: 'source addresses to include; VPP 26.06 supports INCLUDE only, so at least one is required',
    order: 2,
  }),
});
export type IgmpJoin = z.infer<typeof IgmpJoinSchema>;

export const IgmpInterfaceSchema = z.strictObject({
  mode: withUi(z.enum(['host', 'router']).default('router'), {
    title: 'Mode',
    widget: 'select',
    help: 'host = the box joins groups itself (static joins); router = it snoops membership (IGMPv3 querier)',
    order: 1,
  }),
  joins: withUi(z.array(IgmpJoinSchema).max(256).default([]), {
    title: 'Static joins',
    help: 'host-mode only; groups the interface joins with their included sources',
    order: 2,
  }),
});
export type IgmpInterface = z.infer<typeof IgmpInterfaceSchema>;

/** An IGMP proxy: one upstream (querier side) and one or more downstream (member side) interfaces, per VRF. */
export const IgmpProxySchema = z.strictObject({
  upstream: withUi(vppInterfaceName, { title: 'Upstream', widget: 'interface-picker', order: 1 }),
  downstream: withUi(z.array(vppInterfaceName).min(1).max(64), {
    title: 'Downstream',
    widget: 'interface-picker',
    order: 2,
  }),
});

export const IgmpSchema = z.strictObject({
  interfaces: withUi(z.record(vppInterfaceName, IgmpInterfaceSchema).default({}), {
    title: 'IGMP interfaces',
    order: 1,
  }),
  ssmRanges: withUi(z.array(ipv4Cidr).max(16).default(['232.0.0.0/8']), {
    title: 'SSM ranges',
    help: 'source-specific multicast ranges; a join to a group here must name its sources',
    order: 2,
  }),
  proxies: withUi(z.record(vrfName, IgmpProxySchema).default({}), {
    title: 'IGMP proxies',
    help: 'keyed by VRF; forward membership from downstream interfaces to the upstream',
    order: 3,
  }),
});

/** A static multicast route path: an accept (RPF/incoming) or forward (outgoing) interface. */
export const MrouteePathSchema = z.strictObject({
  interface: withUi(vppInterfaceName, { title: 'Interface', widget: 'interface-picker', order: 1 }),
  flags: withUi(z.enum(['accept', 'forward']).default('forward'), {
    title: 'Role',
    widget: 'select',
    help: 'accept = the incoming (RPF) interface; forward = an outgoing interface',
    order: 2,
  }),
});
export type MrouteePath = z.infer<typeof MrouteePathSchema>;

export const MrouteSchema = z
  .strictObject({
    vrf: withUi(vrfName.default('default'), { title: 'VRF', order: 1 }),
    group: withUi(multicastGroup, { order: 2 }),
    source: withUi(ipv4Address.optional(), {
      title: 'Source',
      help: 'a specific source for an (S,G) route; omit for a (*,G) route',
      order: 3,
    }),
    paths: withUi(z.array(MrouteePathSchema).min(1).max(64).default([]), {
      title: 'Paths',
      order: 4,
    }),
  })
  .superRefine((m, ctx) => {
    const accept = m.paths.filter((p) => p.flags === 'accept').map((p) => p.interface);
    if (accept.length > 1) {
      ctx.addIssue({ code: 'custom', path: ['paths'], message: 'a multicast route has at most one accept interface' });
    }
    m.paths.forEach((p, i) => {
      if (p.flags === 'forward' && accept.includes(p.interface)) {
        ctx.addIssue({
          code: 'custom',
          path: ['paths', i, 'interface'],
          message: `'${p.interface}' is the accept interface and cannot also forward`,
        });
      }
    });
  });
export type Mroute = z.infer<typeof MrouteSchema>;

/** A PIM rendezvous point: an RP address for a set of group ranges. */
export const PimRpSchema = z.strictObject({
  address: withUi(ipv4Address, { title: 'RP address', order: 1 }),
  groups: withUi(z.array(ipv4Cidr).min(1).max(64), {
    title: 'Group ranges',
    help: 'multicast group ranges this RP serves',
    order: 2,
  }),
});

export const PimSchema = z.strictObject({
  interfaces: withUi(z.array(vppInterfaceName).max(256).default([]), {
    title: 'PIM interfaces',
    widget: 'interface-picker',
    help: 'interfaces running PIM-SM (via FRR pimd over the linux-cp pairs)',
    order: 1,
  }),
  rp: withUi(z.array(PimRpSchema).max(64).default([]), {
    title: 'Rendezvous points',
    help: 'static RP configuration (bootstrap-router RPs are out of scope)',
    order: 2,
  }),
});

export const MulticastSchema = z.strictObject({
  igmp: withUi(IgmpSchema.prefault({}), { title: 'IGMP', group: GROUP, order: 1 }),
  mroutes: withUi(z.array(MrouteSchema).max(1024).default([]), {
    title: 'Static multicast routes',
    group: GROUP,
    order: 2,
  }),
  pim: withUi(PimSchema.prefault({}), { title: 'PIM-SM', group: GROUP, order: 3 }),
});
export type MulticastConfig = z.infer<typeof MulticastSchema>;

/** `routing.multicast` field. */
export const multicastField = withUi(MulticastSchema.optional(), {
  title: 'Multicast (IGMP / mFIB / PIM)',
  help: 'IPv4 multicast: IGMPv3, static multicast routes, and PIM-SM via FRR',
  group: GROUP,
  order: 62,
});
