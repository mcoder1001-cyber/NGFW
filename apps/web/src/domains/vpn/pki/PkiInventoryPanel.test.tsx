import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { createMemoryRouter, RouterProvider } from 'react-router';
import { DomainTabsPage } from '../../DomainTabsPage';
import { vpnTabs } from '../tabs';
import i18n from '../../../i18n';
import { installFakeApi, resetSession } from '../../../test-api';
import { PkiInventoryPanel } from './PkiInventoryPanel';

const PATH = '/api/v1/state/pki';
const secret = 'PRIVATE_DIAGNOSTIC_CANARY';
const publicFacts = {
  subject: 'CN=Gateway',
  issuer: 'CN=Root',
  serial: '01',
  san: [],
  notBefore: null,
  notAfter: '2027-01-01T00:00:00Z',
  daysLeft: 90,
  fingerprint: 'AA',
  keySpec: null,
  isCa: false,
};
const certificate = {
  ...publicFacts,
  name: 'gateway',
  ca: 'root',
  certificateRef: secret,
  privateKeyRef: secret,
  expiryAlertDays: 30,
  expiring: false,
  revokedByCrl: false,
  ocsp: null,
  problems: [],
};
const state = {
  generatedAt: '2026-10-03T12:00:00Z',
  cas: [],
  certificates: [],
  expiry: { lastCheck: null, active: [] },
  agentFiles: { root: secret, unavailable: null, files: [] },
  unsupported: [],
};
function panel() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  return render(
    <QueryClientProvider client={client}>
      <PkiInventoryPanel />
    </QueryClientProvider>,
  );
}
afterEach(async () => {
  await resetSession();
  await i18n.changeLanguage('en');
});

