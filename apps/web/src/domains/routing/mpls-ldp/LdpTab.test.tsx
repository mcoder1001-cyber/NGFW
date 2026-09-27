import { QueryClientProvider, QueryClient } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, it } from 'vitest';
import i18n from '../../../i18n';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';
import { LdpTab } from './LdpTab';

function harness() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
  return (
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <LdpTab />
      </I18nextProvider>
    </QueryClientProvider>
  );
}

function withState(api: FakeApi, over: { agentError?: string | null } = {}) {
  const agentError = over.agentError ?? null;
  api.on('GET /api/v1/state/routing/mpls/ldp/neighbors', {
    body: { agentError, retrievedAt: null, neighbors: agentError ? [] : [{ lsrId: '10.0.0.2', address: '10.0.0.2', state: 'OPERATIONAL', uptimeSec: 42 }] },
  });
  api.on('GET /api/v1/state/routing/mpls/ldp/bindings', {
    body: { agentError, retrievedAt: null, total: agentError ? 0 : 1, bindings: agentError ? [] : [{ prefix: '10.0.0.2/32', localLabel: 16000, peer: '10.0.0.2', remoteLabel: 3, inUse: true }] },
  });
  api.on('GET /api/v1/state/routing/mpls/ldp/sync', {
    body: { agentError, retrievedAt: null, lastSyncAt: null, installed: agentError ? 0 : 1, conflicts: 0, lastError: '', source: agentError ? '' : 'ldp-bindings' },
  });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('LDP tab', () => {
  it('shows neighbours, bindings and the sync status', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withState(api);
    await signIn();
    render(harness());
    const nb = await screen.findByRole('table', { name: 'Neighbours' }, { timeout: 15_000 });
    expect(within(nb).getByText('OPERATIONAL')).toBeInTheDocument();
    const lib = await screen.findByRole('table', { name: 'Label bindings (LIB)' });
    expect(within(lib).getByText('10.0.0.2/32')).toBeInTheDocument();
    expect(within(lib).getByText('16000')).toBeInTheDocument();
    expect(await screen.findByText('Synced')).toBeInTheDocument();
  });

  it('surfaces the agent error and renders in Persian', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    withState(api, { agentError: 'the agent does not support LDP yet' });
    await signIn();
    render(harness());
    expect(await screen.findByText(/Live LDP state is unavailable/, {}, { timeout: 15_000 })).toBeInTheDocument();
    await i18n.changeLanguage('fa');
    expect(await screen.findByText(/وضعیت زندهٔ LDP در دسترس نیست/)).toBeInTheDocument();
  });
});
