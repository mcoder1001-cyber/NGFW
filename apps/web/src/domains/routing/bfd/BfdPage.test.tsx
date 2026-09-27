import { QueryClient } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { installFakeApi, resetSession, signIn } from '../../../test-api';
import { BfdPage, redistributionMatrix } from './BfdPage';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

function app() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  });
  const router = createMemoryRouter([{ path: '/', element: <BfdPage /> }], {
    initialEntries: ['/'],
  });
  return <App router={router} streamUrl={STREAM} queryClient={queryClient} />;
}

const CAND = {
  bfd: {
    sessions: [
      {
        interface: 'GigabitEthernet0/8/0',
        localAddress: '192.0.2.1',
        peerAddress: '192.0.2.2',
        desiredMinTxUs: 100000,
        requiredMinRxUs: 100000,
        detectMultiplier: 3,
        enabled: true,
      },
    ],
  },
  ospf: { vrf: 'default', areas: {}, interfaces: {}, redistribute: { connected: {}, bgp: {} } },
};

afterEach(async () => {
  cleanup();
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('Routing → BFD and redistribution (WEB-4a, unrouted)', () => {
  it('lists BFD sessions and the redistribution matrix', async () => {
    const api = installFakeApi('admin');
    api.on('GET /api/v1/config/candidate/routing', () => ({ body: CAND }));
    api.on('GET /api/v1/config/routing', () => ({ body: CAND }));
    await signIn();
    render(app());
    const row = await screen.findByTestId('bfd-192.0.2.2');
    expect(within(row).getByText('100000 / 100000 × 3')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('tab', { name: 'Redistribution' }));
    expect(within(await screen.findByTestId('redist-ospf')).getAllByText('✓')).toHaveLength(2);
    expect(
      within(screen.getByTestId('redist-rip')).getByText('not configured'),
    ).toBeInTheDocument();
  });

  it('redistributionMatrix', () => {
    expect(redistributionMatrix({ rip: { redistribute: { static: {} } } })).toEqual({
      bgp: null,
      ospf: null,
      isis: null,
      rip: ['static'],
    });
  });
});
