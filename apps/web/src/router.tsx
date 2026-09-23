import { createBrowserRouter, createMemoryRouter, type RouteObject } from 'react-router';
import { domainPath } from './nav/nav';
import { DashboardPage } from './pages/DashboardPage';
import { DomainPlaceholderPage } from './pages/DomainPlaceholderPage';
import { domains } from './schema/registry';
import { AppShell } from './shell/AppShell';
import { NotAvailablePage } from './shell/NotAvailablePage';
import { NotFoundPage } from './shell/NotFoundPage';
import { RouteErrorPage } from './shell/RouteErrorPage';
import i18n from './i18n';

function NotAvailableByKey({ labelKey }: { labelKey: string }) {
  return <NotAvailablePage title={i18n.t(labelKey)} />;
}

/** Route table: domain routes come from the schema's root keys; dev demos are code-split. */
export function buildRoutes(): RouteObject[] {
  return [
    {
      path: '/',
      element: <AppShell />,
      errorElement: <RouteErrorPage />,
      children: [
        { index: true, element: <DashboardPage /> },
        ...domains.map((d) => ({ path: domainPath(d.key).slice(1), element: <DomainPlaceholderPage domainKey={d.key} /> })),
        { path: 'system/users', element: <NotAvailableByKey labelKey="nav:users" /> },
        { path: 'system/revisions', element: <NotAvailableByKey labelKey="nav:revisions" /> },
        { path: 'tools', element: <NotAvailableByKey labelKey="nav:tools" /> },
        { path: 'dev/schema-form', lazy: async () => ({ Component: (await import('./pages/dev/SchemaFormDemoPage')).SchemaFormDemoPage }) },
        { path: 'dev/data-grid', lazy: async () => ({ Component: (await import('./pages/dev/DataGridDemoPage')).DataGridDemoPage }) },
        { path: 'dev/stream', lazy: async () => ({ Component: (await import('./pages/dev/StreamDemoPage')).StreamDemoPage }) },
        { path: '*', element: <NotFoundPage /> },
      ],
    },
  ];
}

export function createAppRouter() {
  return createBrowserRouter(buildRoutes());
}

export function createTestRouter(initialEntries: string[]) {
  return createMemoryRouter(buildRoutes(), { initialEntries });
}
