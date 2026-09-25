import { lazy } from 'react';
import type { DomainTab } from '../DomainTabsPage';

/**
 * Tabs of the Services page, in display order. A feature adds one entry under its anchor, e.g.
 * `{ id: 'dhcp', labelKey: 'kea-dhcp-relay:tab', Component: lazy(() => import('…')) }`, and adds 'services' to
 * BUILT_DOMAINS (nav/nav.ts) in the same change. Empty: the page renders the domain placeholder (W-seed shell).
 */
export const servicesTabs: readonly DomainTab[] = [
  // wave-BC: F-host-stack
  { id: 'host-stack', labelKey: 'host-stack:tab', Component: lazy(() => import('./host-stack/HostStackTab').then((m) => ({ default: m.HostStackTab }))) },
  // wave-BC: F-lb
  // wave-BC: F-qos-flat
  // wave-BC: F-snmp
  // wave-BC: F-ipfix-sflow
  // wave-A: F-kea-dhcp-relay
  // wave-A: F-unbound-chrony-syslog
];
