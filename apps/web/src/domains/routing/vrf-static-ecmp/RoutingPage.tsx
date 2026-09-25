import type { RootKey } from '../../../schema/registry';
import { DomainTabsPage } from '../../DomainTabsPage';
import { NS } from './model';
import { routingTabs } from './tabs';

/** Domain key of the page; titles come from the feature namespace (`vrf-static-ecmp:title`, `tabs`, `loading`). */
const ROUTING: RootKey = 'routing';

/** `/routing`: static routes (ECMP editor), the FIB browser and the ping tool. */
export function RoutingPage() {
  return <DomainTabsPage domainKey={ROUTING} ns={NS} tabs={routingTabs} />;
}
