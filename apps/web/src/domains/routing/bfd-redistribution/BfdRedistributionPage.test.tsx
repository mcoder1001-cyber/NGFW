import { QueryClient } from '@tanstack/react-query';
import { cleanup, render, screen } from '@testing-library/react';
import { createMemoryRouter } from 'react-router';
import { afterEach, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { installFakeApi, resetSession, signIn } from '../../../test-api';
import { BfdRedistributionPage, RedistributionPage } from './BfdRedistributionPage';
afterEach(async () => {
  cleanup();
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});
it.each(['en', 'fa'])('renders truthful localized live states in %s', async (lang) => {
  await i18n.changeLanguage(lang);
  const fake = installFakeApi('admin');
  fake.on('GET /api/v1/config/candidate/routing', () => ({ body: {} }));
  fake.on('GET /api/v1/config/routing', () => ({ body: {} }));
  fake.on('GET /api/v1/state/routing/bfd/sessions', () => ({
    body: {
      retrievedAt: null,
      agentError: 'FRR unavailable',
      sessions: [
        {
          engine: 'vpp',
          interface: 'loop1401',
          localAddress: '10.14.1.1',
          peerAddress: '10.14.1.2',
          state: 'down',
          desiredMinTxUs: 300000,
          requiredMinRxUs: 300000,
          detectMultiplier: 3,
          lastFlap: null,
          multihop: false,
        },
      ].flatMap((session) =>
        ['down', 'up', 'init', 'admin-down', 'unknown', 'future-state'].map((state, index) => ({
          ...session,
          state,
          peerAddress: `10.14.1.${index + 2}`,
        })),
      ),
    },
  }));
  await signIn();
  const router = createMemoryRouter([{ path: '/', element: <BfdRedistributionPage /> }]);
  render(
    <App
      router={router}
      streamUrl="ws://127.0.0.1:1/api/v1/stream"
      queryClient={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    />,
  );
  expect(await screen.findByText(i18n.t('bfd-redistribution:states.down'))).toBeInTheDocument();
  for (const state of ['up', 'init', 'admin-down']) {
    expect(screen.getByText(i18n.t(`bfd-redistribution:states.${state}`))).toBeInTheDocument();
  }
  expect(screen.getAllByText(i18n.t('bfd-redistribution:states.unknown'))).toHaveLength(2);
  expect(screen.queryByText('future-state')).not.toBeInTheDocument();
  expect(screen.getByText('FRR unavailable')).toBeInTheDocument();
});

it('shows the observed redistribution VRF and localized empty state', async () => {
  const fake = installFakeApi('admin');
  let edges = [
    {
      source: 'static',
      target: 'bgp',
      vrf: 'blue',
      routeMap: 'BLUE_FILTER',
      routeCount: '12',
      readOnly: false,
    },
  ];
  fake.on('GET /api/v1/state/routing/redistribution', () => ({
    body: { retrievedAt: null, agentError: null, edges },
  }));
  await signIn();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createMemoryRouter([{ path: '/', element: <RedistributionPage /> }]);
  render(<App router={router} streamUrl="ws://127.0.0.1:1/api/v1/stream" queryClient={client} />);
  expect(await screen.findByText('BLUE_FILTER (12) · VRF: blue')).toBeInTheDocument();
  edges = [];
  await client.refetchQueries({ queryKey: ['state', 'redistribution'] });
  expect(await screen.findByText(i18n.t('bfd-redistribution:emptyMatrix'))).toBeInTheDocument();
});

it('renders actual Down session state and backend errors without showing synthetic Up', async () => {
  const fake = installFakeApi('admin');
  fake.on('GET /api/v1/config/candidate/routing', () => ({ body: {} }));
  fake.on('GET /api/v1/config/routing', () => ({ body: {} }));
  fake.on('GET /api/v1/state/routing/bfd/sessions', () => ({
    body: {
      retrievedAt: null,
      agentError: 'FRR unavailable',
      sessions: [
        {
          engine: 'vpp',
          interface: 'loop1401',
          localAddress: '10.14.1.1',
          peerAddress: '10.14.1.2',
          state: 'down',
          desiredMinTxUs: 300000,
          requiredMinRxUs: 300000,
          detectMultiplier: 3,
          lastFlap: null,
          multihop: false,
        },
      ],
    },
  }));
  await signIn();
  const router = createMemoryRouter([{ path: '/', element: <BfdRedistributionPage /> }]);
  render(
    <App
      router={router}
      streamUrl="ws://127.0.0.1:1/api/v1/stream"
      queryClient={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    />,
  );
  expect(await screen.findByText('Down')).toBeInTheDocument();
  expect(screen.getByText('FRR unavailable')).toBeInTheDocument();
  expect(screen.queryByText('Up')).not.toBeInTheDocument();
});
