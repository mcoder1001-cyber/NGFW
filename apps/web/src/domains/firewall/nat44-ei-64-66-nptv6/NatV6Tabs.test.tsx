import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import type { Session } from '../nat44-ed-sessions/model';
import { natTabs } from '../nat44-ed-sessions/tabs';
import { driftUnder, eiKillBodyOf, localizeDeep, subtreeSchema } from './model';

/** F-nat44-ei-64-66-nptv6 tabs of the NAT screen in jsdom against a scripted stand-in of the API. */
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

const session = (i: number): Session => ({
  insideAddress: '10.1.1.10',
  insidePort: 10000 + i,
  outsideAddress: '10.1.2.100',
  outsidePort: 20000 + i,
  externalAddress: '10.1.2.2',
  externalPort: 80,
  externalNatAddress: '10.1.2.2',
  externalNatPort: 80,
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

function withNat(api: FakeApi, mode: 'ed' | 'ei') {
  api.on('GET /api/v1/config/candidate/nat', {
    body: {
      mode,
      inside: ['host-w1l0'],
      outside: ['host-w1w0'],
      nat64: {
        enabled: true,
        inside: ['host-w1l0'],
        outside: ['host-w1w0'],
        prefixes: [{ prefix: 'fd00:1:64::/96' }],
        pools: [{ range: '10.1.64.1-10.1.64.2' }],
        staticBibs: [],
      },
      nat66: {
        enabled: true,
        inside: [],
        outside: [],
        staticMappings: [{ local: 'fd00:1:1::66', external: 'fd00:1:2::66' }],
      },
      nptv6: {
        bindings: [
          { interface: 'host-w1w0', internal: 'fd00:1:10::/48', external: 'fd00:1:20::/48' },
        ],
      },
    },
  });
  api.on('GET /api/v1/config/candidate/interfaces', {
    body: { 'host-w1l0': { enabled: true }, 'host-w1w0': { enabled: true } },
  });
  api.on('GET /api/v1/state/nat/ei/sessions', (req) => {
    const q = new URL(req.url).searchParams;
    const page = Number(q.get('page') ?? '1');
    const pageSize = Number(q.get('pageSize') ?? '100');
    const items = Array.from({ length: Math.min(pageSize, 20) }, (_, i) =>
      session((page - 1) * pageSize + i),
    );
    return { body: { page, pageSize, total: 150, totalUsers: 3, truncated: false, items } };
  });
  api.on('GET /api/v1/state/nat/nat64/sessions', (req) => {
    const q = new URL(req.url).searchParams;
    return {
      body: {
        page: 1,
        pageSize: Number(q.get('pageSize') ?? '100'),
        total: 6,
        totalClients: 5,
        truncated: false,
        items: [
          {
            client: 'fd00:1::10',
            clientPort: 40000,
            poolAddress: '10.1.64.1',
            poolPort: 1024,
            remote: '10.1.2.2',
            remotePort: 80,
            remoteIpv6: 'fd00:1:64::a01:202',
            protocol: q.get('protocol') ?? 'tcp',
            vrf: 'default',
            tableId: 0,
          },
        ],
      },
    };
  });
  api.on('GET /api/v1/state/nat/nptv6', {
    body: {
      writeOnly: true,
      bindings: [
        {
          interface: 'host-w1w0',
          internal: 'fd00:1:10::/48',
          external: 'fd00:1:20::/48',
          description: null,
        },
      ],
    },
  });
  api.on('GET /api/v1/state/drift', {
    body: { subsystems: ['nat'], changes: [], ignored: [] },
  });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('nat44-ei-64-66-nptv6 model', () => {
  it('registers four tabs after the NAT44-ED ones', () => {
    expect(natTabs.map((t) => t.id)).toEqual([
      'outbound',
      'static',
      'pools',
      'sessions',
      'ei',
      'nat64',
      'nat66',
      'nptv6',
    ]);
  });
  it('EI kill body, deep localisation, drift filter', () => {
    expect(eiKillBodyOf(session(1))).toMatchObject({
      protocol: 'tcp',
      insideAddress: '10.1.1.10',
      insidePort: 10001,
      vrf: 'default',
    });
    const s = localizeDeep(subtreeSchema('nat64'), (k, o) =>
      k === 'field.prefix.title' ? 'PFX' : String(o?.['defaultValue'] ?? k),
    );
    const prefixes = (
      s.properties as Record<string, { items: { properties: Record<string, { title: string }> } }>
    )['prefixes'];
    expect(prefixes?.items.properties['prefix']?.title).toBe('PFX');
    expect(
      driftUnder(
        {
          subsystems: ['nat'],
          ignored: [],
          changes: [
            { op: 'replace', pointer: '/nat/nat66/staticMappings', from: [{ local: 'a' }], to: [] },
            { op: 'replace', pointer: '/nat/nat64', from: true, to: false },
            // only a description differs: not drift (VPP never stores descriptions)
            {
              op: 'replace',
              pointer: '/nat/nat66/staticMappings',
              from: [{ local: 'a', description: 'x' }],
              to: [{ local: 'a' }],
            },
          ],
        },
        '/nat/nat66',
      ),
    ).toHaveLength(1);
  });
});

describe('NAT screen: EI, NAT64, NAT66, NPTv6 tabs', () => {
  it('EI in ED mode offers the switch, which writes only `mode`', LONG, async () => {
    const api = installFakeApi();
    withNat(api, 'ed');
    const patches: unknown[] = [];
    api.on('PATCH /api/v1/config/nat', (_r, body) => {
      patches.push(body);
      return { body: { pointer: '/nat', before: null, after: null } };
    });
    await signIn();
    render(app('/firewall/nat?tab=ei'));
    const bar = await screen.findByTestId('nat-ei-mode', {}, { timeout: 30_000 });
    expect(bar).toHaveTextContent('NAT44 runs in ED mode');
    fireEvent.click(within(bar).getByRole('button', { name: 'Switch to EI' }));
    await waitFor(() => expect(patches).toEqual([{ mode: 'ei' }]));
  });

  it('EI sessions: server-side paging and the kill by the inside endpoint', LONG, async () => {
    const api = installFakeApi();
    withNat(api, 'ei');
    let killed: unknown;
    api.on('POST /api/v1/actions/nat/ei/sessions/kill', (_r, body) => {
      killed = body;
      return {
        body: {
          deleted: true,
          summary: 'NAT44-EI session deleted: tcp 10.1.1.10:10000 (table 0)',
          stats: {},
        },
      };
    });
    await signIn();
    render(app('/firewall/nat?tab=ei'));
    await waitFor(
      () =>
        expect(screen.getByTestId('nat-ei-sessions-total')).toHaveTextContent(
          /150 sessions from 3 inside hosts/,
        ),
      { timeout: 30_000 },
    );
    const first = api.calls.find((c) => c.path === '/api/v1/state/nat/ei/sessions');
    expect(first?.search).toContain('pageSize=100');
    const grid = await screen.findByRole('grid');
    fireEvent.click(
      await within(grid).findByRole('button', { name: 'Delete session 10.1.1.10:10000' }),
    );
    const dialog = await screen.findByRole('dialog', { name: 'Delete NAT44-EI session?' });
    expect(within(dialog).getByTestId('nat-ei-kill-tuple')).toHaveTextContent(
      'tcp 10.1.1.10:10000 (vrf default)',
    );
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete session' }));
    await waitFor(() =>
      expect(killed).toMatchObject({
        protocol: 'tcp',
        insideAddress: '10.1.1.10',
        insidePort: 10000,
      }),
    );
    expect(await screen.findByText(/NAT44-EI session deleted/)).toBeInTheDocument();
  });

  it('NAT64: the schema form and the session table with its protocol filter', LONG, async () => {
    const api = installFakeApi();
    withNat(api, 'ei');
    await signIn();
    render(app('/firewall/nat?tab=nat64'));
    await waitFor(
      () =>
        expect(screen.getByTestId('nat64-sessions-total')).toHaveTextContent(
          /6 sessions from 5 IPv6 clients/,
        ),
      { timeout: 30_000 },
    );
    expect(screen.getAllByText('NAT64 prefixes').length).toBeGreaterThan(0);
    expect(await screen.findByText('[fd00:1::10]:40000')).toBeInTheDocument();
  });

  it(
    'NAT66 and NPTv6: mappings with their live status; bindings marked write-only (fa, RTL)',
    LONG,
    async () => {
      const api = installFakeApi();
      withNat(api, 'ei');
      await signIn();
      const { unmount } = render(app('/firewall/nat?tab=nat66'));
      const table = await screen.findByRole(
        'table',
        { name: 'Static mappings' },
        { timeout: 30_000 },
      );
      expect(
        await within(table).findByText('fd00:1:1::66', {}, { timeout: 30_000 }),
      ).toBeInTheDocument();
      expect(await within(table).findByText('applied')).toBeInTheDocument();
      unmount();
      render(app('/firewall/nat?tab=nptv6'));
      await screen.findByRole(
        'table',
        { name: 'Bindings in the running configuration' },
        { timeout: 30_000 },
      );
      await act(async () => {
        await i18n.changeLanguage('fa');
      });
      const running = await screen.findByRole(
        'table',
        { name: 'اتصال‌های پیکربندی جاری' },
        { timeout: 30_000 },
      );
      expect(within(running).getByText('fd00:1:20::/48')).toBeInTheDocument();
      expect(within(running).getByText('اعمال‌شده (فقط‌نوشتنی)')).toBeInTheDocument();
      const tabs = screen.getAllByRole('tab').map((t) => t.textContent);
      expect(tabs.slice(4)).toEqual(['NAT44-EI', 'NAT64', 'NAT66', 'NPTv6']);
    },
  );
});
