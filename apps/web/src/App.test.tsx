import { ROOT_KEYS } from '@ngfw/schema';
import { QueryClient } from '@tanstack/react-query';
import { act, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { DEV_ROUTES } from './build-flags';
import i18n from './i18n';
import { App } from './App';
import { buildNav, isCollapsible } from './nav/nav';
import { buildRoutes, createTestRouter, type RouteOptions } from './router';
import { domains } from './schema/registry';
import { installFakeApi, resetSession, signIn } from './test-api';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

/** Signed in through the scripted fake API; queries stay disabled (the health card then shows its loading state). */
function app(path: string, options?: RouteOptions) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
  return <App router={createTestRouter([path], options)} streamUrl={STREAM} queryClient={queryClient} />;
}

describe('App frame', () => {
  beforeEach(async () => {
    installFakeApi();
    await signIn();
  });
  afterEach(async () => {
    await resetSession();
    localStorage.clear();
    await i18n.changeLanguage('en');
  });

  it('renders the shell with navigation groups built from the schema root keys', async () => {
    render(app('/'));
    expect(await screen.findByRole('heading', { level: 2, name: 'Dashboard' })).toBeInTheDocument();
    const nav = screen.getByRole('navigation', { name: 'Main navigation' });
    // derived from the nav model, so new wave-A entries do not break this: groups with one entry are plain links, the others
    // start collapsed under a header button and open on a click
    const model = buildNav(domains, { devRoutes: DEV_ROUTES });
    const collapsible = model.filter(isCollapsible);
    for (const group of model.filter((g) => !isCollapsible(g))) {
      const item = group.items[0]!;
      expect(within(nav).getByRole('link', { name: new RegExp(`^${i18n.t(item.labelKey, { defaultValue: item.fallbackLabel })}`) })).toBeVisible();
    }
    const headers = [...nav.querySelectorAll('[aria-expanded]')];
    expect(headers.map((h) => h.textContent)).toEqual(collapsible.map((g) => i18n.t(g.labelKey)));
    const user = userEvent.setup();
    for (const header of headers) {
      expect(header).toHaveAttribute('aria-expanded', 'false');
      await user.click(header);
      expect(header).toHaveAttribute('aria-expanded', 'true');
    }
    for (const key of ROOT_KEYS) {
      expect(await within(nav).findByRole('link', { name: new RegExp(`^${i18n.t(`nav:domains.${key}`)}`) })).toBeVisible();
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

  it('opens the group of a deep-linked page, marks its entry current, and lets the user close it (D-117)', async () => {
    render(app('/vpn/tunnels'));
    await screen.findByRole('heading', { level: 2, name: 'Tunnels' });
    const nav = screen.getByRole('navigation', { name: 'Main navigation' });
    const vpn = within(nav).getByRole('button', { name: 'VPN' });
    expect(vpn).toHaveAttribute('aria-expanded', 'true');
    const tunnels = within(nav).getByRole('link', { name: /^Tunnels/ });
    expect(tunnels).toBeVisible();
    expect(tunnels).toHaveAttribute('aria-current', 'page');
    expect(within(nav).getByRole('button', { name: 'System' })).toHaveAttribute('aria-expanded', 'false');
    expect(vpn).not.toHaveAttribute('aria-current');
    await userEvent.setup().click(vpn);
    expect(vpn).toHaveAttribute('aria-expanded', 'false');
    // the current link is hidden now: the header tells assistive technology this section holds the current page
    expect(vpn).toHaveAttribute('aria-current', 'true');
  });

  it('opens the group of a page reached by in-app navigation, and reopens it after the user closed it (D-117)', async () => {
    const router = createTestRouter(['/']);
    const queryClient = new QueryClient({ defaultOptions: { queries: { enabled: false, retry: false } } });
    render(<App router={router} streamUrl={STREAM} queryClient={queryClient} />);
    await screen.findByRole('heading', { level: 2, name: 'Dashboard' });
    const nav = screen.getByRole('navigation', { name: 'Main navigation' });
    const vpn = within(nav).getByRole('button', { name: 'VPN' });
    expect(vpn).toHaveAttribute('aria-expanded', 'false');
    await act(() => router.navigate('/vpn/tunnels'));
    await screen.findByRole('heading', { level: 2, name: 'Tunnels' });
    expect(vpn).toHaveAttribute('aria-expanded', 'true');
    expect(await within(nav).findByRole('link', { name: /^Tunnels/ })).toBeVisible();
    await userEvent.setup().click(vpn);
    expect(vpn).toHaveAttribute('aria-expanded', 'false');
    // same path, other query: not a navigation to a new page, the user's choice stands
    await act(() => router.navigate('/vpn/tunnels?tab=x'));
    expect(vpn).toHaveAttribute('aria-expanded', 'false');
    // another page of the group opens it again
    await act(() => router.navigate('/vpn'));
    expect(vpn).toHaveAttribute('aria-expanded', 'true');
  });

  it('phone drawer: unique ids per list and the same expanded groups as the side drawer', async () => {
    render(app('/'));
    await screen.findByRole('heading', { level: 2, name: 'Dashboard' });
    const user = userEvent.setup();
    const [side] = screen.getAllByRole('navigation', { name: 'Main navigation' });
    await user.click(within(side!).getByRole('button', { name: 'System' }));
    await user.click(screen.getByRole('button', { name: 'Open navigation' }));
    const navs = screen.getAllByRole('navigation', { name: 'Main navigation', hidden: true });
    expect(navs).toHaveLength(2);
    const controls = [...document.querySelectorAll('[aria-controls]')].map((e) => e.getAttribute('aria-controls')!);
    expect(new Set(controls).size).toBe(controls.length);
    for (const id of controls) expect(document.querySelectorAll(`[id="${id}"]`)).toHaveLength(1);
    for (const n of navs) expect(within(n).getByRole('button', { name: 'System', hidden: true })).toHaveAttribute('aria-expanded', 'true');
  });

  it('keeps groups collapsed until their header is clicked, and toggles them closed again', async () => {
    render(app('/'));
    await screen.findByRole('heading', { level: 2, name: 'Dashboard' });
    const nav = screen.getByRole('navigation', { name: 'Main navigation' });
    expect(within(nav).queryByRole('link', { name: /^Users/ })).toBeNull();
    const user = userEvent.setup();
    const system = within(nav).getByRole('button', { name: 'System' });
    await user.click(system);
    expect(await within(nav).findByRole('link', { name: /^Users/ })).toBeVisible();
    await user.click(system);
    expect(system).toHaveAttribute('aria-expanded', 'false');
  });

  it('placeholder titles follow a language switch without navigating (review L3)', async () => {
    render(app('/tools'));
    expect(await screen.findByRole('heading', { level: 2, name: 'Tools' })).toBeInTheDocument();
    await act(async () => {
      await i18n.changeLanguage('fa');
    });
    expect(await screen.findByRole('heading', { level: 2, name: i18n.t('nav:tools', { lng: 'fa' }) })).toBeInTheDocument();
  });

  it('renders a not-found page for unknown paths', async () => {
    render(app('/no/such/page'));
    expect(await screen.findByRole('heading', { level: 2, name: 'Page not found' })).toBeInTheDocument();
  });
});
