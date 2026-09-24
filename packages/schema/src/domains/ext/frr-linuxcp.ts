import { z } from 'zod';
import { uint32 } from '../../primitives.js';
import { withUi } from '../../ui.js';

/**
 * P12 sub-schemas (wave-A-hotspots C1): the `interfaces` and `routing` domain objects carry one key line each under their
 * `wave-A: P12` anchor; the shapes live here. Cross-object rules are in `../../semantic/bgp.ts`.
 */

/** `x-vrx-ui` group of every field this task adds (group = task slug, C1). */
const GROUP = 'frr-linuxcp';

/**
 * Linux interface name (IFNAMSIZ 15): what the linux-cp host tap is called. The same alphabet FRR accepts for an
 * interface (`frr.IfName` in the agent): letters, digits, `_`, `.`, `-`; not `.`/`..`; not something that parses as an IP
 * address (FRR would read it as a gateway).
 */
export const lcpHostIfName = withUi(
  z
    .string()
    .min(1)
    .max(15)
    .regex(
      /^[A-Za-z0-9_.-]+$/,
      'letters, digits, `_`, `.` and `-` only (a Linux interface name, max 15)',
    )
    .refine((n) => n !== '.' && n !== '..', 'not a name')
    .refine((n) => !/^[0-9.]+$/.test(n), {
      message: 'an interface name must not look like an IP address',
    }),
  { title: 'Linux name' },
);

/** Linux network namespace name (VPP keeps it as /var/run/netns/<name>, max 31). */
export const lcpNetnsName = withUi(
  z
    .string()
    .min(1)
    .max(31)
    .regex(/^[A-Za-z0-9_.-]+$/, 'letters, digits, `_`, `.` and `-` only (max 31)')
    .refine((n) => n !== '.' && n !== '..', 'not a name'),
  { title: 'Network namespace' },
);

/**
 * `interfaces.<name>.lcp` — mirror this VPP interface into Linux with a linux-cp pair (VPP `lcp_itf_pair_add_del_v3`):
 * packets for the router's own addresses on it reach the Linux host (FRR's BGP/OSPF sessions), and linux-nl syncs the
 * routes FRR installs in Linux back into VPP's FIB. Present = the pair exists; absent = no pair.
 */
export const InterfaceLcpSchema = z.strictObject({
  hostIfName: withUi(lcpHostIfName.optional(), {
    title: 'Linux interface name',
    help: 'name of the host tap; default: the VPP name when it is a valid Linux name (max 15 characters)',
    order: 1,
  }),
  hostIfType: withUi(z.enum(['tap', 'tun']).default('tap'), {
    title: 'Host interface type',
    help: 'tap (Ethernet, default) or tun (L3 only, point-to-point links)',
    widget: 'select',
    order: 2,
  }),
  netns: withUi(lcpNetnsName.optional(), {
    title: 'Network namespace',
    help: 'namespace of the host tap; default: the linux-cp default namespace (where FRR runs)',
    order: 3,
  }),
});
export type InterfaceLcpConfig = z.infer<typeof InterfaceLcpSchema>;

/** The `lcp` member of `InterfaceSchema` (optional: absent = no Linux pair). */
export const interfaceLcp = withUi(InterfaceLcpSchema.optional(), {
  title: 'Linux pair (linux-cp)',
  help: 'mirror the interface into Linux so FRR can run routing protocols on it',
  group: GROUP,
  order: 30,
});

/**
 * The `tag` member of `StaticRouteSchema` (RF-1 Q1): the route tag FRR's staticd carries (`ip route … tag N`), matched by
 * `match tag` in route maps when the route is redistributed. Rendered only for FRR-programmed routes (`viaFrr`); VPP has
 * no route tag, so a tag on a route the agent programs is a warning.
 */
export const staticRouteTag = withUi(uint32.min(1).optional(), {
  title: 'Route tag',
  help: 'tag carried into routing protocols on redistribution (FRR routes only, see Program via FRR)',
  group: GROUP,
  order: 8,
});
