import type { RootKey } from '../../schema/registry';
import { DomainTabsPage } from '../DomainTabsPage';
import { vpnTabs } from './tabs';

/** Domain key and i18n namespace of the page (`vpn:title`, `vpn:tabs`, `vpn:loading`). */
const VPN: RootKey = 'vpn';

/** The VPN domain screen: one tab per feature (P11 IPsec, F-wireguard), registered in `./tabs.ts`. */
export function VpnPage() {
  return <DomainTabsPage domainKey={VPN} ns={VPN} tabs={vpnTabs} />;
}
