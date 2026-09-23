import { ROOT_KEYS } from '@ngfw/schema';
import { QueryClient } from '@tanstack/react-query';
import { act, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import i18n from './i18n';
import { App } from './App';
import { buildRoutes, createTestRouter, type RouteOptions } from './router';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

/** No network in unit tests: queries stay disabled (the health card then shows its loading state). */
function app(path: string, options?: RouteOptions) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  return <App router={createTestRouter([path], options)} streamUrl={STREAM} queryClient={queryClient} />;
}

describe('App frame', () => {
  afterEach(async () => {
    localStorage.clear();
    await i18n.changeLanguage('en');
  });

  it('renders the shell with navigation groups built from the schema root keys', async () => {
    render(app('/'));
    expect(await screen.findByRole('heading', { level: 2, name: 'Dashboard' })).toBeInTheDocument();
    const nav = screen.getByRole('navigation', { name: 'Main navigation' });
    for (const key of ROOT_KEYS) {
      expect(within(nav).getByRole('link', { name: new RegExp(`^${i18n.t(`nav:domains.${key}`)}`) })).toBeInTheDocument();
    }
    for (const group of ['Interfaces', 'Routing', 'Firewall / NAT', 'VPN', 'Services', 'System', 'Tools', 'Developer']) {
      expect(within(nav).getByText(group, { selector: '.MuiListSubheader-root' })).toBeInTheDocument();
    }
    expect(screen.getByText('Dashboard widgets (throughput, CPU, sessions, alarms, tunnel health) are not yet available.')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Skip to main content' })).toHaveAttribute('href', '#main');
  });

  it('shows "not yet available" for unbuilt domain screens, with the schema title and no data', async () => {
    render(app('/routing/vrfs'));
    expect(await screen.findByRole('heading', { level: 2, name: 'VRFs' })).toBeInTheDocument();
    expect(screen.getByText('Not yet available')).toBeInTheDocument();
    expect(screen.getByText('Configuration domain: VRFs')).toBeInTheDocument();
    expect(screen.queryByRole('grid')).toBeNull();
  });

  it('switches to Persian: RTL direction, lang attribute and translated labels', async () => {
    render(app('/'));
    await screen.findByRole('heading', { level: 2, name: 'Dashboard' });
    await userEvent.click(screen.getByRole('button', { name: 'Settings' }));
    await userEvent.click(await screen.findByLabelText('Language'));
    await userEvent.click(await screen.findByRole('option', { name: 'فارسی' }));
    await userEvent.keyboard('{Escape}'); // close the settings popover (a modal hides the page from role queries)
    expect(await screen.findByRole('heading', { level: 2, name: 'داشبورد' })).toBeInTheDocument();
    expect(document.documentElement).toHaveAttribute('dir', 'rtl');
    expect(document.documentElement).toHaveAttribute('lang', 'fa');
  });

  it('code-splits the developer routes and renders the SchemaForm demo', async () => {
    render(app('/dev/schema-form'));
    expect(await screen.findByRole('button', { name: 'Save' })).toBeInTheDocument();
    expect(screen.getByLabelText('MTU', { exact: false })).toHaveValue(1500);
  });

  it('has no /dev routes and no Developer nav group when dev routes are off (production builds, review M1)', async () => {
    const paths = (routes: ReturnType<typeof buildRoutes>): string[] => routes.flatMap((r) => [r.path ?? '', ...paths(r.children ?? [])]);
    expect(paths(buildRoutes({ devRoutes: false })).filter((p) => p.startsWith('dev'))).toEqual([]);
    expect(paths(buildRoutes({ devRoutes: true })).filter((p) => p.startsWith('dev'))).toEqual(['dev/schema-form', 'dev/data-grid', 'dev/stream']);
    render(app('/dev/schema-form', { devRoutes: false }));
    expect(await screen.findByRole('heading', { level: 2, name: 'Page not found' })).toBeInTheDocument();
    const nav = screen.getByRole('navigation', { name: 'Main navigation' });
    expect(within(nav).queryByText('Developer')).toBeNull();
    expect(within(nav).queryByRole('link', { name: /demo/i })).toBeNull();
  });

  it('marks exactly one navigation item as the current page on nested domain paths (review L4)', async () => {
    render(app('/vpn/tunnels'));
    await screen.findByRole('heading', { level: 2, name: 'Tunnels' });
    const nav = screen.getByRole('navigation', { name: 'Main navigation' });
    const current = nav.querySelectorAll('[aria-current="page"]');
    expect(current).toHaveLength(1);
    expect(current[0]).toHaveTextContent('Tunnels');
    expect(nav.querySelectorAll('.Mui-selected')).toHaveLength(1);
  });

  it('placeholder titles follow a language switch without navigating (review L3)', async () => {
    render(app('/system/users'));
    expect(await screen.findByRole('heading', { level: 2, name: 'Users' })).toBeInTheDocument();
    await act(async () => {
      await i18n.changeLanguage('fa');
    });
    expect(await screen.findByRole('heading', { level: 2, name: i18n.t('nav:users', { lng: 'fa' }) })).toBeInTheDocument();
  });

  it('renders a not-found page for unknown paths', async () => {
    render(app('/no/such/page'));
    expect(await screen.findByRole('heading', { level: 2, name: 'Page not found' })).toBeInTheDocument();
  });
});
