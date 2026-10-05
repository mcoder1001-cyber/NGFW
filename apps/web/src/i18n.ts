import enSetup from './locales/en/setup.json';
import faSetup from './locales/fa/setup.json';
import { UI_KIT_NS, uiKitResources } from '@ngfw/ui-kit';
import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import { DEV_ROUTES } from './build-flags';
import enAuth from './locales/en/auth.json';
import enCommon from './locales/en/common.json';
import enConfig from './locales/en/config.json';
import enDev from './locales/en/dev.json';
import enInterfaces from './locales/en/interfaces.json';
import enNav from './locales/en/nav.json';
import enRevisions from './locales/en/revisions.json';
import enUsers from './locales/en/users.json';
import faAuth from './locales/fa/auth.json';
import faCommon from './locales/fa/common.json';
import faConfig from './locales/fa/config.json';
import faDev from './locales/fa/dev.json';
import faInterfaces from './locales/fa/interfaces.json';
import faNav from './locales/fa/nav.json';
import faRevisions from './locales/fa/revisions.json';
import faUsers from './locales/fa/users.json';
import enServices from './locales/en/services.json';
import faServices from './locales/fa/services.json';
import enVpn from './locales/en/vpn.json';
import faVpn from './locales/fa/vpn.json';
// wave-A: UI-domain-editor (no anchor was seeded for this task by W-seed; added directly, see
// UI-domain-editor-questions.md — the same top-level style as `interfaces`/`services`/`vpn` above, not the
// per-feature anchor list below, since `advanced` is a standing namespace of this task, not a future feature).
import enAdvanced from './locales/en/advanced.json';
import faAdvanced from './locales/fa/advanced.json';
// Feature namespaces (locale namespace = task slug): the en and fa import under the feature's anchor (wave-A-hotspots W3).
// wave-BC: F-det44-map-dslite-cnat
import enDet44MapDsliteCnat from './locales/en/det44-map-dslite-cnat.json';
import faDet44MapDsliteCnat from './locales/fa/det44-map-dslite-cnat.json';
// F-nat46 (unanchored)
import enNat46 from './locales/en/nat46.json';
import faNat46 from './locales/fa/nat46.json';
// wave-BC: F-tunnels
import enTunnels from './locales/en/tunnels.json';
import faTunnels from './locales/fa/tunnels.json';
// wave-BC: P10
// wave-BC: F-vrrp-config-sync
import enHa from './locales/en/ha.json';
import faHa from './locales/fa/ha.json';
// wave-BC: F-pki
import enPkiInventory from './locales/en/pki-inventory.json';
import faPkiInventory from './locales/fa/pki-inventory.json';
// wave-BC: F-ikev2-native
import enIpsec from './locales/en/ipsec.json';
import faIpsec from './locales/fa/ipsec.json';
// wave-BC: F-ospf
// wave-BC: F-isis-rip
// wave-BC: P14
// wave-BC: F-mpls-srmpls
import enMplsSrmpls from './locales/en/mpls-srmpls.json';
import faMplsSrmpls from './locales/fa/mpls-srmpls.json';
// wave-BC: F-lb
import enLb from './locales/en/lb.json';
import faLb from './locales/fa/lb.json';
// wave-BC: F-qos-flat
import enQosFlat from './locales/en/qos-flat.json';
import faQosFlat from './locales/fa/qos-flat.json';
// wave-BC: F-host-stack
import enHostStack from './locales/en/host-stack.json';
import faHostStack from './locales/fa/host-stack.json';
// F-dataplane-ui (unanchored)
import enDataplane from './locales/en/dataplane.json';
import faDataplane from './locales/fa/dataplane.json';
import enManagement from './locales/en/management.json'; // F-management-ui (unanchored)
import faManagement from './locales/fa/management.json'; // F-management-ui (unanchored)
// F-system-identity (unanchored)
import enSystemIdentity from './locales/en/system-identity.json';
import faSystemIdentity from './locales/fa/system-identity.json';
// wave-BC: F-snmp
import enSnmp from './locales/en/snmp.json';
import faSnmp from './locales/fa/snmp.json';
// wave-BC: F-ipfix-sflow
import enIpfixSflow from './locales/en/ipfix-sflow.json';
import faIpfixSflow from './locales/fa/ipfix-sflow.json';
// wave-BC: F-capture-trace
import enCaptureTrace from './locales/en/capture-trace.json';
import faCaptureTrace from './locales/fa/capture-trace.json';
// wave-BC: F-srv6
import enSrv6 from './locales/en/srv6.json';
import faSrv6 from './locales/fa/srv6.json';
// wave-BC: F-lisp
import enLisp from './locales/en/lisp.json';
import faLisp from './locales/fa/lisp.json';
// wave-BC: F-bfd-redistribution
import enBfdRedistribution from "./locales/en/bfd-redistribution.json";
import faBfdRedistribution from "./locales/fa/bfd-redistribution.json";
// wave-BC: F-ra-vpn
import enRaVpn from './locales/en/ra-vpn.json';
import faRaVpn from './locales/fa/ra-vpn.json';
// wave-BC: F-mpls-ldp
import enMplsLdp from './locales/en/mpls-ldp.json';
import faMplsLdp from './locales/fa/mpls-ldp.json';
// wave-BC: F-igmp-mfib
import enIgmpMfib from './locales/en/igmp-mfib.json';
import faIgmpMfib from './locales/fa/igmp-mfib.json';
// wave-BC: F-dashboard-prom-alarms
// web: WEB-dashboard
import enDashboard from './locales/en/dashboard.json';
import faDashboard from './locales/fa/dashboard.json';
// wave-BC: F-hardening-lite
// wave-BC: F-aaa
import enAaa from './locales/en/aaa.json';
import faAaa from './locales/fa/aaa.json';
// wave-BC: F-licensing
import enLicensing from './locales/en/licensing.json';
import faLicensing from './locales/fa/licensing.json';
// wave-BC: F-restconf-yang
import enRestconfYang from './locales/en/restconf-yang.json';
import faRestconfYang from './locales/fa/restconf-yang.json';
// wave-BC: F-ha-state-sync
import enHaStateSync from './locales/en/ha-state-sync.json';
import faHaStateSync from './locales/fa/ha-state-sync.json';
// wave-BC: F-ab-upgrade
// wave-BC: F-images
// wave-BC: F-backup-restore
// wave-A: UI-domain-editor
// wave-A: F-vlan-qinq
import enVlanQinq from './locales/en/vlan-qinq.json';
import faVlanQinq from './locales/fa/vlan-qinq.json';
// wave-A: F-bonding
import enBonding from './locales/en/bonding.json';
import faBonding from './locales/fa/bonding.json';
// wave-A: F-bridge-l2
import enBridgeL2 from './locales/en/bridge-l2.json';
import faBridgeL2 from './locales/fa/bridge-l2.json';
// wave-A: F-loopback-bvi-gso-lldp-span
import enLoopbackBviGsoLldpSpan from './locales/en/loopback-bvi-gso-lldp-span.json';
import faLoopbackBviGsoLldpSpan from './locales/fa/loopback-bvi-gso-lldp-span.json';
// wave-A: F-vrf-static-ecmp
import enVrfStaticEcmp from './locales/en/vrf-static-ecmp.json';
import faVrfStaticEcmp from './locales/fa/vrf-static-ecmp.json';
// wave-A: F-neighbors-ra
import enNeighborsRa from './locales/en/neighbors-ra.json';
import faNeighborsRa from './locales/fa/neighbors-ra.json';
import './domains/routing/neighbors-ra/interface-strings';
// wave-A: F-rpf-adl-pbr
import './domains/routing/rpf-adl-pbr/drawer-i18n';
import enRpfAdlPbr from './locales/en/rpf-adl-pbr.json';
import faRpfAdlPbr from './locales/fa/rpf-adl-pbr.json';
// wave-A: F-object-model
import enObjectModel from './locales/en/object-model.json';
import faObjectModel from './locales/fa/object-model.json';
// wave-A: F-acl
import enAcl from './locales/en/acl.json';
import faAcl from './locales/fa/acl.json';
// wave-A: F-host-acl-nftables
import enHostAclNftables from './locales/en/host-acl-nftables.json';
import faHostAclNftables from './locales/fa/host-acl-nftables.json';
// F-global-blocking (unanchored)
import enGlobalBlocking from './locales/en/global-blocking.json';
import faGlobalBlocking from './locales/fa/global-blocking.json';
// wave-BC: F-dashboard-prom-alarms
import enDashPromAlarms from './locales/en/dashboard-prom-alarms.json';
import faDashPromAlarms from './locales/fa/dashboard-prom-alarms.json';
// F-multiwan (unanchored)
import enMultiwan from './locales/en/multiwan.json';
import faMultiwan from './locales/fa/multiwan.json';
// F-bruteforce-block (unanchored)
import enAutoBlock from './locales/en/auto-block.json';
import faAutoBlock from './locales/fa/auto-block.json';
// wave-A: F-nat44-ed-sessions
import enNat44EdSessions from './locales/en/nat44-ed-sessions.json';
import faNat44EdSessions from './locales/fa/nat44-ed-sessions.json';
// wave-A: F-nat44-ei-64-66-nptv6
import enNat44Ei6466Nptv6 from './locales/en/nat44-ei-64-66-nptv6.json';
import faNat44Ei6466Nptv6 from './locales/fa/nat44-ei-64-66-nptv6.json';
// wave-A: P11
// wave-A: F-wireguard
import enWireguard from './locales/en/wireguard.json';
import faWireguard from './locales/fa/wireguard.json';
// wave-A: P12
import enBgp from './locales/en/bgp.json';
import faBgp from './locales/fa/bgp.json';
// wave-A: F-kea-dhcp-relay
import enKeaDhcpRelay from './locales/en/kea-dhcp-relay.json';
import faKeaDhcpRelay from './locales/fa/kea-dhcp-relay.json';
// wave-A: F-unbound-chrony-syslog
import enUcs from './locales/en/unbound-chrony-syslog.json';
import faUcs from './locales/fa/unbound-chrony-syslog.json';
import { loadSettings } from './settings/storage';

