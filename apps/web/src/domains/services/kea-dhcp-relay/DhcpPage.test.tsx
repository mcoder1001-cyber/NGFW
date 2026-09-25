import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import en from '../../../locales/en/kea-dhcp-relay.json';
import fa from '../../../locales/fa/kea-dhcp-relay.json';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import { leasesQuery } from './LeasesTab';
import { DHCP_POLL_MS } from './queries';
import {
  localize,
  nestPatch,
  relayItemSchema,
  relayRows,
  reservationRows,
  serverFormSchema,
  serverRows,
  subnetRows,
  usagePercent,
  type DhcpCfg,
  type ServerStatus,
} from './model';

/** F-kea-dhcp-relay screen in jsdom against a scripted stand-in of the API (the real stack: test/topology/kea-dhcp-relay). */
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

const dhcp: DhcpCfg = {
  servers: {
    lan: {
      enabled: true,
      family: 'ipv4',
      vrf: 'w1-dhcp',
      interfaces: ['host-w1l0'],
      subnets: {
        lan: {
          subnet: '10.1.1.0/24',
          pools: [{ start: '10.1.1.100', end: '10.1.1.150' }],
          reservations: {
            printer: { mac: '02:00:00:00:01:01', ip: '10.1.1.20', hostname: 'printer' },
          },
        },
      },
    },
  },
  relays: {
    'to-kea': {
      vrf: 'w1-dhcp',
      interfaces: ['host-w1l0'],
      servers: ['10.1.2.2'],
      sourceAddress: '10.1.2.1',
    },
  },
};

const status: ServerStatus[] = [
  {
    family: 'ipv4',
    running: true,
    active: true,
    actionRequired: '',
    reloadSec: 3,
    subnets: [
      {
        server: 'lan',
        subnet: 'lan',
        prefix: '10.1.1.0/24',
        subnetId: 7,
        total: '51',
        assigned: '30',
        declined: '0',
      },
    ],
    error: '',
  },
  {
    family: 'ipv6',
    running: false,
    active: false,
    actionRequired: '',
    reloadSec: 0,
    subnets: [],
    error: '',
  },
];

function lease(i: number) {
  return {
    family: 'ipv4',
    address: `10.1.1.${100 + i}`,
    hwAddress: `02:00:00:00:00:${(16 + i).toString(16)}`,
    clientId: '',
    duid: '',
    hostname: `pc${i}`,
    server: 'lan',
    subnet: 'lan',
    subnetId: 7,
    validLifetimeSec: 600,
    expiresAt: '2026-09-25T10:00:00.000Z',
    state: 'default',
    leaseType: '',
    prefixLen: 0,
  };
}

