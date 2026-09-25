import { z } from 'zod';
import { withUi } from '../ui.js';
import { ipAddress, ipv4Cidr, ipv6Cidr, objectName, vppInterfaceName } from '../primitives.js';
import { LispSchema } from './ext/lisp.js';
import { isMulticast, isUnspecified } from '../semantic/tunnels-common.js';
import {
  descriptionField,
  enabledFlag,
  mtuField,
  transportPort,
  u32Int,
  underlayVrfRef,
  vrfRef,
} from './_shared/primitives.js';

/**
 * `tunnels` — GRE (L3 / TEB / ERSPAN), VXLAN and IPIP tunnels, each a record keyed by tunnel name
 * (docs/04-api-datamodel.md, WBS D6.6). VXLAN-GPE, GTP-U, L2TPv3 and PPPoE (D6.6 "schedule last") are additive
 * later keys of this object.
 *
 * Every tunnel has two VRFs (vdom.md #1): `underlayVrf` is the FIB the encapsulated packets are looked up in
 * (src/dst live there); `vrf` is the FIB the tunnel interface itself belongs to (overlay). Names are unique within
 * the whole `tunnels` domain, not only within a kind (vdom.md #2; `semantic/tunnels.ts`).
 *
 * `instance` fixes the VPP interface name (`gre<n>`, `ipip<n>`, `vxlan_tunnel<n>`) so other domains (VRRP, LLDP,
 * IPFIX, DHCP …) can reference the tunnel as an interface; without it VPP allocates the name at runtime.
 *
 * Domain root: `gre`, `vxlan`, `ipip` default to empty records (D-017, D-053). Helper primitives come from
 * `./_shared/primitives.ts` (not re-exported, D-054). Owner: P02c. Only P02c edits this file.
 */

const underlayVrf = withUi(underlayVrfRef, {
  title: 'Underlay VRF',
  widget: 'vrf-picker',
  help: 'FIB used to reach the tunnel destination; the source address must be configured on an interface in it',
});

const tunnelCommon = {
  enabled: enabledFlag,
  description: descriptionField.optional(),
  instance: withUi(u32Int, {
    title: 'Instance',
    widget: 'number',
    help: 'Fixes the VPP interface name (gre<n>, ipip<n>, vxlan_tunnel<n>); required to reference the tunnel elsewhere',
  }).optional(),
  src: withUi(ipAddress, {
    title: 'Source address',
    help: 'Must be configured on an interface in the underlay VRF',
  }),
  underlayVrf,
  vrf: withUi(vrfRef, {
    title: 'VRF',
    widget: 'vrf-picker',
    help: 'FIB the tunnel interface belongs to (overlay)',
  }),
  mtu: mtuField.optional(),
  ipv4: withUi(z.array(ipv4Cidr).max(16).default([]), {
    title: 'IPv4 addresses',
    widget: 'cidr',
    help: 'Addresses of the tunnel interface',
  }),
  ipv6: withUi(z.array(ipv6Cidr).max(16).default([]), {
    title: 'IPv6 addresses',
    widget: 'cidr',
  }),
  bridgeDomain: withUi(u32Int, {
    title: 'Bridge domain',
    widget: 'number',
    help: 'L2 tunnels only: bridge domain the tunnel interface is attached to',
  }).optional(),
};

type Issue = (path: string, message: string) => void;

function issuer(ctx: z.RefinementCtx): Issue {
  return (path, message) => ctx.addIssue({ code: 'custom', path: [path], message });
}

/** Common endpoint sanity: families match, no unspecified/multicast source, dst ≠ src. */
function checkEndpoints(
  src: string,
  dst: string | undefined,
  issue: Issue,
  options: { multicastDst: boolean },
): void {
  if (isUnspecified(src)) issue('src', 'source address must not be unspecified');
  if (isMulticast(src)) issue('src', 'source address must not be a multicast address');
  if (dst === undefined) return;
  if (isUnspecified(dst)) issue('dst', 'destination address must not be unspecified');
  if (!options.multicastDst && isMulticast(dst)) {
    issue('dst', 'destination address must not be a multicast address');
  }
  if (src.includes(':') !== dst.includes(':')) {
    issue('dst', 'source and destination must be of the same address family');
  } else if (src === dst) {
    issue('dst', 'destination address equals the source address');
  }
}

