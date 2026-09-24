import { useTranslation } from 'react-i18next';
import { createBrowserRouter, createMemoryRouter, type RouteObject } from 'react-router';
import { DEV_ROUTES } from './build-flags';
import { domainPath } from './nav/nav';
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
        ...domains.map((d) => ({
          path: domainPath(d.key).slice(1),
          lazy: async () => {
            const { DomainPlaceholderPage } = await import('./pages/DomainPlaceholderPage');
            return { element: <DomainPlaceholderPage domainKey={d.key} /> };
          },
        })),
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
