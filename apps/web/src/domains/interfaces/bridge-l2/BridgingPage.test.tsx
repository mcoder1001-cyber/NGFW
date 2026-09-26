import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import { pageOfDomains, toDomainRow } from './BridgingPage';
import { BRIDGE_POLL_MS } from './queries';
import {
  l2Patch,
  portPatch,
  portsOf,
  rangesText,
  tagRewriteText,
  type BridgeDomainItem,
} from './model';

/** F-bridge-l2 screen in jsdom against a scripted stand-in of the API (the real stack: test/topology/bridge-l2). */
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

const lanRunning = {
  id: 7001,
  flood: true,
  uuFlood: true,
  forward: true,
  learn: true,
  arpTerm: false,
  macAgeMin: 5,
  staticMacs: [],
};
const item = (extra: Partial<BridgeDomainItem> = {}): BridgeDomainItem => ({
  name: 'lan',
  id: 7001,
  running: lanRunning,
  hasPendingChange: false,
  state: {
    id: 7001,
    flood: true,
    uuFlood: true,
    forward: true,
    learn: true,
    arpTerm: false,
    arpUfwd: false,
    macAgeMin: 5,
    bvi: 'loop7000',
    uuFwd: '',
    members: [
      { interface: 'loop7000', swIfIndex: 3, portType: 'bvi', shg: 0, tagRewrite: '' },
      { interface: 'host-w7l0.100', swIfIndex: 5, portType: 'normal', shg: 1, tagRewrite: 'pop-1' },
    ],
    learnedMacs: 12,
    staticMacs: 1,
  },
  ...extra,
});

