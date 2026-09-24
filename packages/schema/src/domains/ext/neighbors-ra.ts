import { z } from 'zod';
import {
  ipAddress,
  ipv4Address,
  ipv6Address,
  ipv6Network,
  macAddress,
  uint32,
  vppInterfaceName,
} from '../../primitives.js';
import { parseIpv4 } from '../../ip.js';
import { withUi } from '../../ui.js';

/**
 * F-neighbors-ra (WBS D2.3): the neighbour database and IPv6 router advertisements.
 *
 * - `interfaces.<if>.ipv6Ra` / `.proxyArp` / `.proxyNd` (and the same three on sub-interfaces): per-interface router
 *   advertisements (VPP `sw_interface_ip6nd_ra_config` + `…_ra_prefix`), proxy ARP (`proxy_arp_intfc_enable_disable`)
 *   and proxied IPv6 addresses (`ip6nd_proxy_add_del`, experimental and off by default: docs/vpp-code-track.md V12).
 * - `vrfs.<name>.proxyArpRanges`: proxy-ARP address ranges of the VRF's FIB (`proxy_arp_add_del`, IPv4 only).
 * - `routing.neighbors`: static ARP/ND entries (`ip_neighbor_add_del`), the neighbour-DB limits per family
 *   (`ip_neighbor_config`) and IPv6 duplicate address detection (`ip6_dad_enable_disable`). Limits and DAD are VPP-wide
 *   settings: only the globals owner (the product agent on a real box, D-071) applies them.
 *
 * Absent or default means "off" everywhere (interface drawer saves write defaults back): an `ipv6Ra` object that holds
 * only its defaults is VPP's own fresh-interface state (RAs suppressed, lifetime 600 s, intervals 200/150 s), `proxyArp`
 * absent or false is off, an empty `proxyNd` list proxies nothing.
 *
 * Cross-object rules (connected subnet of a static neighbour, IPv6 enabled under RA/proxy-ND, /64 for SLAAC prefixes,
 * interval ordering, unique keys) live in `../../semantic/neighbors-ra.ts`.
 */

/** UI group of every per-interface field of this feature (x-vrx-ui group = task slug, wave-A-hotspots C1). */
export const NEIGHBORS_RA_GROUP = 'neighbors-ra';

/** VPP 26.06 RA defaults (ip6_ra.c; DF-2 verified them on vrx-a with sw_interface_ip6nd_ra_dump). */
export const RA_DEFAULTS = {
  suppress: true,
  managed: false,
  other: false,
  lifetimeSec: 600,
  maxIntervalSec: 200,
  minIntervalSec: 150,
} as const;

/** VPP bounds: router lifetime is clamped to 9000 s (MAX_DEF_RTR_LIFETIME); RFC 4861 caps MaxRtrAdvInterval at 1800 s. */
export const RA_MAX_LIFETIME_SEC = 9000;
export const RA_MAX_INTERVAL_SEC = 1800;
export const RA_MIN_INTERVAL_SEC = 3;

/** Prefix-information lifetimes VPP uses when none are given (DEF_ADV_VALID_LIFETIME / DEF_ADV_PREF_LIFETIME). */
export const RA_PREFIX_DEFAULTS = { validSec: 2592000, preferredSec: 604800 } as const;

/**
 * One advertised prefix (prefix-information option) of `ipv6Ra.prefixes`, keyed by the IPv6 network. Lifetimes are ≥ 1:
 * DF-2 sends 0 as "VPP default" (NormalizeRaPrefix), so a 0 could never be applied as written.
 */
export const Ipv6RaPrefixSchema = z
  .strictObject({
    validSec: withUi(uint32.min(1).default(RA_PREFIX_DEFAULTS.validSec), {
      title: 'Valid lifetime (s)',
      help: 'valid lifetime advertised for the prefix, at least 1; 4294967295 = infinite',
      order: 1,
    }),
    preferredSec: withUi(uint32.min(1).default(RA_PREFIX_DEFAULTS.preferredSec), {
      title: 'Preferred lifetime (s)',
      help: 'preferred lifetime, at least 1; must not exceed the valid lifetime',
      order: 2,
    }),
    offLink: withUi(z.boolean().default(false), {
      title: 'Off-link',
      help: 'clear the on-link (L) flag',
      order: 3,
    }),
    noAutoconfig: withUi(z.boolean().default(false), {
      title: 'No autoconfiguration',
      help: 'clear the autonomous (A) flag: hosts do not form SLAAC addresses from this prefix',
      order: 4,
    }),
  })
  .refine((p) => p.preferredSec <= p.validSec, {
    message: 'the preferred lifetime must not exceed the valid lifetime',
    path: ['preferredSec'],
  });
