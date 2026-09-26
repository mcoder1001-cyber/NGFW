import { lazy } from 'react';
import type { DomainTab } from '../../DomainTabsPage';

/**
 * Tabs of the Routing page, in display order. Kept in the feature's own folder this wave (F-vrf-static-ecmp owns only
 * `domains/routing/vrf-static-ecmp/**`); P12 (BGP/OSPF…) can move it to `domains/routing/tabs.ts` and add its tabs there.
 */
export const routingTabs: readonly DomainTab[] = [
  { id: 'static', labelKey: 'vrf-static-ecmp:tab.static', Component: lazy(async () => ({ default: (await import('./StaticRoutesTab')).StaticRoutesTab })) },
  { id: 'fib', labelKey: 'vrf-static-ecmp:tab.fib', Component: lazy(async () => ({ default: (await import('./FibTab')).FibTab })) },
  { id: 'ping', labelKey: 'vrf-static-ecmp:tab.ping', Component: lazy(async () => ({ default: (await import('./PingTab')).PingTab })) },
];
