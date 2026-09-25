import { QueryClient } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';
import {
  clientConfig,
  foldPeerEvents,
  ifaceChip,
  interfaceFormSchema,
  networkOf,
  peerChip,
  peerFormSchema,
  peerStatus,
  type WgInterfaceState,
  type WireguardInterface,
} from './model';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';
const PUB = 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw=';
const SRV = 'xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=';

const cfg: WireguardInterface = {
  enabled: true,
  instance: 1001,
  vrf: 'default',
  underlayVrf: 'default',
  listenAddress: '10.1.51.1',
  listenPort: 20110,
  privateKeyRef: 'key/w1-a',
  address: ['10.1.52.1/24'],
  mtu: 1420,
  routeAllowedIps: false,
  peers: {
    branch: {
      publicKey: PUB,
      allowedIps: ['10.1.52.2/32'],
      persistentKeepaliveSec: 25,
      endpoint: { address: '10.1.51.2', port: 20111 },
    },
  },
};
const st: WgInterfaceState = {
  name: 'site-a',
  vppName: 'wg1001',
  instance: 1001,
  swIfIndex: 7,
  publicKey: SRV,
  listenAddress: '10.1.51.1',
  listenPort: 20110,
  adminUp: true,
  linkUp: true,
  rxPackets: 1,
  rxBytes: 1200,
  txPackets: 1,
  txBytes: 800,
  peers: [
    {
      name: 'branch',
      publicKey: PUB,
      peerIndex: 0,
      status: 'down',
      established: false,
      dead: false,
      endpoint: null,
      endpointPort: 0,
      lastHandshake: null,
      persistentKeepaliveSec: 25,
      allowedIps: ['10.1.52.2/32'],
    },
  ],
};

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('WireGuard model', () => {
  it('extracts the one schema for the forms (no peers in the interface form)', () => {
    const itf = interfaceFormSchema();
    expect(Object.keys(itf.properties ?? {})).toEqual(
      expect.arrayContaining(['instance', 'privateKeyRef', 'routeAllowedIps']),
    );
    expect(Object.keys(itf.properties ?? {})).not.toContain('peers');
    expect(Object.keys(peerFormSchema().properties ?? {})).toEqual(
      expect.arrayContaining(['publicKey', 'allowedIps', 'endpoint']),
    );
  });

  it('folds live peer events over the state snapshot (newer wins)', () => {
    const live = foldPeerEvents(new Map(), [
      {
        kind: 'wireguard_peer_changed',
        interface: 'wg1001',
        ts: '2026-09-25T10:00:01Z',
        attributes: { public_key: PUB, established: 'true', dead: 'false' },
      },
      { kind: 'link_up', interface: 'x' },
    ]);
    expect(peerStatus(st, st.peers[0]!, live, '2026-09-25T10:00:00Z')).toBe('established');
    expect(peerStatus(st, st.peers[0]!, live, '2026-09-25T10:00:02Z')).toBe('down'); // the snapshot is newer
    expect([
      peerChip('established'),
      peerChip('dead'),
      peerChip('down'),
      ifaceChip(undefined),
      ifaceChip(st),
    ]).toEqual(['up', 'down', 'adminDown', 'down', 'up']);
  });

  it('client configuration: the private key only when generated in this session; networks of the interface', () => {
    expect(networkOf('10.1.52.1/24')).toBe('10.1.52.0/24');
    const withKey = clientConfig({
      clientPrivateKey: 'CLIENTKEY=',
      peer: cfg.peers['branch']!,
      iface: cfg,
      serverPublicKey: SRV,
    });
    expect(withKey).toBe(
      [
        '[Interface]',
        'PrivateKey = CLIENTKEY=',
        'Address = 10.1.52.2/32',
        '',
        '[Peer]',
        `PublicKey = ${SRV}`,
        'Endpoint = 10.1.51.1:20110',
        'AllowedIPs = 10.1.52.0/24',
        'PersistentKeepalive = 25',
        '',
      ].join('\n'),
    );
    expect(
      clientConfig({
        clientPrivateKey: undefined,
        peer: cfg.peers['branch']!,
        iface: cfg,
        serverPublicKey: SRV,
      }),
    ).toMatch(/PrivateKey = <the client private key/);
  });
});

describe('WireGuard tab', () => {
  it('lists interfaces with peers and live status, key-pair action for admins', async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/state/vpn/wireguard', {
      body: { retrievedAt: '2026-09-25T10:00:00Z', eventsActive: true, interfaces: [st] },
    });
    api.on('GET /api/v1/config/candidate/vpn', {
      body: { wireguard: { interfaces: { 'site-a': cfg } } },
    });
    await signIn();
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: 0 } },
    });
    render(
      <App
        router={createTestRouter(['/vpn?tab=wireguard'], { devRoutes: false })}
        streamUrl={STREAM}
        queryClient={queryClient}
      />,
    );
    const card = await screen.findByTestId('wg-site-a', {}, { timeout: 15_000 });
    expect(within(card).getByText('wg1001')).toBeInTheDocument();
    expect(within(card).getByText(SRV)).toBeInTheDocument();
    const row = within(card).getByTestId('wg-peer-site-a-branch');
    expect(within(row).getByText('No handshake')).toBeInTheDocument();
    expect(within(row).getByText('10.1.51.2:20111')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Generate key pair' })).toBeInTheDocument();
    expect(screen.getByText('Live status on')).toBeInTheDocument();
  });
});
