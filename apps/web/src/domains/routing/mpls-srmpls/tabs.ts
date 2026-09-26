import { lazy } from 'react';
import type { DomainTab } from '../../DomainTabsPage';

/**
 * Tabs of the Routing → MPLS page, in display order — a plain array, so F-mpls-ldp adds its "LDP" tab with one entry
 * under its anchor (wave-BC-numbers.md, F-mpls-srmpls obligation).
 */
export const mplsTabs: readonly DomainTab[] = [
  {
    id: 'interfaces',
    labelKey: 'mpls-srmpls:tab.interfaces',
    Component: lazy(async () => ({ default: (await import('./InterfacesTab')).InterfacesTab })),
  },
  {
    id: 'routes',
    labelKey: 'mpls-srmpls:tab.routes',
    Component: lazy(async () => ({ default: (await import('./LabelRoutesTab')).LabelRoutesTab })),
  },
  {
    id: 'tunnels',
    labelKey: 'mpls-srmpls:tab.tunnels',
    Component: lazy(async () => ({ default: (await import('./TunnelsTab')).TunnelsTab })),
  },
  {
    id: 'sr',
    labelKey: 'mpls-srmpls:tab.sr',
    Component: lazy(async () => ({ default: (await import('./SrTab')).SrTab })),
  },
  {
    id: 'fib',
    labelKey: 'mpls-srmpls:tab.fib',
    Component: lazy(async () => ({ default: (await import('./FibTab')).FibTab })),
  },
  // wave-BC: F-mpls-ldp
];
