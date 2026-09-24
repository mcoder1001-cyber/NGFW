import { z } from 'zod';
import { withUi } from '../ui.js';
import { hostname, ipAddress, objectName, vppInterfaceName } from '../primitives.js';
import {
  descriptionField,
  enabledFlag,
  secretRefOf,
  transportPort,
  vrfRef,
} from './_shared/primitives.js';

/**
 * `ha` — High availability: VRRPv3 virtual routers (WBS D9.1; native VPP `vrrp` plugin or keepalived, a separate
 * GPL process driven by rendered config — 00-CONTEXT rule 7) and cluster membership / configuration sync (D9.2,
 * state sync flags for D9.3).
 *
 * `vrrp` is a record keyed by name (D-053: pointer-addressable, merge-patchable — docs/04's `"vrrp": [...]` sketch
 * is amended); a virtual router's *network* identity is (interface, addressFamily, vrId), which `semantic/ha.ts`
 * keeps unique. VRRPv3 (RFC 5798) allows the same VRID for an IPv4 and an IPv6 virtual router on one interface,
 * and so does VPP (`vrrp_vr_add_del` keys on sw_if_index + is_ipv6 + vr_id).
 *
 * Domain root: `vrrp` defaults to an empty record (D-017, D-053); `cluster` stays optional (it has required
 * fields). Helper primitives come from `./_shared/primitives.ts` (not re-exported, D-054).
 * Owner: P02c. Only P02c edits this file.
 */

const isIpv6 = (a: string): boolean => a.includes(':');

export const VrrpTrackSchema = z.strictObject({
  interface: withUi(vppInterfaceName, { title: 'Tracked interface' }),
  priorityDecrement: withUi(z.int().min(1).max(253).default(10), {
    title: 'Priority decrement',
    widget: 'number',
    help: 'Subtracted from the priority while the tracked interface is down',
  }),
});

export const VrrpInstanceSchema = z
  .strictObject({
    enabled: enabledFlag,
    description: descriptionField.optional(),
    interface: withUi(vppInterfaceName, {
      title: 'Interface',
      help: 'Interface the virtual router runs on',
    }),
    vrId: withUi(z.int().min(1).max(255), {
      title: 'VRID',
      widget: 'number',
      help: 'Virtual router id, unique per interface and address family',
    }),
    addressFamily: withUi(z.enum(['ipv4', 'ipv6']).default('ipv4'), {
      title: 'Address family',
      widget: 'select',
    }),
    priority: withUi(z.int().min(1).max(255).default(100), {
      title: 'Priority',
      widget: 'number',
      help: '255 = address owner',
    }),
    advertisementIntervalMs: withUi(z.int().min(10).max(40950).multipleOf(10).default(1000), {
      title: 'Advertisement interval (ms)',
      widget: 'number',
      help: 'Multiple of 10 ms (VRRPv3 centiseconds)',
    }),
    preempt: withUi(z.boolean().default(true), { title: 'Preempt', widget: 'switch' }),
    acceptMode: withUi(z.boolean().default(false), {
      title: 'Accept mode',
      widget: 'switch',
      help: 'Master accepts packets addressed to the virtual addresses it does not own',
    }),
    unicast: withUi(
      z.strictObject({
        peers: withUi(z.array(ipAddress).min(1).max(16), { title: 'Peer addresses' }),
      }),
      { title: 'Unicast VRRP', help: 'Omit for multicast advertisements' },
    ).optional(),
    addresses: withUi(z.array(ipAddress).min(1).max(32), {
      title: 'Virtual addresses',
      help: 'Same address family as the virtual router',
    }),
    vrf: vrfRef,
    engine: withUi(z.enum(['vpp', 'keepalived']).default('vpp'), {
      title: 'Engine',
      widget: 'select',
      help: 'Native VPP vrrp plugin or keepalived',
    }),
    track: withUi(z.array(VrrpTrackSchema).max(16).default([]), {
      title: 'Tracked interfaces',
    }),
  })
  .superRefine((v, ctx) => {
    const wantV6 = v.addressFamily === 'ipv6';
    const seen = new Set<string>();
    v.addresses.forEach((a, i) => {
      if (isIpv6(a) !== wantV6) {
        ctx.addIssue({
          code: 'custom',
          path: ['addresses', i],
          message: `${a} does not match address family ${v.addressFamily}`,
        });
      }
      if (seen.has(a)) {
        ctx.addIssue({ code: 'custom', path: ['addresses', i], message: `duplicate address ${a}` });
      }
      seen.add(a);
    });
    const peers = new Set<string>();
    v.unicast?.peers.forEach((p, i) => {
      if (isIpv6(p) !== wantV6) {
        ctx.addIssue({
          code: 'custom',
          path: ['unicast', 'peers', i],
          message: `${p} does not match address family ${v.addressFamily}`,
        });
      }
      if (peers.has(p)) {
        ctx.addIssue({
          code: 'custom',
          path: ['unicast', 'peers', i],
          message: `duplicate peer ${p}`,
        });
      }
      peers.add(p);
    });
    const tracked = new Set<string>();
    v.track.forEach((t, i) => {
      if (t.interface === v.interface) {
        ctx.addIssue({
          code: 'custom',
          path: ['track', i, 'interface'],
          message: 'a virtual router cannot track its own interface',
        });
      }
      if (tracked.has(t.interface)) {
        ctx.addIssue({
          code: 'custom',
          path: ['track', i, 'interface'],
          message: `interface ${t.interface} is tracked twice`,
        });
      }
      tracked.add(t.interface);
    });
  });

