import { z } from 'zod';
import { hostOrIp, ipAddress, objectName, vppInterfaceName } from '../../primitives.js';
import { withUi } from '../../ui.js';

/**
 * F-multiwan: `routing.wanGroups` — two or more WAN links kept up by health monitors, with failover (default route via
 * the best healthy member) or weighted load balancing (ECMP over healthy members). Policy rules (ABF, F-rpf-adl-pbr)
 * can pin traffic to a group or member; NAT is per-member with sticky sessions. The routing/NAT/data-plane wiring is
 * the agent's; this is the contract, evaluated against member health the agent publishes.
 */

const GROUP = 'multiwan';

/** How a member learns its next hop / gateway. */
export const WanNextHopKind = z.enum(['gateway', 'dhcp', 'pppoe']);
export type WanNextHopKind = z.infer<typeof WanNextHopKind>;

export const WanMemberSchema = z
  .strictObject({
    interface: withUi(vppInterfaceName, {
      title: 'Interface',
      widget: 'interface-picker',
      order: 1,
    }),
    nextHop: withUi(WanNextHopKind.default('dhcp'), {
      title: 'Next hop',
      widget: 'select',
      help: 'gateway (static), dhcp (from the DHCP client), or pppoe (the PPPoE peer)',
      order: 2,
    }),
    gateway: withUi(ipAddress.optional(), {
      title: 'Gateway',
      help: 'required when Next hop is gateway; the upstream router on this link',
      order: 3,
    }),
    weight: withUi(z.number().int().min(1).max(255).default(1), {
      title: 'Weight',
      help: 'share of traffic in balance mode (ignored in failover)',
      order: 4,
    }),
    priority: withUi(z.number().int().min(0).max(255).default(100), {
      title: 'Priority',
      help: 'lower wins in failover mode (the healthy member with the lowest priority carries the default route)',
      order: 5,
    }),
  })
  .refine((m) => m.nextHop !== 'gateway' || m.gateway !== undefined, {
    message: 'a gateway address is required when Next hop is gateway',
    path: ['gateway'],
  });
export type WanMember = z.infer<typeof WanMemberSchema>;

/** A link-health probe sourced from each member (per-link source address / VRF). */
export const WanMonitorSchema = z.strictObject({
  type: withUi(z.enum(['icmp', 'http', 'dns']).default('icmp'), {
    title: 'Type',
    widget: 'select',
    order: 1,
  }),
  target: withUi(hostOrIp, {
    title: 'Target',
    help: 'icmp/dns: an address or host to probe; http: a host (probed with a HEAD request)',
    order: 2,
  }),
  intervalMs: withUi(z.number().int().min(100).max(600_000).default(1_000), {
    title: 'Interval (ms)',
    order: 3,
  }),
  timeoutMs: withUi(z.number().int().min(50).max(60_000).default(1_000), {
    title: 'Timeout (ms)',
    order: 4,
  }),
  lossPct: withUi(z.number().int().min(0).max(100).default(100), {
    title: 'Loss threshold (%)',
    help: 'the member is unhealthy when probe loss over the window reaches this',
    order: 5,
  }),
  latencyMs: withUi(z.number().int().min(0).max(60_000).default(0), {
    title: 'Latency threshold (ms)',
    help: 'the member is unhealthy above this average latency; 0 = no latency check',
    order: 6,
  }),
  downAfter: withUi(z.number().int().min(1).max(100).default(3), {
    title: 'Down after',
    help: 'consecutive failing checks before the member is marked down (hysteresis)',
    order: 7,
  }),
  upAfter: withUi(z.number().int().min(1).max(100).default(3), {
    title: 'Up after',
    help: 'consecutive passing checks before a down member is marked up again',
    order: 8,
  }),
});
export type WanMonitor = z.infer<typeof WanMonitorSchema>;

export const WanGroupSchema = z
  .strictObject({
    name: withUi(objectName, { title: 'Name', order: 1 }),
    mode: withUi(z.enum(['failover', 'balance']).default('failover'), {
      title: 'Mode',
      widget: 'select',
      help: 'failover = the best healthy member carries the default route; balance = weighted ECMP over healthy members',
      order: 2,
    }),
    stickySessions: withUi(z.boolean().default(true), {
      title: 'Sticky sessions',
      help: 'keep an established flow on its member while it is up; on failover the dead link’s NAT sessions are cleared',
      order: 3,
    }),
    members: withUi(z.array(WanMemberSchema).min(1).max(16).default([]), {
      title: 'Members',
      order: 4,
    }),
    monitors: withUi(z.array(WanMonitorSchema).min(1).max(8).default([]), {
      title: 'Health monitors',
      order: 5,
    }),
  })
  .superRefine((g, ctx) => {
    const seen = new Set<string>();
    g.members.forEach((m, i) => {
      if (seen.has(m.interface)) {
        ctx.addIssue({
          code: 'custom',
          path: ['members', i, 'interface'],
          message: `interface '${m.interface}' is a member twice`,
        });
      }
      seen.add(m.interface);
    });
  });
export type WanGroup = z.infer<typeof WanGroupSchema>;

/** `routing.wanGroups` field. */
export const wanGroupsField = withUi(z.array(WanGroupSchema).max(16).default([]), {
  title: 'WAN groups (multi-WAN)',
  help: 'multi-WAN failover / load-balancing groups with link health monitors',
  itemKey: ['name'],
  group: GROUP,
  order: 60,
});
