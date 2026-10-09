import { DomainTabsPage } from '../DomainTabsPage';
import { bgpTabs } from './bgp/BgpPage';
import { NS } from './bgp/model';

const ROUTING = 'routing' as const;
const TITLE_KEY = 'routingObjectsTitle';
const TABS_LABEL_KEY = 'routingObjectsTabs';

// Prefix lists and route maps belong to routing.policy and are shared by routing protocols.
const tabs = bgpTabs.filter((tab) => tab.id === 'prefix-lists' || tab.id === 'route-maps');

export function RoutingObjectsPage() {
  return (
    <DomainTabsPage
      domainKey={ROUTING}
      ns={NS}
      tabs={tabs}
      titleKey={TITLE_KEY}
      tabsLabelKey={TABS_LABEL_KEY}
    />
  );
}
