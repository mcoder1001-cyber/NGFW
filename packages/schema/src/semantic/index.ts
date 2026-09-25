import type { RootConfig, RootKey } from '../index.js';
import { SemanticRegistry, type SemanticIssue, type ValidatorDefinition } from './registry.js';
import { systemValidators } from './system.js';
import { dataplaneValidators } from './dataplane.js';
import { interfacesValidators } from './interfaces.js';
import { vrfsValidators } from './vrfs.js';
import { routingValidators } from './routing.js';
import { natValidators } from './nat.js';
import { objectsValidators } from './objects.js';
import { aclValidators } from './acl.js';
import { vpnValidators } from './vpn.js';
import { tunnelsValidators } from './tunnels.js';
import { servicesValidators } from './services.js';
import { haValidators } from './ha.js';
import { managementValidators } from './management.js';
// Feature rule files (semantic/<slug>.ts exporting `<slug>Validators`): one import under the feature's anchor.
// wave-BC: F-det44-map-dslite-cnat
// wave-BC: F-tunnels
// wave-BC: F-vrrp-config-sync
// wave-BC: F-pki
// wave-BC: F-ikev2-native
// wave-BC: F-ospf
// wave-BC: F-isis-rip
// wave-BC: F-mpls-srmpls
// wave-BC: F-srv6
// wave-BC: F-lisp
// wave-BC: F-bfd-redistribution
// wave-BC: F-ra-vpn
// wave-BC: F-mpls-ldp
// wave-BC: F-igmp-mfib
// wave-BC: F-ha-state-sync
// wave-A: F-bonding
// wave-A: F-bridge-l2
// wave-A: F-loopback-bvi-gso-lldp-span
// wave-A: F-vrf-static-ecmp
// wave-A: F-neighbors-ra
// wave-A: F-rpf-adl-pbr
// wave-A: F-object-model
// wave-A: F-host-acl-nftables
// wave-A: F-nat44-ed-sessions
// wave-A: P11
// wave-A: F-wireguard
// wave-A: P12
// wave-A: F-kea-dhcp-relay
// wave-A: F-unbound-chrony-syslog
import { hostStackValidators } from './host-stack.js'; // F-host-stack (unanchored)

export * from './registry.js';

/**
 * All semantic validators, one array per domain (same layout and ownership as `../domains/`): each group adds
 * rules only to its own `semantic/<key>.ts`, so this aggregator never needs editing.
 */
export const SEMANTIC_VALIDATORS: readonly ValidatorDefinition[] = [
  ...systemValidators,
  ...dataplaneValidators,
  ...interfacesValidators,
  ...vrfsValidators,
  ...routingValidators,
  ...natValidators,
  ...objectsValidators,
  ...aclValidators,
  ...vpnValidators,
  ...tunnelsValidators,
  ...servicesValidators,
  ...haValidators,
  ...managementValidators,
  // Feature rules: one spread line under the feature's anchor (wave-A-hotspots C2).
  // wave-BC: F-det44-map-dslite-cnat
  // wave-BC: F-tunnels
  // wave-BC: F-vrrp-config-sync
  // wave-BC: F-pki
  // wave-BC: F-ikev2-native
  // wave-BC: F-ospf
  // wave-BC: F-isis-rip
  // wave-BC: F-mpls-srmpls
  // wave-BC: F-srv6
  // wave-BC: F-lisp
  // wave-BC: F-bfd-redistribution
  // wave-BC: F-ra-vpn
  // wave-BC: F-mpls-ldp
  // wave-BC: F-igmp-mfib
  // wave-BC: F-ha-state-sync
  // wave-A: F-bonding
  // wave-A: F-bridge-l2
  // wave-A: F-loopback-bvi-gso-lldp-span
  // wave-A: F-vrf-static-ecmp
  // wave-A: F-neighbors-ra
  // wave-A: F-rpf-adl-pbr
  // wave-A: F-object-model
  // wave-A: F-host-acl-nftables
  // wave-A: F-nat44-ed-sessions
  // wave-A: P11
  // wave-A: F-wireguard
  // wave-A: P12
  // wave-A: F-kea-dhcp-relay
  // wave-A: F-unbound-chrony-syslog
  ...hostStackValidators, // F-host-stack (unanchored)
];

/** Process-wide registry populated from {@link SEMANTIC_VALIDATORS}. */
export const semanticRegistry = new SemanticRegistry();
for (const v of SEMANTIC_VALIDATORS) semanticRegistry.register(v);

/**
 * Tier (b) validation entry point: run every registered validator against a schema-valid document.
 * Pass `domains` to run only the validators that read those top-level keys. `[]` means no findings.
 */
export function validateSemantics(
  config: RootConfig,
  domains?: readonly RootKey[],
): SemanticIssue[] {
  return semanticRegistry.run(config, domains);
}
