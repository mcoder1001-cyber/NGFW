import { QueryClient } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../../../test-api';

/** Review M2 (F-unbound-chrony-syslog): the log explorer reads the whole host journal, so only administrators get it. */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';
const qc = () => new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
const ADMIN_ONLY = /available to administrators only/;

function routes(api: FakeApi) {
  api.on('GET /api/v1/config/candidate/management', { body: { syslog: [] } });
  api.on('GET /api/v1/state/syslog', {
    body: {
      running: true,
      configPath: '/etc/rsyslog.d/50-vrx-export.conf',
      targets: [],
      pendingActions: [],
    },
  });
  api.on('GET /api/v1/state/logs', { body: { items: [], total: 0, scanned: 0, truncated: false } });
}

const logsCalls = (api: FakeApi) => api.calls.filter((c) => c.path === '/api/v1/state/logs');

afterEach(async () => {
  await resetSession();
  await i18n.changeLanguage('en');
});

describe('Logging tab — log explorer (review M2)', () => {
  it.each(['readonly', 'operator'] as const)(
    '%s sees the notice and never requests /state/logs',
    async (role) => {
      const api = installFakeApi(role, `w10${role}`);
      routes(api);
      await signIn();
      render(
        <App
          router={createTestRouter(['/services?tab=logging'], { devRoutes: false })}
          streamUrl={STREAM}
          queryClient={qc()}
        />,
      );
      expect(await screen.findByText(ADMIN_ONLY)).toBeInTheDocument();
      await waitFor(() =>
        expect(api.calls.some((c) => c.path === '/api/v1/state/syslog')).toBe(true),
      );
      expect(logsCalls(api)).toEqual([]);
    },
  );

  it('admin gets the explorer and its query', async () => {
    const api = installFakeApi('admin');
    routes(api);
    await signIn();
    render(
      <App
        router={createTestRouter(['/services?tab=logging'], { devRoutes: false })}
        streamUrl={STREAM}
        queryClient={qc()}
      />,
    );
    await waitFor(() => expect(logsCalls(api).length).toBeGreaterThan(0));
    expect(screen.queryByText(ADMIN_ONLY)).toBeNull();
  });
});
