import { z } from 'zod';
import { vppInterfaceName } from '../../primitives.js';
import { withUi } from '../../ui.js';

/**
 * Interface extras (F-loopback-bvi-gso-lldp-span, WBS D1.8 / D1.10 / D1.9): per-interface generic segmentation
 * offload, port mirroring (SPAN, and ERSPAN by mirroring to a GRE tunnel of type `erspan`) and the network delay
 * simulator (`nsim`, a lab tool — WBS D1.9 is T3, "test tool, not a product feature").
 *
 * - `interfaces.<if>.gso` ({@link interfaceGsoField}): VPP's software GSO feature (`feature_gso_enable_disable`) on
 *   the interface's output path; parent interfaces only (sub-interfaces have no such key).
 * - `interfaces.<if>.mirror[]` ({@link interfaceMirrorField}): the interface is the SOURCE of one mirror session per
 *   entry (`sw_interface_span_enable_disable`), copying rx, tx or both to `destination` at the device level or in
 *   the L2 path.
 * - `services.nsim` ({@link servicesNsimField}): the VPP-global delay / bandwidth / loss model (`nsim_configure2`),
 *   the optional cross-connect of two interfaces and the per-interface output feature. VPP holds exactly one nsim
 *   configuration and one cross-connect pair, so only the globals owner applies it (D-071).
 *
 * Loopbacks (and loopbacks as bridge BVI) need no key here: `interfaces.loop<N>` creates a loopback (P08) and
 * `interfaces.loop<N>.l2 = { bridgeDomain, bvi: true }` makes it the BVI (F-bridge-l2). LLDP is `services.lldp`
 * (P02c). Cross-field rules are in `../../semantic/loopback-bvi-gso-lldp-span.ts`
 * (`interfaces.loopback-bvi-gso-lldp-span-*`, `services.loopback-bvi-gso-lldp-span-*`).
 */

const UI_GROUP = 'loopback-bvi-gso-lldp-span';

/** Loopback instances reserved for the agent's V19 quarantine holders (TD-3, D-105): never configurable. */
export const RESERVED_LOOPBACK_MIN = 16000;
export const RESERVED_LOOPBACK_MAX = 16383;

/** Mirrored direction of a session (VPP `span_state_t`: rx, tx, rx+tx). */
export const MirrorDirection = z.enum(['rx', 'tx', 'both']);
export type MirrorDirection = z.infer<typeof MirrorDirection>;

/** Where the copies are taken: at the device (all traffic) or in the L2 path (bridged / cross-connected traffic). */
export const MirrorLevel = z.enum(['device', 'l2']);
export type MirrorLevel = z.infer<typeof MirrorLevel>;

/** Maximum mirror sessions per source interface. */
export const MIRROR_MAX_SESSIONS = 8;

/** One mirror session with this interface as its source. */
export const MirrorSessionSchema = z.strictObject({
  destination: withUi(vppInterfaceName, {
    title: 'Destination',
    help: 'interface that transmits the copies — a monitor port, or a GRE tunnel of type erspan for ERSPAN',
    widget: 'interface-picker',
    order: 1,
  }),
  direction: withUi(MirrorDirection.default('both'), {
    title: 'Direction',
    help: 'received (rx), transmitted (tx) or both',
    widget: 'select',
    order: 2,
  }),
  level: withUi(MirrorLevel.default('device'), {
    title: 'Level',
    help: 'device: every frame of the interface; l2: only frames in the L2 path (bridge member or cross-connect)',
    widget: 'select',
    order: 3,
  }),
});
export type MirrorSessionConfig = z.infer<typeof MirrorSessionSchema>;

/** Bounds of the nsim model (VPP 26.06 nsim.c: bandwidth > 0, delay > 0, packet size 64–9000). */
export const NSIM_DELAY_MS_MAX = 10000;
export const NSIM_BANDWIDTH_MBPS_MAX = 100000;
export const NSIM_PACKET_SIZE_MIN = 64;
export const NSIM_PACKET_SIZE_MAX = 9000;
/**
 * Upper bound of the scheduler wheel VPP allocates per worker: delay × bandwidth / 8 / packet size slots of 32 bytes
 * (nsim_wheel_entry_t). 2^20 slots = 32 MiB per thread — the model is refused above it (a lab tool must not eat the
 * shared host's memory).
 */