describe('public PKI inventory', () => {
  it.each([
    ['en', 'PKI inventory'],
    ['fa', 'فهرست PKI'],
  ])('opens the mounted PKI tab with preloaded %s labels', async (language, label) => {
    await i18n.changeLanguage(language);
    const fake = installFakeApi('readonly');
    fake.on(`GET ${PATH}`, { body: state });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
    const router = createMemoryRouter(
      [{ path: '/vpn', element: <DomainTabsPage domainKey="vpn" ns="vpn" tabs={vpnTabs} /> }],
      { initialEntries: ['/vpn?tab=pki'] },
    );
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );
    expect(screen.getByRole('tab', { name: label })).toHaveAttribute('aria-selected', 'true');
    await screen.findByText(i18n.t('pkiInventory:empty'));
    expect(fake.calls.every((call) => call.method === 'GET' && call.path === PATH)).toBe(true);
  });

  it('shows accessible loading and empty states using only the public state route', async () => {
    const fake = installFakeApi('readonly');
    fake.on(`GET ${PATH}`, { body: state });
    panel();
    expect(screen.getByRole('status')).toHaveTextContent('Loading PKI inventory');
    expect(
      await screen.findByText('No PKI certificates or authorities configured.'),
    ).toBeInTheDocument();
    expect(fake.calls.every((call) => call.method === 'GET' && call.path === PATH)).toBe(true);
  });

  it('renders revocation and expiry metadata without references or raw diagnostics', async () => {
    const fake = installFakeApi('readonly');
    fake.on(`GET ${PATH}`, {
      body: {
        ...state,
        certificates: [
          { ...certificate, revokedByCrl: true, problems: [secret] },
          { ...certificate, name: 'expiring', expiring: true },
          { ...certificate, name: 'ocsp-revoked', ocsp: { status: 'revoked', error: secret } },
        ],
        agentFiles: { ...state.agentFiles, unavailable: secret },
      },
    });
    const view = panel();
    const table = await screen.findByRole('table', { name: 'Certificates' });
    expect(within(table).getAllByText('Revoked')).toHaveLength(2);
    expect(within(table).getByText('Expiring')).toBeInTheDocument();
    expect(screen.getByText('Agent certificate inventory is unavailable.')).toBeInTheDocument();
    expect(view.container.textContent).not.toContain(secret);
    expect(screen.queryByRole('button', { name: /import|export|sign/i })).not.toBeInTheDocument();
  });

  it('recovers from API errors with an accessible retry without showing server detail', async () => {
    const fake = installFakeApi();
    fake.on(`GET ${PATH}`, { status: 503, body: { title: secret, detail: secret } });
    const view = panel();
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'PKI inventory could not be loaded.',
    );
    expect(view.container.textContent).not.toContain(secret);
    fake.on(`GET ${PATH}`, { body: state });
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await screen.findByText('No PKI certificates or authorities configured.');
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument());
  });

  it('bounds CA and certificate rows together and announces omitted entries', async () => {
    const fake = installFakeApi();
    fake.on(`GET ${PATH}`, {
      body: {
        ...state,
        cas: [
          {
            ...publicFacts,
            name: 'root',
            signingKey: false,
            crl: null,
            problems: [],
            ocspUrl: null,
            certificateRef: secret,
          },
        ],
        certificates: Array.from({ length: 101 }, (_, i) => ({
          ...certificate,
          name: `cert-${i}`,
        })),
      },
    });
    panel();
    await screen.findByText('Showing 100 of 102 entries.');
    const tables = screen.getAllByRole('table');
    expect(
      tables.reduce((count, table) => count + within(table).getAllByRole('row').length - 1, 0),
    ).toBe(100);
    expect(screen.queryByText('cert-99')).not.toBeInTheDocument();
  });

  it('uses precise inventory dates for expired authorities and certificates, preserving revocation priority', async () => {
    const fake = installFakeApi();
    const dates = [
      { name: 'past', notAfter: '2026-10-03T11:59:59Z', status: 'Expired' },
      { name: 'boundary', notAfter: state.generatedAt, status: 'Configured' },
      { name: 'future', notAfter: '2026-10-03T12:00:01Z', status: 'Configured' },
      { name: 'missing', notAfter: null, status: 'Configured' },
    ];
    fake.on(`GET ${PATH}`, {
      body: {
        ...state,
        cas: dates.map(({ name, notAfter }) => ({
          ...publicFacts,
          name: `ca-${name}`,
          notAfter,
          daysLeft: 0,
          signingKey: false,
          crl: null,
          problems: [],
          ocspUrl: null,
          certificateRef: null,
        })),
        certificates: [
          ...dates.map(({ name, notAfter }) => ({
            ...certificate,
            name: `cert-${name}`,
            notAfter,
            daysLeft: 0,
          })),
          {
            ...certificate,
            name: 'expired-expiring',
            notAfter: dates[0]!.notAfter,
            daysLeft: -1,
            expiring: true,
          },
          {
            ...certificate,
            name: 'expired-revoked',
            notAfter: dates[0]!.notAfter,
            daysLeft: -1,
            expiring: true,
            revokedByCrl: true,
          },
        ],
      },
    });
    panel();
    await screen.findByRole('table', { name: 'Certificate authorities' });
    for (const { name, status } of dates) {
      for (const prefix of ['ca', 'cert']) {
        const row = screen.getByRole('row', { name: new RegExp(`${prefix}-${name} `) });
        expect(within(row).getByText(status)).toBeInTheDocument();
      }
    }
    expect(
      within(screen.getByRole('row', { name: /expired-expiring / })).getByText('Expired'),
    ).toBeInTheDocument();
    expect(
      within(screen.getByRole('row', { name: /expired-revoked / })).getByText('Revoked'),
    ).toBeInTheDocument();
  });

  it('provides Persian labels independently of shared locale registration', async () => {
    const fake = installFakeApi();
    fake.on(`GET ${PATH}`, { body: state });
    await i18n.changeLanguage('fa');
    panel();
    expect(
      await screen.findByText('گواهی یا مرجع صدور گواهی پیکربندی نشده است.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'تازه‌سازی' })).toBeInTheDocument();
  });
});
