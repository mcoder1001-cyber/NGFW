import type { RootKey } from '../../schema/registry';
import { DomainTabsPage } from '../DomainTabsPage';
import { securityTabs } from './tabs';

/** Domain key and i18n namespace of the page (`security:title`, `security:tabs`, `security:loading`). */
const SECURITY: RootKey = 'security';

/**
 * Firewall › Security (F-security-page): the box's own defensive policy. One tab edits the committed
 * `security.autoBlock` policy, the other shows the sources blocked right now (`./tabs.ts`).
 */
export function SecurityPage() {
  return <DomainTabsPage domainKey={SECURITY} ns={SECURITY} tabs={securityTabs} />;
}
