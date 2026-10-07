import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, it, vi } from 'vitest';
import i18n from '../../../i18n';
import en from '../../../locales/en/ha-state-sync.json';
import fa from '../../../locales/fa/ha-state-sync.json';
import { StateSyncPanel } from './Panel';
const mock = vi.hoisted(() => ({
  role: 'admin',
  actionsAllowed: true,
  fail: false,
  observationError: '',
  gate: undefined as undefined | (() => Promise<void>),
}));
vi.mock('../../../auth/AuthProvider', () => ({ usePermissions: () => ({ role: mock.role }) }));
vi.mock('../../../api', () => ({
  api: {
    GET: async () => {
      await mock.gate?.();
      if (mock.fail) throw new Error('fixture refresh unavailable');
      return {
        response: new Response(null, { status: 200 }),
        data: {
          owner: 'w18',
          kinds: [
            { kind: 'nat44-ei', supported: true, configured: true, active: true, reason: 'native' },
            { kind: 'ipsec', supported: false, configured: true, active: false, reason: 'rekey' },
          ],
          lastResync: null,
          lastMissedCount: null,
          resyncCount: '0',
          packetCountersAvailable: false,
          actionsAllowed: mock.actionsAllowed,
          observationError: mock.observationError,
          retrievedAt: '',
        },
      };
    },
    POST: async () => ({
      response: new Response(null, { status: 200 }),
      data: { summary: 'done' },
    }),
  },
}));
function mount(client = new QueryClient({ defaultOptions: { queries: { retry: false } } })) {
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <StateSyncPanel />
      </QueryClientProvider>
    </I18nextProvider>,
  );
}
afterEach(() => {
  cleanup();
  mock.role = 'admin';
  mock.actionsAllowed = true;
  mock.fail = false;
  mock.observationError = '';
  mock.gate = undefined;
});
describe('HA state-sync panel', () => {
  it('reports unsupported native IPsec and absent counters honestly', async () => {
    await i18n.changeLanguage('en');
    mount();
    expect(await screen.findByText(en['reason-ipsec'])).toBeInTheDocument();
    expect(screen.getByText(en.counters)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole('button', { name: en.resync })).toBeEnabled());
  });
  it('read-only and non-owner observations cannot enable resync', async () => {
    await i18n.changeLanguage('en');
    mock.role = 'readonly';
    mount();
    await screen.findByText(en['reason-ipsec']);
    expect(screen.getByRole('button', { name: en.resync })).toBeDisabled();
    cleanup();
    mock.role = 'admin';
    mock.actionsAllowed = false;
    mount();
    await screen.findByText(en['reason-ipsec']);
    expect(screen.getByRole('button', { name: en.resync })).toBeDisabled();
  });
  it('has complete Persian translations', () => {
    expect(Object.keys(fa).sort()).toEqual(Object.keys(en).sort());
  });
});

it('hides cached active observations and disables resync after an actual failed TanStack refresh', async () => {
  await i18n.changeLanguage('en');
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  mount(client);
  await waitFor(() => expect(screen.getByRole('button', { name: en.resync })).toBeEnabled());
  expect(screen.getByText(en.active)).toBeInTheDocument();
  mock.fail = true;
  await client.refetchQueries({ queryKey: ['state', 'ha', 'sync'] });
  expect(client.getQueryData(['state', 'ha', 'sync'])).toBeDefined();
  expect(await screen.findByText(en.failed)).toBeInTheDocument();
  expect(screen.getByText(en.stale)).toBeInTheDocument();
  expect(screen.queryByText(en.active)).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: en.resync })).toBeDisabled();
  mock.fail = false;
  await client.refetchQueries({ queryKey: ['state', 'ha', 'sync'] });
  await waitFor(() => expect(screen.getByRole('button', { name: en.resync })).toBeEnabled());
  expect(screen.getByText(en.active)).toBeInTheDocument();
});
it('does not authorize actions from an observation error response', async () => {
  await i18n.changeLanguage('en');
  mock.observationError = 'fixture observation failure';
  mount();
  await screen.findByText(mock.observationError);
  expect(screen.queryByText(en.active)).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: en.resync })).toBeDisabled();
});

it('disables resync while refreshing cached observations and for operators', async () => {
  await i18n.changeLanguage('en');
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  mount(client);
  await waitFor(() => expect(screen.getByRole('button', { name: en.resync })).toBeEnabled());
  let finish!: () => void;
  mock.gate = () =>
    new Promise<void>((resolve) => {
      finish = resolve;
    });
  const refresh = client.refetchQueries({ queryKey: ['state', 'ha', 'sync'] });
  await waitFor(() => expect(screen.getByRole('button', { name: en.resync })).toBeDisabled());
  finish();
  await refresh;
  await waitFor(() => expect(screen.getByRole('button', { name: en.resync })).toBeEnabled());
  cleanup();
  mock.gate = undefined;
  mock.role = 'operator';
  mount();
  await screen.findByText(en.active);
  expect(screen.getByRole('button', { name: en.resync })).toBeDisabled();
});
