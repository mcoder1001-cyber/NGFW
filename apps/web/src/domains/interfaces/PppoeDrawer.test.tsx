import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../App';
import i18n from '../../i18n';
import { createTestRouter } from '../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../test-api';

/** F-pppoe-client: the drawer's live PPPoE panel and the Reconnect action. */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

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

const wanCfg = {
  enabled: true,
  ipv4: [],
  ipv6: [],
  vrf: 'default',
  promiscuous: false,
  subinterfaces: {},
  pppoe: {
    enabled: true,
    username: 'alice@isp',
    passwordRef: 'password/isp',
    mtu: 1492,
    mssClamp: true,
    defaultRoute: true,
    dnsFromPeer: true,
    ipv6: 'off',
    reconnect: { holdoffSec: 5, maxFail: 0 },
  },
};
const pppoeUp = {
  phase: 'up',
  sessionId: 42,
  acMac: '02:ac:00:00:00:01',
  acName: 'fake-ac',
  localIpv4: '203.0.113.5/32',
  peerIpv4: '203.0.113.1',
  ipv6: '',
  dns: ['203.0.113.53'],
  since: '2026-09-27T00:00:00.000Z',
  failCount: 0,
  lastError: '',
};
const wanLive = {
  name: 'wan0',
  vppName: 'wan0',
  swIfIndex: 6,
  type: 'af-packet',
  adminUp: true,
  linkUp: true,
  mtu: 1492,
  linkMtu: 1500,
  mac: '02:fe:00:00:00:09',
  ipv4: ['203.0.113.5/32'],
  ipv6: [],
  vrf: 'default',
  tableId: 0,
  parent: '',
  vlanId: 0,
  innerVlanId: 0,
  managed: true,
  linkSpeedKbps: '0',
  rxMode: 'interrupt',
  description: '',
  pppoe: pppoeUp,
};

function withWan(api: FakeApi) {
  api.on('GET /api/v1/state/interfaces', {
    body: {
      items: [
        {
          name: 'wan0',
          kind: 'interface',
          parent: null,
          state: wanLive,
          config: wanCfg,
          running: wanCfg,
          counters: null,
          hasPendingChange: false,
        },
      ],
    },
  });
  api.on('GET /api/v1/config/candidate/interfaces', { body: { wan0: wanCfg } });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('PPPoE drawer panel', () => {
  it('shows the live session and reconnects', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withWan(api);
    let reconnected = 0;
    api.on('POST /api/v1/actions/interfaces/wan0/pppoe/reconnect', () => {
      reconnected++;
      return { body: { accepted: true, message: 'redialling' } };
    });
    await signIn();
    render(app('/interfaces'));
    const grid = await screen.findByRole('grid', {}, { timeout: 15_000 });
    fireEvent.click(await within(grid).findByText('wan0'));
    const drawer = await screen.findByRole('region', { name: 'Interface wan0' });

    const panel = within(drawer).getByRole('table', { name: 'PPPoE session' });
    expect(within(panel).getByText('203.0.113.5/32')).toBeInTheDocument();
    expect(within(panel).getByText('203.0.113.1')).toBeInTheDocument();
    expect(within(panel).getByText('203.0.113.53')).toBeInTheDocument();
    // status chip carries the phase
    expect(within(drawer).getByText(/Session: up/)).toBeInTheDocument();

    fireEvent.click(within(drawer).getByRole('button', { name: 'Reconnect' }));
    await waitFor(() => expect(reconnected).toBe(1));
    expect(await within(drawer).findByText('Redialling the session…')).toBeInTheDocument();
  });

  it(
    'shows a failed session with the last error, and renders in Persian',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      api.on('GET /api/v1/state/interfaces', {
        body: {
          items: [
            {
              name: 'wan0',
              kind: 'interface',
              parent: null,
              state: {
                ...wanLive,
                linkUp: false,
                ipv4: [],
                pppoe: {
                  ...pppoeUp,
                  phase: 'failed',
                  localIpv4: '',
                  peerIpv4: '',
                  dns: [],
                  since: null,
                  failCount: 3,
                  lastError: 'auth failed',
                },
              },
              config: wanCfg,
              running: wanCfg,
              counters: null,
              hasPendingChange: false,
            },
          ],
        },
      });
      api.on('GET /api/v1/config/candidate/interfaces', { body: { wan0: wanCfg } });
      await signIn();
      render(app('/interfaces'));
      const grid = await screen.findByRole('grid', {}, { timeout: 15_000 });
      fireEvent.click(await within(grid).findByText('wan0'));
      const drawer = await screen.findByRole('region', { name: 'Interface wan0' });
      expect(within(drawer).getByText('auth failed')).toBeInTheDocument();
      expect(within(drawer).getByText(/Session: failed/)).toBeInTheDocument();

      await i18n.changeLanguage('fa');
      expect(await within(drawer).findByText('نشست PPPoE')).toBeInTheDocument();
    },
  );
});
