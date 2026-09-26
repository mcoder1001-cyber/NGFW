import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import { protoPort, removedCopies, statusOf, type LbVipItem } from './model';
import { LB_POLL_MS } from './queries';

/** F-lb Load balancer tab in jsdom against a scripted stand-in of the API (the real endpoint: docs/status/tasks/F-lb.md). */
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

const web = {
  prefix: '10.2.250.1/32',
  protocol: 'tcp',
  port: 80,
  encap: 'gre4',
  newFlowsTableLength: 1024,
  srcIpSticky: false,
  servers: [
    { address: '10.2.2.10', flushOnDelete: false },
    { address: '10.2.2.11', flushOnDelete: true },
  ],
};
const item = (over: Partial<LbVipItem> = {}): LbVipItem => ({
  name: 'web',
  prefix: '10.2.250.1/32',
  protocol: 'tcp',
  port: 80,
  encap: 'gre4',
  status: 'active',
  applied: true,
  vppEntries: 2,
  vppEncap: 'gre4',
  dscp: 0,
  targetPort: 0,
  servers: [
    { address: '10.2.2.10', inUse: true, inUseSince: 9, configured: true },
    { address: '10.2.2.11', inUse: true, inUseSince: 9, configured: true },
    { address: '10.2.2.9', inUse: false, inUseSince: 3, configured: false },
  ],
  ...over,
});

function withLb(api: FakeApi) {
  api.on('GET /api/v1/config/candidate/services', {
    body: { lb: { vips: { web }, natInterfaces: [] } },
  });
  api.on('GET /api/v1/config/candidate/interfaces', { body: {} });
  api.on('GET /api/v1/state/lb/vips', {
    body: { retrievedAt: '2026-09-25T10:00:00Z', totalVppVips: 2, items: [item()] },
  });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('lb model', () => {
  it('maps the status column, protocol/port and removed copies; never polls faster than 30 s (D-132)', () => {
    expect(statusOf('active')).toBe('up');
    expect(statusOf('no-servers')).toBe('degraded');
    expect(statusOf('missing')).toBe('down');
    expect(statusOf('not-applied')).toBe('adminDown');
    expect(protoPort({ protocol: 'tcp', port: 80 })).toBe('tcp/80');
    expect(protoPort({ protocol: 'any' })).toBe('any');
    expect(removedCopies(item())).toBe(1);
    expect(removedCopies(item({ vppEntries: 0 }))).toBe(0);
    expect(LB_POLL_MS).toBeGreaterThanOrEqual(30_000);
  });
});

describe('load balancer tab', () => {
  it(
    'lists the VIPs with state and the write-only notice; expands the servers; flushes; deletes through a merge patch',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withLb(api);
      let flushed = '';
      api.on('POST /api/v1/actions/lb/vips/web/flush', () => {
        flushed = 'web';
        return { body: { vip: 'lb.vip/10.2.250.1/32/tcp/80' } };
      });
      let patched: unknown;
      api.on('PATCH /api/v1/config/services', (_r, body) => {
        patched = body;
        return { body: { pointer: '/services', before: null, after: null } };
      });
      await signIn();
      render(app('/services?tab=lb'));
      expect(
        await screen.findByRole('tab', { name: 'Load balancer' }, { timeout: 15_000 }),
      ).toBeInTheDocument();
      expect(await screen.findByTestId('lb-writeonly-notice')).toHaveTextContent(/removed/);
      const table = await screen.findByRole('table', { name: 'Virtual IPs' });
      const row = (await within(table).findByText('web')).closest('tr')!;
      expect(within(row).getByText('10.2.250.1/32')).toBeInTheDocument();
      expect(within(row).getByText('tcp/80')).toBeInTheDocument();
      expect(await within(row).findByText('active')).toBeInTheDocument();
      expect(within(row).getByText('2 in use / 2 configured')).toBeInTheDocument();

      fireEvent.click(
        within(row).getByRole('button', { name: 'Show the application servers of web' }),
      );
      const servers = await screen.findByRole('table', { name: 'Application servers of web' });
      expect(within(servers).getByText('10.2.2.9')).toBeInTheDocument();
      expect(within(servers).getByText('removed')).toBeInTheDocument();

      fireEvent.click(within(row).getByRole('button', { name: 'Flush' }));
      expect(
        await screen.findByText('Flow table of web flushed (lb.vip/10.2.250.1/32/tcp/80).'),
      ).toBeInTheDocument();
      expect(flushed).toBe('web');

      fireEvent.click(within(row).getByRole('button', { name: 'Delete' }));
      await waitFor(() => expect(patched).toEqual({ lb: { vips: { web: null } } }));
    },
  );

  it(
    'a readonly user sees the state but cannot flush, edit or delete',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi('readonly', 'ro');
      withLb(api);
      await signIn();
      render(app('/services?tab=lb'));
      const table = await screen.findByRole('table', { name: 'Virtual IPs' }, { timeout: 15_000 });
      const row = (await within(table).findByText('web')).closest('tr')!;
      for (const name of ['Flush', 'Edit', 'Delete'])
        expect(within(row).getByRole('button', { name })).toBeDisabled();
      expect(screen.getByRole('button', { name: 'Add VIP' })).toBeDisabled();
    },
  );

  it('a server-side problem for a VIP lands in the dialog', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withLb(api);
    api.on('PATCH /api/v1/config/services', {
      status: 400,
      body: {
        type: 'https://vrx.dev/problems/validation',
        title: 'Validation failed',
        status: 400,
        detail: 'the configuration is invalid',
        errors: [
          {
            pointer: '/services/lb/vips/web/servers/0/address',
            message: 'encap gre4 needs IPv4 application servers, 2001:db8::1 is IPv6',
          },
        ],
      },
    });
    await signIn();
    render(app('/services?tab=lb'));
    const table = await screen.findByRole('table', { name: 'Virtual IPs' }, { timeout: 15_000 });
    const row = (await within(table).findByText('web')).closest('tr')!;
    fireEvent.click(within(row).getByRole('button', { name: 'Edit' }));
    const dialog = await screen.findByRole('dialog');
    const prefix = await within(dialog).findByLabelText(/^VIP prefix/);
    fireEvent.change(prefix, { target: { value: '10.2.250.5/32' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Save to candidate' }));
    expect(await within(dialog).findByText(/needs IPv4 application servers/)).toBeInTheDocument();
  });
});
