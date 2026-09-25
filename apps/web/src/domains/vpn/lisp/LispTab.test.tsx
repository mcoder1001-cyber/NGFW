import { QueryClient } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';

/** F-lisp tab in jsdom against a scripted stand-in of the API (unit level; the screenshots ran against the real API). */
function app(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
  return <App router={createTestRouter([path], { devRoutes: false })} streamUrl="ws://127.0.0.1:1/api/v1/stream" queryClient={queryClient} />;
}

const lisp = {
  enabled: true,
  gpe: true,
  locatorSets: { 'w11-rloc': { locators: [{ interface: 'host-w11-eth0', priority: 1, weight: 1 }] } },
  localEids: [{ vni: 1100, eid: '10.11.100.0/24', locatorSet: 'w11-rloc' }],
  eidTables: { '1100': { vrf: 'overlay' } },
  remoteMappings: [],
  adjacencies: [],
  gpeEntries: [],
  mapResolvers: ['10.11.1.254'],
  mapServers: [],
};

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('VPN page, LISP tab', () => {
  it('shows the sub-tabs, the status column and saves a section as a merge patch of tunnels', async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/config/candidate/tunnels', { body: { gre: {}, vxlan: {}, ipip: {}, lisp } });
    api.on('GET /api/v1/state/lisp', {
      body: {
        enabled: true, gpeEnabled: true, pitr: '', retrievedAt: null, adjacencies: [], mapServers: [], gpeVnis: [],
        locatorSets: [{ name: 'w11-rloc', locators: [{ interface: 'host-w11-eth0', swIfIndex: 1, priority: 1, weight: 1 }] }],
        mappings: [], eidTables: [{ vni: 1100, dpTable: 1100, isL2: false }], mapResolvers: ['10.11.1.254'],
      },
    });
    api.on('PATCH /api/v1/config/tunnels', (_req, body) => ({ body: { gre: {}, vxlan: {}, ipip: {}, lisp: { ...lisp, ...(body as { lisp: object }).lisp } } }));
    await signIn();
    render(app('/vpn?tab=lisp'));
    const tabs = await screen.findByRole('tablist', { name: 'LISP sections' }, { timeout: 15_000 });
    expect(within(tabs).getAllByRole('tab').map((t) => t.textContent)).toEqual(['Locators', 'EIDs', 'Mappings', 'Resolvers']);
    const table = await screen.findByRole('table', { name: 'Configured objects and data-plane status' });
    await waitFor(() => expect(within(table).getByText('w11-rloc')).toBeInTheDocument());
    expect(within(table).getAllByText('In VPP').length).toBe(3);

    await userEvent.click(within(tabs).getByRole('tab', { name: 'EIDs' }));
    await waitFor(() => expect(within(screen.getByRole('table', { name: 'Configured objects and data-plane status' })).getByText('1100/10.11.100.0/24')).toBeInTheDocument());
    expect(screen.getByText('Not in VPP')).toBeInTheDocument(); // the local EID is not in the (scripted) state

    await userEvent.click(within(tabs).getByRole('tab', { name: 'Resolvers' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Save to candidate' }));
    await waitFor(() => expect(api.calls.some((c) => c.method === 'PATCH' && c.path === '/api/v1/config/tunnels')).toBe(true));
    const patch = api.calls.find((c) => c.method === 'PATCH')!.body as { lisp: Record<string, unknown> };
    expect(Object.keys(patch.lisp).sort()).toEqual(['mapResolvers', 'mapServers', 'pitr']);
    expect(patch.lisp['mapResolvers']).toEqual(['10.11.1.254']);
  });
});
