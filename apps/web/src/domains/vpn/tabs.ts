import { lazy } from 'react';
import type { DomainTab } from '../DomainTabsPage';

/**
 * Tabs of the VPN page, in display order. A feature adds one entry under its anchor, e.g.
 * `{ id: 'wireguard', labelKey: 'wireguard:tab', Component: lazy(() => import('…')) }`, and adds 'vpn' to BUILT_DOMAINS
 * (nav/nav.ts) in the same change. Empty: the page renders the domain placeholder (W-seed shell).
 */
export const vpnTabs: readonly DomainTab[] = [
  // wave-BC: F-ikev2-native
  // wave-BC: F-srv6
  {
    id: 'srv6',
    labelKey: 'srv6:tab',
    Component: lazy(() => import('./srv6/Srv6Page').then((m) => ({ default: m.Srv6Page }))),
  },
  // wave-BC: F-lisp
  { id: 'lisp', labelKey: 'lisp:tab', Component: lazy(() => import('./lisp/LispTab')) },
  // wave-BC: F-ra-vpn
  // wave-A: P11
  // wave-A: F-wireguard
  {
    id: 'wireguard',
    labelKey: 'wireguard:tab',
    Component: lazy(() =>
      import('./wireguard/WireguardPage').then((m) => ({ default: m.WireguardPage })),
    ),
  },
  // wave-BC: F-pki
  {
    id: 'pki',
    labelKey: 'pkiInventory:title',
    Component: lazy(() =>
      import('./pki/PkiInventoryPanel').then((m) => ({ default: m.PkiInventoryPanel })),
    ),
  },
];
