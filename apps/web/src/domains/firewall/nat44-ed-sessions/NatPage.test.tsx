import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import {
  filterQuery,
  isSessionLevelFilter,
  killBodyOf,
  usageFor,
  EMPTY_FILTER,
  type PoolUsage,
  type Session,
} from './model';
import { NAT_POLL_MS, NAT_SUMMARY_POLL_MS } from './queries';
import { natTabs } from './tabs';

/** F-nat44-ed-sessions NAT screen in jsdom against a scripted stand-in of the API (the real stack: test/topology). */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';
const LONG = { timeout: 120_000 };

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

const nat = {
  mode: 'ed',
  inside: ['host-w1l0'],
  outside: ['host-w1w0'],
  outputFeature: [],
  forwarding: false,
  staticMappingOnly: false,
  connectionTracking: false,
  timeouts: { udp: 300, tcpEstablished: 7440, tcpTransitory: 240, icmp: 60 },
  pools: [
    { name: 'out', range: '10.1.2.100-10.1.2.103', twiceNat: false },
    { name: 'wan', interface: 'host-w1w0', twiceNat: false },
  ],
  staticMappings: [
    {
      name: 'web',
      protocol: 'tcp',
      local: { ip: '10.1.1.2', port: 80 },
      external: { ip: '10.1.2.110', port: 8080 },
      twiceNat: false,
      selfTwiceNat: false,
      out2inOnly: false,
    },
  ],
  identityMappings: [],
  loadBalancedMappings: [],
};

const usage = (p: Partial<PoolUsage>): PoolUsage => ({
  name: null,
  kind: 'range',
  range: null,
  interface: null,
  vrf: 'default',
  twiceNat: false,
  addresses: 0,
  sessions: 0,
  utilisation: 0,
  applied: true,
  configured: true,
  ...p,
});

const summary = {
  enabled: true,
  sessionLimit: 64512,
  totalUsers: 22,
  totalSessions: 2101,
  staticSessions: 0,
  truncated: false,
  byProtocol: { tcp: 2100, udp: 1 },
  pools: [
    usage({
      name: 'out',
      range: '10.1.2.100-10.1.2.103',
      addresses: 4,
      sessions: 2101,
      utilisation: 2101 / (4 * 64512),
    }),
    usage({
      name: 'wan',
      kind: 'interface',
      interface: 'host-w1w0',
      vrf: null,
      addresses: 1,
      sessions: 0,
    }),
  ],
};

const session = (i: number): Session => ({
  insideAddress: '10.1.1.10',
  insidePort: 10000 + i,
  outsideAddress: '10.1.2.100',
  outsidePort: 20000 + i,
  externalAddress: '10.1.2.2',
  externalPort: 80,
  externalNatAddress: '0.0.0.0', // VPP reports it only for twice-NAT sessions
  externalNatPort: 0,
  protocol: 'tcp',
  vrf: 'default',
  tableId: 0,
  static: false,
  twiceNat: false,
  timedOut: false,
  idleSeconds: 3,
  bytes: 120,
  packets: 2,
});

