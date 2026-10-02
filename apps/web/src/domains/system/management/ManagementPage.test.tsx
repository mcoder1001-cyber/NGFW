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
  users: [],
  tls: { certificateRef: 'cert/old', privateKeyRef: 'key/old', minVersion: '1.2' },
  syslog: [],
};
const CANDIDATE = {
  ...RUNNING,
  tls: { certificateRef: 'cert/api', privateKeyRef: 'key/api', minVersion: '1.3' },
};
const STATE = {
  configured: true,
  certificateRef: 'cert/old',
  minVersion: '1.2',
  active: {
    subject: 'CN=vrx.example.test',
    issuer: 'CN=vrx.example.test',
    subjectAltNames: ['DNS:vrx.example.test', 'IP Address:192.0.2.1'],
    serialNumber: '01',
    notBefore: '2026-01-01T00:00:00.000Z',
    notAfter: '2027-01-01T00:00:00.000Z',
    fingerprintSha256: 'AA:BB',
    chainLength: 1,
    daysLeft: 96,
  },
  listener: { enabled: false, port: null },
  loadedRevision: 12,
  loadedAt: '2026-09-27T00:00:00.000Z',
  error: null,
};

afterEach(async () => {
  cleanup();
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

function mock(api: ReturnType<typeof installFakeApi>) {
  api.on('GET /api/v1/config/candidate/management', () => ({ body: CANDIDATE }));
  api.on('GET /api/v1/config/management', () => ({ body: RUNNING }));
  api.on('GET /api/v1/state/management/tls', () => ({ body: STATE }));
}

describe('System → Management (F-management-ui)', () => {
  it('the Management nav entry is available (no "soon")', () => {
    const item = buildNav(domains, { devRoutes: false })
      .flatMap((g) => g.items)
      .find((i) => i.id === 'management');
    expect(item).toMatchObject({ path: '/system/management', available: true });
  });

  it('shows the four tabs; AAA says honestly that it is not built', async () => {
    const api = installFakeApi('admin');
    await signIn();
    mock(api);
    render(app('/system/management?tab=aaa'));
    expect(
      await screen.findByRole('heading', { level: 2, name: 'Management' }),
    ).toBeInTheDocument();
    const tabs = within(screen.getByRole('tablist', { name: 'Management sections' })).getAllByRole(
      'tab',
    );
    expect(tabs.map((t) => t.textContent)).toEqual([
      'Notifications',
      'Users',
      'AAA',
      'API TLS',
      'Remote syslog',
    ]);
    expect(await screen.findByTestId('aaa-not-available')).toHaveTextContent(/not built yet/);
  });

  it('/system/users redirects to the Users tab', async () => {
    const api = installFakeApi('admin');
    await signIn();
    mock(api);
    render(app('/system/users'));
    expect(
      await screen.findByRole('heading', { level: 2, name: 'Management' }),
    ).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Users' })).toHaveAttribute('aria-selected', 'true');
  });

  it('API TLS: shows the loaded certificate (no key), the listener note, and saves the refs as a replace patch', async () => {
    const api = installFakeApi('admin');
    await signIn();
    mock(api);
    api.on('PATCH /api/v1/config/management', () => ({ body: CANDIDATE }));
    render(app('/system/management?tab=tls'));
    expect(await screen.findByTestId('tls-subject')).toHaveTextContent('CN=vrx.example.test');
    expect(screen.getByTestId('tls-sans')).toHaveTextContent(
      'DNS:vrx.example.test, IP Address:192.0.2.1',
    );
    expect(screen.getByTestId('tls-revision')).toHaveTextContent('12');
    expect(screen.getByText(/VRX_HTTPS_PORT is not set/)).toBeInTheDocument();
    expect(await screen.findByText('uncommitted')).toBeInTheDocument();
    const cert = await screen.findByLabelText('Certificate');
    expect(cert).toHaveValue('cert/api');
    fireEvent.change(cert, { target: { value: 'cert/new' } });
    fireEvent.change(screen.getByLabelText('Private key'), { target: { value: 'key/new' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save to candidate' }));
    await waitFor(() => expect(api.calls.some((c) => c.method === 'PATCH')).toBe(true));
    const body = api.calls.find((c) => c.method === 'PATCH')?.body as {
      tls?: Record<string, unknown>;
    };
    expect(body.tls).toEqual({
      certificateRef: 'cert/new',
      privateKeyRef: 'key/new',
      minVersion: '1.3',
    });
  });

  it('API TLS: a mismatched key problem lands on the key field', async () => {
    const api = installFakeApi('admin');
    await signIn();
    mock(api);
    api.on('PATCH /api/v1/config/management', () => ({
      status: 400,
      body: {
        type: 'https://vrx.dev/problems/validation',
        title: 'Validation failed',
        status: 400,
        errors: [
          {
            pointer: '/management/tls/privateKeyRef',
            message: 'the private key does not match the certificate',
          },
        ],
      },
    }));
    render(app('/system/management?tab=tls'));
    fireEvent.click(await screen.findByRole('button', { name: 'Save to candidate' }));
    expect(
      await screen.findByText('the private key does not match the certificate'),
    ).toBeInTheDocument();
  });

  it('API TLS: a load error of the committed certificate is shown', async () => {
    const api = installFakeApi('admin');
    await signIn();
    mock(api);
    api.on('GET /api/v1/state/management/tls', () => ({
      body: { ...STATE, error: 'the certificate expired on 2026-01-01' },
    }));
    render(app('/system/management?tab=tls'));
    expect(await screen.findByTestId('tls-error')).toHaveTextContent(
      /previous one is still in use/,
    );
  });
  it('Notifications: operators can inspect but cannot save configuration', async () => {
    const api = installFakeApi('operator');
    await signIn();
    mock(api);
    api.on('GET /api/v1/state/management/notifications', () => ({
      body: { queued: 0, busy: false, error: null, configuredChannels: 0, deliveries: [] },
    }));
    render(app('/system/management?tab=notifications'));
    expect(await screen.findByRole('button', { name: 'Save' })).toBeDisabled();
    expect(api.calls.some((c) => c.method === 'PATCH')).toBe(false);
  });
});
