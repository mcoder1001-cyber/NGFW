import { isisRipValidators } from './isis-rip.js';
import { ospfValidators } from './ospf.js';
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
// wave-BC: F-default-vpp-nics
import { defaultVppNicsValidators } from './default-vpp-nics.js';
// wave-BC: F-det44-map-dslite-cnat
import { det44MapDsliteCnatValidators } from './det44-map-dslite-cnat.js';
import { nat46Validators } from './nat46.js'; // F-nat46 (unanchored)
// wave-BC: F-tunnels
// wave-BC: F-vrrp-config-sync
import { vrrpConfigSyncValidators } from './vrrp-config-sync.js';
// wave-BC: F-pki
// wave-BC: F-ikev2-native
import { ipsecValidators } from './ipsec.js';
// wave-BC: F-ospf
// wave-BC: F-isis-rip
// wave-BC: F-mpls-srmpls
import { mplsSrmplsValidators } from './mpls-srmpls.js';
// wave-BC: F-srv6
import { srv6Validators } from './srv6.js';
// wave-BC: F-lisp
import { lispValidators } from './lisp.js';
// wave-BC: F-bfd-redistribution
import { bfdRedistributionValidators } from "./bfd-redistribution.js";
// wave-BC: F-ra-vpn
import { raVpnValidators, raVpnTransportValidator } from './ra-vpn.js';
// wave-BC: F-mpls-ldp
// wave-BC: F-igmp-mfib
// wave-BC: F-ha-state-sync
import { haStateSyncValidators } from './ha-state-sync.js';
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
import { rpfAdlPbrValidators } from './rpf-adl-pbr.js';
// wave-A: F-object-model
// wave-A: F-host-acl-nftables
import { hostAclNftablesValidators } from './host-acl-nftables.js';
// wave-A: F-nat44-ed-sessions
import { nat44EdSessionsValidators } from './nat44-ed-sessions.js';
// wave-A: P11
// wave-A: F-wireguard
import { wireguardValidators } from './wireguard.js';
// wave-A: P12
import { bgpValidators } from './bgp.js';
// wave-A: F-kea-dhcp-relay
import { keaDhcpRelayValidators } from './kea-dhcp-relay.js';
// wave-A: F-unbound-chrony-syslog
import { snmpValidators } from './snmp.js'; // F-snmp (unanchored)
import { hostStackValidators } from './host-stack.js'; // F-host-stack (unanchored)
// wave-BC: F-ipfix-sflow (unanchored)
import { ipfixSflowValidators } from './ipfix-sflow.js';
// F-qos-flat (unanchored: no `wave-BC: F-qos-flat` anchor was seeded here)
import { qosFlatValidators } from './qos-flat.js';
import { unboundChronySyslogValidators } from './unbound-chrony-syslog.js';
import { lbValidators } from './lb.js';
import { globalBlockingValidators } from './global-blocking.js'; // F-global-blocking
import { pppoeValidators } from './pppoe.js'; // F-pppoe-client (unanchored)
import { dashboardPromAlarmsValidators } from './dashboard-prom-alarms.js'; // wave-BC: F-dashboard-prom-alarms
import { multiwanValidators } from './multiwan.js'; // F-multiwan (unanchored)
import { aaaValidators } from './aaa.js'; // wave-BC: F-aaa
import { autoBlockValidators } from './auto-block.js'; // F-bruteforce-block
import { igmpMfibValidators } from './igmp-mfib.js'; // wave-BC: F-igmp-mfib
import { mplsLdpValidators } from './mpls-ldp.js'; // wave-BC: F-mpls-ldp

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
  ...ospfValidators,
  ...natValidators,
  ...objectsValidators,
  ...aclValidators,
  ...vpnValidators,
  ...tunnelsValidators,
  ...servicesValidators,
  ...haValidators,
  ...haStateSyncValidators,
  ...managementValidators,
  // Feature rules: one spread line under the feature's anchor (wave-A-hotspots C2).
  // wave-BC: F-default-vpp-nics
  ...defaultVppNicsValidators,
  // wave-BC: F-det44-map-dslite-cnat
  ...det44MapDsliteCnatValidators,
  ...nat46Validators, // F-nat46 (unanchored)
  // wave-BC: F-tunnels
  // wave-BC: F-vrrp-config-sync
  ...vrrpConfigSyncValidators,
  // wave-BC: F-pki
  // wave-BC: F-ikev2-native
  ...ipsecValidators,
  // wave-BC: F-ospf
  // wave-BC: F-isis-rip
  ...isisRipValidators,
  // wave-BC: F-mpls-srmpls
  ...mplsSrmplsValidators,
  // wave-BC: F-srv6
  ...srv6Validators,
  // wave-BC: F-lisp
  ...lispValidators,
  // wave-BC: F-bfd-redistribution
  ...bfdRedistributionValidators,
  // wave-BC: F-ra-vpn
  ...raVpnValidators,
  raVpnTransportValidator,
  // wave-BC: F-mpls-ldp
  ...mplsLdpValidators,
  // wave-BC: F-igmp-mfib
  ...igmpMfibValidators,
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
  ...rpfAdlPbrValidators,
  // wave-A: F-object-model
  // wave-A: F-host-acl-nftables
  ...hostAclNftablesValidators,
  // wave-A: F-nat44-ed-sessions
  ...nat44EdSessionsValidators,
  // wave-A: P11
  // wave-A: F-wireguard
  ...wireguardValidators,
  // wave-A: P12
  ...bgpValidators,
  // wave-A: F-kea-dhcp-relay
  ...keaDhcpRelayValidators,
  // wave-A: F-unbound-chrony-syslog
  ...snmpValidators, // F-snmp (unanchored)
  ...hostStackValidators, // F-host-stack (unanchored)
  // wave-BC: F-ipfix-sflow (unanchored)
  ...ipfixSflowValidators,
  // F-qos-flat (unanchored)
  ...qosFlatValidators,
  ...unboundChronySyslogValidators,
  ...lbValidators,
  ...globalBlockingValidators, // F-global-blocking
  ...pppoeValidators, // F-pppoe-client (unanchored)
  ...dashboardPromAlarmsValidators, // wave-BC: F-dashboard-prom-alarms
  ...multiwanValidators, // F-multiwan (unanchored)
  ...aaaValidators, // wave-BC: F-aaa
  ...autoBlockValidators, // F-bruteforce-block
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
