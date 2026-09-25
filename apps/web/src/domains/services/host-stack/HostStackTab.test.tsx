import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
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

const HS = {
  enabled: true,
  namespaces: { 'w1-app': { vrf: 'default' } },
  sessionRules: [{ tag: 'w1-deny', scope: 'global', transport: 'tcp', local: '10.1.1.0/24', localPort: 3190, remote: '10.1.2.0/24', action: 'deny' }],
};

function withHostStack(api: FakeApi) {
  api.on('GET /api/v1/state/host-stack', {
    body: { sessionEnabled: true, sessionDetail: '', namespaces: ['w1-app'], ruleCount: 1, ruleCountTotal: 3, rules: [], retrievedAt: null },
  });
  api.on('GET /api/v1/config/candidate/services', { body: { hostStack: HS } });
  api.on('PATCH /api/v1/config/services', { body: { ok: true } });
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('Services → Host stack (F-host-stack)', () => {
  it('shows the T3 banner, live state, namespaces and rules; deleting a rule patches services.hostStack', async () => {
    const api = installFakeApi();
    withHostStack(api);
    await signIn();
    render(app('/services?tab=host-stack'));
    expect(await screen.findByTestId('host-stack-banner')).toHaveTextContent('Advanced (T3)');
    expect(await screen.findByText('Session layer on')).toBeInTheDocument();
    expect(screen.getByText('1 own rules / 3 in VPP')).toBeInTheDocument();
    const rules = await screen.findByRole('table', { name: 'Session rules' });
    expect(within(rules).getByText('w1-deny')).toBeInTheDocument();
    expect(within(screen.getByRole('table', { name: 'App namespaces' })).getByText('w1-app')).toBeInTheDocument();
    fireEvent.click(within(rules).getByRole('button', { name: 'Delete w1-deny' }));
    await waitFor(() => expect(api.calls.some((c) => c.method === 'PATCH' && c.path === '/api/v1/config/services')).toBe(true));
    const patch = api.calls.find((c) => c.method === 'PATCH')?.body as { hostStack: { sessionRules: unknown[] } };
    expect(patch.hostStack.sessionRules).toEqual([]);
  });

  it('renders in Persian', async () => {
    const api = installFakeApi();
    withHostStack(api);
    await signIn();
    render(app('/services?tab=host-stack'));
    await screen.findByText('Session layer on');
    await act(async () => {
      await i18n.changeLanguage('fa');
    });
    expect(await screen.findByText('لایه نشست روشن')).toBeInTheDocument();
  });
});