export type Ipv6RaPrefixConfig = z.infer<typeof Ipv6RaPrefixSchema>;

/**
 * IPv6 router advertisements of one interface. Present = managed by the configuration; the defaults are VPP's
 * fresh-interface state, so an object holding only defaults changes nothing (RAs stay suppressed).
 */
export const Ipv6RaSchema = z
  .strictObject({
    suppress: withUi(z.boolean().default(RA_DEFAULTS.suppress), {
      title: 'Suppress router advertisements',
      help: 'untick to send RAs on this interface (VPP suppresses them by default)',
      order: 1,
    }),
    managed: withUi(z.boolean().default(RA_DEFAULTS.managed), {
      title: 'Managed flag (M)',
      help: 'hosts obtain addresses by DHCPv6',
      order: 2,
    }),
    other: withUi(z.boolean().default(RA_DEFAULTS.other), {
      title: 'Other configuration flag (O)',
      help: 'hosts obtain other settings (DNS …) by DHCPv6',
      order: 3,
    }),
    lifetimeSec: withUi(
      z.number().int().min(0).max(RA_MAX_LIFETIME_SEC).default(RA_DEFAULTS.lifetimeSec),
      {
        title: 'Router lifetime (s)',
        help: '0 = not a default router; otherwise greater than the maximum interval, at most 9000',
        widget: 'number',
        order: 4,
      },
    ),
    maxIntervalSec: withUi(
      z.number().int().min(4).max(RA_MAX_INTERVAL_SEC).default(RA_DEFAULTS.maxIntervalSec),
      {
        title: 'Maximum interval (s)',
        help: 'between unsolicited RAs, 4–1800',
        widget: 'number',
        order: 5,
      },
    ),
    minIntervalSec: withUi(
      z
        .number()
        .int()
        .min(RA_MIN_INTERVAL_SEC)
        .max(RA_MAX_INTERVAL_SEC)
        .default(RA_DEFAULTS.minIntervalSec),
      {
        title: 'Minimum interval (s)',
        help: 'at least 3 and at most 0.75 × the maximum interval',
        widget: 'number',
        order: 6,
      },
    ),
    prefixes: withUi(z.record(ipv6Network, Ipv6RaPrefixSchema).default({}), {
      title: 'Advertised prefixes',
      help: 'prefix-information options keyed by prefix, e.g. 2001:db8:1::/64 (SLAAC needs a /64)',
      order: 7,
    }),
  })
  // VPP rejects these with VNET_API_ERROR_INVALID_VALUE (ip6_ra_config): min ≤ 0.75 × max, lifetime 0 or > max.
  .refine((ra) => ra.minIntervalSec <= 0.75 * ra.maxIntervalSec, {
    message: 'the minimum interval must be at most 0.75 × the maximum interval',
    path: ['minIntervalSec'],
  })
  .refine((ra) => ra.lifetimeSec === 0 || ra.lifetimeSec > ra.maxIntervalSec, {
    message:
      'the router lifetime must be 0 (not a default router) or greater than the maximum interval',
    path: ['lifetimeSec'],
  });
export type Ipv6RaConfig = z.infer<typeof Ipv6RaSchema>;

/** `interfaces.<if>.ipv6Ra` (and on sub-interfaces). */
export const ipv6RaField = withUi(Ipv6RaSchema.optional(), {
  title: 'IPv6 router advertisements',
  help: 'absent = VPP defaults (RAs suppressed); the interface needs an IPv6 address',
  group: NEIGHBORS_RA_GROUP,
  order: 42,
});

/** `interfaces.<if>.proxyArp`: answer ARP requests for addresses in the VRF's proxy-ARP ranges. */
export const proxyArpField = withUi(z.boolean().optional(), {
  title: 'Proxy ARP',
  help: 'answer ARP for the proxy-ARP ranges of this interface’s VRF; absent = off',
  group: NEIGHBORS_RA_GROUP,
  order: 40,
});

/** `interfaces.<if>.proxyNd`: IPv6 addresses this interface answers neighbour solicitations for (experimental, V12). */
export const proxyNdField = withUi(z.array(ipv6Address).max(64).optional(), {
  title: 'Proxy ND addresses (experimental)',
  help: 'experimental and off by default: applied only by an agent started with VRX_DF2_PROXY_ND=1 (VPP V12)',
  group: NEIGHBORS_RA_GROUP,
  order: 41,
});

