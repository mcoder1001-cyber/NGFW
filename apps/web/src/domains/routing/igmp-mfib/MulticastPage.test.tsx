import { render, screen, within } from '@testing-library/react';
import { QueryClient } from '@tanstack/react-query';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

function app(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
  return <App router={createTestRouter([path], { devRoutes: false })} streamUrl={STREAM} queryClient={queryClient} />;
}

function withState(api: FakeApi, over: { agentError?: string | null } = {}) {
  const agentError = over.agentError ?? null;
  api.on('GET /api/v1/state/routing/multicast/groups', {
    body: { agentError, retrievedAt: null, groups: agentError ? [] : [{ interface: 'eth0', group: '239.1.1.1', sources: ['10.0.0.5'] }] },
  });
  api.on('GET /api/v1/state/routing/multicast/mroutes', {
    body: { agentError, retrievedAt: null, mroutes: agentError ? [] : [{ vrf: 'default', group: '239.2.2.2', source: '10.0.0.9', accept: 'eth0', forward: ['eth1'], packets: '0', bytes: '0' }] },
  });
  api.on('GET /api/v1/state/routing/multicast/pim-neighbors', {
    body: { agentError, retrievedAt: null, neighbors: [] },
  });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('multicast page', () => {
  it('shows live IGMP groups and the mFIB', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withState(api);
    await signIn();
    render(app('/routing/multicast'));
    expect(
      await screen.findByRole('heading', { level: 2, name: /Multicast/ }, { timeout: 15_000 }),
    ).toBeInTheDocument();
    const groups = await screen.findByRole('table', { name: 'IGMP group memberships' });
    expect(within(groups).getByText('239.1.1.1')).toBeInTheDocument();
    const mroutes = screen.getByRole('table', { name: 'Multicast FIB (mroutes)' });
    expect(within(mroutes).getByText('239.2.2.2')).toBeInTheDocument();
    expect(within(mroutes).getByText('eth1')).toBeInTheDocument();
    expect(screen.getByText('No PIM neighbours.')).toBeInTheDocument();
  });

  it('surfaces the agent error and renders in Persian', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withState(api, { agentError: 'the agent does not support multicast yet' });
    await signIn();
    render(app('/routing/multicast'));
    expect(
      await screen.findByText(/Live multicast state is unavailable/, {}, { timeout: 15_000 }),
    ).toBeInTheDocument();
    await i18n.changeLanguage('fa');
    expect(await screen.findByText(/وضعیت زندهٔ چندپخشی در دسترس نیست/)).toBeInTheDocument();
  });
});
