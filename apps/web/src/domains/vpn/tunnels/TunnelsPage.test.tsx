import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { buildNav } from '../../../nav/nav';
import { createTestRouter } from '../../../router';
import { domains } from '../../../schema/registry';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import { tunnelStatus, vppName } from './model';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';
const LONG = { timeout: 60_000 };
const WAIT = { timeout: 15_000 };

function app(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
  return <App router={createTestRouter([path], { devRoutes: false })} streamUrl={STREAM} queryClient={queryClient} />;
}

const gre = { enabled: true, instance: 7001, src: '10.7.1.1', dst: '10.7.1.2', underlayVrf: 'default', vrf: 'default', ipv4: [], ipv6: [], type: 'l3' };
const vx = { enabled: true, instance: 7003, src: '10.7.1.1', dst: '10.7.1.4', vni: 100, underlayVrf: 'default', vrf: 'default', ipv4: [], ipv6: [], srcPort: 4789, dstPort: 4789, decap: 'l2' };

function withTunnels(api: FakeApi) {
  api.on('GET /api/v1/config/candidate/tunnels', { body: { gre: { 'to-dc': gre, 'to-br': { ...gre, instance: 7002, dst: '10.7.1.3' } }, vxlan: { 'vx-100': vx }, ipip: {} } });
  api.on('GET /api/v1/config/tunnels', { body: { gre: { 'to-dc': gre }, vxlan: { 'vx-100': vx }, ipip: {} } });
  api.on('GET /api/v1/state/interfaces', {
    body: {
      items: [
        { name: 'gre7001', kind: 'interface', parent: null, state: { name: 'gre7001', vppName: 'gre7001', adminUp: true, linkUp: true, ipv4: [], ipv6: [] }, config: null, running: null, counters: null, hasPendingChange: false },
      ],
    },
  });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('tunnels model', () => {
  it('names the engine interface from the instance and finds its link state', () => {
    expect(vppName('vxlan', { instance: 7 })).toBe('vxlan_tunnel7');
    expect(vppName('gre', {})).toBeUndefined();
    const items = [{ name: 'gre1', state: { adminUp: true, linkUp: false } }] as never;
    expect(tunnelStatus(items, 'gre1')).toBe('down');
    expect(tunnelStatus(items, 'gre2')).toBeUndefined();
  });
});

describe('Tunnels page', () => {
  it('is an available nav entry under VPN', () => {
    const vpn = buildNav(domains).find((g) => g.id === 'vpn');
    const item = vpn?.items.find((i) => i.id === 'tunnels');
    expect(item?.path).toBe('/vpn/tunnels');
    expect(item?.available).toBe(true);
  });

  it('lists the tunnels of each kind with their engine interface, pending state and live status', LONG, async () => {
    const api = installFakeApi();
    withTunnels(api);
    await signIn();
    render(app('/vpn/tunnels'));
    expect(await screen.findByRole('heading', { name: 'Tunnels' }, WAIT)).toBeInTheDocument();
    const grid = await screen.findByRole('grid', { name: 'GRE' }, WAIT);
    expect(await within(grid).findByRole('button', { name: 'Open to-dc' }, WAIT)).toBeInTheDocument();
    expect(within(grid).getByText('gre7001')).toBeInTheDocument();
    expect(within(grid).getByText('new')).toBeInTheDocument(); // to-br is not in running
    expect(within(grid).getByText('Up')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('tab', { name: 'VXLAN' }));
    const vgrid = await screen.findByRole('grid', { name: 'VXLAN' }, WAIT);
    expect(await within(vgrid).findByText('vxlan_tunnel7003', {}, WAIT)).toBeInTheDocument();
    expect(within(vgrid).getByText('100')).toBeInTheDocument();
  });

  it('opens the schema form of a tunnel in the drawer', LONG, async () => {
    const api = installFakeApi();
    withTunnels(api);
    await signIn();
    render(app('/vpn/tunnels'));
    const grid = await screen.findByRole('grid', { name: 'GRE' }, WAIT);
    fireEvent.click(await within(grid).findByRole('button', { name: 'Open to-dc' }, WAIT));
    const drawer = await screen.findByRole('region', { name: /GRE tunnel to-dc$/ }, WAIT);
    expect(within(drawer).getByLabelText(/Source address/)).toHaveValue('10.7.1.1');
  });
});