export const NAMESPACES = [
  'common',
  'nav',
  'auth',
  'config',
  'revisions',
  'users',
  'interfaces',
  'services',
  'vpn',
  'advanced',
  // Feature namespaces: one line under the feature's anchor.
  // wave-BC: F-det44-map-dslite-cnat
  'det44-map-dslite-cnat',
  'nat46', // F-nat46 (unanchored)
  // wave-BC: F-tunnels
  'tunnels',
  // wave-BC: P10
  // wave-BC: F-vrrp-config-sync
  'ha',
  // wave-BC: F-pki
  'pkiInventory',
  // wave-BC: F-ikev2-native
  'ipsec',
  // wave-BC: F-ospf
  // wave-BC: F-isis-rip
  // wave-BC: P14
  // wave-BC: F-mpls-srmpls
  'mpls-srmpls',
  // wave-BC: F-lb
  'lb',
  // wave-BC: F-qos-flat
  'qos-flat',
  // wave-BC: F-host-stack
  'host-stack',
  // F-system-identity (unanchored)
  'system-identity',
  'dataplane', // F-dataplane-ui (unanchored)
  'management', // F-management-ui (unanchored)
  // wave-BC: F-snmp
  'snmp',
  // wave-BC: F-ipfix-sflow
  'ipfix-sflow',
  // wave-BC: F-capture-trace
  'capture-trace',
  // wave-BC: F-srv6
  'srv6',
  // wave-BC: F-lisp
  'lisp',
  // wave-BC: F-bfd-redistribution
  'bfd-redistribution',
  // wave-BC: F-ra-vpn
  'ra-vpn',
  // wave-BC: F-mpls-ldp
  'mpls-ldp',
  // wave-BC: F-igmp-mfib
  'igmp-mfib',
  // wave-BC: F-dashboard-prom-alarms
  // web: WEB-dashboard
  'dashboard',
  // wave-BC: F-hardening-lite
  // wave-BC: F-aaa
  'aaa',
  // wave-BC: F-licensing
  'licensing',
  // wave-BC: F-restconf-yang
  'restconf-yang',
  // wave-BC: F-ha-state-sync
  'ha-state-sync',
  // wave-BC: F-ab-upgrade
  // wave-BC: F-images
  // wave-BC: F-backup-restore
  // wave-A: UI-domain-editor
  // wave-A: F-vlan-qinq
  'vlan-qinq',
  // wave-A: F-bonding
  'bonding',
  // wave-A: F-bridge-l2
  'bridge-l2',
  // wave-A: F-loopback-bvi-gso-lldp-span
  'loopback-bvi-gso-lldp-span',
  // wave-A: F-vrf-static-ecmp
  'vrf-static-ecmp',
  // wave-A: F-neighbors-ra
  'neighbors-ra',
  // wave-A: F-rpf-adl-pbr
  'rpf-adl-pbr',
  // wave-A: F-object-model
  'object-model',
  // wave-A: F-acl
  'acl',
  // wave-A: F-host-acl-nftables
  'host-acl-nftables',
  'global-blocking', // F-global-blocking (unanchored)
  'dashboard-prom-alarms', // wave-BC: F-dashboard-prom-alarms
  'multiwan', // F-multiwan (unanchored)
  'auto-block', // F-bruteforce-block (unanchored)
  // wave-A: F-nat44-ed-sessions
  'nat44-ed-sessions',
  // wave-A: F-nat44-ei-64-66-nptv6
  'nat44-ei-64-66-nptv6',
  // wave-A: P11
  // wave-A: F-wireguard
  'wireguard',
  // wave-A: P12
  'bgp',
  // wave-A: F-kea-dhcp-relay
  'kea-dhcp-relay',
  // wave-A: F-unbound-chrony-syslog
  'unbound-chrony-syslog',
  'dev',
  UI_KIT_NS,
] as const;

