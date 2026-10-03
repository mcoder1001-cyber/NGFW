import { QueryClient } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { buildNav } from '../../../nav/nav';
import { createTestRouter } from '../../../router';
import { domains } from '../../../schema/registry';
import { installFakeApi, resetSession, signIn } from '../../../test-api';

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

const RUNNING = {
  hostname: 'ngfw',
  timezone: 'UTC',
  banner: {},
  dns: { servers: [], searchDomains: [], vrf: 'default' },
};
const CANDIDATE = {
  hostname: 'ngfw-a',
  timezone: 'Asia/Tehran',
  banner: { login: 'Authorised access only' },
  dns: { servers: ['10.10.53.1'], searchDomains: ['lab.example'], vrf: 'default' },
};

afterEach(async () => {
  cleanup();
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('System → Identity (F-system-identity)', () => {
  it('the System nav entry is available (no "soon")', () => {
    const item = buildNav(domains, { devRoutes: false })
      .flatMap((g) => g.items)
      .find((i) => i.id === 'system');
    expect(item).toMatchObject({ path: '/system', available: true });
  });

  it('shows the candidate next to running, marks uncommitted rows and saves a merge patch', async () => {
    const api = installFakeApi('admin');
    await signIn();
    api.on('GET /api/v1/state/system', {
      body: {
        agent: { reachable: true }, api: { version: 'test' }, sync: { state: 'in-sync' },
        identity: {
          hostname: 'installed-router',
          timezone: 'UTC',
          uptimeSeconds: 123,
          resolverStatus: 'unavailable',
          configuredNameServers: [],
          configuredSearchDomains: [],
          observedNameServers: [],
          errors: ['resolver-runtime'],
          retrievedAt: null,
        },
      },
    });
    api.on('GET /api/v1/config/candidate/system', () => ({ body: CANDIDATE }));
    api.on('GET /api/v1/config/system', () => ({ body: RUNNING }));
    api.on('PATCH /api/v1/config/system', () => ({ body: CANDIDATE }));
    render(app('/system'));
    expect(await screen.findByRole('heading', { level: 2, name: 'System' })).toBeInTheDocument();
    expect(await screen.findByText('Installed hostname: installed-router')).toBeInTheDocument();
    expect(screen.getByText('Some observations are unavailable.')).toBeInTheDocument();
    const host = await screen.findByTestId('sys-hostname');
    await waitFor(() => expect(within(host).getByText('ngfw')).toBeInTheDocument());
    expect(within(host).getByText('ngfw-a')).toBeInTheDocument();
    expect(within(host).getByText('Not committed')).toBeInTheDocument();
    expect(within(screen.getByTestId('sys-servers')).getByText('10.10.53.1')).toBeInTheDocument();
    const field = await screen.findByLabelText('Hostname');
    expect(field).toHaveValue('ngfw-a');
    fireEvent.change(field, { target: { value: 'ngfw-b' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save to candidate' }));
    await waitFor(() => expect(api.calls.some((c) => c.method === 'PATCH')).toBe(true));
    const body = api.calls.find((c) => c.method === 'PATCH')?.body as { hostname?: string };
    expect(body.hostname).toBe('ngfw-b');
  });

  it('maps a server problem pointer onto the field', async () => {
    const api = installFakeApi('admin');
    await signIn();
    api.on('GET /api/v1/config/candidate/system', () => ({ body: CANDIDATE }));
    api.on('GET /api/v1/config/system', () => ({ body: RUNNING }));
    api.on('PATCH /api/v1/config/system', () => ({
      status: 400,
      body: {
        type: 'https://ngfw.dev/problems/validation',
        title: 'Validation failed',
        status: 400,
        errors: [
          {
            pointer: '/system/banner/login',
            message: 'banner contains the control character U+001B',
          },
        ],
      },
    }));
    render(app('/system'));
    fireEvent.click(await screen.findByRole('button', { name: 'Save to candidate' }));
    expect(await screen.findByText(/U\+001B/)).toBeInTheDocument();
    await waitFor(() => expect(within(screen.getByRole('region', { name: 'Observed system state' })).getByText('Unavailable')).toBeInTheDocument());
  });
  it('formats observed uptime with Persian digits when enabled', async () => {
    localStorage.setItem(
      'ngfw.ui.settings',
      JSON.stringify({ mode: 'light', lang: 'fa', persianDigits: true, dense: true }),
    );
    await i18n.changeLanguage('fa');
    const api = installFakeApi('admin');
    await signIn();
    api.on('GET /api/v1/config/candidate/system', { body: CANDIDATE });
    api.on('GET /api/v1/config/system', { body: RUNNING });
    api.on('GET /api/v1/state/system', {
      body: {
        agent: { reachable: true },
        api: { version: 'test' },
        sync: { state: 'in-sync' },
        identity: {
          hostname: 'installed-router',
          timezone: 'UTC',
          uptimeSeconds: 123,
          resolverStatus: 'slot-only',
          configuredNameServers: [],
          configuredSearchDomains: [],
          observedNameServers: [],
          errors: [],
          retrievedAt: null,
        },
      },
    });
    render(app('/system'));
    expect(await screen.findByText('زمان روشن بودن میزبان (ثانیه): ۱۲۳')).toBeInTheDocument();
  });
});
