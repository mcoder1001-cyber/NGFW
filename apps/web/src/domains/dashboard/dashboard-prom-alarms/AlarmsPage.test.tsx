import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';

/** F-dashboard-prom-alarms: the alarms page (list + acknowledge). */
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

const alarm = (over: Record<string, unknown> = {}) => ({
  id: 1,
  rule: 'wan-down',
  instance: 'wan0',
  metric: 'interface_link_down',
  severity: 'critical',
  state: 'active',
  value: '1',
  threshold: '1',
  message: 'interface_link_down on wan0 = 1 (threshold 1)',
  raisedAt: '2026-09-27T10:00:00.000Z',
  clearedAt: null,
  ackedAt: null,
  ackedBy: null,
  ...over,
});

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('alarms page', () => {
  it('lists active alarms and acknowledges one', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/state/alarms', { body: { total: 1, items: [alarm()] } });
    let acked = 0;
    api.on('POST /api/v1/actions/alarms/1/ack', () => {
      acked++;
      return { body: { acked: true } };
    });
    await signIn();
    render(app('/system/alarms'));
    expect(
      await screen.findByRole('heading', { level: 2, name: 'Alarms' }, { timeout: 15_000 }),
    ).toBeInTheDocument();
    const table = await screen.findByRole('table', { name: 'Alarms' });
    expect(within(table).getByText('wan-down')).toBeInTheDocument();
    expect(within(table).getByText(/interface_link_down on wan0/)).toBeInTheDocument();
    fireEvent.click(within(table).getByRole('button', { name: 'Acknowledge' }));
    await waitFor(() => expect(acked).toBe(1));
  });

  it('shows the empty state and renders in Persian', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/state/alarms', { body: { total: 0, items: [] } });
    await signIn();
    render(app('/system/alarms'));
    expect(await screen.findByText('No alarms.', {}, { timeout: 15_000 })).toBeInTheDocument();
    await i18n.changeLanguage('fa');
    expect(await screen.findByRole('heading', { level: 2, name: 'هشدارها' })).toBeInTheDocument();
  });
});
