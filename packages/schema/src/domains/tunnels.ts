import { z } from 'zod';
import { withUi } from '../ui.js';
import {
  ipAddress,
  ipv4Cidr,
  ipv6Address,
  ipv6Cidr,
  macAddress,
  objectName,
  vppInterfaceName,
} from '../primitives.js';
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
 * `tunnels` — GRE (L3 / TEB / ERSPAN), VXLAN, IPIP (+ 6RD), VXLAN-GPE, GTP-U, L2TPv3 and PPPoE tunnels, each a
 * record keyed by tunnel name (docs/04-api-datamodel.md, WBS D6.6). `vxlanGpe`, `gtpu`, `l2tpv3`, `pppoe` and
 * `ipip.<name>.sixrd` are the T1 additive keys (S-tunnels-contract, RV-C F-tunnels MAJOR 2).
 *
 * Every tunnel has two VRFs (vdom.md #1): `underlayVrf` is the FIB the encapsulated packets are looked up in
 * (src/dst live there); `vrf` is the FIB the tunnel interface itself belongs to (overlay). Names are unique within
 * the whole `tunnels` domain, not only within a kind (vdom.md #2; `semantic/tunnels.ts`).
 *
 * `instance` fixes the VPP interface name (`gre<n>`, `ipip<n>`, `vxlan_tunnel<n>`) so other domains (VRRP, LLDP,
 * IPFIX, DHCP …) can reference the tunnel as an interface. The agent keys these three kinds by that name, so the
 * semantic rule `tunnels.instance-required` (D-205) refuses a GRE / VXLAN / IPIP tunnel without one at the API
 * (400 with a pointer). VXLAN-GPE, GTP-U, L2TPv3 and PPPoE have no instance: VPP names those interfaces itself and
 * the agent references them by the configuration name (the engine's owner tag).
 *
 * Domain root: every kind defaults to an empty record (D-017, D-053). Helper primitives come from
 * `./_shared/primitives.ts` (not re-exported, D-054). Owner: P02c; F-tunnels / S-tunnels-contract edit it.
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
    help: 'Fixes the engine interface name (gre<n>, ipip<n>, vxlan_tunnel<n>); required to reference the tunnel elsewhere',
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

/** What the kinds VPP names itself share: `tunnelCommon` without `instance` (S-tunnels-contract). */
const tunnelCommonNamed = Object.fromEntries(
  Object.entries(tunnelCommon).filter(([k]) => k !== 'instance'),
) as Omit<typeof tunnelCommon, 'instance'>;

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

/**
 * `tunnels.ipip.<name>.sixrd` — the tunnel is a 6RD border relay (RFC 5969, `ipip_6rd_add_tunnel`) instead of a
 * point-to-point IPIP tunnel: `src` is the IPv4 border-relay address, `underlayVrf` the IPv4 FIB, `vrf` the IPv6 FIB;
 * `dst` and `dscp` do not apply. The engine cannot read the 6RD parameters back (no dump): the agent keeps them
 * next to the tunnel name (write-only object, like LISP-GPE entries).
 */
export const IpipSixrdSchema = z.strictObject({
  ip6Prefix: withUi(ipv6Cidr, {
    title: '6RD IPv6 prefix',
    help: 'Delegated IPv6 prefix of the 6RD domain',
  }),
  ip4Prefix: withUi(ipv4Cidr, {
    title: '6RD IPv4 prefix',
    help: 'Common IPv4 prefix of the 6RD domain (its length is dropped from the customer prefix)',
  }),
  securityCheck: withUi(z.boolean().default(false), {
    title: 'Security check',
    help: 'Drop packets whose IPv6 source does not embed the outer IPv4 source',
  }),
  tcTos: withUi(z.int().min(0).max(255), {
    title: 'Outer TOS',
    widget: 'number',
    help: 'Traffic class copied to the outer IPv4 header; omit for 0',
  }).optional(),
});

export const IpipTunnelSchema = z
  .strictObject({
    ...tunnelCommon,
    sixrd: withUi(IpipSixrdSchema, {
      title: '6RD',
      group: 'sixrd',
      help: 'Set to make this tunnel a 6RD border relay (dst and dscp do not apply)',
    }).optional(),
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
    if (t.sixrd !== undefined) {
      if (t.dst !== undefined)
        issue('dst', 'a 6RD tunnel has no destination (the IPv4 prefix is the domain)');
      if (t.dscp !== undefined)
        issue('dscp', 'dscp does not apply to a 6RD tunnel (use sixrd.tcTos)');
      if (t.mode !== 'p2p') issue('mode', 'a 6RD tunnel is point-to-point');
      if (t.src.includes(':')) issue('src', 'the 6RD border-relay address is IPv4');
    } else {
      if (t.mode === 'p2p' && t.dst === undefined) {
        issue('dst', 'a point-to-point tunnel needs a destination address');
      }
      if (t.mode === 'p2mp' && t.dst !== undefined) {
        issue('dst', 'a point-to-multipoint tunnel has no fixed destination');
      }
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
// VXLAN-GPE (S-tunnels-contract, T1)
// ---------------------------------------------------------------------------------------------------------------

/** VXLAN-GPE next protocols; `ethernet` and `nsh` carry no IP addresses, `ip4`/`ip6` are routed in `vrf`. */
export const VXLAN_GPE_PROTOCOLS = ['ip4', 'ip6', 'ethernet', 'nsh'] as const;

export const VxlanGpeTunnelSchema = z
  .strictObject({
    ...tunnelCommonNamed,
    dst: withUi(ipAddress, {
      title: 'Destination address',
      help: 'Unicast peer, or a multicast group (then mcastInterface is required)',
    }),
    vni: withUi(z.int().min(0).max(VXLAN_VNI_MAX), { title: 'VNI', widget: 'number' }),
    srcPort: withUi(transportPort.default(4790), { title: 'Source UDP port' }),
    dstPort: withUi(transportPort.default(4790), { title: 'Destination UDP port' }),
    mcastInterface: withUi(vppInterfaceName, {
      title: 'Multicast interface',
      help: 'Interface used to join the multicast group (multicast dst only)',
    }).optional(),
    protocol: withUi(z.enum(VXLAN_GPE_PROTOCOLS).default('ethernet'), {
      title: 'Next protocol',
      widget: 'select',
      help: 'ethernet = bridge the inner frame (default); ip4/ip6 = route the inner packet in the tunnel VRF; nsh = network service header',
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
    checkL2(t, t.protocol !== 'ip4' && t.protocol !== 'ip6', issue);
    if (t.protocol === 'nsh' && t.bridgeDomain !== undefined) {
      issue('bridgeDomain', 'an NSH tunnel is not bridged');
    }
    checkAddressesUnique(t.ipv4, 'ipv4', ctx);
    checkAddressesUnique(t.ipv6, 'ipv6', ctx);
  });

// ---------------------------------------------------------------------------------------------------------------
// GTP-U (S-tunnels-contract, T1; advanced)
// ---------------------------------------------------------------------------------------------------------------

/** Where decapsulated GTP-U packets go (`gtpu_decap_next_type`). */
export const GTPU_DECAP = ['drop', 'l2', 'ip4', 'ip6'] as const;

export const GtpuTunnelSchema = z
  .strictObject({
    ...tunnelCommonNamed,
    dst: withUi(ipAddress, {
      title: 'Destination address',
      help: 'Unicast peer, or a multicast group (then mcastInterface is required)',
    }),
    mcastInterface: withUi(vppInterfaceName, {
      title: 'Multicast interface',
      help: 'Interface used to join the multicast group (multicast dst only)',
    }).optional(),
    teid: withUi(z.int().min(0).max(4294967295), {
      title: 'TEID',
      widget: 'number',
      help: 'Local (receive) tunnel endpoint id',
    }),
    tteid: withUi(z.int().min(1).max(4294967295), {
      title: 'Transmit TEID',
      widget: 'number',
      help: 'Remote tunnel endpoint id; omit to use the TEID',
    }).optional(),
    decap: withUi(z.enum(GTPU_DECAP).default('ip4'), {
      title: 'Decapsulation',
      widget: 'select',
      help: 'ip4/ip6 route the inner packet in the tunnel VRF, l2 bridges it, drop discards it',
    }),
    pduExtension: withUi(z.boolean().default(false), {
      title: 'PDU session container',
      help: 'Add the PDU session container extension header (5G) with the QFI',
    }),
    qfi: withUi(z.int().min(0).max(63), {
      title: 'QFI',
      widget: 'number',
      help: 'QoS flow identifier of the PDU session container (needs pduExtension)',
    }).optional(),
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
    if (t.qfi !== undefined && !t.pduExtension) issue('qfi', 'qfi needs pduExtension');
    checkL2(t, t.decap !== 'ip4' && t.decap !== 'ip6', issue);
    if (t.decap === 'drop' && t.bridgeDomain !== undefined) {
      issue('bridgeDomain', 'a drop tunnel is not bridged');
    }
    checkAddressesUnique(t.ipv4, 'ipv4', ctx);
    checkAddressesUnique(t.ipv6, 'ipv6', ctx);
  });

// ---------------------------------------------------------------------------------------------------------------
// L2TPv3 (S-tunnels-contract, T1; advanced). IPv6 only, always L2 (bridge domain), no addresses.
// ---------------------------------------------------------------------------------------------------------------

/** 64-bit L2TPv3 cookies are JSON numbers; the safe-integer bound keeps them exact (uint64 in the proto). */
const cookie = z.int().min(0).max(Number.MAX_SAFE_INTEGER);

export const L2tpv3TunnelSchema = z
  .strictObject({
    ...tunnelCommonNamed,
    src: withUi(ipv6Address, {
      title: 'Local address',
      help: 'Our IPv6 address; must be configured on an interface in the underlay VRF',
    }),
    dst: withUi(ipv6Address, { title: 'Client address', help: 'Remote IPv6 address of the peer' }),
    localSessionId: withUi(z.int().min(0).max(4294967295), {
      title: 'Local session id',
      widget: 'number',
    }),
    remoteSessionId: withUi(z.int().min(0).max(4294967295), {
      title: 'Remote session id',
      widget: 'number',
    }),
    localCookie: withUi(cookie.default(0), { title: 'Local cookie', widget: 'number' }),
    remoteCookie: withUi(cookie.default(0), { title: 'Remote cookie', widget: 'number' }),
    l2Sublayer: withUi(z.boolean().default(false), {
      title: 'L2-specific sublayer',
      help: 'The peer sends the L2-specific sublayer (RFC 3931 §4.6)',
    }),
  })
  .superRefine((t, ctx) => {
    const issue = issuer(ctx);
    checkEndpoints(t.src, t.dst, issue, { multicastDst: false });
    checkL2(t, true, issue);
    if (t.underlayVrf !== 'default') {
      issue('underlayVrf', 'the engine encapsulates L2TPv3 in the default VRF only');
    }
  });

// ---------------------------------------------------------------------------------------------------------------
// PPPoE server sessions (S-tunnels-contract, T1; advanced): one interface per learned client.
// ---------------------------------------------------------------------------------------------------------------

export const PppoeSessionSchema = z
  .strictObject({
    enabled: enabledFlag,
    description: descriptionField.optional(),
    sessionId: withUi(z.int().min(1).max(65535), {
      title: 'Session id',
      widget: 'number',
      help: 'PPPoE session id negotiated with the client (PADS)',
    }),
    clientMac: withUi(macAddress, {
      title: 'Client MAC',
      help: 'The engine creates the session once it has seen this client in discovery',
    }),
    clientIp: withUi(ipAddress, { title: 'Client IP address' }),
    vrf: withUi(vrfRef, {
      title: 'VRF',
      widget: 'vrf-picker',
      help: 'FIB the decapsulated client traffic is routed in',
    }),
    mtu: mtuField.optional(),
  })
  .superRefine((t, ctx) => {
    const issue = issuer(ctx);
    if (isUnspecified(t.clientIp)) issue('clientIp', 'client address must not be unspecified');
    if (isMulticast(t.clientIp))
      issue('clientIp', 'client address must not be a multicast address');
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
    vxlanGpe: withUi(z.record(objectName, VxlanGpeTunnelSchema).default({}), {
      title: 'VXLAN-GPE tunnels',
      widget: 'record',
      group: 'vxlanGpe',
      order: 4,
    }),
    gtpu: withUi(z.record(objectName, GtpuTunnelSchema).default({}), {
      title: 'GTP-U tunnels',
      widget: 'record',
      group: 'gtpu',
      order: 5,
      help: 'Advanced: mobile-core user-plane tunnels',
    }),
    l2tpv3: withUi(z.record(objectName, L2tpv3TunnelSchema).default({}), {
      title: 'L2TPv3 tunnels',
      widget: 'record',
      group: 'l2tpv3',
      order: 6,
      help: 'Advanced: L2 over IPv6; the engine cannot delete an L2TPv3 tunnel without a restart',
    }),
    pppoe: withUi(z.record(objectName, PppoeSessionSchema).default({}), {
      title: 'PPPoE sessions',
      widget: 'record',
      group: 'pppoe',
      order: 7,
      help: 'Advanced: PPPoE server sessions (one interface per client)',
    }),
    // wave-BC: F-lisp
    lisp: LispSchema.optional(),
  }),
  {
    title: 'Tunnels',
    description:
      'GRE (L3, L2, ERSPAN), VXLAN, IPIP (+ 6RD), VXLAN-GPE, GTP-U, L2TPv3 and PPPoE tunnels keyed by name.',
    order: 100,
  },
);

export type TunnelsConfig = z.infer<typeof TunnelsSchema>;
export type GreTunnel = z.infer<typeof GreTunnelSchema>;
export type IpipTunnel = z.infer<typeof IpipTunnelSchema>;
export type VxlanTunnel = z.infer<typeof VxlanTunnelSchema>;
export type IpipSixrd = z.infer<typeof IpipSixrdSchema>;
export type VxlanGpeTunnel = z.infer<typeof VxlanGpeTunnelSchema>;
export type GtpuTunnel = z.infer<typeof GtpuTunnelSchema>;
export type L2tpv3Tunnel = z.infer<typeof L2tpv3TunnelSchema>;
export type PppoeSession = z.infer<typeof PppoeSessionSchema>;

/**
 * Tunnel kinds in their documented order with the VPP interface-name prefix: `instance: true` kinds are named
 * `<vppPrefix><instance>` by the configuration (and need one, `tunnels.instance-required`); the others are named
 * `<vppPrefix><n>` by VPP at creation and referenced by their configuration name. `advanced` kinds sit behind the
 * UI's advanced toggle.
 */
export const TUNNEL_KINDS = [
  { key: 'gre', vppPrefix: 'gre', instance: true, advanced: false },
  { key: 'vxlan', vppPrefix: 'vxlan_tunnel', instance: true, advanced: false },
  { key: 'ipip', vppPrefix: 'ipip', instance: true, advanced: false },
  { key: 'vxlanGpe', vppPrefix: 'vxlan_gpe_tunnel', instance: false, advanced: false },
  { key: 'gtpu', vppPrefix: 'gtpu_tunnel', instance: false, advanced: true },
  { key: 'l2tpv3', vppPrefix: 'l2tpv3_tunnel', instance: false, advanced: true },
  { key: 'pppoe', vppPrefix: 'pppoe_session', instance: false, advanced: true },
] as const;