function checkAddressesUnique(
  addresses: readonly string[],
  field: string,
  ctx: z.RefinementCtx,
): void {
  const seen = new Set<string>();
  addresses.forEach((a, i) => {
    if (seen.has(a)) {
      ctx.addIssue({ code: 'custom', path: [field, i], message: `duplicate address ${a}` });
    }
    seen.add(a);
  });
}

function checkL2(
  t: { ipv4: string[]; ipv6: string[]; bridgeDomain?: number | undefined },
  isL2: boolean,
  issue: Issue,
): void {
  if (isL2 && (t.ipv4.length > 0 || t.ipv6.length > 0)) {
    issue('ipv4', 'an L2 tunnel carries no IP addresses; attach it to a bridge domain instead');
  }
  if (!isL2 && t.bridgeDomain !== undefined) {
    issue('bridgeDomain', 'bridgeDomain applies to L2 tunnels only');
  }
}

// ---------------------------------------------------------------------------------------------------------------
// GRE
// ---------------------------------------------------------------------------------------------------------------

/** VPP GRE tunnel types: `l3` (IP over GRE), `teb` (transparent Ethernet bridging), `erspan` (type II). */
export const GRE_TUNNEL_TYPES = ['l3', 'teb', 'erspan'] as const;

export const GreTunnelSchema = z
  .strictObject({
    ...tunnelCommon,
    dst: withUi(ipAddress, { title: 'Destination address' }),
    type: withUi(z.enum(GRE_TUNNEL_TYPES).default('l3'), {
      title: 'Type',
      widget: 'select',
      help: 'l3 = IP over GRE, teb = L2 Ethernet over GRE, erspan = ERSPAN type II mirror destination',
    }),
    sessionId: withUi(z.int().min(0).max(1023), {
      title: 'ERSPAN session id',
      widget: 'number',
    }).optional(),
  })
  .superRefine((t, ctx) => {
    const issue = issuer(ctx);
    checkEndpoints(t.src, t.dst, issue, { multicastDst: false });
    if (t.type === 'erspan' && t.sessionId === undefined) {
      issue('sessionId', 'an ERSPAN tunnel needs a session id');
    }
    if (t.type !== 'erspan' && t.sessionId !== undefined) {
      issue('sessionId', 'sessionId applies to ERSPAN tunnels only');
    }
    checkL2(t, t.type !== 'l3', issue);
    checkAddressesUnique(t.ipv4, 'ipv4', ctx);
    checkAddressesUnique(t.ipv6, 'ipv6', ctx);
  });

// ---------------------------------------------------------------------------------------------------------------
// IPIP (also the protected interface of route-based IPsec tunnels — vpn.ipsec.tunnels[].routeBased.ipipInterface)
// ---------------------------------------------------------------------------------------------------------------

export const IpipTunnelSchema = z
  .strictObject({
    ...tunnelCommon,
    mode: withUi(z.enum(['p2p', 'p2mp']).default('p2p'), {
      title: 'Mode',
      widget: 'select',
      help: 'p2mp = point-to-multipoint (responder-only IPsec with many peers); dst is then omitted',
    }),
    dst: withUi(ipAddress, { title: 'Destination address' }).optional(),
    dscp: withUi(z.int().min(0).max(63), {
      title: 'Outer DSCP',
      widget: 'number',
      help: 'Omit to copy the inner DSCP',
    }).optional(),
  })
  .superRefine((t, ctx) => {
    const issue = issuer(ctx);
    if (t.mode === 'p2p' && t.dst === undefined) {
      issue('dst', 'a point-to-point tunnel needs a destination address');
    }
    if (t.mode === 'p2mp' && t.dst !== undefined) {
      issue('dst', 'a point-to-multipoint tunnel has no fixed destination');
    }
    checkEndpoints(t.src, t.dst, issue, { multicastDst: false });
    checkL2(t, false, issue);
    checkAddressesUnique(t.ipv4, 'ipv4', ctx);
    checkAddressesUnique(t.ipv6, 'ipv6', ctx);
  });

