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
// wave-A: F-bonding
// wave-A: F-bridge-l2
import { bridgeL2Validators } from './bridge-l2.js';
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
  // wave-A: F-bonding
  // wave-A: F-bridge-l2
  ...bridgeL2Validators,
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
