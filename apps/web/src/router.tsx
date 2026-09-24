import { useTranslation } from 'react-i18next';
import { createBrowserRouter, createMemoryRouter, type RouteObject } from 'react-router';
import { DEV_ROUTES } from './build-flags';
import { BUILT_DOMAINS, domainPath } from './nav/nav';
import { domains } from './schema/registry';
import { RequireAuth } from './auth/RequireAuth';
import { LoginPage } from './pages/LoginPage';
import { AppShell } from './shell/AppShell';
import { NotAvailablePage } from './shell/NotAvailablePage';
import { NotFoundPage } from './shell/NotFoundPage';
import { RouteErrorPage } from './shell/RouteErrorPage';

/** Subscribes to the language (review L3): the title follows a language switch without navigating away. */
function NotAvailableByKey({ labelKey }: { labelKey: string }) {
  const { t } = useTranslation();
  return <NotAvailablePage title={t(labelKey)} />;
}

/**
 * Developer demos (code-split). `DEV_ROUTES` is a build-time literal: in a production build this list is empty and the
 * demo chunks are not emitted at all (scripts/check-no-dev-routes.mjs).
 */
const DEV_ROUTE_OBJECTS: RouteObject[] = DEV_ROUTES
  ? [
      { path: 'dev/schema-form', lazy: async () => ({ Component: (await import('./pages/dev/SchemaFormDemoPage')).SchemaFormDemoPage }) },
      { path: 'dev/data-grid', lazy: async () => ({ Component: (await import('./pages/dev/DataGridDemoPage')).DataGridDemoPage }) },
      { path: 'dev/stream', lazy: async () => ({ Component: (await import('./pages/dev/StreamDemoPage')).StreamDemoPage }) },
    ]
  : [];

export interface RouteOptions {
  /** Include the `/dev/*` demo routes. Defaults to the build flag (off in production builds). */
  devRoutes?: boolean;
}

/**
 * Domain screens shared by several features (W-seed shells): the page renders the domain placeholder until a feature registers
 * a tab in `domains/<key>/tabs.ts`, so these routes change nothing a user can see.
 */
const SHELL_ROUTES: RouteObject[] = [
  { path: domainPath('vpn').slice(1), lazy: async () => ({ Component: (await import('./domains/vpn/VpnPage')).VpnPage }) },
  { path: domainPath('services').slice(1), lazy: async () => ({ Component: (await import('./domains/services/ServicesPage')).ServicesPage }) },
];
const SHELL_DOMAINS: ReadonlySet<string> = new Set(['vpn', 'services']);

/** Route table: domain routes come from the schema's root keys; dev demos exist only in dev builds. */
export function buildRoutes({ devRoutes = DEV_ROUTES }: RouteOptions = {}): RouteObject[] {
  return [
    { path: '/login', element: <LoginPage />, errorElement: <RouteErrorPage /> },
    {
      path: '/',
      element: (
        <RequireAuth>
          <AppShell devRoutes={devRoutes} />
        </RequireAuth>
      ),
      errorElement: <RouteErrorPage />,
      children: [
        // Screens are code-split per route; feature screens (P08+) plug in the same way.
        { index: true, lazy: async () => ({ Component: (await import('./pages/DashboardPage')).DashboardPage }) },
        // P08: the first built domain screen; the others keep their placeholder until their feature task lands
        { path: domainPath('interfaces').slice(1), lazy: async () => ({ Component: (await import('./domains/interfaces/InterfacesPage')).InterfacesPage }) },
        ...SHELL_ROUTES,
        ...domains.filter((d) => !BUILT_DOMAINS.has(d.key) && !SHELL_DOMAINS.has(d.key)).map((d) => ({
          path: domainPath(d.key).slice(1),
          lazy: async () => {
            const { DomainPlaceholderPage } = await import('./pages/DomainPlaceholderPage');
            return { element: <DomainPlaceholderPage domainKey={d.key} /> };
          },
        })),
        // Feature screens: one lazy route line under the feature's anchor (wave-A-hotspots W1).
        // wave-A: F-bonding
        { path: 'interfaces/bonds', lazy: async () => ({ Component: (await import('./domains/interfaces/bonding/BondsPage')).BondsPage }) },
        // wave-A: F-bridge-l2
        // wave-A: F-loopback-bvi-gso-lldp-span
        // wave-A: F-vrf-static-ecmp
        // wave-A: F-neighbors-ra
        // wave-A: F-rpf-adl-pbr
        // wave-A: F-object-model
        // wave-A: F-acl
        // wave-A: F-host-acl-nftables
        // wave-A: F-nat44-ed-sessions
        // wave-A: P11
        // wave-A: F-wireguard
        // wave-A: P12
        // wave-A: F-kea-dhcp-relay
        // wave-A: F-unbound-chrony-syslog
        { path: 'system/users', lazy: async () => ({ Component: (await import('./pages/UsersPage')).UsersPage }) },
        { path: 'system/revisions', lazy: async () => ({ Component: (await import('./pages/RevisionsPage')).RevisionsPage }) },
        { path: 'tools', element: <NotAvailableByKey labelKey="nav:tools" /> },
        ...(devRoutes ? DEV_ROUTE_OBJECTS : []),
        { path: '*', element: <NotFoundPage /> },
      ],
    },
  ];
}

export function createAppRouter() {
  return createBrowserRouter(buildRoutes());
}

export function createTestRouter(initialEntries: string[], options?: RouteOptions) {
  return createMemoryRouter(buildRoutes(options), { initialEntries });
}
