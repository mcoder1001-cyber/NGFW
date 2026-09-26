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
// Feature namespaces (locale namespace = task slug): the en and fa import under the feature's anchor (wave-A-hotspots W3).
// wave-BC: F-det44-map-dslite-cnat
// wave-BC: F-tunnels
// wave-BC: P10
// wave-BC: F-vrrp-config-sync
// wave-BC: F-pki
// wave-BC: F-ikev2-native
// wave-BC: F-ospf
// wave-BC: F-isis-rip
// wave-BC: P14
// wave-BC: F-mpls-srmpls
// wave-BC: F-lb
// wave-BC: F-qos-flat
import enQosFlat from './locales/en/qos-flat.json';
import faQosFlat from './locales/fa/qos-flat.json';
// wave-BC: F-host-stack
import enHostStack from './locales/en/host-stack.json';
import faHostStack from './locales/fa/host-stack.json';
// wave-BC: F-snmp
import enSnmp from './locales/en/snmp.json';
import faSnmp from './locales/fa/snmp.json';
// wave-BC: F-ipfix-sflow
import enIpfixSflow from './locales/en/ipfix-sflow.json';
import faIpfixSflow from './locales/fa/ipfix-sflow.json';
// wave-BC: F-capture-trace
// wave-BC: F-srv6
// wave-BC: F-lisp
import enLisp from './locales/en/lisp.json';
import faLisp from './locales/fa/lisp.json';
// wave-BC: F-bfd-redistribution
// wave-BC: F-ra-vpn
// wave-BC: F-mpls-ldp
// wave-BC: F-igmp-mfib
// wave-BC: F-dashboard-prom-alarms
// wave-BC: F-hardening-lite
// wave-BC: F-aaa
// wave-BC: F-licensing
import enLicensing from './locales/en/licensing.json';
import faLicensing from './locales/fa/licensing.json';
// wave-BC: F-restconf-yang
// wave-BC: F-ha-state-sync
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
// wave-A: F-nat44-ed-sessions
import enNat44EdSessions from './locales/en/nat44-ed-sessions.json';
import faNat44EdSessions from './locales/fa/nat44-ed-sessions.json';
// wave-A: F-nat44-ei-64-66-nptv6
// wave-A: P11
// wave-A: F-wireguard
// wave-A: P12
// wave-A: F-kea-dhcp-relay
// wave-A: F-unbound-chrony-syslog
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
  // Feature namespaces: one line under the feature's anchor.
  // wave-BC: F-det44-map-dslite-cnat
  // wave-BC: F-tunnels
  // wave-BC: P10
  // wave-BC: F-vrrp-config-sync
  // wave-BC: F-pki
  // wave-BC: F-ikev2-native
  // wave-BC: F-ospf
  // wave-BC: F-isis-rip
  // wave-BC: P14
  // wave-BC: F-mpls-srmpls
  // wave-BC: F-lb
  // wave-BC: F-qos-flat
  'qos-flat',
  // wave-BC: F-host-stack
  'host-stack',
  // wave-BC: F-snmp
  'snmp',
  // wave-BC: F-ipfix-sflow
  'ipfix-sflow',
  // wave-BC: F-capture-trace
  // wave-BC: F-srv6
  // wave-BC: F-lisp
  'lisp',
  // wave-BC: F-bfd-redistribution
  // wave-BC: F-ra-vpn
  // wave-BC: F-mpls-ldp
  // wave-BC: F-igmp-mfib
  // wave-BC: F-dashboard-prom-alarms
  // wave-BC: F-hardening-lite
  // wave-BC: F-aaa
  // wave-BC: F-licensing
  'licensing',
  // wave-BC: F-restconf-yang
  // wave-BC: F-ha-state-sync
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
  // wave-A: F-nat44-ed-sessions
  'nat44-ed-sessions',
  // wave-A: F-nat44-ei-64-66-nptv6
  // wave-A: P11
  // wave-A: F-wireguard
  // wave-A: P12
  // wave-A: F-kea-dhcp-relay
  // wave-A: F-unbound-chrony-syslog
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
  // Feature namespaces: one line under the feature's anchor.
  // wave-BC: F-det44-map-dslite-cnat
  // wave-BC: F-tunnels
  // wave-BC: P10
  // wave-BC: F-vrrp-config-sync
  // wave-BC: F-pki
  // wave-BC: F-ikev2-native
  // wave-BC: F-ospf
  // wave-BC: F-isis-rip
  // wave-BC: P14
  // wave-BC: F-mpls-srmpls
  // wave-BC: F-lb
  // wave-BC: F-qos-flat
  'qos-flat': enQosFlat,
  // wave-BC: F-host-stack
  'host-stack': enHostStack,
  // wave-BC: F-snmp
  snmp: enSnmp,
  // wave-BC: F-ipfix-sflow
  'ipfix-sflow': enIpfixSflow,
  // wave-BC: F-capture-trace
  // wave-BC: F-srv6
  // wave-BC: F-lisp
  lisp: enLisp,
  // wave-BC: F-bfd-redistribution
  // wave-BC: F-ra-vpn
  // wave-BC: F-mpls-ldp
  // wave-BC: F-igmp-mfib
  // wave-BC: F-dashboard-prom-alarms
  // wave-BC: F-hardening-lite
  // wave-BC: F-aaa
  // wave-BC: F-licensing
  licensing: enLicensing,
  // wave-BC: F-restconf-yang
  // wave-BC: F-ha-state-sync
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
  // wave-A: F-nat44-ed-sessions
  'nat44-ed-sessions': enNat44EdSessions,
  // wave-A: F-nat44-ei-64-66-nptv6
  // wave-A: P11
  // wave-A: F-wireguard
  // wave-A: P12
  // wave-A: F-kea-dhcp-relay
  // wave-A: F-unbound-chrony-syslog
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
  // Feature namespaces: one line under the feature's anchor.
  // wave-BC: F-det44-map-dslite-cnat
  // wave-BC: F-tunnels
  // wave-BC: P10
  // wave-BC: F-vrrp-config-sync
  // wave-BC: F-pki
  // wave-BC: F-ikev2-native
  // wave-BC: F-ospf
  // wave-BC: F-isis-rip
  // wave-BC: P14
  // wave-BC: F-mpls-srmpls
  // wave-BC: F-lb
  // wave-BC: F-qos-flat
  'qos-flat': faQosFlat,
  // wave-BC: F-host-stack
  'host-stack': faHostStack,
  // wave-BC: F-snmp
  snmp: faSnmp,
  // wave-BC: F-ipfix-sflow
  'ipfix-sflow': faIpfixSflow,
  // wave-BC: F-capture-trace
  // wave-BC: F-srv6
  // wave-BC: F-lisp
  lisp: faLisp,
  // wave-BC: F-bfd-redistribution
  // wave-BC: F-ra-vpn
  // wave-BC: F-mpls-ldp
  // wave-BC: F-igmp-mfib
  // wave-BC: F-dashboard-prom-alarms
  // wave-BC: F-hardening-lite
  // wave-BC: F-aaa
  // wave-BC: F-licensing
  licensing: faLicensing,
  // wave-BC: F-restconf-yang
  // wave-BC: F-ha-state-sync
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
  // wave-A: F-nat44-ed-sessions
  'nat44-ed-sessions': faNat44EdSessions,
  // wave-A: F-nat44-ei-64-66-nptv6
  // wave-A: P11
  // wave-A: F-wireguard
  // wave-A: P12
  // wave-A: F-kea-dhcp-relay
  // wave-A: F-unbound-chrony-syslog
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