// ---------------------------------------------------------------------------------------------------------------
// VXLAN
// ---------------------------------------------------------------------------------------------------------------

/** 24-bit VXLAN network identifier. */
export const VXLAN_VNI_MAX = 16777215;

export const VxlanTunnelSchema = z
  .strictObject({
    ...tunnelCommon,
    dst: withUi(ipAddress, {
      title: 'Destination address',
      help: 'Unicast peer, or a multicast group (then mcastInterface is required)',
    }),
    vni: withUi(z.int().min(0).max(VXLAN_VNI_MAX), { title: 'VNI', widget: 'number' }),
    srcPort: withUi(transportPort.default(4789), { title: 'Source UDP port' }),
    dstPort: withUi(transportPort.default(4789), { title: 'Destination UDP port' }),
    mcastInterface: withUi(vppInterfaceName, {
      title: 'Multicast interface',
      help: 'Interface used to join the multicast group (multicast dst only)',
    }).optional(),
    decap: withUi(z.enum(['l2', 'ip4', 'ip6']).default('l2'), {
      title: 'Decapsulation',
      widget: 'select',
      help: 'l2 = bridge the inner frame (default); ip4/ip6 = route the inner packet in the tunnel VRF',
    }),
  })
  .superRefine((t, ctx) => {
    const issue = issuer(ctx);
    checkEndpoints(t.src, t.dst, issue, { multicastDst: true });
    const mcast = isMulticast(t.dst);
    if (mcast && t.mcastInterface === undefined) {
      issue('mcastInterface', 'a multicast destination needs mcastInterface');
    }
    if (!mcast && t.mcastInterface !== undefined) {
      issue('mcastInterface', 'mcastInterface applies to a multicast destination only');
    }
    checkL2(t, t.decap === 'l2', issue);
    checkAddressesUnique(t.ipv4, 'ipv4', ctx);
    checkAddressesUnique(t.ipv6, 'ipv6', ctx);
  });

// ---------------------------------------------------------------------------------------------------------------
// Domain root
// ---------------------------------------------------------------------------------------------------------------

export const TunnelsSchema = withUi(
  z.strictObject({
    gre: withUi(z.record(objectName, GreTunnelSchema).default({}), {
      title: 'GRE tunnels',
      widget: 'record',
      group: 'gre',
      order: 1,
    }),
    vxlan: withUi(z.record(objectName, VxlanTunnelSchema).default({}), {
      title: 'VXLAN tunnels',
      widget: 'record',
      group: 'vxlan',
      order: 2,
    }),
    ipip: withUi(z.record(objectName, IpipTunnelSchema).default({}), {
      title: 'IPIP tunnels',
      widget: 'record',
      group: 'ipip',
      order: 3,
    }),
    // Feature keys (sub-schema in domains/ext/<slug>.ts): one key line under the feature's anchor.
    // wave-BC: F-tunnels
    // wave-BC: F-lisp
    lisp: LispSchema.optional(),
  }),
  {
    title: 'Tunnels',
    description: 'GRE (L3, L2, ERSPAN), VXLAN and IPIP tunnels keyed by name.',
    order: 100,
  },
);

export type TunnelsConfig = z.infer<typeof TunnelsSchema>;
export type GreTunnel = z.infer<typeof GreTunnelSchema>;
export type IpipTunnel = z.infer<typeof IpipTunnelSchema>;
export type VxlanTunnel = z.infer<typeof VxlanTunnelSchema>;

/** Tunnel kinds in their documented order, with the VPP interface-name prefix used when `instance` is set. */
export const TUNNEL_KINDS = [
  { key: 'gre', vppPrefix: 'gre' },
  { key: 'vxlan', vppPrefix: 'vxlan_tunnel' },
  { key: 'ipip', vppPrefix: 'ipip' },
] as const;
