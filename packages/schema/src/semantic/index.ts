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
import { lispValidators } from './lisp.js';
// wave-BC: F-bfd-redistribution
// wave-BC: F-ra-vpn
// wave-BC: F-mpls-ldp
// wave-BC: F-igmp-mfib
// wave-BC: F-ha-state-sync
// wave-A: F-bonding
import { bondingValidators } from './bonding.js';
// wave-A: F-bridge-l2
import { bridgeL2Validators } from './bridge-l2.js';
// wave-A: F-loopback-bvi-gso-lldp-span
import { loopbackBviGsoLldpSpanValidators } from './loopback-bvi-gso-lldp-span.js';
// wave-A: F-vrf-static-ecmp
import { vrfStaticEcmpValidators } from './vrf-static-ecmp.js';
// wave-A: F-neighbors-ra
import { neighborsRaValidators } from './neighbors-ra.js';
// wave-A: F-rpf-adl-pbr
// wave-A: F-object-model
// wave-A: F-host-acl-nftables
// wave-A: F-nat44-ed-sessions
// wave-A: P11
// wave-A: F-wireguard
// wave-A: P12
// wave-A: F-kea-dhcp-relay
// wave-A: F-unbound-chrony-syslog
import { snmpValidators } from './snmp.js'; // F-snmp (unanchored)
import { hostStackValidators } from './host-stack.js'; // F-host-stack (unanchored)
// wave-BC: F-ipfix-sflow (unanchored)
import { ipfixSflowValidators } from './ipfix-sflow.js';
// F-qos-flat (unanchored: no `wave-BC: F-qos-flat` anchor was seeded here)
import { qosFlatValidators } from './qos-flat.js';

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
  ...lispValidators,
  // wave-BC: F-bfd-redistribution
  // wave-BC: F-ra-vpn
  // wave-BC: F-mpls-ldp
  // wave-BC: F-igmp-mfib
  // wave-BC: F-ha-state-sync
  // wave-A: F-bonding
  ...bondingValidators,
  // wave-A: F-bridge-l2
  ...bridgeL2Validators,
  // wave-A: F-loopback-bvi-gso-lldp-span
  ...loopbackBviGsoLldpSpanValidators,
  // wave-A: F-vrf-static-ecmp
  ...vrfStaticEcmpValidators,
  // wave-A: F-neighbors-ra
  ...neighborsRaValidators,
  // wave-A: F-rpf-adl-pbr
  // wave-A: F-object-model
  // wave-A: F-host-acl-nftables
  // wave-A: F-nat44-ed-sessions
  // wave-A: P11
  // wave-A: F-wireguard
  // wave-A: P12
  // wave-A: F-kea-dhcp-relay
  // wave-A: F-unbound-chrony-syslog
  ...snmpValidators, // F-snmp (unanchored)
  ...hostStackValidators, // F-host-stack (unanchored)
  // wave-BC: F-ipfix-sflow (unanchored)
  ...ipfixSflowValidators,
  // F-qos-flat (unanchored)
  ...qosFlatValidators,
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