export const HaClusterSchema = z
  .strictObject({
    enabled: withUi(z.boolean().default(false), { title: 'Enabled', widget: 'switch' }),
    nodeName: withUi(hostname, { title: 'This node', help: 'Name of this member' }),
    peers: withUi(
      z
        .array(
          z.strictObject({
            name: withUi(hostname, { title: 'Peer name' }),
            address: withUi(ipAddress, { title: 'Peer address' }),
          }),
        )
        .min(1)
        .max(8),
      { title: 'Peers' },
    ),
    port: withUi(transportPort.default(4370), { title: 'Cluster port' }),
    secretRef: withUi(secretRefOf('key'), {
      title: 'Cluster authentication key (reference)',
      help: 'Shared key protecting the sync channel; stored through POST /api/v1/secrets (kind key)',
    }),
    interface: withUi(vppInterfaceName, {
      title: 'Sync interface',
      help: 'Interface carrying the cluster traffic (optional, informational for the renderer)',
    }).optional(),
    vrf: vrfRef,
    configSync: withUi(z.boolean().default(true), {
      title: 'Configuration synchronisation',
      widget: 'switch',
    }),
    stateSync: withUi(
      z.strictObject({
        nat: withUi(z.boolean().default(false), { title: 'NAT sessions', widget: 'switch' }),
        ipsec: withUi(z.boolean().default(false), { title: 'IPsec SAs', widget: 'switch' }),
        acl: withUi(z.boolean().default(false), { title: 'ACL sessions', widget: 'switch' }),
      }),
      { title: 'State synchronisation' },
    ).prefault({}),
  })
  .superRefine((c, ctx) => {
    const names = new Set<string>();
    const addresses = new Set<string>();
    c.peers.forEach((p, i) => {
      if (p.name === c.nodeName) {
        ctx.addIssue({
          code: 'custom',
          path: ['peers', i, 'name'],
          message: 'a peer cannot have the name of this node',
        });
      }
      if (names.has(p.name)) {
        ctx.addIssue({
          code: 'custom',
          path: ['peers', i, 'name'],
          message: `duplicate peer ${p.name}`,
        });
      }
      if (addresses.has(p.address)) {
        ctx.addIssue({
          code: 'custom',
          path: ['peers', i, 'address'],
          message: `duplicate peer address ${p.address}`,
        });
      }
      names.add(p.name);
      addresses.add(p.address);
    });
  });

export const HaSchema = withUi(
  z.strictObject({
    vrrp: withUi(z.record(objectName, VrrpInstanceSchema).default({}), {
      title: 'VRRP virtual routers',
      widget: 'record',
      group: 'vrrp',
      order: 1,
      help: 'Keyed by name; a virtual router is (interface, address family, VRID) on the wire',
    }),
    cluster: withUi(HaClusterSchema, { title: 'Cluster', group: 'cluster', order: 2 }).optional(),
  }),
  {
    title: 'High availability',
    description:
      'VRRPv3 virtual routers (VPP plugin or keepalived) and cluster membership / config sync.',
    order: 120,
  },
);

export type HaConfig = z.infer<typeof HaSchema>;
export type VrrpInstance = z.infer<typeof VrrpInstanceSchema>;
export type HaCluster = z.infer<typeof HaClusterSchema>;
