import { lazy } from 'react';
import type { DomainTab } from '../../DomainTabsPage';

/** i18n namespace of the NAT page (locale namespace = task slug). */
export const NAT_NS = 'nat44-ed-sessions';

/**
 * Tabs of the NAT page, in display order (F-nat44-ed-sessions). The sibling NAT features append their tabs below their
 * own anchor — F-nat44-ei-64-66-nptv6 (NAT44-EI, NAT64, NAT66, NPTv6) and F-det44-map-dslite-cnat (DET44, DS-Lite, MAP,
 * CNAT) — one entry each, e.g. `{ id: 'nat64', labelKey: 'nat44-ei-64-66-nptv6:tab.nat64', Component: lazy(…) }`.
 */
export const natTabs: readonly DomainTab[] = [
  {
    id: 'outbound',
    labelKey: `${NAT_NS}:tab.outbound`,
    Component: lazy(async () => ({ default: (await import('./OutboundTab')).OutboundTab })),
  },
  {
    id: 'static',
    labelKey: `${NAT_NS}:tab.static`,
    Component: lazy(async () => ({ default: (await import('./MappingsTab')).MappingsTab })),
  },
  {
    id: 'pools',
    labelKey: `${NAT_NS}:tab.pools`,
    Component: lazy(async () => ({ default: (await import('./PoolsTab')).PoolsTab })),
  },
  {
    id: 'sessions',
    labelKey: `${NAT_NS}:tab.sessions`,
    Component: lazy(async () => ({ default: (await import('./SessionsTab')).SessionsTab })),
  },
  // wave-A: F-nat44-ei-64-66-nptv6
  // wave-BC: F-det44-map-dslite-cnat
];
