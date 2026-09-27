import { QueryClient } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';

/** F-multiwan: the WAN groups page (live member health). */
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

const wanState = {
  agentError: null,
  retrievedAt: '2026-09-27T00:00:00.000Z',
  groups: [
    {
      name: 'internet',
      mode: 'failover',
      active: 'wan0',
      members: [
        {
          interface: 'wan0',
          up: true,
          lossPct: 0,
          latencyMs: 12,
          weight: 1,
          priority: 10,
          since: '2026-09-27T00:00:00.000Z',
        },
        {
          interface: 'wan1',
          up: false,
          lossPct: 100,
          latencyMs: 0,
          weight: 1,
          priority: 20,
          since: null,
        },
      ],
    },
  ],
};

function withWan(api: FakeApi, body: unknown = wanState) {
  api.on('GET /api/v1/state/wan', { body });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('WAN groups page', () => {
  it(
    'shows members with health, loss/latency and the active member',
    { timeout: 60_000 },
    async () => {
      const api = installFakeApi();
      withWan(api);
      await signIn();
      render(app('/routing/wan'));
      expect(
        await screen.findByRole('heading', { level: 2, name: 'WAN groups' }, { timeout: 15_000 }),
      ).toBeInTheDocument();
      const table = await screen.findByRole('table', { name: 'Members of internet' });
      const wan0 = within(table).getByText('wan0').closest('tr')!;
      expect(within(wan0).getByText('up')).toBeInTheDocument();
      expect(within(wan0).getByText('12 ms')).toBeInTheDocument();
      expect(within(wan0).getByText('active')).toBeInTheDocument();
      const wan1 = within(table).getByText('wan1').closest('tr')!;
      expect(within(wan1).getByText('down')).toBeInTheDocument();
      expect(within(wan1).getByText('100%')).toBeInTheDocument();
    },
  );

  it('shows the agent-unavailable notice and renders in Persian', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withWan(api, { agentError: 'agent unreachable', retrievedAt: null, groups: [] });
    await signIn();
    render(app('/routing/wan'));
    expect(
      await screen.findByText(/Live state unavailable/, {}, { timeout: 15_000 }),
    ).toBeInTheDocument();
    await i18n.changeLanguage('fa');
    expect(
      await screen.findByRole('heading', { level: 2, name: 'گروه‌های WAN' }),
    ).toBeInTheDocument();
  });
});
