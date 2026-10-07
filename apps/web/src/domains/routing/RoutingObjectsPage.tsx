import { DomainTabsPage } from '../DomainTabsPage';
import { bgpTabs } from './bgp/BgpPage';

// Prefix lists and route maps belong to routing.policy and are shared by routing protocols.
const tabs = bgpTabs.filter((tab) => tab.id === 'prefix-lists' || tab.id === 'route-maps');

export function RoutingObjectsPage() {
  return (
    <DomainTabsPage
      domainKey="routing"
      ns="bgp"
      tabs={tabs}
      titleKey="routingObjectsTitle"
      tabsLabelKey="routingObjectsTabs"
    />
  );
}
