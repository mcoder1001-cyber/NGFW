import { lazy } from 'react';
import type { DomainTab } from '../DomainTabsPage';

/**
 * Tabs of the Security page (F-security-page), in display order. A later security feature (IPS signatures,
 * honeypots) adds one entry under its own anchor.
 */
export const securityTabs: readonly DomainTab[] = [
  // F-security-page: the committed policy, `security.autoBlock` (thresholds, allow-list, cap)
  { id: 'policy', labelKey: 'security:tab.policy', Component: lazy(() => import('./policy/PolicyTab')) },
  // F-bruteforce-block: the live blocked set (the page also still answers at /firewall/auto-block, which redirects here)
  {
    id: 'blocked',
    labelKey: 'security:tab.blocked',
    Component: lazy(async () => ({ default: (await import('./auto-block/AutoBlockPage')).AutoBlockPage })),
  },
];
