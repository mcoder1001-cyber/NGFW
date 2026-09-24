import { z } from 'zod';
import { ipAddress, objectName, uint32, vppInterfaceName, vrfName } from '../../primitives.js';
import { withUi } from '../../ui.js';
import { DEFAULT_VRF } from '../vrfs.js';

/**
 * F-rpf-adl-pbr (WBS D2.4, D2.7 ABF part, D3.9): anti-spoofing and policy routing.
 *
 *   interfaces.<if>.urpf   uRPF check per address family (VPP `urpf` plugin, `urpf_update_v2`)
 *   interfaces.<if>.adl    allow/deny list: source addresses checked against an allow-list VRF (VPP `adl` plugin)
 *   routing.pbr            ACL-based forwarding: packets matching an ACL of `acl.lists` are forwarded over the
 *                          policy's paths instead of the FIB (VPP `abf` plugin); attachments bind policies to interfaces
 *   services.autoSdl       automatic source deny list of the VPP host stack (VPP `auto_sdl` plugin, a VPP-global)
 *
 * Absent means off everywhere: an interface without `urpf`/`adl` has no check, `routing` without `pbr` has no policy.
 * The per-interface objects are optional objects whose defaults are "off" too, so a form that writes defaults back
 * changes nothing (P08 drawer). Cross-object rules (the ACL, VRFs, interfaces and policies that are referenced exist;
 * attachments are unique) live in `../../semantic/rpf-adl-pbr.ts`.
 */

/** `x-vrx-ui` group of every field of this feature (wave-A-hotspots C1: group = task slug). */
export const RPF_ADL_PBR_GROUP = 'rpf-adl-pbr';

/* ------------------------------------------------------------------------------------------------------ uRPF */

/** uRPF mode: `loose` — the source must be reachable at all; `strict` — reachable through the receiving interface. */
export const UrpfMode = z.enum(['loose', 'strict']);
export type UrpfMode = z.infer<typeof UrpfMode>;

/** Which packets are checked: received (`rx`, the usual anti-spoofing check) or sent (`tx`). */
export const UrpfDirection = z.enum(['rx', 'tx']);
export type UrpfDirection = z.infer<typeof UrpfDirection>;

export const UrpfSchema = z.strictObject({
  ipv4: withUi(UrpfMode.optional(), {
    title: 'IPv4 check',
    help: 'loose: the source must be routable; strict: routable through this interface; absent = no check',
    widget: 'select',
    order: 1,
  }),
  ipv6: withUi(UrpfMode.optional(), {
    title: 'IPv6 check',
    help: 'loose: the source must be routable; strict: routable through this interface; absent = no check',
    widget: 'select',
    order: 2,
  }),
  direction: withUi(UrpfDirection.default('rx'), {
    title: 'Direction',
    help: 'rx checks packets received on the interface, tx packets sent out of it',
    widget: 'select',
    order: 3,
  }),
});
export type UrpfConfig = z.infer<typeof UrpfSchema>;

/** `interfaces.<if>.urpf` — the key line of `domains/interfaces.ts`. */
export const urpfField = withUi(UrpfSchema.optional(), {
  title: 'Unicast RPF',
  help: 'reverse-path check of the source address; absent = off',
  group: RPF_ADL_PBR_GROUP,
  order: 40,
});

/* ------------------------------------------------------------------------------------------------------- ADL */

export const AdlSchema = z
  .strictObject({
    ipv4: withUi(z.boolean().default(false), {
      title: 'Check IPv4 sources',
      help: 'drop IPv4 packets whose source is not in the allow-list VRF',
      order: 1,
    }),
    ipv6: withUi(z.boolean().default(false), {
      title: 'Check IPv6 sources',
      help: 'drop IPv6 packets whose source is not in the allow-list VRF',
      order: 2,
    }),
    allowVrf: withUi(vrfName.optional(), {
      title: 'Allow-list VRF',
      help: 'VRF whose table holds the allowed source prefixes as local (receive) entries; required when a family is checked',
      order: 3,
    }),
    defaultAllow: withUi(z.boolean().default(true), {
      title: 'Pass non-IP frames',
      help: 'ARP and other non-IP frames are not checked (VPP 26.06 cannot filter them)',
      order: 4,
    }),
  })
  .refine((a) => !(a.ipv4 || a.ipv6) || a.allowVrf !== undefined, {
    message: 'allowVrf is required when ipv4 or ipv6 is checked',
    path: ['allowVrf'],
  });
export type AdlConfig = z.infer<typeof AdlSchema>;

/** `interfaces.<if>.adl` — the key line of `domains/interfaces.ts`. */
export const adlField = withUi(AdlSchema.optional(), {
  title: 'Allow/deny list (ADL)',
  help: 'source allow-list on the device input path (physical ports); off unless ipv4 or ipv6 is checked',
  group: RPF_ADL_PBR_GROUP,
  order: 41,
});

