import type { RootKey } from '../../schema/registry';
import { DomainTabsPage } from '../DomainTabsPage';
import { servicesTabs } from './tabs';

/** Domain key and i18n namespace of the page (`services:title`, `services:tabs`, `services:loading`). */
const SERVICES: RootKey = 'services';

/** The Services domain screen: one tab per feature (F-kea-dhcp-relay, F-unbound-chrony-syslog), registered in `./tabs.ts`. */
export function ServicesPage() {
  return <DomainTabsPage domainKey={SERVICES} ns={SERVICES} tabs={servicesTabs} />;
}