function withNat(api: FakeApi) {
  api.on('GET /api/v1/config/candidate/nat', { body: nat });
  api.on('GET /api/v1/config/candidate/interfaces', {
    body: { 'host-w1l0': { enabled: true }, 'host-w1w0': { enabled: true } },
  });
  api.on('GET /api/v1/state/nat/summary', { body: summary });
  api.on('GET /api/v1/state/nat/sessions', (req) => {
    const q = new URL(req.url).searchParams;
    const page = Number(q.get('page') ?? '1');
    const pageSize = Number(q.get('pageSize') ?? '100');
    const items = Array.from({ length: Math.min(pageSize, 25) }, (_, i) =>
      session((page - 1) * pageSize + i),
    );
    return {
      body: {
        page,
        pageSize,
        total: 2101,
        totalUsers: 22,
        truncated: false,
        retrievedAt: new Date().toISOString(),
        items,
      },
    };
  });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('nat model', () => {
  it('kill body, filter query and pool usage join', () => {
    expect(killBodyOf(session(5))).toEqual({
      protocol: 'tcp',
      insideAddress: '10.1.1.10',
      insidePort: 10005,
      externalAddress: '10.1.2.2',
      externalPort: 80,
      vrf: 'default',
    });
    expect(
      filterQuery({
        inside: ' 10.1.1.10 ',
        outside: '',
        external: '',
        port: '80',
        protocol: 'udp',
        vrf: '',
      }),
    ).toEqual({ inside: '10.1.1.10', port: 80, protocol: 'udp' });
    expect(usageFor({ name: 'out' }, summary.pools)?.sessions).toBe(2101);
    // an unnamed usage row matches a configured pool by identity (range + VRF + twice-NAT class)
    expect(usageFor({ name: 'x', range: '10.1.9.1' }, [usage({ range: '10.1.9.1' })])?.range).toBe(
      '10.1.9.1',
    );
    expect(
      usageFor({ name: 'x', range: '10.1.9.1', twiceNat: true }, [usage({ range: '10.1.9.1' })]),
    ).toBeUndefined();
  });

  it("review M1: a twice-NAT row is killed by its NAT'd external end; other protocols are not killable", () => {
    const tw: Session = {
      ...session(1),
      twiceNat: true,
      externalNatAddress: '10.1.2.120',
      externalNatPort: 1024,
    };
    expect(killBodyOf(tw)).toEqual({
      protocol: 'tcp',
      insideAddress: '10.1.1.10',
      insidePort: 10001,
      externalAddress: '10.1.2.120',
      externalPort: 1024,
      vrf: 'default',
    });
    expect(killBodyOf({ ...session(2), protocol: 'udp' })?.protocol).toBe('udp');
    expect(killBodyOf({ ...session(3), protocol: 'icmp' })?.protocol).toBe('icmp');
    expect(killBodyOf({ ...session(4), protocol: '47' })).toBeNull(); // never a silent `tcp`
  });

  it('review H1 / D-132: filters that scan sessions switch polling off; nothing polls faster than 30 s', () => {
    expect(isSessionLevelFilter(EMPTY_FILTER)).toBe(false);
    expect(isSessionLevelFilter({ ...EMPTY_FILTER, vrf: 'cust' })).toBe(false); // selects users only
    for (const k of ['inside', 'outside', 'external', 'port', 'protocol'] as const) {
      expect(isSessionLevelFilter({ ...EMPTY_FILTER, [k]: k === 'port' ? '80' : 'x' })).toBe(true);
    }
    expect(isSessionLevelFilter({ ...EMPTY_FILTER, port: '  ' })).toBe(false);
    // D-132: nothing that walks VPP is polled faster than every 30 s
    expect(NAT_SUMMARY_POLL_MS).toBeGreaterThanOrEqual(30_000);
    expect(NAT_POLL_MS).toBeGreaterThanOrEqual(30_000);
  });

  it('natTabs: the four NAT44-ED tabs in order (siblings append after them)', () => {
    expect(natTabs.map((t) => t.id).slice(0, 4)).toEqual([
      'outbound',
      'static',
      'pools',
      'sessions',
    ]);
  });
});

describe('NAT screen', () => {
  it(
    'renders the four tabs, ?tab= selects one, an unknown tab falls back to the first',
    LONG,
    async () => {
      const api = installFakeApi();
      withNat(api);
      await signIn();
      const { unmount } = render(app('/firewall/nat?tab=pools'));
      expect(
        await screen.findByRole('heading', { level: 2, name: 'NAT' }, { timeout: 30_000 }),
      ).toBeInTheDocument();
      const tabs = screen.getByRole('tablist', { name: 'NAT sections' });
      expect(
        within(tabs)
          .getAllByRole('tab')
          .map((t) => t.textContent)
          .slice(0, 4), // the sibling NAT features' tabs follow (F-nat44-ei-64-66-nptv6)
      ).toEqual(['Outbound', 'Static & port forwards', 'Pools', 'Sessions']);
      expect(within(tabs).getByRole('tab', { name: 'Pools' })).toHaveAttribute(
        'aria-selected',
        'true',
      );
      unmount();
      render(app('/firewall/nat?tab=bogus'));
      const t2 = await screen.findByRole('tablist', { name: 'NAT sections' }, { timeout: 30_000 });
      expect(within(t2).getByRole('tab', { name: 'Outbound' })).toHaveAttribute(
        'aria-selected',
        'true',
      );
    },
  );

  it('pools: a utilisation bar per pool from the live summary', LONG, async () => {
    const api = installFakeApi();
    withNat(api);
    await signIn();
    render(app('/firewall/nat?tab=pools'));
    const bar = await screen.findByRole(
      'progressbar',
      { name: 'Utilisation of pool out' },
      { timeout: 30_000 },
    );
    expect(bar).toHaveAttribute('aria-valuenow', String(Math.round((2101 / (4 * 64512)) * 100))); // MUI rounds
    expect(bar).toHaveAttribute('aria-valuetext', '0.8 percent');
    expect(screen.getByText(/2,101 sessions on 4 addresses/)).toBeInTheDocument();
    expect(
      screen.getByRole('status', { name: 'NAT44-ED status on the data plane' }),
    ).toHaveTextContent('NAT44-ED enabled');
  });

  it(
    'sessions: server-side paging (page/pageSize in the request), total shown, filter, kill with confirmation',
    LONG,
    async () => {
      const api = installFakeApi();
      withNat(api);
      let killed: unknown;
      api.on('POST /api/v1/actions/nat/sessions/kill', (_r, body) => {
        killed = body;
        return {
          body: {
            deleted: true,
            summary: 'session deleted: tcp 10.1.1.10:10000 -> 10.1.2.2:80 (table 0)',
            stats: {},
          },
        };
      });
      await signIn();
      render(app('/firewall/nat?tab=sessions'));
      await waitFor(
        () =>
          expect(screen.getByTestId('nat-sessions-total')).toHaveTextContent(
            /2,101 sessions from 22 inside hosts/,
          ),
        { timeout: 30_000 },
      );
      const first = api.calls.find((c) => c.path === '/api/v1/state/nat/sessions');
      expect(first?.search).toContain('page=1');
      expect(first?.search).toContain('pageSize=100');

      // filter → the next request carries it
      fireEvent.change(screen.getByLabelText('Inside address'), { target: { value: '10.1.1.10' } });
      fireEvent.click(screen.getByRole('button', { name: 'Filter' }));
      await waitFor(() =>
        expect(
          api.calls.some(
            (c) => c.path === '/api/v1/state/nat/sessions' && c.search.includes('inside=10.1.1.10'),
          ),
        ).toBe(true),
      );

      // a session-level filter: no polling, a visible note, the Refresh button fetches again
      expect(await screen.findByTestId('nat-sessions-manual')).toHaveTextContent(
        'Filtered: refreshed on demand only',
      );
      const before = api.calls.filter((c) => c.path === '/api/v1/state/nat/sessions').length;
      fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
      await waitFor(() =>
        expect(api.calls.filter((c) => c.path === '/api/v1/state/nat/sessions').length).toBe(
          before + 1,
        ),
      );

      // kill asks first, then POSTs the 5-tuple
      const grid = await screen.findByRole('grid');
      fireEvent.click(
        await within(grid).findByRole('button', { name: 'Delete session 10.1.1.10:10000' }),
      );
      const dialog = await screen.findByRole('dialog', { name: 'Delete NAT session?' });
      expect(within(dialog).getByTestId('nat-kill-tuple')).toHaveTextContent(
        'tcp 10.1.1.10:10000 → 10.1.2.2:80 (vrf default)',
      );
      expect(killed).toBeUndefined();
      fireEvent.click(within(dialog).getByRole('button', { name: 'Delete session' }));
      await waitFor(() =>
        expect(killed).toEqual({
          protocol: 'tcp',
          insideAddress: '10.1.1.10',
          insidePort: 10000,
          externalAddress: '10.1.2.2',
          externalPort: 80,
          vrf: 'default',
        }),
      );
      expect(await screen.findByText(/session deleted/)).toBeInTheDocument();
    },
  );

  it('sessions: a session that is already gone (404) is reported, not an error', LONG, async () => {
    const api = installFakeApi();
    withNat(api);
    api.on('POST /api/v1/actions/nat/sessions/kill', {
      status: 404,
      body: {
        type: 'https://vrx.dev/problems/not-found',
        title: 'Not found',
        status: 404,
        detail: 'agent: no such session',
      },
    });
    await signIn();
    render(app('/firewall/nat?tab=sessions'));
    const grid = await screen.findByRole('grid', {}, { timeout: 30_000 });
    fireEvent.click(
      await within(grid).findByRole(
        'button',
        { name: 'Delete session 10.1.1.10:10000' },
        { timeout: 30_000 },
      ),
    );
    fireEvent.click(
      within(await screen.findByRole('dialog')).getByRole('button', { name: 'Delete session' }),
    );
    expect(await screen.findByText(/no longer exists/)).toBeInTheDocument();
  });

  it('outbound: the NAT44 switch is tri-state and writes only `enabled`', LONG, async () => {
    const api = installFakeApi();
    withNat(api);
    const patches: unknown[] = [];
    api.on('PATCH /api/v1/config/nat', (_r, body) => {
      patches.push(body);
      return { body: { pointer: '/nat', before: null, after: null } };
    });
    await signIn();
    render(app('/firewall/nat'));
    const select = await screen.findByRole('combobox', { name: 'NAT44' }, { timeout: 30_000 });
    expect(select).toHaveTextContent('Automatic (on when NAT44 is configured)');
    fireEvent.mouseDown(select);
    fireEvent.click(
      await screen.findByRole('option', { name: 'Off (keep the configuration, apply nothing)' }),
    );
    await waitFor(() => expect(patches).toEqual([{ enabled: false }]));
  });
});
