import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import enMpls from '../../../locales/en/mpls-srmpls.json';
import faMpls from '../../../locales/fa/mpls-srmpls.json';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import {
  itemSchema,
  keyedSchema,
  labelRouteRows,
  mergePatchFor,
  mplsSchema,
  pathText,
  pickSchema,
  type MplsConfig,
} from './model';
import { mplsTabs } from './tabs';

/** F-mpls-srmpls screen in jsdom against a scripted stand-in of the API (the real stack: test/topology/mpls-srmpls). */
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

const MPLS: MplsConfig = {
  interfaces: ['loop5001'],
  tables: { '5001': {} },
  labelRoutes: [
    {
      table: 5001,
      label: 50016,
      eos: true,
      paths: [{ nextHop: '10.5.1.2', interface: 'loop5001', outLabels: [50017, 50018], weight: 1 }],
    },
    {
      table: 0,
      label: 50020,
      eos: false,
      paths: [{ interface: 't1', outLabels: [50021], weight: 2 }],
    },
    {
      table: 0,
      label: 50030,
      eos: true,
      payload: 'ip6',
      paths: [{ vrf: 'red', outLabels: [], weight: 1 }],
    },
  ],
  ipBindings: [{ label: 50040, vrf: 'red', prefix: '10.5.40.0/24' }],
  tunnels: {
    t1: {
      paths: [{ nextHop: '10.5.1.2', interface: 'loop5001', outLabels: [50050], weight: 1 }],
      l2Only: false,
    },
  },
  sr: {
    policies: { '50100': { segmentLists: [{ labels: [50101, 50102], weight: 1 }], spray: false } },
    steering: [{ vrf: 'default', prefix: '10.5.60.0/24', bsid: 50100, vpnLabel: 50061 }],
  },
};