/** One proxy-ARP range (IPv4 only: ARP is IPv4) of `vrfs.<name>.proxyArpRanges`. */
export const ProxyArpRangeSchema = z
  .strictObject({
    low: withUi(ipv4Address, { title: 'First address', order: 1 }),
    high: withUi(ipv4Address, { title: 'Last address', order: 2 }),
  })
  .refine((r) => (parseIpv4(r.low) ?? 0n) <= (parseIpv4(r.high) ?? -1n), {
    message: 'the last address must not be below the first address',
    path: ['high'],
  });
export type ProxyArpRangeConfig = z.infer<typeof ProxyArpRangeSchema>;

/** `vrfs.<name>.proxyArpRanges`. */
export const proxyArpRangesField = withUi(z.array(ProxyArpRangeSchema).max(256).optional(), {
  title: 'Proxy-ARP ranges',
  help: 'IPv4 ranges answered by proxy ARP on interfaces of this VRF that enable it',
  itemKey: ['low', 'high'],
  group: NEIGHBORS_RA_GROUP,
  order: 10,
});

/** One static ARP / ND entry of `routing.neighbors.static`. */
export const StaticNeighborSchema = z.strictObject({
  interface: withUi(vppInterfaceName, { title: 'Interface', widget: 'interface-picker', order: 1 }),
  ip: withUi(ipAddress, {
    title: 'IP address',
    help: 'must lie in a connected subnet of the interface',
    order: 2,
  }),
  mac: withUi(macAddress, { title: 'MAC address', order: 3 }),
  noFibEntry: withUi(z.boolean().default(false), {
    title: 'No FIB entry',
    help: 'do not install the /32 or /128 host route for this neighbour',
    order: 4,
  }),
});
export type StaticNeighborConfig = z.infer<typeof StaticNeighborSchema>;

/** Neighbour-database limits of one address family (`ip_neighbor_config`); VPP defaults 50000 / 0 / false. */
export const NeighborLimitsSchema = z.strictObject({
  maxNumber: withUi(z.number().int().min(1).max(4294967295).default(50000), {
    title: 'Maximum entries',
    widget: 'number',
    order: 1,
  }),
  maxAgeSec: withUi(uint32.default(0), {
    title: 'Maximum age (s)',
    help: '0 = learned entries never age out',
    order: 2,
  }),
  recycle: withUi(z.boolean().default(false), {
    title: 'Recycle',
    help: 'replace the oldest entry when the table is full',
    order: 3,
  }),
});
export type NeighborLimitsConfig = z.infer<typeof NeighborLimitsSchema>;

/** IPv6 duplicate address detection (VPP 26.06 `ip6_dad`); present = enabled. RFC 4862 defaults 1 × 1000 ms. */
export const NeighborDadSchema = z.strictObject({
  transmits: withUi(z.number().int().min(1).max(10).default(1), {
    title: 'Probes',
    help: 'neighbour solicitations sent per tentative address (1–10)',
    widget: 'number',
    order: 1,
  }),
  delayMs: withUi(z.number().int().min(100).max(10000).default(1000), {
    title: 'Retransmit delay (ms)',
    help: 'between probes, 100–10000 ms',
    widget: 'number',
    order: 2,
  }),
});
export type NeighborDadConfig = z.infer<typeof NeighborDadSchema>;

/** `routing.neighbors`. */
export const NeighborsSchema = z.strictObject({
  static: withUi(z.array(StaticNeighborSchema).max(4096).default([]), {
    title: 'Static neighbours',
    itemKey: ['interface', 'ip'],
    order: 1,
  }),
  ipv4Limits: withUi(NeighborLimitsSchema.optional(), {
    title: 'IPv4 (ARP) table limits',
    help: 'VPP-wide: applied by the globals owner only; absent = VPP defaults',
    order: 2,
  }),
  ipv6Limits: withUi(NeighborLimitsSchema.optional(), {
    title: 'IPv6 (ND) table limits',
    help: 'VPP-wide: applied by the globals owner only; absent = VPP defaults',
    order: 3,
  }),
  dad: withUi(NeighborDadSchema.optional(), {
    title: 'Duplicate address detection',
    help: 'VPP-wide, present = enabled: applied by the globals owner only',
    order: 4,
  }),
});
export type NeighborsConfig = z.infer<typeof NeighborsSchema>;

/** `routing.neighbors` field. */
export const neighborsField = withUi(NeighborsSchema.optional(), {
  title: 'Neighbours',
  help: 'static ARP/ND entries, neighbour-table limits and duplicate address detection',
  group: NEIGHBORS_RA_GROUP,
  order: 8,
});