/* ------------------------------------------------------------------------------------------------------- PBR */

/** Address family of a policy attachment (ABF attaches per family). */
export const PbrFamily = z.enum(['ipv4', 'ipv6']);
export type PbrFamily = z.infer<typeof PbrFamily>;

/**
 * One forwarding path of a policy. `address` + `interface`: an attached next hop; `address` alone: resolved in
 * `vrf`; `interface` alone: out of that interface; neither: the packet is looked up again in `vrf` (e.g. forward
 * matching traffic by another VRF's routes).
 */
export const PbrPathSchema = z
  .strictObject({
    address: withUi(ipAddress.optional(), { title: 'Next-hop address', order: 1 }),
    interface: withUi(vppInterfaceName.optional(), { title: 'Egress interface', order: 2 }),
    vrf: withUi(vrfName.default(DEFAULT_VRF), {
      title: 'Lookup VRF',
      help: 'table the next hop is resolved in, or — without address and interface — the table the packet is looked up in',
      order: 3,
    }),
    weight: withUi(z.number().int().min(1).max(255).default(1), {
      title: 'Weight',
      help: 'relative share among the paths (1–255)',
      widget: 'number',
      order: 4,
    }),
  })
  .refine((p) => p.interface === undefined || p.vrf === DEFAULT_VRF, {
    message:
      'a path with an interface is resolved on that interface; vrf applies only to paths without one',
    path: ['vrf'],
  });
export type PbrPathConfig = z.infer<typeof PbrPathSchema>;

export const PbrPolicySchema = z.strictObject({
  acl: withUi(objectName, {
    title: 'ACL',
    help: 'name of an entry of acl.lists; packets it permits are forwarded over the paths',
    widget: 'acl-picker',
    order: 1,
  }),
  priority: withUi(uint32.default(100), {
    title: 'Priority',
    help: 'order among the policies attached to one interface (lower first)',
    order: 2,
  }),
  paths: withUi(z.array(PbrPathSchema).min(1).max(255), {
    title: 'Paths',
    help: 'next hops matching packets are forwarded over (weighted when more than one)',
    order: 3,
  }),
});
export type PbrPolicyConfig = z.infer<typeof PbrPolicySchema>;

export const PbrAttachmentSchema = z.strictObject({
  policy: withUi(objectName, {
    title: 'Policy',
    help: 'name of an entry of routing.pbr.policies',
    order: 1,
  }),
  interface: withUi(vppInterfaceName, {
    title: 'Interface',
    help: 'where packets are received',
    order: 2,
  }),
  family: withUi(PbrFamily.default('ipv4'), {
    title: 'Address family',
    widget: 'select',
    order: 3,
  }),
});
export type PbrAttachmentConfig = z.infer<typeof PbrAttachmentSchema>;

export const PbrSchema = z.strictObject({
  policies: withUi(z.record(objectName, PbrPolicySchema).default({}), {
    title: 'Policies',
    help: 'keyed by policy name (letters, digits, `_`, `.`, `-`; at most 63)',
    widget: 'record',
    order: 1,
  }),
  attachments: withUi(z.array(PbrAttachmentSchema).max(4096).default([]), {
    title: 'Attachments',
    itemKey: ['policy', 'interface', 'family'],
    order: 2,
  }),
});
export type PbrConfig = z.infer<typeof PbrSchema>;

/** `routing.pbr` — the key line of `domains/routing.ts`. */
export const pbrField = withUi(PbrSchema.optional(), {
  title: 'Policy-based routing',
  help: 'ACL-based forwarding (VPP abf); absent = no policy',
  group: RPF_ADL_PBR_GROUP,
  order: 8,
});

/* -------------------------------------------------------------------------------------------------- Auto-SDL */

export const AutoSdlSchema = z.strictObject({
  enabled: withUi(z.boolean().default(false), {
    title: 'Enabled',
    help: 'deny new host-stack TCP sessions from a source that exceeds the threshold',
    order: 1,
  }),
  threshold: withUi(z.number().int().min(1).max(4294967295).default(5), {
    title: 'Threshold',
    help: 'hits from one source before its deny entry is created',
    widget: 'number',
    order: 2,
  }),
  removeTimeoutSec: withUi(z.number().int().min(1).max(4294967295).default(300), {
    title: 'Remove timeout (s)',
    help: 'lifetime of an automatic deny entry',
    widget: 'number',
    order: 3,
  }),
});
export type AutoSdlConfig = z.infer<typeof AutoSdlSchema>;

/** `services.autoSdl` — the key line of `domains/services.ts`. */
export const autoSdlField = withUi(AutoSdlSchema.optional(), {
  title: 'Auto-SDL',
  help: 'automatic source deny list of the VPP host stack (a VPP-global setting); absent = off',
  group: RPF_ADL_PBR_GROUP,
  order: 8,
});
