import { QueryClient } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';

/** WEB-dashboard in jsdom against the scripted stand-in of the API (unit level; no live stream in jsdom). */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';
const WAIT = { timeout: 15_000 };

function app() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  });
  return (
    <App
      router={createTestRouter(['/'], { devRoutes: false })}
      streamUrl={STREAM}
      queryClient={queryClient}
    />
  );
}

describe('dashboard', () => {
  afterEach(async () => {
    await resetSession();
    await i18n.changeLanguage('en');
  });

  it('shows health, host resources, interface counts, configuration and recent events from the state API', async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/state/system', {
      body: {
        api: { version: '1.2.3', startedAt: '', wsClients: 0 },
        agent: { reachable: true, vppConnected: true, vppVersion: '26.06', degraded: false },
        runningRevision: 42,
        pendingCommit: null,
        sync: { state: 'in-sync', reason: '', txnId: null, since: '' },
      },
    });
    api.on('GET /api/v1/state/interfaces', {
      body: {
        items: [
          {
            name: 'wan',
            kind: 'interface',
            parent: null,
            state: { name: 'wan', vppName: 'wan', adminUp: true, linkUp: true },
            config: null,
            running: null,
            counters: null,
            hasPendingChange: false,
          },
          {
            name: 'lan',
            kind: 'interface',
            parent: null,
            state: { name: 'lan', vppName: 'lan', adminUp: true, linkUp: false },
            config: null,
            running: null,
            counters: null,
            hasPendingChange: false,
          },
          {
            name: 'dmz',
            kind: 'interface',
            parent: null,
            state: { name: 'dmz', vppName: 'dmz', adminUp: false, linkUp: false },
            config: null,
            running: null,
            counters: null,
            hasPendingChange: false,
          },
        ],
      },
    });
    api.on('GET /api/v1/state/host', {
      body: {
        hostname: 'edge-01',
        uptimeSec: 3 * 86_400 + 4 * 3_600,
        cpu: { cores: 8, model: 'x', usagePct: 42, load: [0.5, 0.4, 0.3] },
        memory: { totalBytes: 16 * 2 ** 30, usedBytes: 4 * 2 ** 30, availableBytes: 12 * 2 ** 30 },
        hugepages: { total: 512, free: 128, sizeBytes: 2 ** 21 },
        disks: [{ mount: '/', totalBytes: 100 * 2 ** 30, usedBytes: 93 * 2 ** 30 }],
        history: [
          { at: 1_000, cpuPct: 40, memUsedPct: 25 },
          { at: 6_000, cpuPct: 42, memUsedPct: 25 },
        ],
        sampledAt: '',
      },
    });
    api.on('GET /api/v1/state/events', {
      body: {
        total: 1,
        items: [
          {
            id: 1,
            ts: new Date().toISOString(),
            severity: 'warning',
            subsystem: 'agent',
            code: 'degraded',
            message: 'Agent reconnected to VPP',
            data: null,
          },
        ],
      },
    });
    await signIn();
    render(app());
    expect(
      await screen.findByRole('heading', { level: 2, name: 'Dashboard' }, WAIT),
    ).toBeInTheDocument();
    expect(await screen.findByText('API 1.2.3', {}, WAIT)).toBeInTheDocument();
    expect(screen.getByText('Engine connected')).toBeInTheDocument();
    // one interface is down, so the banner asks for attention instead of claiming everything is fine
    expect(await screen.findByText('1 of 3 interfaces up', {}, WAIT)).toBeInTheDocument();
    expect(screen.getByTestId('banner-state')).toHaveTextContent('Attention needed');
    expect(
      screen.getByRole('img', {
        name: '1 up, 1 down, 1 administratively down, 0 not present in the engine',
      }),
    ).toBeInTheDocument();
    expect(await screen.findByText(/Revision 42/, {}, WAIT)).toHaveTextContent(
      'Up 3 d 4 h · Revision 42 · Engine 26.06',
    );
    // host resources: CPU 42 %, memory 4 of 16 GiB, root disk 93 % (overloaded threshold 92 %), packet memory 75 %
    expect(await screen.findByText('edge-01', {}, WAIT)).toBeInTheDocument();
    expect(screen.getByRole('img', { name: 'Device CPU' })).toHaveTextContent('42%');
    expect(screen.getByText('4 GiB of 16 GiB')).toBeInTheDocument();
    expect(screen.getByRole('img', { name: 'Storage' })).toHaveTextContent('93%');
    expect(screen.getByRole('img', { name: 'Packet memory (hugepages)' })).toHaveTextContent('75%');
    expect(screen.getByText('768 MiB of 1 GiB in use')).toBeInTheDocument();
    const events = screen.getByRole('region', { name: 'Recent events' });
    expect(
      await within(events).findByText('Agent reconnected to the engine', {}, WAIT),
    ).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/VPP/);
    // no counters without the stream: the chart says so instead of drawing an empty plot
    expect(screen.getByRole('region', { name: 'Traffic, all interfaces' })).toHaveTextContent(
      /Live stream is|Collecting samples/,
    );
    expect(screen.getByTestId('throughput-total')).toHaveTextContent('—');
  }, 60_000);

  it('reports an unreachable API and agent without inventing data', async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/state/system', {
      status: 503,
      body: { type: 'about:blank', title: 'Unavailable', status: 503 },
    });
    await signIn();
    render(app());
    expect(await screen.findByText('API unreachable', {}, WAIT)).toBeInTheDocument();
    expect(screen.getByText('Agent unreachable')).toBeInTheDocument();
    expect(screen.getByText('Engine not connected')).toBeInTheDocument();
    expect(screen.getByTestId('banner-state')).toHaveTextContent('Management API unreachable');
  }, 60_000);
});
