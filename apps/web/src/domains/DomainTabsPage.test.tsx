import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { createMemoryRouter, RouterProvider, useLocation } from 'react-router';
import { describe, expect, it } from 'vitest';
import '../i18n';
import { buildRoutes } from '../router';
import { DomainTabsPage, type DomainTab } from './DomainTabsPage';

/** W-seed shells (vpn, services): inert until a feature registers a tab, then a tab per feature. */
function page(tabs: readonly DomainTab[], entry = '/vpn') {
  function Where() {
    return <output data-testid="search">{useLocation().search}</output>;
  }
  const router = createMemoryRouter(
    [{ path: '/vpn', element: (<><DomainTabsPage domainKey="vpn" ns="vpn" tabs={tabs} /><Where /></>) }],
    { initialEntries: [entry] },
  );
  return <RouterProvider router={router} />;
}

const tabs: DomainTab[] = [
  { id: 'ipsec', labelKey: 'nav:domains.vpn', Component: () => <p>ipsec screen</p> },
  { id: 'tunnels', labelKey: 'nav:domains.tunnels', Component: () => <p>tunnels screen</p> },
];

describe('domain tab shells (W-seed)', () => {
  it('renders exactly the domain placeholder while no tab is registered', async () => {
    render(page([]));
    expect(await screen.findByRole('heading', { level: 2, name: 'IPsec / WireGuard' })).toBeInTheDocument();
    expect(screen.getByText('Not yet available')).toBeInTheDocument();
    expect(screen.queryByRole('tablist')).toBeNull();
  });

  it('renders one tab per registered feature and keeps the selection in ?tab=', async () => {
    render(page(tabs));
    expect(await screen.findByRole('heading', { level: 2, name: 'VPN' })).toBeInTheDocument();
    expect(screen.getByRole('tablist', { name: 'VPN sections' })).toBeInTheDocument();
    expect(screen.getByText('ipsec screen')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('tab', { name: 'Tunnels' }));
    expect(await screen.findByText('tunnels screen')).toBeInTheDocument();
    expect(screen.queryByText('ipsec screen')).toBeNull();
    expect(screen.getByTestId('search')).toHaveTextContent('?tab=tunnels');
  });

  it('opens the tab named in ?tab= and falls back to the first for an unknown id', async () => {
    const { unmount } = render(page(tabs, '/vpn?tab=tunnels'));
    expect(await screen.findByText('tunnels screen')).toBeInTheDocument();
    unmount();
    render(page(tabs, '/vpn?tab=nope'));
    expect(await screen.findByText('ipsec screen')).toBeInTheDocument();
  });

  it('registers the vpn and services routes once each (the shell replaces the placeholder route)', () => {
    const children = buildRoutes({ devRoutes: false })[1]!.children ?? [];
    const paths = children.map((r) => r.path);
    expect(paths.filter((p) => p === 'vpn')).toHaveLength(1);
    expect(paths.filter((p) => p === 'services')).toHaveLength(1);
  });
});
