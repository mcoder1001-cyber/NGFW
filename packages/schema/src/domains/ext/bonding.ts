import { z } from 'zod';
import { withUi } from '../../ui.js';

/**
 * Link aggregation (F-bonding, WBS D1.5): `interfaces.<name>.bond` turns the interface into a VPP bond (`bond_create2`) and
 * lists its member NICs (`bond_add_member`). Reference: TNSR "Bond interfaces / LACP".
 *
 * - A bond interface is named `BondEthernet<id>`, VPP's own name for bond `<id>` (TNSR names them the same way): the
 *   configuration key is the logical name (D-065/D-069), `bond.id` is optional and, when set, equals the number in the name.
 *   `BondEthernet<id>` without a `bond` leaf names a bond that already exists and that the agent does not create.
 * - The bond is an ordinary interface otherwise: admin state, MTU, MAC, addresses, VRF and sub-interfaces are the fields
 *   of `interfaces.BondEthernet<id>` itself.
 * - Members are other entries of `interfaces` (physical NICs; `host-<netdev>` in the lab), keyed by their logical name.
 *   A member has no L3 configuration of its own; its `enabled` makes it eligible for the active set.
 *
 * Cross-field rules are semantic validators in `../../semantic/bonding.ts` (`interfaces.bonding-*`): name and id agree,
 * members exist, are not bonds or loopbacks, belong to at most one bond and carry no addresses/VRF/sub-interfaces;
 * `loadBalance` only for xor/lacp, `passive`/`longTimeout` only for lacp, `weight` only for active-backup.
 */

const UI_GROUP = 'bonding';

/**
 * Bond modes (VPP `bond_mode`): LACP (802.3ad), static XOR hash, round-robin, active-backup. VPP's `broadcast` mode is not
 * offered (manager decision Q1, as TNSR): it duplicates every frame on every member, which a switch-side LAG delivers twice.
 * Adding it later is additive.
 */
export const BondMode = z.enum(['lacp', 'xor', 'round-robin', 'active-backup']);
export type BondMode = z.infer<typeof BondMode>;

/** Transmit hash of xor and lacp bonds (VPP `bond_lb_algo` l2 / l23 / l34); absent = l2. */
export const BondLoadBalance = z.enum(['l2', 'l23', 'l34']);
export type BondLoadBalance = z.infer<typeof BondLoadBalance>;

/** Modes that choose a transmit hash; VPP forces its own algorithm for the others. */
export const BOND_HASH_MODES: readonly BondMode[] = ['lacp', 'xor'];

/**
 * Interface names that are never Ethernet NICs, so never bond members (F-bonding review F4): VPP 26.06 `bond_add_member`
 * refuses only a bond and copies the member's hardware address, which a tunnel or other L3 interface does not have. The
 * agent refuses any member without an L2 address, loopbacks and sub-interfaces before it calls VPP; this list gives the
 * user a 400 with a pointer for the names it can recognise.
 */
export const NON_ETHERNET_INTERFACE_RE =
  /^(?:loop|wg|ipip|gre|ipsec|vxlan_tunnel|vxlan_gpe_tunnel|gtpu_tunnel|geneve_tunnel|l2tpv3_tunnel|pppoe_session|mpls-tunnel|bvi|lisp_gpe|sr-tunnel)[0-9]+$/;

/** A bond interface's configuration key: `BondEthernet<id>` (VPP's name). */
export const BOND_INTERFACE_RE = /^BondEthernet(0|[1-9][0-9]{0,9})$/;

/** The bond id encoded in an interface name (`BondEthernet12` → 12), or undefined when the name is not a bond name. */
export function bondIdOf(name: string): number | undefined {
  const m = BOND_INTERFACE_RE.exec(name);
  if (m === null) return undefined;
  const id = Number(m[1]);
  return id <= 4294967294 ? id : undefined; // ~0 is VPP's "choose an id"
}

/**
 * Member key: a parent interface name (same alphabet as `parentInterfaceName` in `../interfaces.ts`, which this file
 * cannot import without a module cycle): sub-interfaces cannot be bond members.
 */
export const bondMemberName = withUi(
  z
    .string()
    .min(1)
    .max(63)
    .regex(
      /^[A-Za-z][A-Za-z0-9_-]*(?:\/[0-9a-fA-F]+)*$/,
      'expected a parent interface name (sub-interfaces cannot be bond members)',
    ),
  { title: 'Member interface', widget: 'interface-picker' },
);

/** One member of a bond (`bond_add_member`, `sw_interface_set_bond_weight`). */
export const BondMemberSchema = z.strictObject({
  passive: withUi(z.boolean().default(false), {
    title: 'Passive',
    help: 'LACP only: do not initiate LACP; the partner must be active',
    group: UI_GROUP,
    order: 1,
  }),
  longTimeout: withUi(z.boolean().default(false), {
    title: 'Long timeout',
    help: 'LACP only: 90 s partner timeout (slow LACPDUs) instead of 3 s',
    group: UI_GROUP,
    order: 2,
  }),
  weight: withUi(z.number().int().min(1).max(255).optional(), {
    title: 'Weight',
    help: 'active-backup only: the up member with the highest weight is the active one (1–255); absent = VPP default',
    widget: 'number',
    group: UI_GROUP,
    order: 3,
  }),
});
export type BondMemberConfig = z.infer<typeof BondMemberSchema>;

/** `interfaces.<BondEthernet<id>>.bond` — the interface is a bond with these members. */
export const BondSchema = z.strictObject({
  mode: withUi(BondMode, {
    title: 'Mode',
    help: 'lacp (802.3ad), xor, round-robin or active-backup; changing it re-creates the bond',
    widget: 'select',
    group: UI_GROUP,
    order: 1,
  }),
  loadBalance: withUi(BondLoadBalance.optional(), {
    title: 'Load balance',
    help: 'xor and lacp only: transmit hash over l2 (MAC), l23 (MAC+IP) or l34 (IP+port); absent = l2',
    widget: 'select',
    group: UI_GROUP,
    order: 2,
  }),
  members: withUi(z.record(bondMemberName, BondMemberSchema).default({}), {
    title: 'Members',
    help: 'member interfaces keyed by name; each needs its own entry in interfaces (enabled, no addresses)',
    group: UI_GROUP,
    order: 3,
  }),
  numaOnly: withUi(z.boolean().default(false), {
    title: 'NUMA-local members only',
    help: 'transmit only on members attached to the bond’s NUMA node',
    group: UI_GROUP,
    order: 4,
  }),
  id: withUi(z.number().int().min(0).max(4294967294).optional(), {
    title: 'Bond ID',
    widget: 'number',
    help: 'VPP bond id; absent = the number in the interface name (BondEthernet<id>)',
    group: UI_GROUP,
    order: 5,
  }),
});
export type BondConfig = z.infer<typeof BondSchema>;

/** The key line of `InterfaceSchema` (wave-A-hotspots C1). */
export const interfaceBondField = withUi(BondSchema.optional(), {
  title: 'Bond',
  help: 'make this interface (BondEthernet<id>) a link aggregation of the member interfaces',
  group: UI_GROUP,
  order: 30,
});
