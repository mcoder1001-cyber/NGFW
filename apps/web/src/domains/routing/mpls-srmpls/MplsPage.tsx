import type { RootKey } from '../../../schema/registry';
import { DomainTabsPage } from '../../DomainTabsPage';
import { NS } from './model';
import { mplsTabs } from './tabs';

/** The page lives in the routing domain; titles come from the feature namespace (`mpls-srmpls:title`, `tabs`, `loading`). */
const ROUTING: RootKey = 'routing';

/** `/routing/mpls`: MPLS interfaces and tables, label routes and bindings, tunnels, SR-MPLS and the live MPLS FIB. */
export function MplsPage() {
  return <DomainTabsPage domainKey={ROUTING} ns={NS} tabs={mplsTabs} />;
}
