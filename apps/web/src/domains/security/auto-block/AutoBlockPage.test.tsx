import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient } from '@tanstack/react-query';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';

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

const entry = {
  source: '203.0.113.7/32',
  reason: 'webLogin',
  hits: 5,
  offences: 2,
  origin: 'auto',
  note: '',
  firstSeen: '2026-09-27T12:00:00.000Z',
  blockedAt: '2026-09-27T12:00:00.000Z',
  expiresAt: '2026-09-27T12:15:00.000Z',
};

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('auto-block page', () => {
  it('lists blocked sources and unblocks one', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    let unblocked = 0;
    api.on('GET /api/v1/state/auto-block', () => ({
      body: { items: unblocked > 0 ? [] : [entry] },
    }));
    api.on('POST /api/v1/actions/auto-block/unblock', () => {
      unblocked++;
      return { body: { unblocked: true } };
    });
    await signIn();
    render(app('/firewall/auto-block'));
    expect(
      await screen.findByRole('heading', { level: 2, name: 'Auto-block' }, { timeout: 15_000 }),
    ).toBeInTheDocument();
    const table = await screen.findByRole('table', { name: 'Auto-block' });
    expect(within(table).getByText('203.0.113.7/32')).toBeInTheDocument();
    expect(within(table).getByText('Web login')).toBeInTheDocument();
    expect(within(table).getByText('5')).toBeInTheDocument();
    fireEvent.click(within(table).getByRole('button', { name: 'Unblock' }));
    await waitFor(() => expect(unblocked).toBe(1));
    expect(await screen.findByText('No sources are blocked right now.')).toBeInTheDocument();
  });

  it('blocks a source by hand from the dialog', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/state/auto-block', { body: { items: [] } });
    let sent: unknown;
    api.on('POST /api/v1/actions/auto-block/block', (_req, reqBody) => {
      sent = reqBody;
      return { body: { ...entry, source: '198.51.100.9/32', reason: 'manual', origin: 'manual' } };
    });
    await signIn();
    render(app('/firewall/auto-block'));
    fireEvent.click(await screen.findByRole('button', { name: 'Block by hand' }, { timeout: 15_000 }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText('Source IP'), {
      target: { value: '198.51.100.9' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Block' }));
    await waitFor(() => expect(sent).toMatchObject({ source: '198.51.100.9' }));
  });

  it('shows the empty state in Persian', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/state/auto-block', { body: { items: [] } });
    await signIn();
    render(app('/firewall/auto-block'));
    expect(
      await screen.findByText('No sources are blocked right now.', {}, { timeout: 15_000 }),
    ).toBeInTheDocument();
    await i18n.changeLanguage('fa');
    expect(await screen.findByText('در حال حاضر هیچ مبدأی مسدود نیست.')).toBeInTheDocument();
  });
});
