import type { RootKey } from '../../../schema/registry';
import { DomainTabsPage } from '../../DomainTabsPage';
import { NAT_NS, natTabs } from './tabs';

const NAT: RootKey = 'nat';

/**
 * The NAT screen (`/firewall/nat`): one tab per registered section (`./tabs.ts`); NAT44-ED here, the sibling NAT
 * features append theirs. Configuration edits go to the candidate through the generic `/config/nat` route; live state
 * comes from `/state/nat/**` (agent NatSummary / NatSessions).
 */
export function NatPage() {
  return <DomainTabsPage domainKey={NAT} ns={NAT_NS} tabs={natTabs} />;
}
