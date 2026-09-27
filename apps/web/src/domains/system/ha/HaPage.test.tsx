import { QueryClient } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { buildNav } from '../../../nav/nav';
import { domains } from '../../../schema/registry';
import { installFakeApi, resetSession, signIn } from '../../../test-api';
import { HaPage } from './HaPage';
import { haEn, haFa } from './locale';
import { vrrpPatch } from './queries';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

/** The page is UNROUTED (WEB-4b), so tests mount it on a one-route memory router inside the real providers. */
function app() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  });
  const router = createMemoryRouter([{ path: '/', element: <HaPage /> }], {
    initialEntries: ['/'],
  });
  return <App router={router} streamUrl={STREAM} queryClient={queryClient} />;
}

const VR = {
  enabled: true,
  interface: 'GigabitEthernet0/8/0',
  vrId: 10,
  addressFamily: 'ipv4',
  priority: 150,
  advertisementIntervalMs: 1000,
  preempt: true,
  acceptMode: false,
  addresses: ['192.0.2.1'],
  vrf: 'default',
  engine: 'vpp',
  track: [],
};
const CLUSTER = {
  enabled: true,
  nodeName: 'fw-a',
  peers: [{ name: 'fw-b', address: '198.51.100.2' }],
  port: 4370,
  secretRef: 'key/cluster',
  vrf: 'default',
  configSync: true,
  stateSync: { nat: true, ipsec: false, acl: false },
};
const RUNNING = { vrrp: { lan: VR } };
const CANDIDATE = { vrrp: { lan: VR, wan: { ...VR, vrId: 20, priority: 90 } }, cluster: CLUSTER };

function fake() {
  const api = installFakeApi('admin');
  api.on('GET /api/v1/config/candidate/ha', () => ({ body: CANDIDATE }));
  api.on('GET /api/v1/config/ha', () => ({ body: RUNNING }));
  return api;
}

afterEach(async () => {
  cleanup();
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('System → High availability (WEB-4b, routed by F-vrrp-config-sync)', () => {
  it('is reachable from nav at /system/ha', () => {
    const item = buildNav(domains, { devRoutes: false })
      .flatMap((g) => g.items)
      .find((i) => i.id === 'ha');
    expect(item?.available).toBe(true);
    expect(item?.path).toBe('/system/ha');
  });

  it('lists virtual routers with committed / pending status', async () => {
    fake();
    await signIn();
    render(app());
    expect(await screen.findByRole('heading', { level: 2, name: 'High availability' })).toBeInTheDocument();
    const lan = await screen.findByTestId('vrrp-lan');
    await waitFor(() => expect(within(lan).getByText('Committed')).toBeInTheDocument());
    const wan = screen.getByTestId('vrrp-wan');
    expect(within(wan).getByText('Not committed')).toBeInTheDocument();
    expect(within(wan).getByText('90')).toBeInTheDocument();
  });

  it('shows the cluster summary on the Cluster tab', async () => {
    fake();
    await signIn();
    render(app());
    fireEvent.click(await screen.findByRole('tab', { name: 'Cluster' }));
    const c = await screen.findByTestId('ha-cluster');
    await waitFor(() => expect(within(c).getByText('fw-a')).toBeInTheDocument());
    expect(within(c).getByText('fw-b (198.51.100.2)')).toBeInTheDocument();
    expect(within(c).getByText('Not committed')).toBeInTheDocument();
  });

  it('maps a server problem pointer onto the cluster form', async () => {
    const api = fake();
    await signIn();
    api.on('PATCH /api/v1/config/ha', () => ({
      status: 400,
      body: {
        type: 'https://vrx.dev/problems/validation',
        title: 'Validation failed',
        status: 400,
        errors: [{ pointer: '/ha/cluster/port', message: 'port 4370 is already in use' }],
      },
    }));
    render(app());
    fireEvent.click(await screen.findByRole('tab', { name: 'Cluster' }));
    const form = await screen.findByTestId('ha-cluster');
    expect(form).toBeInTheDocument();
    const saves = await screen.findAllByRole('button', { name: 'Save to candidate' });
    fireEvent.click(saves[0]!);
    expect(await screen.findByText(/already in use/)).toBeInTheDocument();
    expect(api.calls.some((x) => x.method === 'PATCH')).toBe(true);
  });
});

describe('ha helpers', () => {
  it('vrrpPatch nulls removed names', () => {
    expect(vrrpPatch({ a: 1, b: 2 }, { b: 3, c: 4 })).toEqual({ a: null, b: 3, c: 4 });
  });
  it('en and fa locales have the same keys', () => {
    const keys = (o: object, p = ''): string[] =>
      Object.entries(o).flatMap(([k, v]) =>
        typeof v === 'string' ? [p + k] : keys(v as object, `${p}${k}.`),
      );
    expect(keys(haFa).sort()).toEqual(keys(haEn).sort());
  });
});
