import { QueryClient } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';

const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

function app(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
  return <App router={createTestRouter([path], { devRoutes: false })} streamUrl={STREAM} queryClient={queryClient} />;
}

const VALID = {
  status: 'valid',
  licenseId: 'LIC-1',
  customer: 'ACME',
  issuedAt: '2026-09-01T00:00:00.000Z',
  notBefore: '2026-09-01T00:00:00.000Z',
  expiresAt: '2027-09-01T00:00:00.000Z',
  daysLeft: 341,
  graceDays: 30,
  bound: { machineId: false, serial: true },
  entitlements: { features: ['ipsec', 'ha'], limits: { ipsecTunnels: 50 } },
};

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('System → Licence', () => {
  it('shows status and entitlements; admin uploads a file (PUT with the file JSON)', async () => {
    const api = installFakeApi('admin');
    await signIn();
    let state: Record<string, unknown> = { status: 'community', daysLeft: 0, graceDays: 30, bound: { machineId: false, serial: false }, entitlements: { features: ['wireguard', 'ospf'], limits: { wireguardInterfaces: 2 } } };
    api.on('GET /api/v1/state/license', () => ({ body: state }));
    api.on('PUT /api/v1/system/license', () => {
      state = VALID;
      return { body: VALID };
    });
    render(app('/system/licensing'));
    expect(await screen.findByRole('heading', { level: 2, name: 'Licence' })).toBeInTheDocument();
    expect(await screen.findByText('Community (no licence)')).toBeInTheDocument();
    expect(screen.getByText('WireGuard interfaces (maximum)')).toBeInTheDocument();
    const file = new File([JSON.stringify({ format: 'vrxlic/1', license: { a: 1 }, signature: 'AA==' })], 'x.vrxlic');
    fireEvent.change(screen.getByLabelText('Upload licence file'), { target: { files: [file] } });
    expect(await screen.findByText('Licence installed.')).toBeInTheDocument();
    expect(screen.getByTestId('license-status')).toHaveTextContent('Valid');
    expect(screen.getByText('ACME')).toBeInTheDocument();
    const put = api.calls.find((c) => c.method === 'PUT');
    expect(put?.body).toEqual({ format: 'vrxlic/1', license: { a: 1 }, signature: 'AA==' });
  });

  it('shows the server problem for a tampered file; readonly users see no upload', async () => {
    const api = installFakeApi('admin');
    await signIn();
    api.on('GET /api/v1/state/license', { body: VALID });
    api.on('PUT /api/v1/system/license', { status: 400, body: { type: 'https://vrx.dev/problems/bad-request', title: 'Bad request', status: 400, detail: 'licence signature is invalid' } });
    render(app('/system/licensing'));
    await screen.findByText('ACME');
    fireEvent.change(screen.getByLabelText('Upload licence file'), { target: { files: [new File(['{"format":"vrxlic/1"}'], 'x.vrxlic')] } });
    expect(await screen.findByText(/licence signature is invalid/)).toBeInTheDocument();
    cleanup();
    await resetSession();

    const ro = installFakeApi('readonly', 'ro');
    await signIn();
    ro.on('GET /api/v1/state/license', { body: VALID });
    render(app('/system/licensing'));
    expect((await screen.findAllByText('ACME')).length).toBeGreaterThan(0);
    expect(screen.queryByLabelText('Upload licence file')).toBeNull();
  });

  it('grace banner in the shell on every page (fa/RTL too)', async () => {
    const api = installFakeApi('admin');
    await signIn();
    api.on('GET /api/v1/state/license', { body: { ...VALID, status: 'grace', daysLeft: 12 } });
    render(app('/'));
    expect(await screen.findByTestId('license-banner')).toHaveTextContent('Grace period: 12 day(s) left');
    await i18n.changeLanguage('fa');
    await waitFor(() => expect(screen.getByTestId('license-banner')).toHaveTextContent('دورهٔ مهلت'));
  });
});