function withBridging(api: FakeApi) {
  api.on('GET /api/v1/state/l2/bridge-domains', {
    body: {
      items: [
        item(),
        item({ name: 'dmz', id: 7002, state: null, running: null, hasPendingChange: true }),
      ],
    },
  });
  api.on('GET /api/v1/config/candidate/routing', {
    body: {
      static: [],
      l2: {
        bridgeDomains: { lan: lanRunning, dmz: { ...lanRunning, id: 7002 } },
        xconnects: { 'host-w7w0': { tx: 'host-w7w0.200' } },
        l3xc: {},
        macFilters: {},
      },
    },
  });
  api.on('GET /api/v1/config/candidate/interfaces', {
    body: {
      loop7000: {
        enabled: true,
        ipv4: ['10.7.0.1/24'],
        ipv6: [],
        vrf: 'default',
        promiscuous: false,
        subinterfaces: {},
        l2: { bridgeDomain: 'lan', shg: 0, bvi: true, uuFwd: false, macFilter: false },
      },
      'host-w7l0': {
        enabled: true,
        ipv4: [],
        ipv6: [],
        vrf: 'default',
        promiscuous: false,
        l2: { bridgeDomain: 'lan', shg: 0, bvi: false, uuFwd: false, macFilter: true },
        subinterfaces: {
          '100': {
            vlanId: 100,
            enabled: true,
            ipv4: [],
            ipv6: [],
            vrf: 'default',
            dot1ad: false,
            l2: {
              bridgeDomain: 'lan',
              shg: 1,
              bvi: false,
              uuFwd: false,
              macFilter: false,
              tagRewrite: { op: 'pop-1', dot1ad: false },
            },
          },
        },
      },
      'host-w7w0': {
        enabled: true,
        ipv4: [],
        ipv6: [],
        vrf: 'default',
        promiscuous: false,
        subinterfaces: {
          '200': {
            vlanId: 200,
            enabled: true,
            ipv4: [],
            ipv6: [],
            vrf: 'default',
            dot1ad: false,
          },
        },
        l2: {
          shg: 0,
          bvi: false,
          uuFwd: false,
          macFilter: false,
          tagRewrite: { op: 'translate-1-1', tag1: 300, dot1ad: false },
        },
      },
    },
  });
  api.on('GET /api/v1/state/l2/bridge-domains/7001/macs', {
    body: {
      page: 1,
      pageSize: 25,
      total: 1,
      items: [
        {
          mac: '02:00:00:00:70:01',
          interface: 'host-w7l0',
          swIfIndex: 5,
          static: false,
          filter: false,
          bvi: false,
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

describe('bridge-l2 model', () => {
  it('D-132: the only list timer is at least 30 s', () => {
    expect(BRIDGE_POLL_MS).toBeGreaterThanOrEqual(30_000);
  });

  it('rows, paging and patches', () => {
    const rows = [item(), item({ name: 'dmz', id: 7002, state: null })].map(toDomainRow);
    expect(rows[0]).toMatchObject({
      status: 'live',
      members: 2,
      bvi: 'loop7000',
      learned: 12,
      macAge: 5,
    });
    expect(rows[1]).toMatchObject({ status: 'missing', members: 0, learned: null });
    const req = {
      page: 0,
      pageSize: 25,
      sort: [{ field: 'name', dir: 'asc' as const }],
      filter: [],
      filterLogic: 'and' as const,
      quickFilter: [],
    };
    expect(pageOfDomains(rows, req).rows.map((r) => r.name)).toEqual(['dmz', 'lan']);
    expect(pageOfDomains(rows, { ...req, quickFilter: ['loop7000'] }).total).toBe(1);
    expect(portPatch({ parent: 'GigabitEthernet0/0/0', sub: '10' }, null)).toEqual({
      'GigabitEthernet0/0/0': { subinterfaces: { '10': { l2: null } } },
    });
    expect(l2Patch('xconnects', 'a', { tx: 'b' })).toEqual({
      l2: { xconnects: { a: { tx: 'b' } } },
    });
    expect(
      tagRewriteText({
        shg: 0,
        bvi: false,
        uuFwd: false,
        macFilter: false,
        tagRewrite: { op: 'translate-1-2', tag1: 10, tag2: 20, dot1ad: true },
      }),
    ).toBe('translate-1-2 10/20 (802.1ad)');
    expect(
      rangesText(
        {
          mac: '02:00:00:00:00:01',
          action: 'allow',
          ranges: [{ days: ['mon', 'tue'], start: '09:00', end: '17:00' }],
        },
        'always',
      ),
    ).toBe('mon,tue 09:00–17:00');
    expect(
      portsOf({
        a: {
          enabled: true,
          ipv4: [],
          ipv6: [],
          vrf: 'default',
          promiscuous: false,
          subinterfaces: {
            '5': { vlanId: 5, enabled: false, ipv4: [], ipv6: [], vrf: 'default', dot1ad: false },
          },
        },
      }).map((p) => p.name),
    ).toEqual(['a', 'a.5']);
  });
});

describe('Bridging page', () => {
  it(
    'lists bridge domains with live status, opens the drawer with members and the MAC table; nav entry in the interfaces group',
    { timeout: 45_000 },
    async () => {
      const api = installFakeApi('operator');
      withBridging(api);
      await signIn();
      render(app('/interfaces/bridging'));
      expect(await screen.findByRole('heading', { name: 'Bridging' })).toBeTruthy();
      const nav = await screen.findByRole('link', { name: 'Bridging' });
      expect(nav.getAttribute('href')).toBe('/interfaces/bridging');
      const grid = await screen.findByRole('grid', { name: 'Bridge domains' });
      await waitFor(() => expect(within(grid).getByText('lan')).toBeTruthy());
      expect(within(grid).getByText('not in VPP')).toBeTruthy();
      expect(within(grid).getByText('pending')).toBeTruthy();
      fireEvent.click(within(grid).getByText('lan'));
      const members = await screen.findByRole('table', { name: 'Members' });
      expect(within(members).getByText('host-w7l0.100')).toBeTruthy();
      expect(within(members).getByText('pop-1')).toBeTruthy();
      expect(within(members).getByText('BVI')).toBeTruthy();
      const macs = await screen.findByRole('grid', { name: 'MAC table' });
      await waitFor(() => expect(within(macs).getByText('02:00:00:00:70:01')).toBeTruthy());
      expect(
        api.calls.some(
          (c) =>
            c.path === '/api/v1/state/l2/bridge-domains/7001/macs' &&
            c.search.includes('pageSize=25'),
        ),
      ).toBe(true);
      // D-132: the L2 FIB walk is not polled — it runs again only on Refresh
      const macCalls = () =>
        api.calls.filter((c) => c.path === '/api/v1/state/l2/bridge-domains/7001/macs').length;
      const listCalls = () =>
        api.calls.filter((c) => c.path === '/api/v1/state/l2/bridge-domains').length;
      const before = macCalls();
      const listBefore = listCalls();
      await new Promise((r) => setTimeout(r, 6_000)); // longer than the old 5 s MAC-grid poll
      expect(macCalls()).toBe(before);
      expect(listCalls()).toBe(listBefore); // the list's one timer is 60 s
      const drawer = screen.getByRole('grid', { name: 'MAC table' }).closest('.MuiDrawer-paper');
      fireEvent.click(within(drawer as HTMLElement).getByRole('button', { name: 'Refresh' }));
      await waitFor(() => expect(macCalls()).toBe(before + 1));
    },
  );

  it(
    'removing a member sends l2: null for that (sub-)interface through the generic route',
    { timeout: 30_000 },
    async () => {
      const api = installFakeApi('operator');
      withBridging(api);
      let patched: unknown;
      api.on('PATCH /api/v1/config/interfaces', (_r, body) => {
        patched = body;
        return { body: {} };
      });
      await signIn();
      render(app('/interfaces/bridging'));
      const grid = await screen.findByRole('grid', { name: 'Bridge domains' });
      await waitFor(() => expect(within(grid).getByText('lan')).toBeTruthy());
      fireEvent.click(within(grid).getByText('lan'));
      fireEvent.click(await screen.findByRole('button', { name: 'Remove host-w7l0.100' }));
      await waitFor(() =>
        expect(patched).toEqual({ 'host-w7l0': { subinterfaces: { '100': { l2: null } } } }),
      );
      // review #2: a member with the time-range MAC filter leaves the bridge domain but keeps its filter
      patched = undefined;
      fireEvent.click(screen.getByRole('button', { name: 'Remove host-w7l0' }));
      await waitFor(() =>
        expect(patched).toEqual({
          'host-w7l0': {
            l2: { bridgeDomain: null, shg: null, bvi: null, uuFwd: null, tagRewrite: null },
          },
        }),
      );
    },
  );

  it(
    'removing an L2 cross-connect also drops the tag rewrite of its receive side (review #8)',
    { timeout: 30_000 },
    async () => {
      const api = installFakeApi('operator');
      withBridging(api);
      const patches: { path: string; body: unknown }[] = [];
      for (const path of ['routing', 'interfaces']) {
        api.on(`PATCH /api/v1/config/${path}`, (_r, body) => {
          patches.push({ path, body });
          return { body: {} };
        });
      }
      await signIn();
      render(app('/interfaces/bridging'));
      fireEvent.click(
        await screen.findByRole('tab', { name: 'Cross-connects' }, { timeout: 15_000 }),
      );
      fireEvent.click(await screen.findByRole('button', { name: 'Remove host-w7w0' }));
      await waitFor(() =>
        expect(patches).toEqual([
          { path: 'routing', body: { l2: { xconnects: { 'host-w7w0': null } } } },
          { path: 'interfaces', body: { 'host-w7w0': { l2: null } } },
        ]),
      );
    },
  );

  it('renders in Persian (fa, RTL) and lists the cross-connects', { timeout: 30_000 }, async () => {
    const api = installFakeApi('readonly');
    withBridging(api);
    await signIn();
    render(app('/interfaces/bridging'));
    await screen.findByRole('grid', { name: 'Bridge domains' }, { timeout: 15_000 });
    await act(async () => {
      await i18n.changeLanguage('fa');
    });
    expect(
      await screen.findByRole('tab', { name: 'اتصال‌های متقاطع' }, { timeout: 15_000 }),
    ).toBeTruthy();
    expect(screen.getAllByRole('heading', { level: 2 }).map((h) => h.textContent)).toContain(
      'پل‌زنی',
    );
    fireEvent.click(screen.getByRole('tab', { name: 'اتصال‌های متقاطع' }));
    const table = await screen.findByRole('table', { name: 'اتصال‌های متقاطع' });
    expect(within(table).getByText('host-w7w0.200')).toBeTruthy();
  });
});
