import { z } from 'zod';
import { parseCidr } from '../../ip.js';
import { ipNetwork, vppInterfaceName, vrfName } from '../../primitives.js';
import { withUi } from '../../ui.js';

/**
 * F-vrf-static-ecmp sub-schemas (wave-A-hotspots C1): the `vrfs` and `routing` domain objects carry one key line each under
 * their `wave-A: F-vrf-static-ecmp` anchor; the shapes live here. Cross-object rules are in `../../semantic/vrf-static-ecmp.ts`.
 */

/** `x-vrx-ui` group of every field this feature adds (group = task slug, C1). */
const GROUP = 'vrf-static-ecmp';

/**
 * `vrfs.<name>.sourceSelect[]` — source VRF select (VPP `svs` plugin): a packet that arrives on `interface` with a source
 * address inside `prefix` is routed (destination lookup) in this VRF instead of the VRF the interface is bound to.
 * `0.0.0.0/0` / `::/0` are not accepted: the all-sources entry is the interface's own VRF (VPP adds it when source VRF
 * select is enabled on the interface).
 */
export const VrfSourceSelectSchema = z
  .strictObject({
    prefix: withUi(ipNetwork, {
      title: 'Source prefix',
      help: 'packets whose source address is in this prefix, e.g. 10.20.0.0/16',
      order: 1,
    }),
    interface: withUi(vppInterfaceName, {
      title: 'Ingress interface',
      help: 'interface the packets arrive on',
      widget: 'interface-picker',
      order: 2,
    }),
  })
  .refine((entry) => parseCidr(entry.prefix)?.length !== 0, {
    message: 'a /0 source prefix is the interface’s own VRF; use a longer prefix',
    path: ['prefix'],
  });
export type VrfSourceSelectConfig = z.infer<typeof VrfSourceSelectSchema>;

/** The `sourceSelect` member of `VrfSchema` (optional: absent = no source VRF select into this VRF). */
export const vrfSourceSelect = withUi(z.array(VrfSourceSelectSchema).max(256).optional(), {
  title: 'Source VRF select',
  help: 'route packets from these source prefixes, arriving on these interfaces, in this VRF',
  group: GROUP,
  order: 3,
  itemKey: ['interface', 'prefix'],
});

/**
 * The `vrf` member of `NextHopSchema`: resolve the next-hop address in another VRF (route leaking, VPP path `table_id`).
 * Absent = the route's own VRF. Only for next hops without an egress interface (a next hop on an interface is resolved on
 * that interface) — semantic rule `routing.vrf-static-ecmp-nexthop-vrf`.
 */
export const nextHopVrf = withUi(vrfName.optional(), {
  title: 'Next-hop VRF',
  help: 'resolve the next-hop address in this VRF (default: the route’s VRF)',
  group: GROUP,
  order: 4,
});

/**
 * The `viaFrr` member of `StaticRouteSchema` (D-072: one programmer per static route): `true` = FRR (staticd) owns the route
 * (e.g. to redistribute it into a routing protocol) and the agent does not program it in VPP; absent/`false` = the agent
 * programs it in VPP directly. The FRR side is rendered by P12 / F-bfd-redistribution.
 */
export const staticRouteViaFrr = withUi(z.boolean().optional(), {
  title: 'Program via FRR',
  help: 'hand this route to FRR (staticd) instead of programming it in VPP directly, e.g. to redistribute it',
  group: GROUP,
  order: 7,
});