function withDhcp(api: FakeApi) {
  api.on('GET /api/v1/config/candidate/services', { body: { dhcp } });
  api.on('GET /api/v1/config/candidate/interfaces', { body: { 'host-w1l0': { enabled: true } } });
  api.on('GET /api/v1/state/dhcp/leases', (req) => {
    const q = new URL(req.url).searchParams;
    const page = Number(q.get('page') ?? '1');
    const size = Number(q.get('pageSize') ?? '100');
    const all = Array.from({ length: 60 }, (_, i) => lease(i));
    return {
      body: {
        page,
        pageSize: size,
        total: all.length,
        truncated: false,
        items: all.slice((page - 1) * size, page * size),
        servers: status,
      },
    };
  });
  api.on('GET /api/v1/state/dhcp/relays', {
    body: {
      items: [
        {
          name: 'to-kea',
          config: dhcp.relays!['to-kea'],
          retrieved: dhcp.relays!['to-kea'],
          state: 'applied',
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

function keysOf(o: unknown, prefix = ''): string[] {
  if (typeof o !== 'object' || o === null) return [prefix];
  return Object.entries(o).flatMap(([k, v]) => keysOf(v, prefix ? `${prefix}.${k}` : k));
}

describe('dhcp model', () => {
  it('en and fa have identical key sets', () => {
    expect(keysOf(fa).sort()).toEqual(keysOf(en).sort());
  });

  it('pool utilisation from decimal strings (DHCPv6 totals exceed 2^53)', () => {
    expect(usagePercent('30', '51')).toBe(58.8);
    expect(usagePercent('0', '0')).toBe(0);
    expect(usagePercent('1', '18446744073709551616')).toBe(0);
    expect(usagePercent('9', '9')).toBe(100);
    expect(usagePercent('x', '9')).toBe(0);
  });

  it('rows of servers, subnets (with usage), reservations and relays', () => {
    expect(serverRows(dhcp)).toMatchObject([
      { name: 'lan', family: 'ipv4', vrf: 'w1-dhcp', subnets: 1, enabled: true },
    ]);
    const subs = subnetRows(dhcp, status);
    expect(subs).toMatchObject([
      {
        id: 'lan/lan',
        prefix: '10.1.1.0/24',
        pools: ['10.1.1.100–10.1.1.150'],
        reservations: 1,
        usage: { assigned: '30' },
      },
    ]);
    expect(reservationRows(dhcp)).toMatchObject([
      { id: 'lan/lan/printer', client: '02:00:00:00:01:01', ip: '10.1.1.20' },
    ]);
    const relays = relayRows(
      {
        relays: {
          ...dhcp.relays,
          fresh: { vrf: 'default', servers: ['10.9.9.9'], sourceAddress: '10.9.9.1' },
        },
      },
      [
        { name: 'to-kea', config: {}, retrieved: {}, state: 'drift' },
        {
          name: 'ghost',
          config: null,
          retrieved: { vrf: 'x', servers: ['1.1.1.1'] },
          state: 'unmanaged',
        },
      ],
    );
    expect(relays.map((r) => [r.name, r.state, r.cfg !== undefined])).toEqual([
      ['fresh', 'pending', true],
      ['ghost', 'unmanaged', false],
      ['to-kea', 'drift', true],
    ]);
  });

  it('polls DHCP state no faster than every 30 s (D-132)', () => {
    expect(DHCP_POLL_MS).toBeGreaterThanOrEqual(30_000);
  });

  it('merge patches nest below services; the lease query maps the grid request', () => {
    expect(nestPatch(['dhcp', 'servers', 'lan'], null)).toEqual({
      dhcp: { servers: { lan: null } },
    });
    const req = {
      page: 1,
      pageSize: 25,
      sort: [],
      filter: [],
      filterLogic: 'and' as const,
      quickFilter: ['pc4', 'x'],
    };
    expect(leasesQuery(req, 'lan', 'ipv4')).toEqual({
      page: 2,
      pageSize: 25,
      filter: 'pc4',
      server: 'lan',
      family: 'ipv4',
    });
    expect(leasesQuery({ ...req, page: 0, quickFilter: [] }, '', '')).toEqual({
      page: 1,
      pageSize: 25,
    });
  });

  it('forms come from the one schema and are localized (servers without subnets)', () => {
    const s = localize(
      serverFormSchema(),
      (k, o) => i18n.t(k, { ...o, ns: 'kea-dhcp-relay' }),
      'server',
    );
    const props = s.properties as Record<string, { title?: string }>;
    expect(props['subnets']).toBeUndefined();
    expect(props['leaseTimeSec']?.title).toBe('Lease time (s)');
    const r = localize(
      relayItemSchema(),
      (k, o) => i18n.t(k, { ...o, ns: 'kea-dhcp-relay', lng: 'fa' }),
      'relay',
    );
    expect((r.properties as Record<string, { title?: string }>)['sourceAddress']?.title).toBe(
      'نشانی مبدأ رله',
    );
  });
});

describe('dhcp screen', () => {
  it(
    'renders the DHCP tab with its sections, server status, pool utilisation and a server-side paged lease grid',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withDhcp(api);
      await signIn();
      render(app('/services?tab=dhcp'));
      expect(
        await screen.findByRole('tab', { name: 'DHCP' }, { timeout: 15_000 }),
      ).toBeInTheDocument();
      const sections = await screen.findByRole('tablist', { name: 'DHCP sections' });
      expect(
        within(sections)
          .getAllByRole('tab')
          .map((t) => t.textContent),
      ).toEqual(['Servers', 'Subnets & Pools', 'Reservations', 'Relays', 'Leases']);

      // servers: the candidate's server with the live daemon state
      const servers = await screen.findByRole('table', { name: 'DHCP servers' });
      expect(await within(servers).findByText('lan')).toBeInTheDocument();
      expect(await within(servers).findByText('Running')).toBeInTheDocument();

      // subnets & pools: utilisation bar = assigned / total of Kea's statistics
      fireEvent.click(within(sections).getByRole('tab', { name: 'Subnets & Pools' }));
      const bar = await screen.findByRole('progressbar', { name: 'Pool utilisation of lan/lan' });
      expect(bar).toHaveAttribute('aria-valuenow', '59'); // MUI rounds; the caption keeps one decimal
      expect(screen.getByText('30 of 51 leased (58.8 %)')).toBeInTheDocument();

      // reservations and relays
      fireEvent.click(within(sections).getByRole('tab', { name: 'Reservations' }));
      expect(await screen.findByText('02:00:00:00:01:01')).toBeInTheDocument();
      fireEvent.click(within(sections).getByRole('tab', { name: 'Relays' }));
      const relays = await screen.findByRole('table', { name: 'DHCP relays' });
      expect(await within(relays).findByText('Applied')).toBeInTheDocument();
      expect(within(relays).getByText('10.1.2.1')).toBeInTheDocument();

      // leases: page 2 is requested from the API (server-side paging, never the whole table)
      fireEvent.click(within(sections).getByRole('tab', { name: 'Leases' }));
      const grid = await screen.findByRole('grid');
      expect(await within(grid).findByText('pc0')).toBeInTheDocument();
      const first = api.calls.filter(
        (c) => c.path === '/api/v1/state/dhcp/leases' && c.search.includes('pageSize=25'),
      );
      expect(first.length).toBeGreaterThan(0);
      expect(first[0]!.search).toContain('page=1');
      fireEvent.click(screen.getByRole('button', { name: /next page/i }));
      await waitFor(() =>
        expect(
          api.calls.some(
            (c) =>
              c.path === '/api/v1/state/dhcp/leases' &&
              /[?&]page=2(&|$)/.test(c.search) &&
              c.search.includes('pageSize=25'),
          ),
        ).toBe(true),
      );
      expect(await within(grid).findByText('pc25')).toBeInTheDocument();
    },
  );

  it(
    'edits a server through a merge patch of /config/services (the pending-change bar shows the diff); defaults are sent as-is',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withDhcp(api);
      let patched: unknown;
      api.on('PATCH /api/v1/config/services', (_r, body) => {
        patched = body;
        return { body: { pointer: '/services', before: null, after: null } };
      });
      await signIn();
      render(app('/services?tab=dhcp'));
      const servers = await screen.findByRole(
        'table',
        { name: 'DHCP servers' },
        { timeout: 15_000 },
      );
      // D-132: state polls every 30 s at most; the Refresh button reads status, relays and the candidate on demand
      await within(servers).findByText('Running');
      const reads = () => api.calls.filter((c) => c.path === '/api/v1/state/dhcp/leases').length;
      const before = reads();
      fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
      await waitFor(() => expect(reads()).toBeGreaterThan(before));
      fireEvent.click(await within(servers).findByRole('button', { name: 'Edit lan' }));
      const dialog = await screen.findByRole('dialog');
      const lease = await within(dialog).findByLabelText(/^Lease time \(s\)/);
      fireEvent.change(lease, { target: { value: '1200' } });
      fireEvent.click(within(dialog).getByRole('button', { name: 'Save to candidate' }));
      // only the change (plus defaults the candidate did not spell out yet); nothing else of the server is touched
      await waitFor(() =>
        expect(patched).toMatchObject({ dhcp: { servers: { lan: { leaseTimeSec: 1200 } } } }),
      );
      const sent = (patched as { dhcp: { servers: { lan: Record<string, unknown> } } }).dhcp.servers
        .lan;
      expect(Object.keys(sent).sort()).toEqual(['authoritative', 'leaseTimeSec', 'options']);
      expect(Object.values(sent)).not.toContain(null);
      // removing a server is a null in the merge patch
      await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
      fireEvent.click(within(servers).getByRole('button', { name: 'Remove lan' }));
      await waitFor(() => expect(patched).toEqual({ dhcp: { servers: { lan: null } } }));
    },
  );
});
