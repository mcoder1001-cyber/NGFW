import { z } from 'zod';
import {
  descriptionText,
  hostname,
  ipv4Cidr,
  ipv6Cidr,
  macAddress,
  mtu,
  vlanId,
  vppInterfaceName,
  vrfName,
} from '../primitives.js';
import { withUi } from '../ui.js';
import { DEFAULT_VRF } from './vrfs.js';
import { interfaceL2Field, subinterfaceL2Field } from './ext/bridge-l2.js'; // wave-A: F-bridge-l2

/**
 * `interfaces` — record keyed by VPP interface name → interface settings (docs/04-api-datamodel.md; WBS D1.2–D1.4).
 *
 * - Keys are *parent* interfaces (`TenGigabitEthernet0/0/0`, `loop0`, `BondEthernet0`); 802.1Q / QinQ
 *   sub-interfaces live under `subinterfaces`, keyed by their numeric sub-id, and are named `<parent>.<id>` on VPP.
 * - Guardrail (docs/decisions/vdom.md #1): `vrf` is a first-class field on every (sub-)interface, default `default`.
 * - VPP interface names contain `/`: JSON pointers into this record MUST be built with `jsonPointer()`.
 * - Nothing forwards until `enabled: true` (VPP interfaces start admin-down; same as TNSR `enable`).
 * - Cross-object rules are semantic validators in `../semantic/interfaces.ts`: VRF exists, no overlapping
 *   addresses per VRF, VLAN ids unique per parent, unnumbered target exists.
 */

export const RxMode = z.enum(['polling', 'interrupt', 'adaptive']);
export type RxMode = z.infer<typeof RxMode>;

/**
 * Record key: a parent VPP interface name (no `.sub` suffix — sub-interfaces are modelled under `subinterfaces`).
 * Same alphabet as `vppInterfaceName`.
 */
export const parentInterfaceName = withUi(
  z
    .string()
    .min(1)
    .max(63)
    .regex(
      /^[A-Za-z][A-Za-z0-9_-]*(?:\/[0-9a-fA-F]+)*$/,
      'expected a parent VPP interface name like TenGigabitEthernet0/0/0 (sub-interfaces go under subinterfaces)',
    ),
  { title: 'Interface', widget: 'interface-picker' },
);

/** Sub-interface id as a record key: the `N` in `TenGigabitEthernet0/0/0.N` (VPP u32, usually = the VLAN id). */
export const subInterfaceId = withUi(
  z
    .string()
    .regex(/^(?:0|[1-9][0-9]{0,9})$/, 'expected a sub-interface id (decimal, no leading zeros)')
    .refine((id) => Number(id) <= 4294967295, 'sub-interface id must fit in 32 bits'),
  { title: 'Sub-interface ID', widget: 'number' },
);

/**
 * DHCPv4 client on the interface (VPP `dhcp_client_config`; WBS D7.2, D-050). Present = enabled. The leased
 * address is runtime state and never written back into `ipv4`.
 */
export const DhcpClientSchema = z.strictObject({
  hostname: withUi(hostname.optional(), {
    title: 'Hostname',
    help: 'sent in option 12; absent = the system hostname',
    order: 1,
  }),
  clientId: withUi(
    z
      .string()
      .min(1)
      .max(64)
      .regex(/^[\x21-\x7e]+$/, 'printable ASCII without spaces')
      .optional(),
    { title: 'Client identifier', help: 'option 61; absent = the interface MAC address', order: 2 },
  ),
  setBroadcastFlag: withUi(z.boolean().default(false), {
    title: 'Broadcast flag',
    help: 'ask the server to broadcast its replies',
    order: 3,
  }),
});
export type DhcpClientConfig = z.infer<typeof DhcpClientSchema>;

/** Fields shared by interfaces and sub-interfaces. */
const commonFields = {
  enabled: withUi(z.boolean().default(false), {
    title: 'Enabled',
    help: 'admin state; interfaces stay down until enabled',
    group: 'general',
    order: 1,
  }),
  description: withUi(descriptionText.optional(), {
    title: 'Description',
    group: 'general',
    order: 2,
  }),
  mtu: withUi(mtu.optional(), {
    title: 'MTU',
    help: 'L3 MTU in bytes (68–9216); absent = keep the driver default',
    group: 'general',
    order: 3,
  }),
  ipv4: withUi(z.array(ipv4Cidr).max(32).default([]), {
    title: 'IPv4 addresses',
    help: 'addresses with prefix length, e.g. 10.0.0.1/24',
    group: 'addressing',
    order: 10,
  }),
  ipv6: withUi(z.array(ipv6Cidr).max(32).default([]), {
    title: 'IPv6 addresses',
    help: 'addresses with prefix length, e.g. 2001:db8::1/64',
    group: 'addressing',
    order: 11,
  }),
  vrf: withUi(vrfName.default(DEFAULT_VRF), {
    title: 'VRF',
    help: 'routing table this interface belongs to',
    group: 'addressing',
    order: 12,
  }),
  unnumbered: withUi(vppInterfaceName.optional(), {
    title: 'IP unnumbered',
    help: 'borrow the addresses of this interface (e.g. loop0); ipv4/ipv6 must then be empty',
    group: 'addressing',
    order: 13,
  }),
  dhcpClient: withUi(DhcpClientSchema.optional(), {
    title: 'DHCP client',
    help: 'obtain an IPv4 address by DHCP; cannot be combined with unnumbered',
    group: 'addressing',
    order: 14,
  }),
} as const;

