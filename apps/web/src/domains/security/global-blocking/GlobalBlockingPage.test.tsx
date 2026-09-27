import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import faGb from '../../../locales/fa/global-blocking.json';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';

/** F-global-blocking screen in jsdom against a scripted stand-in of the API. */
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

const feed = {
  enabled: true,
  source: {
    kind: 'url',
    url: 'https://feeds.example.net/bad.txt',
    refreshSec: 3600,
    verifyTls: true,
  },
  allInterfaces: true,
  interfaces: [],
  direction: 'both',
  protectHost: true,
  log: false,
  entries: ['192.0.2.7/32', '198.51.100.0/24'],
};
const status = {
  maxEntries: 200000,
  totalEntries: 2,
  countersError: null,
  lists: [
    {
      name: 'feed',
      description: null,
      enabled: true,
      pending: null,
      source: { kind: 'url', url: feed.source.url, refreshSec: 3600 },
      allInterfaces: true,
      interfaces: [],
      direction: 'both',
      protectHost: true,
      entries: 2,
      runningEntries: 2,
      fetch: {
        lastFetchAt: '2026-09-27T08:00:00.000Z',
        lastResult: 'failed',
        lastError: 'HTTP 503',
        entries: 2,
      },
      nextRefreshAt: '2026-09-27T09:00:00.000Z',
      hits: { dataplanePackets: 1234567, dataplaneBytes: 99, hostPackets: 42 },
    },
  ],
};
const preview = (dryRun: boolean) => ({
  list: 'feed',
  dryRun,
  lines: 4,
  entries: 2,
  added: 1,
  removed: 1,
  unchanged: 1,
  normalised: 0,
  collapsed: 0,
  invalidCount: 1,
  invalid: [{ line: 3, text: 'not-an-ip', reason: 'not an IPv4/IPv6 address or prefix' }],
  addedSample: ['203.0.113.9/32'],
  removedSample: ['198.51.100.0/24'],
  staged: !dryRun,
});

function withGb(api: FakeApi) {
  api.on('GET /api/v1/security/global-blocking', { body: status });
  api.on('GET /api/v1/config/candidate/acl', { body: { globalBlocking: { lists: { feed } } } });
  api.on('GET /api/v1/config/candidate/interfaces', { body: { loop1: {} } });
  api.on('POST /api/v1/security/global-blocking/lists/feed/import', (req) => ({
    body: preview(new URL(req.url).search.includes('dryRun=true')),
  }));
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('global blocking screen', () => {
  it(
    'lists block lists with source, last download, counters; imports a file with a preview first',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withGb(api);
      await signIn();
      render(app('/firewall/global-blocking'));
      expect(
        await screen.findByRole(
          'heading',
          { level: 2, name: 'Global blocking' },
          { timeout: 15_000 },
        ),
      ).toBeInTheDocument();
      const table = await screen.findByRole('table', { name: 'Block lists' });
      const row = within(table).getByText('feed').closest('tr')!;
      expect(within(row).getByText('https://feeds.example.net/bad.txt')).toBeInTheDocument();
      expect(within(row).getByText('refreshed every 1h')).toBeInTheDocument();
      expect(within(row).getByText('failed — last good list kept')).toBeInTheDocument();
      expect(within(row).getByText('1,234,567 packets')).toBeInTheDocument();
      expect(within(row).getByText('42 to the box')).toBeInTheDocument();
      expect(screen.getByText('2 of 200,000 entries used over all lists')).toBeInTheDocument();

      fireEvent.click(within(row).getByRole('button', { name: 'Import file' }));
      const dialog = await screen.findByRole('dialog', { name: 'Import a file into feed' });
      const text = '# feed\n192.0.2.7\nnot-an-ip\n203.0.113.9\n';
      fireEvent.change(dialog.querySelector('input[type=file]')!, {
        target: { files: [new File([text], 'bad.txt', { type: 'text/plain' })] },
      });
      expect(await within(dialog).findByText('bad.txt')).toBeInTheDocument();
      expect(within(dialog).getByRole('button', { name: 'Stage' })).toBeDisabled();
      fireEvent.click(within(dialog).getByRole('button', { name: 'Check' }));
      const result = await within(dialog).findByRole('region', { name: 'Preview' });
      expect(
        within(result).getByText(/4 lines → 2 entries: 1 added, 1 removed, 1 invalid lines\./),
      ).toBeInTheDocument();
      const invalid = within(result).getByRole('table', { name: 'Invalid lines' });
      expect(within(invalid).getByText('not-an-ip')).toBeInTheDocument();
      const imports = api.calls.filter((c) => c.path.endsWith('/import'));
      expect(imports[0]).toMatchObject({ method: 'POST', search: '?dryRun=true', body: text });

      fireEvent.click(within(dialog).getByRole('button', { name: 'Stage' }));
      expect(
        await within(dialog).findByText('2 entries staged in the candidate; commit to apply them.'),
      ).toBeInTheDocument();
      await waitFor(() =>
        expect(api.calls.filter((c) => c.path.endsWith('/import'))[1]?.search).toBe(
          '?dryRun=false',
        ),
      );
    },
  );

  it(
    'shows the page in Persian (RTL) with the URL kept left-to-right',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi('readonly', 'ro');
      withGb(api);
      await signIn();
      render(app('/firewall/global-blocking'));
      await screen.findByRole(
        'heading',
        { level: 2, name: 'Global blocking' },
        { timeout: 15_000 },
      );
      await act(async () => {
        await i18n.changeLanguage('fa');
      });
      expect(
        await screen.findByRole('heading', { level: 2, name: faGb.title }, { timeout: 15_000 }),
      ).toBeInTheDocument();
      const url = await screen.findByText('https://feeds.example.net/bad.txt');
      expect(url).toHaveAttribute('dir', 'ltr');
      expect(screen.getByRole('button', { name: faGb.add })).toBeDisabled();
    },
  );
});
