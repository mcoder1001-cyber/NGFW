import { z } from 'zod';
import { ipv4Address, routerId, secretRefOf, vppInterfaceName } from '../../primitives.js';
import { withUi } from '../../ui.js';

/**
 * F-mpls-ldp: `routing.mpls.ldp` — LDP (RFC 5036, IPv4) label distribution. FRR `ldpd` runs the protocol; the agent
 * renders its config and syncs the resulting label state into the VPP MPLS FIB (V5). This is the contract; the FRR
 * section, the FRR→VPP label sync and the live state come with the host build. Neighbour authentication is a
 * `password/<name>` secret reference, never inline (D-051).
 */

/** MPLS label values usable for a dynamic block (0–15 are reserved). */
const mplsLabel = z.number().int().min(16).max(1_048_575);

/** Per-neighbour LDP settings, keyed by the peer LSR-ID (an IPv4 address). */
export const LdpNeighborSchema = z.strictObject({
  passwordRef: withUi(secretRefOf('password'), {
    title: 'MD5 password',
    help: 'password/<name> for the LDP session (TCP-MD5); never the password itself',
  }).optional(),
});
export type LdpNeighbor = z.infer<typeof LdpNeighborSchema>;

/** The dynamic label block LDP allocates from (FRR `mpls label dynamic-block`). */
export const LdpLabelRangeSchema = z
  .strictObject({
    min: withUi(mplsLabel.default(16), { title: 'Minimum label', order: 1 }),
    max: withUi(mplsLabel.default(1_048_575), { title: 'Maximum label', order: 2 }),
  })
  .refine((r) => r.min <= r.max, { message: 'min must be ≤ max', path: ['max'] });
export type LdpLabelRange = z.infer<typeof LdpLabelRangeSchema>;

export const MplsLdpSchema = z.strictObject({
  routerId: withUi(routerId, {
    title: 'Router ID',
    help: 'the LDP router-id (an IPv4 address, usually a loopback)',
    order: 1,
  }),
  transportAddress: withUi(ipv4Address, {
    title: 'Transport address',
    help: 'the address LDP sessions are established over; must be configured on a default-VRF interface or loopback',
    order: 2,
  }),
  interfaces: withUi(z.array(vppInterfaceName).min(1).max(256).default([]), {
    title: 'Interfaces',
    widget: 'interface-picker',
    help: 'interfaces LDP runs on; each must be MPLS-enabled (listed in routing.mpls.interfaces) and in the default VRF',
    order: 3,
  }),
  neighbors: withUi(z.record(ipv4Address, LdpNeighborSchema).default({}), {
    title: 'Neighbours',
    help: 'per-peer settings keyed by the peer LSR-ID',
    order: 4,
  }),
  labelRange: withUi(LdpLabelRangeSchema.optional(), {
    title: 'Dynamic label range',
    help: 'the block LDP allocates local labels from; omit for FRR’s default',
    order: 5,
  }),
});
export type MplsLdp = z.infer<typeof MplsLdpSchema>;

/** The `ldp` member of `routing.mpls` (optional: absent = LDP not configured). */
export const mplsLdpField = withUi(MplsLdpSchema.optional(), {
  title: 'LDP',
  help: 'LDP label distribution (FRR ldpd; the agent syncs labels into the VPP MPLS FIB)',
  order: 7,
});
