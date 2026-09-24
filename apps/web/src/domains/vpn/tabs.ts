import type { DomainTab } from '../DomainTabsPage';

/**
 * Tabs of the VPN page, in display order. A feature adds one entry under its anchor, e.g.
 * `{ id: 'wireguard', labelKey: 'wireguard:tab', Component: lazy(() => import('…')) }`, and adds 'vpn' to BUILT_DOMAINS
 * (nav/nav.ts) in the same change. Empty: the page renders the domain placeholder (W-seed shell).
 */
export const vpnTabs: readonly DomainTab[] = [
  // wave-A: P11
  // wave-A: F-wireguard
];