export const NSIM_WHEEL_SLOTS_MAX = 1048576;

/** Wheel slots VPP allocates per worker for a model (nsim.c nsim_configure, single worker). */
export function nsimWheelSlots(delayMs: number, bandwidthMbps: number, packetSize: number): number {
  const bytes = Math.floor(((delayMs / 1000) * bandwidthMbps * 1e6) / 8 + 0.5);
  return Math.floor(bytes / packetSize) + 1;
}

/** The nsim cross-connect: frames received on one interface leave on the other after the model. */
export const NsimCrossConnectSchema = z.strictObject({
  a: withUi(vppInterfaceName, { title: 'Interface A', widget: 'interface-picker', order: 1 }),
  b: withUi(vppInterfaceName, { title: 'Interface B', widget: 'interface-picker', order: 2 }),
});
export type NsimCrossConnectConfig = z.infer<typeof NsimCrossConnectSchema>;

/** `services.nsim` — the network delay simulator (lab tool). */
export const NsimSchema = z.strictObject({
  delayMs: withUi(z.number().min(0.001).max(NSIM_DELAY_MS_MAX), {
    title: 'Delay (ms)',
    help: 'one-way delay added to every frame (VPP delay_in_usec; microsecond resolution)',
    widget: 'number',
    order: 1,
  }),
  bandwidthMbps: withUi(z.number().min(0.001).max(NSIM_BANDWIDTH_MBPS_MAX), {
    title: 'Bandwidth (Mbit/s)',
    help: 'simulated link rate; frames beyond it are queued and then dropped',
    widget: 'number',
    order: 2,
  }),
  packetSize: withUi(
    z.number().int().min(NSIM_PACKET_SIZE_MIN).max(NSIM_PACKET_SIZE_MAX).default(1500),
    {
      title: 'Average packet size (bytes)',
      help: 'sizes the scheduler wheel (64–9000)',
      widget: 'number',
      order: 3,
    },
  ),
  dropFraction: withUi(z.number().min(0).max(1).default(0), {
    title: 'Drop fraction',
    help: '0 = no random loss; 0.01 = one frame in 100 (VPP packets_per_drop = 1 / fraction)',
    widget: 'number',
    order: 4,
  }),
  crossConnect: withUi(NsimCrossConnectSchema.optional(), {
    title: 'Cross-connect',
    help: 'two interfaces joined through the simulator (VPP keeps one pair)',
    order: 5,
  }),
  outputInterfaces: withUi(z.array(vppInterfaceName).max(64).default([]), {
    title: 'Output interfaces',
    help: 'interfaces whose transmitted frames go through the simulator (output feature)',
    order: 6,
  }),
});
export type NsimConfig = z.infer<typeof NsimSchema>;

/** The `interfaces.<if>.gso` key line (`Interface` field 20). */
export const interfaceGsoField = withUi(z.boolean().optional(), {
  title: 'GSO',
  help: 'software generic segmentation offload on output (VPP gso feature); absent = off',
  widget: 'switch',
  group: UI_GROUP,
  order: 40,
});

/** The `interfaces.<if>.mirror` key line (`Interface` field 21). */
export const interfaceMirrorField = withUi(
  z.array(MirrorSessionSchema).max(MIRROR_MAX_SESSIONS).optional(),
  {
    title: 'Port mirroring',
    help: 'mirror sessions with this interface as the source (SPAN; ERSPAN through a GRE erspan tunnel)',
    group: UI_GROUP,
    order: 41,
  },
);

/** The `services.nsim` key line (`ServicesConfig` field 9). */
export const servicesNsimField = withUi(NsimSchema.optional(), {
  title: 'Network delay simulator (lab)',
  description:
    'VPP nsim: delay, bandwidth and loss between two cross-connected interfaces or on output. A lab tool; applied only by the globals owner.',
  group: 'nsim',
  order: 20,
});