function withMpls(api: FakeApi, mpls: MplsConfig | undefined = MPLS) {
  api.on('GET /api/v1/config/candidate/routing', {
    body: { static: [], ...(mpls ? { mpls } : {}) },
  });
  api.on('GET /api/v1/config/candidate/interfaces', { body: { loop5001: {} } });
  api.on('GET /api/v1/config/candidate/vrfs', { body: { red: { id: 5010 } } });
  api.on('GET /api/v1/state/routing/mpls/tunnels', {
    body: {
      tables: [{ tableId: 0, name: 'vrx:0' }],
      items: [
        {
          name: 't1',
          interface: 'mpls-tunnel0',
          swIfIndex: 9,
          tunnelIndex: 0,
          l2Only: false,
          multicast: false,
          owned: true,
          paths: [],
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

describe('mpls model', () => {
  it('takes its forms from the one schema (routing.mpls) and keeps the tab list a plain array', () => {
    expect(Object.keys(mplsSchema().properties ?? {})).toEqual([
      'interfaces',
      'tables',
      'labelRoutes',
      'ipBindings',
      'tunnels',
      'sr',
    ]);
    expect(Object.keys(itemSchema('labelRoutes').properties ?? {})).toEqual([
      'table',
      'label',
      'eos',
      'payload',
      'paths',
    ]);
    expect(Object.keys(itemSchema('sr', 'steering').properties ?? {})).toEqual([
      'vrf',
      'prefix',
      'bsid',
      'vpnLabel',
    ]);
    expect(Object.keys(pickSchema('interfaces', 'tables').properties ?? {})).toEqual([
      'interfaces',
      'tables',
    ]);
    const keyed = keyedSchema(itemSchema('tunnels'), { type: 'string' });
    expect(Object.keys(keyed.properties ?? {})).toEqual(['name', 'paths', 'l2Only']);
    expect(mplsTabs.map((t) => t.id)).toEqual(['interfaces', 'routes', 'tunnels', 'sr', 'fib']);
  });

  it('writes a merge patch that removes deleted record entries and replaces arrays whole', () => {
    const next: MplsConfig = { ...MPLS, tunnels: {}, labelRoutes: MPLS.labelRoutes.slice(1) };
    expect(mergePatchFor(MPLS, next)).toEqual({
      labelRoutes: MPLS.labelRoutes.slice(1),
      tunnels: { t1: null },
    });
    expect(mergePatchFor(MPLS, structuredClone(MPLS))).toBeUndefined();
  });

  it('describes paths and rows in words', () => {
    const t = (k: string, o?: Record<string, unknown>) => `${k}${o ? JSON.stringify(o) : ''}`;
    expect(
      pathText({ nextHop: '10.5.1.2', interface: 'loop5001', outLabels: [50017], weight: 3 }, t),
    ).toBe('10.5.1.2 path.via{"interface":"loop5001"} path.push{"labels":"50017"} ×3');
    expect(pathText({ vrf: 'red', outLabels: [], weight: 1 }, t)).toBe('path.lookup{"vrf":"red"}');
    expect(labelRouteRows(MPLS, t).map((r) => [r.table, r.label, r.eos])).toEqual([
      [5001, 50016, 'eos.yes'],
      [0, 50020, 'eos.no'],
      [0, 50030, 'eos.yes'],
    ]);
  });

  it('has the same keys in en and fa', () => {
    const keys = (o: object, p = ''): string[] =>
      Object.entries(o).flatMap(([k, v]) =>
        v !== null && typeof v === 'object' ? keys(v as object, `${p}${k}.`) : [`${p}${k}`],
      );
    expect(keys(faMpls).sort()).toEqual(keys(enMpls).sort());
  });
});

describe('MPLS screen', () => {
  it(
    'shows the label routes of the candidate and saves an edited route as a merge patch of /routing',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withMpls(api);
      let patched: unknown;
      api.on('PATCH /api/v1/config/routing', (_r, body) => {
        patched = body;
        return { body: { pointer: '/routing', before: null, after: null } };
      });
      await signIn();
      render(app('/routing/mpls?tab=routes'));
      const grid = await screen.findByRole(
        'grid',
        { name: 'Static label routes' },
        { timeout: 15_000 },
      );
      await waitFor(() => expect(within(grid).getByText('50016')).toBeInTheDocument());
      expect(within(grid).getByText(/push \[50017 50018\]/)).toBeInTheDocument();
      fireEvent.click(within(grid).getByText('50020'));
      const dialog = await screen.findByRole('dialog', { name: 'Label 50020 in MPLS table 0' });
      fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));
      await waitFor(() => expect(patched).toBeDefined());
      expect(patched).toEqual({
        mpls: { labelRoutes: [MPLS.labelRoutes[0], MPLS.labelRoutes[2]] },
      });
    },
  );

  it(
    'lists tunnels with their live state and SR-MPLS policies; the FIB tab reads one page on demand',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withMpls(api);
      api.on('GET /api/v1/state/routing/mpls/fib', {
        body: {
          page: 1,
          pageSize: 50,
          total: 1,
          tableId: 0,
          tables: [
            { tableId: 0, name: 'vrx:0' },
            { tableId: 5001, name: 'w5:5001' },
          ],
          items: [
            {
              label: 50020,
              eos: false,
              paths: [
                {
                  type: 'normal',
                  proto: 'ip4',
                  interface: 't1',
                  tableId: 0,
                  outLabels: [50021],
                  weight: 2,
                  preference: 0,
                },
              ],
            },
          ],
        },
      });
      await signIn();
      const { unmount } = render(app('/routing/mpls?tab=tunnels'));
      const tunnels = await screen.findByRole(
        'grid',
        { name: 'MPLS tunnels' },
        { timeout: 15_000 },
      );
      await waitFor(() => expect(within(tunnels).getByText('present')).toBeInTheDocument());
      expect(within(tunnels).getByText('mpls-tunnel0 (#9)')).toBeInTheDocument();
      unmount();
      const sr = render(app('/routing/mpls?tab=sr'));
      const policies = await screen.findByRole(
        'grid',
        { name: 'SR-MPLS policies' },
        { timeout: 15_000 },
      );
      await waitFor(() =>
        expect(within(policies).getByText('[50101 50102]×1')).toBeInTheDocument(),
      );
      sr.unmount();
      render(app('/routing/mpls?tab=fib'));
      const fib = await screen.findByRole('grid', { name: 'MPLS FIB' }, { timeout: 15_000 });
      await waitFor(() => expect(within(fib).getByText('50020')).toBeInTheDocument());
      const reads = api.calls.filter((c) => c.path === '/api/v1/state/routing/mpls/fib');
      expect(reads.length).toBe(1);
      expect(reads[0]?.search).toContain('table=0');
    },
  );

  it('renders in Persian (RTL) with the feature strings', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withMpls(api);
    await signIn();
    render(app('/routing/mpls?tab=interfaces'));
    await screen.findByRole('tab', { name: 'Interfaces & tables' }, { timeout: 15_000 });
    await act(async () => {
      await i18n.changeLanguage('fa');
    });
    expect(
      await screen.findByRole('tab', { name: faMpls.tab.interfaces }, { timeout: 15_000 }),
    ).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: faMpls.tab.routes })).toBeInTheDocument();
    expect(screen.getAllByRole('heading', { level: 2 }).map((h) => h.textContent)).toContain(
      faMpls.title,
    );
  });
});
