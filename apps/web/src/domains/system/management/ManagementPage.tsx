import type { RootKey } from '../../../schema/registry';
import { DomainTabsPage } from '../../DomainTabsPage';
import { managementTabs } from './tabs';

/** Domain key and i18n namespace of the page (`management:title`, `management:tabs`, `management:loading`). */
const MANAGEMENT: RootKey = 'management';

/** System › Management (F-management-ui): local users, AAA, API TLS and remote syslog, one tab each (`./tabs.ts`). */
export function ManagementPage() {
  return <DomainTabsPage domainKey={MANAGEMENT} ns={MANAGEMENT} tabs={managementTabs} />;
}
