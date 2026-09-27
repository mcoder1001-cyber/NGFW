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
import { SecuritySchema } from './domains/security.js';

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
  security: SecuritySchema.prefault({}),
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
export * from './domains/security.js'; // F-bruteforce-block
// Feature sub-schemas: one `export * from './domains/ext/<slug>.js'` under the feature's anchor (wave-A-hotspots C3).
// wave-BC: F-det44-map-dslite-cnat
export * from './domains/ext/det44-map-dslite-cnat.js';
export * from './domains/ext/nat46.js'; // F-nat46 (unanchored)
// wave-BC: F-pki
// wave-BC: F-ospf
// wave-BC: F-isis-rip
// wave-BC: F-mpls-srmpls
export * from './domains/ext/mpls-srmpls.js';
// wave-BC: F-srv6
export * from './domains/ext/srv6.js';
// wave-BC: F-lisp
export * from './domains/ext/lisp.js';
// wave-BC: F-bfd-redistribution
// wave-BC: F-mpls-ldp
export * from './domains/ext/mpls-ldp.js';
// wave-BC: F-igmp-mfib
export * from './domains/ext/igmp-mfib.js';
// wave-A: F-bonding
export * from './domains/ext/bonding.js';
// wave-A: F-bridge-l2
export * from './domains/ext/bridge-l2.js';
// wave-A: F-loopback-bvi-gso-lldp-span
export * from './domains/ext/loopback-bvi-gso-lldp-span.js';
// wave-A: F-vrf-static-ecmp
export * from './domains/ext/vrf-static-ecmp.js';
// wave-A: F-neighbors-ra
export * from './domains/ext/neighbors-ra.js';
// wave-A: F-rpf-adl-pbr
export * from './domains/ext/rpf-adl-pbr.js';
// wave-A: F-object-model
// wave-A: F-host-acl-nftables
export * from './domains/ext/host-acl-nftables.js';
// wave-A: P12
export * from './domains/ext/frr-linuxcp.js';
// wave-A: F-kea-dhcp-relay
// wave-A: F-unbound-chrony-syslog
export * from './domains/ext/snmp.js'; // F-snmp (unanchored)
export * from './domains/ext/host-stack.js'; // F-host-stack (unanchored)
export * from './domains/ext/syslog.js';
export * from './domains/ext/lb.js';
export * from './domains/ext/rule-expiry.js'; // F-rule-expiry
export * from './domains/ext/global-blocking.js'; // F-global-blocking
export * from './domains/ext/global-blocking-parse.js'; // F-global-blocking
export * from './domains/ext/pppoe.js'; // F-pppoe-client (unanchored)
export * from './domains/ext/dashboard-prom-alarms.js'; // wave-BC: F-dashboard-prom-alarms
export * from './domains/ext/multiwan.js'; // F-multiwan (unanchored)
export * from './domains/ext/aaa.js'; // wave-BC: F-aaa
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