const UNNUMBERED_EXCLUSIVE =
  'an unnumbered interface cannot have its own ipv4/ipv6 addresses or a DHCP client';

function noAddressesWhenUnnumbered(value: {
  unnumbered?: string | undefined;
  ipv4: readonly string[];
  ipv6: readonly string[];
  dhcpClient?: DhcpClientConfig | undefined;
}): boolean {
  return (
    value.unnumbered === undefined ||
    (value.ipv4.length === 0 && value.ipv6.length === 0 && value.dhcpClient === undefined)
  );
}

export const SubinterfaceSchema = z
  .strictObject({
    vlanId: withUi(vlanId, {
      title: 'VLAN ID',
      help: 'outer 802.1Q (or 802.1ad) tag',
      group: 'encapsulation',
      order: 5,
    }),
    innerVlanId: withUi(vlanId.optional(), {
      title: 'Inner VLAN ID',
      help: 'inner tag for QinQ (two-tag) sub-interfaces',
      group: 'encapsulation',
      order: 6,
    }),
    dot1ad: withUi(z.boolean().default(false), {
      title: '802.1ad outer tag',
      help: 'use an 802.1ad (0x88a8) outer tag instead of 802.1Q (0x8100)',
      group: 'encapsulation',
      order: 7,
    }),
    ...commonFields,
    // Feature keys (sub-schema in domains/ext/<slug>.ts): one key line under the feature's anchor.
    // wave-A: F-bridge-l2
    l2: subinterfaceL2Field,
    // wave-A: F-neighbors-ra
    // wave-A: F-rpf-adl-pbr
  })
  .refine(noAddressesWhenUnnumbered, { message: UNNUMBERED_EXCLUSIVE, path: ['unnumbered'] });

export type SubinterfaceConfig = z.infer<typeof SubinterfaceSchema>;

export const InterfaceSchema = z
  .strictObject({
    ...commonFields,
    mac: withUi(macAddress.optional(), {
      title: 'MAC address',
      help: 'override the hardware address; absent = keep the burned-in MAC',
      group: 'general',
      order: 4,
    }),
    promiscuous: withUi(z.boolean().default(false), {
      title: 'Promiscuous',
      help: 'accept frames for any destination MAC',
      group: 'general',
      order: 5,
    }),
    rxMode: withUi(RxMode.optional(), {
      title: 'RX mode',
      help: 'polling (default for DPDK), interrupt or adaptive',
      widget: 'select',
      group: 'general',
      order: 6,
    }),
    subinterfaces: withUi(z.record(subInterfaceId, SubinterfaceSchema).default({}), {
      title: 'Sub-interfaces',
      help: '802.1Q / QinQ sub-interfaces keyed by sub-interface id (usually the VLAN id)',
      group: 'subinterfaces',
      order: 20,
    }),
    // Feature keys (sub-schema in domains/ext/<slug>.ts): one key line under the feature's anchor.
    // wave-A: F-bonding
    // wave-A: F-bridge-l2
    l2: interfaceL2Field,
    // wave-A: F-loopback-bvi-gso-lldp-span
    // wave-A: F-neighbors-ra
    // wave-A: F-rpf-adl-pbr
    // wave-A: P12
  })
  .refine(noAddressesWhenUnnumbered, { message: UNNUMBERED_EXCLUSIVE, path: ['unnumbered'] });

export type InterfaceConfig = z.infer<typeof InterfaceSchema>;

export const InterfacesSchema = withUi(z.record(parentInterfaceName, InterfaceSchema), {
  title: 'Interfaces',
  description: 'Physical, virtual and sub-interfaces keyed by VPP interface name.',
  order: 30,
});

export type InterfacesConfig = z.infer<typeof InterfacesSchema>;

/** VPP name of a sub-interface: `<parent>.<id>`. */
export function subInterfaceName(parent: string, id: string | number): string {
  return `${parent}.${id}`;
}

/**
 * Every interface name present in the document (parents and `<parent>.<id>` sub-interfaces) — the set that
 * `unnumbered`, static-route next hops, NAT bindings, ACL attachments … may reference.
 */
export function interfaceNames(interfaces: InterfacesConfig): Set<string> {
  const names = new Set<string>();
  for (const [parent, iface] of Object.entries(interfaces)) {
    names.add(parent);
    for (const id of Object.keys(iface.subinterfaces)) names.add(subInterfaceName(parent, id));
  }
  return names;
}
