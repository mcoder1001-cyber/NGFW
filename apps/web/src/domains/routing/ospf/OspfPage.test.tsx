import { QueryClient } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { buildNav } from '../../../nav/nav';
import { domains } from '../../../schema/registry';
import { installFakeApi, resetSession, signIn } from '../../../test-api';
import { igpEn, igpFa } from './locale';
import { OspfPage } from './OspfPage';
import { mergePatch } from './queries';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

/** The page is UNROUTED (WEB-4a), so tests mount it on a one-route memory router inside the real providers. */
function app() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  });
  const router = createMemoryRouter([{ path: '/', element: <OspfPage /> }], {
    initialEntries: ['/'],
  });
  return <App router={router} streamUrl={STREAM} queryClient={queryClient} />;
}

const OSPF = {
  routerId: '10.0.0.1',
  vrf: 'default',
  areas: {
    '0': { type: 'normal', noSummary: false },
    '0.0.0.51': { type: 'stub', noSummary: true },
  },
  interfaces: {
    loop0: { area: '0', passive: true, bfd: false },
    'GigabitEthernet0/8/0': { area: '0.0.0.51', cost: 10, passive: false, bfd: true },
  },
  redistribute: { connected: {} },
  defaultInformationOriginate: 'off',
};

function fake(running: unknown = { ospf: OSPF }) {
  const api = installFakeApi('admin');
  api.on('GET /api/v1/config/candidate/routing', () => ({ body: { static: [], ospf: OSPF } }));
  api.on('GET /api/v1/config/routing', () => ({ body: running }));
  return api;
}

afterEach(async () => {
  cleanup();
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('Routing → OSPF (WEB-4a, routed by F-ospf)', () => {
  it('is reachable from nav at /routing/ospf; BFD is not yet', () => {
    const items = buildNav(domains, { devRoutes: false }).flatMap((g) => g.items);
    expect(items.find((i) => i.id === 'ospf')).toMatchObject({ path: '/routing/ospf', available: true });
    expect(items.find((i) => i.id === 'isis-rip')).toMatchObject({ path: '/routing/isis-rip', available: true });
    for (const id of ['bfd'])
      expect(items.find((i) => i.id === id)?.available).not.toBe(true);
  });

  it('lists areas and interfaces and shows committed state', async () => {
    fake();
    await signIn();
    render(app());
    expect(await screen.findByRole('heading', { level: 2, name: 'OSPF' })).toBeInTheDocument();
    const stub = await screen.findByTestId('ospf-area-0.0.0.51');
    expect(within(stub).getByText('stub')).toBeInTheDocument();
    const ge = screen.getByTestId('ospf-if-GigabitEthernet0/8/0');
    expect(within(ge).getByText('10')).toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId('ospf-status')).toHaveTextContent('Committed'));
  });

  it('removes the protocol with a null merge patch', async () => {
    const api = fake({});
    let body: unknown;
    api.on('PATCH /api/v1/config/routing', (_req, b) => {
      body = b;
      return { body: {} };
    });
    await signIn();
    render(app());
    await waitFor(() =>
      expect(screen.getByTestId('ospf-status')).toHaveTextContent('Not committed'),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Remove from configuration' }));
    await waitFor(() => expect(body).toEqual({ ospf: null }));
  });
});

describe('routing IGP helpers', () => {
  it('mergePatch nulls removed record entries and recurses', () => {
    expect(
      mergePatch(
        { areas: { '0': { type: 'normal' }, '1': { type: 'stub' } }, vrf: 'a', networks: ['x'] },
        { areas: { '0': { type: 'nssa' } }, vrf: 'a', networks: ['y'] },
      ),
    ).toEqual({ areas: { '0': { type: 'nssa' }, '1': null }, vrf: 'a', networks: ['y'] });
  });
  it('en and fa locales have the same keys', () => {
    const keys = (o: object, p = ''): string[] =>
      Object.entries(o).flatMap(([k, v]) =>
        typeof v === 'string' ? [p + k] : keys(v as object, `${p}${k}.`),
      );
    expect(keys(igpFa).sort()).toEqual(keys(igpEn).sort());
  });
});
