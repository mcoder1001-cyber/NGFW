import { lazy } from 'react';
import type { DomainTab } from '../../DomainTabsPage';
import { DomainTabsPage } from '../../DomainTabsPage';
import type { RootKey } from '../../../schema/registry';
import { NS } from './model';

/** Tabs of the BGP screen, in display order. */
export const bgpTabs: readonly DomainTab[] = [
  {
    id: 'neighbors',
    labelKey: 'bgp:tab.neighbors',
    Component: lazy(async () => ({ default: (await import('./NeighborsTab')).NeighborsTab })),
  },
  {
    id: 'prefix-lists',
    labelKey: 'bgp:tab.prefixLists',
    Component: lazy(async () => ({ default: (await import('./PolicyTab')).PrefixListsTab })),
  },
  {
    id: 'route-maps',
    labelKey: 'bgp:tab.routeMaps',
    Component: lazy(async () => ({ default: (await import('./PolicyTab')).RouteMapsTab })),
  },
  {
    id: 'pairs',
    labelKey: 'bgp:tab.pairs',
    Component: lazy(async () => ({ default: (await import('./PairsTab')).PairsTab })),
  },
];

const ROUTING: RootKey = 'routing';

/** `/routing/bgp`: BGP (global, neighbours with live state, peer groups), prefix lists, route maps, linux-cp pairs. */
export function BgpPage() {
  return <DomainTabsPage domainKey={ROUTING} ns={NS} tabs={bgpTabs} />;
}
