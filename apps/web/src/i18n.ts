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
// wave-A: F-vlan-qinq
import enVlanQinq from './locales/en/vlan-qinq.json';
import faVlanQinq from './locales/fa/vlan-qinq.json';
// wave-A: F-bonding
// wave-A: F-bridge-l2
// wave-A: F-loopback-bvi-gso-lldp-span
// wave-A: F-vrf-static-ecmp
// wave-A: F-neighbors-ra
// wave-A: F-rpf-adl-pbr
// wave-A: F-object-model
// wave-A: F-acl
// wave-A: F-host-acl-nftables
// wave-A: F-nat44-ed-sessions
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
  // wave-A: F-vlan-qinq
  'vlan-qinq',
  // wave-A: F-bonding
  // wave-A: F-bridge-l2
  // wave-A: F-loopback-bvi-gso-lldp-span
  // wave-A: F-vrf-static-ecmp
  // wave-A: F-neighbors-ra
  // wave-A: F-rpf-adl-pbr
  // wave-A: F-object-model
  // wave-A: F-acl
  // wave-A: F-host-acl-nftables
  // wave-A: F-nat44-ed-sessions
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
  // wave-A: F-vlan-qinq
  'vlan-qinq': enVlanQinq,
  // wave-A: F-bonding
  // wave-A: F-bridge-l2
  // wave-A: F-loopback-bvi-gso-lldp-span
  // wave-A: F-vrf-static-ecmp
  // wave-A: F-neighbors-ra
  // wave-A: F-rpf-adl-pbr
  // wave-A: F-object-model
  // wave-A: F-acl
  // wave-A: F-host-acl-nftables
  // wave-A: F-nat44-ed-sessions
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
  // wave-A: F-vlan-qinq
  'vlan-qinq': faVlanQinq,
  // wave-A: F-bonding
  // wave-A: F-bridge-l2
  // wave-A: F-loopback-bvi-gso-lldp-span
  // wave-A: F-vrf-static-ecmp
  // wave-A: F-neighbors-ra
  // wave-A: F-rpf-adl-pbr
  // wave-A: F-object-model
  // wave-A: F-acl
  // wave-A: F-host-acl-nftables
  // wave-A: F-nat44-ed-sessions
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
