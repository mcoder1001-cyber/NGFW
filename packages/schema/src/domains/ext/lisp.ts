import { z } from 'zod';
import { withUi } from '../../ui.js';
import { ipAddress, objectName, vppInterfaceName } from '../../primitives.js';

/**
 * `tunnels.lisp` — the minimal LISP / LISP-GPE set (F-lisp, WBS D6.8, tier T3): the global switches, local locator
 * sets, local EIDs, map-resolvers / map-servers, static remote mappings, adjacencies, EID-table ↔ VRF / bridge-domain
 * maps, the proxy-ITR locator set and static LISP-GPE forwarding entries. Realised by DF-6's `lisp` descriptors.
 *
 * EIDs are strings: an IP prefix in canonical (masked) form (`10.11.0.0/24`, `fd11::/64`) or a MAC address
 * (`02:0b:00:00:00:01`, L2 EIDs). NSH EIDs, map-register authentication keys (secrets), the `one` API and LISP-GPE
 * over IPsec are not modelled. Cross-field rules live in `semantic/lisp.ts` (rule ids `tunnels.lisp-…`).
 *
 * VPP facts that shape the model: enabling LISP also enables LISP-GPE (`gpe` requires `enabled`); every
 * enable/disable leaks a `<remote-N>` locator set and `lisp_gpe*` interfaces (V14); GPE forwarding entries cannot be
 * read back (V13) — the agent re-applies them once per VPP boot and reports them only through the state endpoint.
 */

/** VNI / instance id: 24 bits. */
export const lispVni = withUi(z.int().min(0).max(16777215), {
  title: 'VNI',
  widget: 'number',
  help: 'LISP instance id (24 bits); 0 = the default instance',
});

const EID_PATTERN =
  /^(?:[0-9a-fA-F]{2}(?::[0-9a-fA-F]{2}){5}|[0-9.]+\/\d{1,2}|[0-9a-fA-F:.]*:[0-9a-fA-F:.]*\/\d{1,3})$/;

/** An EID: an IP prefix (canonical, masked — checked by `tunnels.lisp-eid-canonical`) or a MAC address. */
export const lispEid = withUi(
  z
    .string()
    .min(1)
    .max(64)
    .regex(EID_PATTERN, 'expected an IP prefix like 10.11.0.0/24 or a MAC address'),
  { title: 'EID', help: 'IP prefix (masked, e.g. 10.11.0.0/24) or MAC address (L2 EID)' },
);

/** Locator-set name: VPP keeps ≤ 63 bytes. */
const locatorSetName = withUi(objectName, { title: 'Locator set' });

/** Actions of a negative mapping / forwarding entry (VPP `lisp_types` action 0–3). */
export const LISP_ACTIONS = ['no-action', 'natively-forward', 'send-map-request', 'drop'] as const;

const priority = withUi(z.int().min(0).max(255).default(1), {
  title: 'Priority',
  widget: 'number',
});
const weight = withUi(z.int().min(0).max(255).default(1), { title: 'Weight', widget: 'number' });

export const LispLocatorSchema = z.strictObject({
  interface: withUi(vppInterfaceName, {
    title: 'Interface',
    widget: 'interface-picker',
    help: 'RLOC interface (its address is the locator)',
  }),
  priority,
  weight,
});

export const LispLocatorSetSchema = z.strictObject({
  locators: withUi(z.array(LispLocatorSchema).max(16).default([]), {
    title: 'Locators',
    help: 'Interfaces whose addresses are the RLOCs of this set',
  }),
});

export const LispLocalEidSchema = z.strictObject({
  vni: lispVni.default(0),
  eid: lispEid,
  locatorSet: locatorSetName,
});

export const LispRlocSchema = z.strictObject({
  address: withUi(ipAddress, { title: 'RLOC address' }),
  priority,
  weight,
});

export const LispRemoteMappingSchema = z.strictObject({
  vni: lispVni.default(0),
  eid: lispEid,
  rlocs: withUi(z.array(LispRlocSchema).max(16).default([]), {
    title: 'RLOCs',
    help: 'Remote locators; none = a negative mapping (then action applies)',
  }),
  action: withUi(z.enum(LISP_ACTIONS).default('no-action'), {
    title: 'Action',
    widget: 'select',
    help: 'Negative mappings only (no RLOCs)',
  }),
});

