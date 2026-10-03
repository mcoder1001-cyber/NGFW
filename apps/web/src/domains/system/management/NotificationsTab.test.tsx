import { VrxThemeProvider } from '@ngfw/ui-kit';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import i18n from '../../../i18n';
import { installFakeApi } from '../../../test-api';
import NotificationsTab from './NotificationsTab';

vi.mock('../../../auth/AuthProvider', () => ({ usePermissions: () => ({ role: 'admin' }) }));

const channel = (name: string, enabled = true) => ({
  name,
  enabled,
  vrf: 'default',
  type: 'webhook',
  webhook: { url: 'https://example.test/notify', secretRef: 'token/notify' },
});
function view() {
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    >
      <VrxThemeProvider mode="light" lang={i18n.language} dir={i18n.language.startsWith('fa') ? 'rtl' : 'ltr'}>
        <NotificationsTab />
      </VrxThemeProvider>
    </QueryClientProvider>,
  );
}
afterEach(async () => {
  cleanup();
  await i18n.changeLanguage('en');
  vi.unstubAllGlobals();
});

describe('notification loading accessibility', () => {
  it.each([
    ['en', 'Loading active notification channels'],
    ['fa', 'در حال دریافت کانال‌های فعال اعلان'],
  ])('names the running-channel loading indicator in %s and replaces it with an error', async (language, label) => {
    const api = installFakeApi('admin');
    api.on('GET /api/v1/config/candidate/management', () => ({
      body: { notifications: { channels: [], rules: [] } },
    }));
    api.on('GET /api/v1/config/management', () => ({
      status: 503,
      body: { type: 'about:blank', title: 'Running configuration unavailable', status: 503 },
    }));
    api.on('GET /api/v1/state/management/notifications', () => ({
      body: { queued: 0, deliveries: [] },
    }));
    const fetch = globalThis.fetch;
    let release!: () => void;
    const delayed = new Promise<void>((resolve) => { release = resolve; });
    vi.stubGlobal('fetch', async (request: Request) => {
      if (new URL(request.url).pathname === '/api/v1/config/management') await delayed;
      return fetch(request);
    });
    await i18n.changeLanguage(language);
    view();
    try {
      expect(await screen.findByRole('progressbar', { name: label })).toBeInTheDocument();
    } finally {
      await act(async () => { release(); });
    }
    await waitFor(() => expect(screen.queryByRole('progressbar', { name: label })).not.toBeInTheDocument());
    expect(await screen.findByText(/Running configuration unavailable/)).toBeInTheDocument();
  });
});

describe('notification test channel selection', () => {
  it('offers enabled running channels when the candidate adds and removes channels', async () => {
    const api = installFakeApi('admin');
    api.on('GET /api/v1/config/candidate/management', () => ({
      body: {
        notifications: { channels: [channel('unsaved')], rules: [] },
      },
    }));
    api.on('GET /api/v1/config/management', () => ({
      body: {
        notifications: { channels: [channel('active'), channel('disabled', false)], rules: [] },
      },
    }));
    api.on('GET /api/v1/state/management/notifications', () => ({
      body: { queued: 0, deliveries: [] },
    }));
    api.on('POST /api/v1/actions/management/notifications/active/test', () => ({
      body: { queued: true },
    }));
    view();
    const send = await screen.findByRole('button', { name: /active/ });
    expect(screen.queryByRole('button', { name: /unsaved/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /disabled/ })).not.toBeInTheDocument();
    fireEvent.click(send);
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === 'POST' && c.path.endsWith('/active/test'))).toBe(
        true,
      ),
    );
  });
  it('does not fall back to candidate channels if running configuration fails', async () => {
    const api = installFakeApi('admin');
    api.on('GET /api/v1/config/candidate/management', () => ({
      body: {
        notifications: { channels: [channel('unsaved')], rules: [] },
      },
    }));
    api.on('GET /api/v1/config/management', () => ({
      status: 503,
      body: {
        type: 'about:blank',
        title: 'Running configuration unavailable',
        status: 503,
      },
    }));
    api.on('GET /api/v1/state/management/notifications', () => ({
      body: { queued: 0, deliveries: [] },
    }));
    view();
    expect(await screen.findByText(/Running configuration unavailable/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /unsaved/ })).not.toBeInTheDocument();
  });
});
