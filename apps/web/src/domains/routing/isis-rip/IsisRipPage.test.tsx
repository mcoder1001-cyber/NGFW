import { QueryClient } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { buildNav } from '../../../nav/nav';
import { domains } from '../../../schema/registry';
import { installFakeApi, resetSession, signIn } from '../../../test-api';
import { IsisRipPage } from './IsisRipPage';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

function app() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  });
  const router = createMemoryRouter([{ path: '/', element: <IsisRipPage /> }], {
    initialEntries: ['/'],
  });
  return <App router={router} streamUrl={STREAM} queryClient={queryClient} />;
}

const CAND = {
  isis: {
    net: '49.0001.1921.6800.1001.00',
    level: 'level-2',
    vrf: 'default',
    interfaces: { loop0: { passive: true, metric: 20, circuitType: 'level-2', bfd: false } },
  },
};

afterEach(async () => {
  cleanup();
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('Routing → IS-IS and RIP (WEB-4a, routed by F-isis-rip)', () => {
  it('is reachable from nav at /routing/isis-rip', () => {
    const items = buildNav(domains, { devRoutes: false }).flatMap((g) => g.items);
    expect(items.find((i) => i.id === 'isis-rip')).toMatchObject({ path: '/routing/isis-rip', available: true });
  });

  it('shows IS-IS interfaces and an unconfigured RIP tab', async () => {
    const api = installFakeApi('admin');
    api.on('GET /api/v1/config/candidate/routing', () => ({ body: CAND }));
    api.on('GET /api/v1/config/routing', () => ({ body: CAND }));
    await signIn();
    render(app());
    const row = await screen.findByTestId('isis-if-loop0');
    expect(within(row).getByText('20')).toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId('isis-status')).toHaveTextContent('Committed'));
    fireEvent.click(screen.getByRole('tab', { name: 'RIP' }));
    expect(await screen.findByText(/RIP is not configured/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Remove from configuration' })).toBeNull();
  });
});
