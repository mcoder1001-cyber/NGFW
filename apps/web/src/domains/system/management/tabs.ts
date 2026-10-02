import { lazy } from 'react';
import type { DomainTab } from '../../DomainTabsPage';

/**
 * Tabs of the Management page (F-management-ui, D-152), in display order. A feature adds one entry under its anchor,
 * e.g. F-aaa replaces the AAA entry with its own screen.
 */
export const managementTabs: readonly DomainTab[] = [
  {
    id: 'notifications',
    labelKey: 'management:tab.notifications',
    Component: lazy(() => import('./NotificationsTab')),
  },
  // P07b: local users (the page also still answers at /system/users, which redirects here)
  {
    id: 'users',
    labelKey: 'management:tab.users',
    Component: lazy(async () => ({
      default: (await import('../../../pages/UsersPage')).UsersPage,
    })),
  },
  // F-aaa: RADIUS / TACACS+ (not built yet: the tab says so)
  { id: 'aaa', labelKey: 'management:tab.aaa', Component: lazy(() => import('./AaaTab')) },
  // F-management-ui: API TLS certificate
  { id: 'tls', labelKey: 'management:tab.tls', Component: lazy(() => import('./TlsTab')) },
  // F-unbound-chrony-syslog: remote syslog targets (the same screen as Services › Logging)
  {
    id: 'syslog',
    labelKey: 'management:tab.syslog',
    Component: lazy(() => import('../../services/unbound-chrony-syslog/LoggingTab')),
  },
];
