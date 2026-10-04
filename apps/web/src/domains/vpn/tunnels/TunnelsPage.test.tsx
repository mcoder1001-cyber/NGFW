import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { buildNav } from '../../../nav/nav';
import { createTestRouter } from '../../../router';
import { domains } from '../../../schema/registry';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import { liveTunnel, liveStatus, tunnelStatus, vppName } from './model';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';
const LONG = { timeout: 60_000 };
const WAIT = { timeout: 15_000 };

function app(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  });
  return (
    <App
      router={createTestRouter([path], { devRoutes: false })}
      streamUrl={STREAM}
      queryClient={queryClient}
    />
  );
}

const gre = {
  enabled: true,
  instance: 7001,
  src: '10.7.1.1',
  dst: '10.7.1.2',
  underlayVrf: 'default',
  vrf: 'default',
  ipv4: [],
  ipv6: [],
  type: 'l3',
};
const vx = {
  enabled: true,
  instance: 7003,
  src: '10.7.1.1',
  dst: '10.7.1.4',
  vni: 100,
  underlayVrf: 'default',
  vrf: 'default',
  ipv4: [],
  ipv6: [],
  srcPort: 4789,
  dstPort: 4789,
  decap: 'l2',
};

function withTunnels(api: FakeApi) {
  api.on('GET /api/v1/config/candidate/tunnels', {
    body: {
      gre: { 'to-dc': gre, 'to-br': { ...gre, instance: 7002, dst: '10.7.1.3' } },
      vxlan: { 'vx-100': vx },
      ipip: {},
    },
  });
  api.on('GET /api/v1/config/tunnels', {
    body: { gre: { 'to-dc': gre }, vxlan: { 'vx-100': vx }, ipip: {} },
  });
  api.on('GET /api/v1/state/tunnels', {
    body: {
      retrievedAt: null,
      items: [
        {
          name: 'to-dc',
          kind: 'gre',
          interface: 'gre7001',
          adminUp: true,
          linkUp: true,
          src: '10.7.1.1',
          dst: '10.7.1.2',
          ipv4TableId: 0,
          ipv6TableId: 0,
          underlayTableId: 0,
          counters: null,
          notes: [],
        },
        {
          name: 'vx-100',
          kind: 'vxlan',
          interface: 'vxlan_tunnel7003',
          adminUp: true,
          linkUp: true,
          counters: null,
          notes: [],
        },
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
    expect(vppName('ipip', { instance: 7, sixrd: {} })).toBeUndefined();
    expect(vppName('vxlanGpe', { instance: 7 })).toBeUndefined();
    const live = [
      {
        name: 'gpe-config',
        kind: 'vxlanGpe',
        interface: 'vxlan_gpe_tunnel42',
        adminUp: false,
        linkUp: true,
      },
    ] as never;
    expect(liveTunnel(live, 'vxlanGpe', 'gpe-config')?.interface).toBe('vxlan_gpe_tunnel42');
    expect(liveStatus(liveTunnel(live, 'vxlanGpe', 'gpe-config'))).toBe('adminDown');
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

  it(
    'lists the tunnels of each kind with their engine interface, pending state and live status',
    LONG,
    async () => {
      const api = installFakeApi();
      withTunnels(api);
      await signIn();
      render(app('/vpn/tunnels'));
      expect(await screen.findByRole('heading', { name: 'Tunnels' }, WAIT)).toBeInTheDocument();
      const grid = await screen.findByRole('grid', { name: 'GRE' }, WAIT);
      expect(
        await within(grid).findByRole('button', { name: 'Open to-dc' }, WAIT),
      ).toBeInTheDocument();
      expect(within(grid).getByText('gre7001')).toBeInTheDocument();
      expect(within(grid).getByText('new')).toBeInTheDocument(); // to-br is not in running
      expect(within(grid).getByText('Up')).toBeInTheDocument();
      fireEvent.click(screen.getByRole('tab', { name: 'VXLAN' }));
      const vgrid = await screen.findByRole('grid', { name: 'VXLAN' }, WAIT);
      expect(await within(vgrid).findByText('vxlan_tunnel7003', {}, WAIT)).toBeInTheDocument();
      expect(within(vgrid).getByText('100')).toBeInTheDocument();
    },
  );

  it(
    'shows VXLAN-GPE and advanced kind limits without guessed live allocations',
    LONG,
    async () => {
      const api = installFakeApi();
      withTunnels(api);
      api.on('GET /api/v1/config/candidate/tunnels', {
        body: {
          gre: {},
          ipip: {},
          vxlan: {},
          vxlanGpe: { 'named-gpe': { src: '10.7.1.1', dst: '10.7.1.2', vni: 15 } },
          gtpu: {},
          l2tpv3: {},
          pppoe: {},
        },
      });
      await signIn();
      render(app('/vpn/tunnels'));
      fireEvent.click(await screen.findByRole('tab', { name: 'VXLAN-GPE' }, WAIT));
      const grid = await screen.findByRole('grid', { name: 'VXLAN-GPE' }, WAIT);
      expect(
        await within(grid).findByRole('button', { name: 'Open named-gpe' }, WAIT),
      ).toBeInTheDocument();
      expect(screen.queryByRole('tab', { name: 'GTP-U' })).not.toBeInTheDocument();
      fireEvent.click(screen.getByRole('switch', { name: 'Show advanced kinds' }));
      fireEvent.click(screen.getByRole('tab', { name: 'L2TPv3' }));
      expect(
        await screen.findByText(/This build cannot delete L2TPv3 tunnels/, {}, WAIT),
      ).toBeInTheDocument();
      fireEvent.click(screen.getByRole('switch', { name: 'Show advanced kinds' }));
      expect(screen.getByRole('tab', { name: 'GRE' })).toHaveAttribute('aria-selected', 'true');
    },
  );

  it('suppresses cached green state after a failed live refetch', LONG, async () => {
    const api = installFakeApi();
    withTunnels(api);
    await signIn();
    render(app('/vpn/tunnels'));
    const grid = await screen.findByRole('grid', { name: 'GRE' }, WAIT);
    expect(await within(grid).findByText('Up', {}, WAIT)).toBeInTheDocument();
    api.on('GET /api/v1/state/tunnels', {
      status: 503,
      body: { title: 'Unavailable', status: 503 },
    });
    expect(
      await screen.findByText(
        'Live tunnel state could not be read. Configuration remains editable.',
        {},
        WAIT,
      ),
    ).toBeInTheDocument();
    expect(within(grid).queryByText('Up')).not.toBeInTheDocument();
    expect(within(grid).queryByText('gre7001')).not.toBeInTheDocument();
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
