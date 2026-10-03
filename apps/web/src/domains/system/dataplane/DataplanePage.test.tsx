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

const RUNNING = { workers: 2, mainCore: 1, pciWhitelist: [], managementPci: [], devices: {} };
const CANDIDATE = { workers: 4, mainCore: 1, pciWhitelist: [], managementPci: [], devices: {} };
const STATE = {
  startupPath: '/etc/vpp/startup.conf',
  startupPresent: true,
  workers: 2,
  corelistWorkers: '',
  mainCore: 1,
  plugins: { 'linux_cp_plugin.so': true },
  onlineCpus: '0-7',
  hugepagesTotalBytes: '2147483648',
  hugepagesFreeBytes: '1073741824',
  error: '',
  retrievedAt: null,
};
const PREVIEW = {
  rendered: 'cpu {\n  main-core 1\n  workers 4\n}\n',
  startupPath: '/etc/vpp/startup.conf',
  diff: '--- /etc/vpp/startup.conf\n+++ rendered\n-  workers 2\n+  workers 4\n',
  changed: true,
  warnings: [],
  sha256: '0'.repeat(64),
  restartRequired: true,
  applyAvailable: false,
};

function fake() {
  const api = installFakeApi('admin');
  api.on('GET /api/v1/config/candidate/dataplane', () => ({ body: CANDIDATE }));
  api.on('GET /api/v1/config/dataplane', () => ({ body: RUNNING }));
  api.on('GET /api/v1/state/dataplane', () => ({ body: STATE }));
  api.on('POST /api/v1/actions/dataplane/preview', () => ({ body: PREVIEW }));
  return api;
}

afterEach(async () => {
  cleanup();
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('System → Dataplane (F-dataplane-ui)', () => {
  it('the Dataplane nav entry is available (no "soon")', () => {
    const item = buildNav(domains, { devRoutes: false })
      .flatMap((g) => g.items)
      .find((i) => i.id === 'dataplane');
    expect(item).toMatchObject({ path: '/system/dataplane', available: true });
  });

  it('shows the restart banner, candidate vs running, the installed file and a disabled apply', async () => {
    const api = fake();
    await signIn();
    render(app('/system/dataplane'));
    expect(await screen.findByTestId('dp-restart-banner')).toHaveTextContent(/data plane restart/);
    const workers = await screen.findByTestId('dp-workers');
    await waitFor(() => expect(within(workers).getByText('2')).toBeInTheDocument());
    expect(within(workers).getByText('4')).toBeInTheDocument();
    expect(within(workers).getByText('Not committed')).toBeInTheDocument();
    const installed = screen.getByTestId('dp-installed');
    await waitFor(() => expect(within(installed).getByText('0-7')).toBeInTheDocument());
    const apply = screen.getByRole('button', { name: 'Apply and restart the data plane' });
    expect(apply).toBeDisabled();
    expect(screen.getByText(/not available yet/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Preview startup settings' }));
    expect(await screen.findByTestId('dp-preview-summary')).toHaveTextContent(
      'Review candidate and running settings',
    );
    expect(screen.queryByTestId('dp-diff')).not.toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/FRR|VPP|strongSwan|\/etc\/vpp/i);
    expect(PREVIEW.diff).toContain('/etc/vpp/startup.conf');
    expect(api.calls.some((c) => c.method === 'POST')).toBe(true);
  });

  it('maps a server problem pointer onto the field', async () => {
    const api = fake();
    await signIn();
    api.on('PATCH /api/v1/config/dataplane', () => ({
      status: 400,
      body: {
        type: 'https://ngfw.dev/problems/validation',
        title: 'Validation failed',
        status: 400,
        errors: [
          {
            pointer: '/dataplane/corelist',
            message: 'workers (4) must equal the corelist length (2)',
          },
        ],
      },
    }));
    render(app('/system/dataplane'));
    fireEvent.click(await screen.findByRole('button', { name: 'Save to candidate' }));
    expect(await screen.findByText(/must equal the corelist length/)).toBeInTheDocument();
  });
});
