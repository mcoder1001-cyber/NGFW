import { QueryClient } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { buildNav } from '../../../nav/nav';
import { createTestRouter } from '../../../router';
import { domains } from '../../../schema/registry';
import { installFakeApi, resetSession, signIn } from '../../../test-api';

function app(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  });
  return (
    <App
      router={createTestRouter([path], { devRoutes: false })}
      streamUrl="ws://127.0.0.1:1/api/v1/stream"
      queryClient={queryClient}
    />
  );
}

const STATE = {
  captures: [
    {
      id: 'vrx-20260927T100000-1',
      state: 'done',
      interface: 'loop501',
      direction: 'rx,tx',
      bpf: 'icmp',
      startedAt: '2026-09-27T10:00:00.000Z',
      stoppedAt: '2026-09-27T10:00:30.000Z',
      size: '1234',
      packets: '10',
      sha256: 'a'.repeat(64),
      maxPackets: 1000,
      seconds: 30,
      snaplen: 9000,
      reason: 'timeout',
    },
  ],
  maxFiles: 10,
  maxBytes: '524288000',
  trace: { available: false, reason: 'banned by D-128/TD-20' },
  pg: { available: false, reason: 'no stream API' },
};

afterEach(async () => {
  cleanup();
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('Tools → Packet capture (F-capture-trace)', () => {
  it('replaces the Tools "not available" item', () => {
    const items = buildNav(domains, { devRoutes: false }).flatMap((g) => g.items);
    expect(items.find((i) => i.id === 'capture')).toMatchObject({
      path: '/tools/capture',
      available: true,
    });
    expect(items.find((i) => i.id === 'tools')).toBeUndefined();
  });

  it('lists kept files, shows trace/PG as unavailable, starts a capture and maps a server pointer', async () => {
    const api = installFakeApi('admin');
    api.on('GET /api/v1/state/captures', () => ({ body: STATE }));
    api.on('POST /api/v1/actions/capture', () => ({
      status: 400,
      body: {
        type: 'x',
        title: 'Bad request',
        status: 400,
        errors: [{ pointer: '/interface', message: 'not owned' }],
      },
    }));
    await signIn();
    render(app('/tools/capture'));
    expect(await screen.findByText('vrx-20260927T100000-1')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Download' })).toBeInTheDocument();
    fireEvent.change(screen.getByTestId('bpf'), { target: { value: 'host "x"' } });
    expect(screen.getByText(/Only the pcap-filter alphabet/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Start capture' })).toBeDisabled();
    fireEvent.change(screen.getByTestId('bpf'), { target: { value: 'icmp' } });
    fireEvent.click(screen.getByRole('button', { name: 'Start capture' }));
    expect(await screen.findByText('not owned')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('tab', { name: 'Trace' }));
    expect(screen.getByTestId('trace-unavailable')).toHaveTextContent(/D-128/);
    fireEvent.click(screen.getByRole('tab', { name: 'Packet generator' }));
    await waitFor(() =>
      expect(screen.getByTestId('pg-unavailable')).toHaveTextContent(/no stream API/),
    );
  });
});
