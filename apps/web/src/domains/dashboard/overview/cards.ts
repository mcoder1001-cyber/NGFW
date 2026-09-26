import type { ComponentType } from 'react';

/** A card a feature adds to the dashboard's bottom row (it fetches its own data and renders "not available" itself). */
export interface DashboardCard {
  id: string;
  Component: ComponentType;
}

/**
 * Feature cards of the dashboard, in display order (WEB-dashboard). A feature adds one entry under its anchor, e.g.
 * `{ id: 'alarms', Component: lazy(() => import('…')) }` — the page itself stays owned by WEB-dashboard.
 */
export const dashboardCards: readonly DashboardCard[] = [
  // wave-BC: F-dashboard-prom-alarms (active alarms card)
  // wave-BC: F-tunnels (tunnel health card)
  // wave-A: P11 (IPsec SA card)
  // wave-BC: F-vrrp-config-sync (HA / cluster card)
];
