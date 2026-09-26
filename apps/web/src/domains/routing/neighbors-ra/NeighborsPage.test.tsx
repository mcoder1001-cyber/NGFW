import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import { interfaceFormSchema, localizeSchema } from '../../interfaces/model';
import { toQuery } from './queries';
import { neighborsSchema } from './StaticNeighborsForm';

/** F-neighbors-ra screen in jsdom against a scripted stand-in of the API (unit level; the real stack: docs/status/tasks/F-neighbors-ra.md). */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';
/** Host load can make jsdom renders slow (P08 5204b81): explicit waits, generous per-test timeouts. */
const WAIT = { timeout: 15_000 };
const SLOW = 120_000;

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

const row = (extra: Record<string, unknown>) => ({
  interface: 'host-w9l0',
  ip: '10.9.1.7',
  mac: '02:00:00:00:09:07',
  family: 'ipv4',
  state: 'dynamic',
  noFibEntry: false,
  ageSec: 3.5,
  vrf: 'default',
  tableId: 0,
  ...extra,
});

function withNeighbors(api: FakeApi) {
  api.on('GET /api/v1/state/neighbors', {
    body: {
      page: 1,
      pageSize: 25,
      total: 2,
      retrievedAt: '2026-09-24T18:00:00Z',
      items: [
        row({}),
        row({
          ip: '10.9.1.50',
          mac: '02:00:00:00:09:50',
          state: 'static',
          ageSec: 0,
          noFibEntry: true,
        }),
      ],
    },
  });
  api.on('GET /api/v1/state/interfaces', {
    body: {
      items: [
        {
          name: 'host-w9l0',
          kind: 'interface',
          parent: null,
          state: { name: 'host-w9l0' },
          config: null,
          running: null,
          counters: null,
          hasPendingChange: false,
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

describe('toQuery', () => {
  const req = {
    page: 0,
    pageSize: 25,
    sort: [],
    filter: [],
    filterLogic: 'and' as const,
    quickFilter: [],
  };
  it('maps the grid request and the filters onto the API query (1-based page, server-side sort)', () => {
    expect(toQuery(req, {})).toEqual({ page: 1, pageSize: 25 });
    expect(
      toQuery(
        {
          ...req,
          page: 2,
          pageSize: 50,
          sort: [{ field: 'ageSec', dir: 'desc' }],
          quickFilter: ['02:00', '09'],
        },
        { family: 'ipv6', state: 'dynamic', interface: 'host-w9l0', vrf: 'w9red' },
      ),
    ).toEqual({
      page: 3,
      pageSize: 50,
      sort: 'age',
      dir: 'desc',
      search: '02:00 09',
      family: 'ipv6',
      state: 'dynamic',
      interface: 'host-w9l0',
      vrf: 'w9red',
    });
    expect(toQuery({ ...req, sort: [{ field: 'noFibEntry', dir: 'asc' }] }, {})).toEqual({
      page: 1,
      pageSize: 25,
    }); // not sortable server-side
  });
});

describe('Neighbours screen', () => {
  it(
    'lists the live table, is in the routing nav, and flushes after a confirm (audited server-side)',
    async () => {
      const api = installFakeApi('admin');
      withNeighbors(api);
      api.on('POST /api/v1/actions/arp-flush', {
        body: { deleted: 3, interfaces: 2, summary: 'deleted 3', lines: [] },
      });
      await signIn();
      render(app('/routing/neighbors'));
      expect(await screen.findByRole('heading', { name: 'Neighbours' }, WAIT)).toBeInTheDocument();
      const grid = await screen.findByRole('grid', { name: 'Neighbour table' }, WAIT);
      expect(await within(grid).findByText('10.9.1.50', {}, WAIT)).toBeInTheDocument();
      expect(within(grid).getByText('02:00:00:00:09:07')).toBeInTheDocument();
      expect(within(grid).getByText('Static')).toBeInTheDocument();
      expect(within(grid).getByText('no FIB entry')).toBeInTheDocument();
      expect(within(grid).getByText('3.5 s')).toBeInTheDocument();
      const q = api.calls.find((c) => c.path === '/api/v1/state/neighbors');
      expect(q?.search).toBe('?page=1&pageSize=25');
      expect(screen.getByRole('link', { name: /^Neighbours/ })).toHaveAttribute(
        'href',
        '/routing/neighbors',
      );

      fireEvent.click(screen.getByRole('button', { name: 'Flush…' }));
      const dlg = await screen.findByRole('dialog', { name: 'Flush learned entries' }, WAIT);
      fireEvent.click(within(dlg).getByRole('button', { name: 'Flush' }));
      expect(
        await screen.findByText('Deleted 3 learned entries on 2 interfaces.', {}, WAIT),
      ).toBeInTheDocument();
      const post = api.calls.find((c) => c.path === '/api/v1/actions/arp-flush');
      expect(post).toMatchObject({ method: 'POST', body: {} });
    },
    SLOW,
  );

  it(
    'shows the problem when the flush fails and keeps the dialog open',
    async () => {
      const api = installFakeApi('admin');
      withNeighbors(api);
      api.on('POST /api/v1/actions/arp-flush', {
        status: 400,
        body: {
          type: 'https://vrx.dev/problems/bad-request',
          title: 'Bad request',
          status: 400,
          detail: 'agent: interface "loop301" is tagged "w3:loop301"',
        },
      });
      await signIn();
      render(app('/routing/neighbors'));
      fireEvent.click(await screen.findByRole('button', { name: 'Flush…' }, WAIT));
      const dlg = await screen.findByRole('dialog', { name: 'Flush learned entries' }, WAIT);
      fireEvent.click(within(dlg).getByRole('button', { name: 'Flush' }));
      expect(await within(dlg).findByText(/is tagged "w3:loop301"/, {}, WAIT)).toBeInTheDocument();
    },
    SLOW,
  );

  it(
    'a readonly user can read the table but not flush',
    async () => {
      const api = installFakeApi('readonly', 'ro');
      withNeighbors(api);
      await signIn();
      render(app('/routing/neighbors'));
      expect(await screen.findByRole('button', { name: 'Flush…' }, WAIT)).toBeDisabled();
    },
    SLOW,
  );

  it(
    'static entries: a schema-driven form that writes the candidate without phantom DAD/limits',
    async () => {
      const api = installFakeApi('admin');
      withNeighbors(api);
      api.on('GET /api/v1/config/candidate/routing', {
        body: { static: [], policy: { prefixLists: {}, routeMaps: {} } },
      });
      api.on('PATCH /api/v1/config/routing', (_req, body) => ({
        body: { pointer: '/routing', before: null, after: body },
      }));
      await signIn();
      render(app('/routing/neighbors?tab=static'));
      expect(await screen.findByText(/Static ARP\/ND entries/, {}, WAIT)).toBeInTheDocument();
      expect(screen.getByText('Duplicate address detection')).toBeInTheDocument();
      fireEvent.click(screen.getByRole('button', { name: 'Save to candidate' }));
      await waitFor(() => expect(api.calls.some((c) => c.method === 'PATCH')).toBe(true), WAIT);
      const patch = api.calls.find((c) => c.method === 'PATCH');
      expect(patch?.body).toEqual({ neighbors: { static: [] } }); // untouched optional objects (limits, DAD) are not written
      expect(await screen.findByText(/Saved to the candidate/, {}, WAIT)).toBeInTheDocument();
    },
    SLOW,
  );

  it('fa: Persian strings, and the interface drawer localises the per-interface RA/proxy fields', async () => {
    await i18n.changeLanguage('fa');
    expect(i18n.t('neighbors-ra:title')).toBe('همسایه‌ها');
    const localized = localizeSchema(interfaceFormSchema(), (k, o) =>
      i18n.t(`interfaces:${k}`, o ?? {}),
    );
    const props = localized.properties as Record<
      string,
      { title?: string; 'x-vrx-ui'?: { group?: string } }
    >;
    expect(props['ipv6Ra']).toMatchObject({
      title: 'اعلان‌های مسیریاب IPv6',
      'x-vrx-ui': { group: 'IPv6 RA و proxy ARP/ND' },
    });
    expect(props['proxyArp']?.title).toBe('Proxy ARP');
    expect(props['proxyNd']?.title).toBe('آدرس‌های proxy ND (آزمایشی)');
    // no existing interfaces key was overwritten by the merge
    expect(i18n.t('interfaces:group.addressing')).toBe('آدرس‌دهی');
    await i18n.changeLanguage('en');
    expect(i18n.t('interfaces:group.neighbors-ra')).toBe('IPv6 RA, proxy ARP/ND');
  });

  it('the form edits exactly routing.neighbors of the one schema', () => {
    const s = neighborsSchema();
    expect(Object.keys(s.properties ?? {})).toEqual(['static', 'ipv4Limits', 'ipv6Limits', 'dad']);
  });
});
