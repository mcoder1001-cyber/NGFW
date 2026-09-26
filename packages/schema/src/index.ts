import { z } from 'zod';
import { SystemSchema } from './domains/system.js';
import { DataplaneSchema } from './domains/dataplane.js';
import { InterfacesSchema } from './domains/interfaces.js';
import { VrfsSchema } from './domains/vrfs.js';
import { RoutingSchema } from './domains/routing.js';
import { NatSchema } from './domains/nat.js';
import { ObjectsSchema } from './domains/objects.js';
import { AclSchema } from './domains/acl.js';
import { VpnSchema } from './domains/vpn.js';
import { TunnelsSchema } from './domains/tunnels.js';
import { ServicesSchema } from './domains/services.js';
import { HaSchema } from './domains/ha.js';
import { ManagementSchema } from './domains/management.js';

/**
 * Root configuration document — the single JSON object stored as `config_revision.payload`
 * (docs/04-api-datamodel.md "Config document shape"). One definition, three consumers: TS types (tsc),
 * JSON Schema per domain (UI form renderer) and OpenAPI 3.1 components (API) — see `gen.ts`.
 *
 * - Every top-level key is optional-with-default: `RootConfig.parse({})` is valid. `.prefault({})` runs each
 *   domain schema on `{}`, so nested field defaults are filled in (`.default({})` would short-circuit them).
 * - The root is strict: unknown top-level keys are rejected. Domain schemas are passthrough placeholders until
 *   P02a (system, dataplane, interfaces, vrfs, routing, management), P02b (nat, objects, acl) and
 *   P02c (vpn, tunnels, services, ha) land — each group edits only its own `domains/<key>.ts`.
 * - Guardrail (docs/decisions/vdom.md #4): UI navigation and the pending-change bar iterate `ROOT_KEYS`;
 *   never hardcode this list anywhere else.
 * - Changing this package requires a `contract(schema): …` commit (tools/ci.sh contract guard).
 */
export const RootConfig = z.strictObject({
  system: SystemSchema.prefault({}),
  dataplane: DataplaneSchema.prefault({}),
  interfaces: InterfacesSchema.prefault({}),
  vrfs: VrfsSchema.prefault({}),
  routing: RoutingSchema.prefault({}),
  nat: NatSchema.prefault({}),
  objects: ObjectsSchema.prefault({}),
  acl: AclSchema.prefault({}),
  vpn: VpnSchema.prefault({}),
  tunnels: TunnelsSchema.prefault({}),
  services: ServicesSchema.prefault({}),
  ha: HaSchema.prefault({}),
  management: ManagementSchema.prefault({}),
});

export type RootConfig = z.infer<typeof RootConfig>;

/** What the API accepts (`PUT`/`PATCH`/import): every field with a default may be omitted. */
export type RootConfigInput = z.input<typeof RootConfig>;

/** A top-level key of the configuration document. */
export type RootKey = keyof RootConfig;

/** Top-level keys in their stable, documented order — used by the diff engine, gen and the UI navigation. */
export const ROOT_KEYS = Object.keys(RootConfig.shape) as readonly RootKey[];

export * from './domains/system.js';
export * from './domains/dataplane.js';
export * from './domains/interfaces.js';
export * from './domains/vrfs.js';
export * from './domains/routing.js';
export * from './domains/nat.js';
export * from './domains/objects.js';
export * from './domains/acl.js';
export * from './domains/vpn.js';
export * from './domains/tunnels.js';
export * from './domains/services.js';
export * from './domains/ha.js';
export * from './domains/management.js';
// Feature sub-schemas: one `export * from './domains/ext/<slug>.js'` under the feature's anchor (wave-A-hotspots C3).
// wave-BC: F-det44-map-dslite-cnat
// wave-BC: F-pki
// wave-BC: F-ospf
// wave-BC: F-isis-rip
// wave-BC: F-mpls-srmpls
// wave-BC: F-srv6
// wave-BC: F-lisp
export * from './domains/ext/lisp.js';
// wave-BC: F-bfd-redistribution
// wave-BC: F-mpls-ldp
// wave-BC: F-igmp-mfib
// wave-A: F-bonding
export * from './domains/ext/bonding.js';
// wave-A: F-bridge-l2
export * from './domains/ext/bridge-l2.js';
// wave-A: F-loopback-bvi-gso-lldp-span
export * from './domains/ext/loopback-bvi-gso-lldp-span.js';
// wave-A: F-vrf-static-ecmp
// wave-A: F-neighbors-ra
// wave-A: F-rpf-adl-pbr
// wave-A: F-object-model
// wave-A: F-host-acl-nftables
// wave-A: P12
// wave-A: F-kea-dhcp-relay
// wave-A: F-unbound-chrony-syslog
export * from './domains/ext/snmp.js'; // F-snmp (unanchored)
export * from './domains/ext/host-stack.js'; // F-host-stack (unanchored)
export * from './primitives.js';
export * from './ip.js';
export * from './ui.js';
export * from './json.js';
export * from './pointer.js';
export * from './diff.js';
export * from './merge-patch.js';
export * from './semantic/index.js';
export * from './validate.js';
export * from './secrets.js';