const en = {
  common: enCommon,
  nav: enNav,
  auth: enAuth,
  config: enConfig,
  revisions: enRevisions,
  users: enUsers,
  interfaces: enInterfaces,
  services: enServices,
  vpn: enVpn,
  advanced: enAdvanced,
  // Feature namespaces: one line under the feature's anchor.
  // wave-BC: F-det44-map-dslite-cnat
  'det44-map-dslite-cnat': enDet44MapDsliteCnat,
  nat46: enNat46, // F-nat46 (unanchored)
  // wave-BC: F-tunnels
  tunnels: enTunnels,
  // wave-BC: P10
  // wave-BC: F-vrrp-config-sync
  ha: enHa,
  // wave-BC: F-pki
  pkiInventory: enPkiInventory,
  // wave-BC: F-ikev2-native
  ipsec: enIpsec,
  // wave-BC: F-ospf
  // wave-BC: F-isis-rip
  // wave-BC: P14
  // wave-BC: F-mpls-srmpls
  'mpls-srmpls': enMplsSrmpls,
  // wave-BC: F-lb
  lb: enLb,
  // wave-BC: F-qos-flat
  'qos-flat': enQosFlat,
  // wave-BC: F-host-stack
  'host-stack': enHostStack,
  // F-system-identity (unanchored)
  'system-identity': enSystemIdentity,
  dataplane: enDataplane, // F-dataplane-ui (unanchored)
  setup: enSetup,
  management: enManagement, // F-management-ui (unanchored)
  // wave-BC: F-snmp
  snmp: enSnmp,
  // wave-BC: F-ipfix-sflow
  'ipfix-sflow': enIpfixSflow,
  // wave-BC: F-capture-trace
  'capture-trace': enCaptureTrace,
  // wave-BC: F-srv6
  srv6: enSrv6,
  // wave-BC: F-lisp
  lisp: enLisp,
  // wave-BC: F-bfd-redistribution
  "bfd-redistribution": enBfdRedistribution,
  // wave-BC: F-ra-vpn
  'ra-vpn': enRaVpn,
  // wave-BC: F-mpls-ldp
  'mpls-ldp': enMplsLdp,
  // wave-BC: F-igmp-mfib
  'igmp-mfib': enIgmpMfib,
  // wave-BC: F-dashboard-prom-alarms
  // web: WEB-dashboard
  dashboard: enDashboard,
  // wave-BC: F-hardening-lite
  // wave-BC: F-aaa
  aaa: enAaa,
  // wave-BC: F-licensing
  licensing: enLicensing,
  // wave-BC: F-restconf-yang
  'restconf-yang': enRestconfYang,
  // wave-BC: F-ha-state-sync
  'ha-state-sync': enHaStateSync,
  // wave-BC: F-ab-upgrade
  // wave-BC: F-images
  // wave-BC: F-backup-restore
  // wave-A: UI-domain-editor
  // wave-A: F-vlan-qinq
  'vlan-qinq': enVlanQinq,
  // wave-A: F-bonding
  bonding: enBonding,
  // wave-A: F-bridge-l2
  'bridge-l2': enBridgeL2,
  // wave-A: F-loopback-bvi-gso-lldp-span
  'loopback-bvi-gso-lldp-span': enLoopbackBviGsoLldpSpan,
  // wave-A: F-vrf-static-ecmp
  'vrf-static-ecmp': enVrfStaticEcmp,
  // wave-A: F-neighbors-ra
  'neighbors-ra': enNeighborsRa,
  // wave-A: F-rpf-adl-pbr
  'rpf-adl-pbr': enRpfAdlPbr,
  // wave-A: F-object-model
  'object-model': enObjectModel,
  // wave-A: F-acl
  acl: enAcl,
  // wave-A: F-host-acl-nftables
  'host-acl-nftables': enHostAclNftables,
  'global-blocking': enGlobalBlocking,
  'dashboard-prom-alarms': enDashPromAlarms,
  multiwan: enMultiwan,
  'auto-block': enAutoBlock,
  // wave-A: F-nat44-ed-sessions
  'nat44-ed-sessions': enNat44EdSessions,
  // wave-A: F-nat44-ei-64-66-nptv6
  'nat44-ei-64-66-nptv6': enNat44Ei6466Nptv6,
  // wave-A: P11
  // wave-A: F-wireguard
  wireguard: enWireguard,
  // wave-A: P12
  bgp: enBgp,
  // wave-A: F-kea-dhcp-relay
  'kea-dhcp-relay': enKeaDhcpRelay,
  // wave-A: F-unbound-chrony-syslog
  'unbound-chrony-syslog': enUcs,
};
const fa = {
  common: faCommon,
  nav: faNav,
  auth: faAuth,
  config: faConfig,
  revisions: faRevisions,
  users: faUsers,
  interfaces: faInterfaces,
  services: faServices,
  vpn: faVpn,
  advanced: faAdvanced,
  // Feature namespaces: one line under the feature's anchor.
  // wave-BC: F-det44-map-dslite-cnat
  'det44-map-dslite-cnat': faDet44MapDsliteCnat,
  nat46: faNat46, // F-nat46 (unanchored)
  // wave-BC: F-tunnels
  tunnels: faTunnels,
  // wave-BC: P10
  // wave-BC: F-vrrp-config-sync
  ha: faHa,
  // wave-BC: F-pki
  pkiInventory: faPkiInventory,
  // wave-BC: F-ikev2-native
  ipsec: faIpsec,
  // wave-BC: F-ospf
  // wave-BC: F-isis-rip
  // wave-BC: P14
  // wave-BC: F-mpls-srmpls
  'mpls-srmpls': faMplsSrmpls,
  // wave-BC: F-lb
  lb: faLb,
  // wave-BC: F-qos-flat
  'qos-flat': faQosFlat,
  // wave-BC: F-host-stack
  'host-stack': faHostStack,
  // F-system-identity (unanchored)
  'system-identity': faSystemIdentity,
  dataplane: faDataplane, // F-dataplane-ui (unanchored)
  setup: faSetup,
  management: faManagement, // F-management-ui (unanchored)
  // wave-BC: F-snmp
  snmp: faSnmp,
  // wave-BC: F-ipfix-sflow
  'ipfix-sflow': faIpfixSflow,
  // wave-BC: F-capture-trace
  'capture-trace': faCaptureTrace,
  // wave-BC: F-srv6
  srv6: faSrv6,
  // wave-BC: F-lisp
  lisp: faLisp,
  // wave-BC: F-bfd-redistribution
  "bfd-redistribution": faBfdRedistribution,
  // wave-BC: F-ra-vpn
  'ra-vpn': faRaVpn,
  // wave-BC: F-mpls-ldp
  'mpls-ldp': faMplsLdp,
  // wave-BC: F-igmp-mfib
  'igmp-mfib': faIgmpMfib,
  // wave-BC: F-dashboard-prom-alarms
  // web: WEB-dashboard
  dashboard: faDashboard,
  // wave-BC: F-hardening-lite
  // wave-BC: F-aaa
  aaa: faAaa,
  // wave-BC: F-licensing
  licensing: faLicensing,
  // wave-BC: F-restconf-yang
  'restconf-yang': faRestconfYang,
  // wave-BC: F-ha-state-sync
  'ha-state-sync': faHaStateSync,
  // wave-BC: F-ab-upgrade
  // wave-BC: F-images
  // wave-BC: F-backup-restore
  // wave-A: UI-domain-editor
  // wave-A: F-vlan-qinq
  'vlan-qinq': faVlanQinq,
  // wave-A: F-bonding
  bonding: faBonding,
  // wave-A: F-bridge-l2
  'bridge-l2': faBridgeL2,
  // wave-A: F-loopback-bvi-gso-lldp-span
  'loopback-bvi-gso-lldp-span': faLoopbackBviGsoLldpSpan,
  // wave-A: F-vrf-static-ecmp
  'vrf-static-ecmp': faVrfStaticEcmp,
  // wave-A: F-neighbors-ra
  'neighbors-ra': faNeighborsRa,
  // wave-A: F-rpf-adl-pbr
  'rpf-adl-pbr': faRpfAdlPbr,
  // wave-A: F-object-model
  'object-model': faObjectModel,
  // wave-A: F-acl
  acl: faAcl,
  // wave-A: F-host-acl-nftables
  'host-acl-nftables': faHostAclNftables,
  'global-blocking': faGlobalBlocking,
  'dashboard-prom-alarms': faDashPromAlarms,
  multiwan: faMultiwan,
  'auto-block': faAutoBlock,
  // wave-A: F-nat44-ed-sessions
  'nat44-ed-sessions': faNat44EdSessions,
  // wave-A: F-nat44-ei-64-66-nptv6
  'nat44-ei-64-66-nptv6': faNat44Ei6466Nptv6,
  // wave-A: P11
  // wave-A: F-wireguard
  wireguard: faWireguard,
  // wave-A: P12
  bgp: faBgp,
  // wave-A: F-kea-dhcp-relay
  'kea-dhcp-relay': faKeaDhcpRelay,
  // wave-A: F-unbound-chrony-syslog
  'unbound-chrony-syslog': faUcs,
};

/** The `dev` namespace (developer demo pages) is loaded only when the demo routes are built in (review P07a M1). */
export const resources = DEV_ROUTES
  ? {
      en: { ...en, dev: enDev, [UI_KIT_NS]: uiKitResources.en },
      fa: { ...fa, dev: faDev, [UI_KIT_NS]: uiKitResources.fa },
    }
  : {
      en: { ...en, [UI_KIT_NS]: uiKitResources.en },
      fa: { ...fa, [UI_KIT_NS]: uiKitResources.fa },
    };

void i18n.use(initReactI18next).init({
  resources,
  lng: loadSettings().lang,
  fallbackLng: 'en',
  supportedLngs: ['en', 'fa'],
  defaultNS: 'common',
  ns: [...NAMESPACES],
  interpolation: { escapeValue: false },
  returnNull: false,
  initImmediate: false,
});

export default i18n;