export const LispAdjacencySchema = z.strictObject({
  vni: lispVni.default(0),
  reid: withUi(lispEid, { title: 'Remote EID' }),
  leid: withUi(lispEid, { title: 'Local EID' }),
});

export const LispEidTableSchema = z.strictObject({
  vrf: withUi(objectName, {
    title: 'VRF',
    widget: 'vrf-picker',
    help: 'L3: IP EIDs of this VNI live in this VRF',
  }).optional(),
  bridgeDomain: withUi(z.int().min(1).max(16777215), {
    title: 'Bridge domain',
    widget: 'number',
    help: 'L2: MAC EIDs of this VNI live in this bridge domain (by id)',
  }).optional(),
});

export const LispLocatorPairSchema = z.strictObject({
  local: withUi(ipAddress, { title: 'Local RLOC' }),
  remote: withUi(ipAddress, { title: 'Remote RLOC' }),
  weight,
});

export const LispGpeEntrySchema = z.strictObject({
  vni: lispVni.default(0),
  vrf: withUi(objectName.default('default'), { title: 'VRF', widget: 'vrf-picker' }),
  reid: withUi(lispEid, { title: 'Remote EID' }),
  leid: withUi(lispEid, { title: 'Local EID' }),
  pairs: withUi(z.array(LispLocatorPairSchema).max(16).default([]), {
    title: 'Locator pairs',
    help: 'none = a negative entry (then action applies)',
  }),
  action: withUi(z.enum(LISP_ACTIONS).default('no-action'), { title: 'Action', widget: 'select' }),
});

export const LispSchema = withUi(
  z.strictObject({
    enabled: withUi(z.boolean().default(false), {
      title: 'Enable LISP',
      help: 'VPP-global switch, set by the globals owner only; other agents require it',
      order: 1,
    }),
    gpe: withUi(z.boolean().default(false), {
      title: 'Enable LISP-GPE',
      help: 'LISP-GPE data plane (requires LISP)',
      order: 2,
    }),
    locatorSets: withUi(z.record(locatorSetName, LispLocatorSetSchema).default({}), {
      title: 'Locator sets',
      widget: 'record',
      group: 'locators',
      order: 3,
    }),
    localEids: withUi(z.array(LispLocalEidSchema).max(1024).default([]), {
      title: 'Local EIDs',
      group: 'eids',
      order: 4,
    }),
    eidTables: withUi(
      z.record(z.string().regex(/^(0|[1-9]\d{0,7})$/, 'VNI'), LispEidTableSchema).default({}),
      {
        title: 'EID tables',
        help: 'VNI → VRF (L3) or bridge domain (L2)',
        group: 'eids',
        order: 5,
      },
    ),
    remoteMappings: withUi(z.array(LispRemoteMappingSchema).max(1024).default([]), {
      title: 'Remote mappings',
      group: 'mappings',
      order: 6,
    }),
    adjacencies: withUi(z.array(LispAdjacencySchema).max(1024).default([]), {
      title: 'Adjacencies',
      group: 'mappings',
      order: 7,
    }),
    gpeEntries: withUi(z.array(LispGpeEntrySchema).max(1024).default([]), {
      title: 'GPE forwarding entries',
      help: 'Static LISP-GPE entries; write-only in VPP 26.06 (V13)',
      group: 'mappings',
      order: 8,
    }),
    mapResolvers: withUi(z.array(ipAddress).max(8).default([]), {
      title: 'Map resolvers',
      group: 'resolvers',
      order: 9,
    }),
    mapServers: withUi(z.array(ipAddress).max(8).default([]), {
      title: 'Map servers',
      help: 'Map-register authentication is not modelled',
      group: 'resolvers',
      order: 10,
    }),
    pitr: withUi(locatorSetName, {
      title: 'Proxy-ITR locator set',
      help: 'VPP-global (globals owner only)',
      group: 'resolvers',
      order: 11,
    }).optional(),
  }),
  {
    title: 'LISP',
    description: 'LISP / LISP-GPE (advanced): locators, EIDs, mappings, resolvers.',
    order: 90,
  },
);

export type LispConfig = z.infer<typeof LispSchema>;
export type LispRemoteMapping = z.infer<typeof LispRemoteMappingSchema>;
export type LispGpeEntry = z.infer<typeof LispGpeEntrySchema>;
