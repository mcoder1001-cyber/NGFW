import { RootConfig } from '@ngfw/schema';
import { QueryClient } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';

const doc = RootConfig.parse({ interfaces: { wan0: {}, lan0: {} } });
function app(path = '/system/setup') {
  return (
    <App
      router={createTestRouter([path], { devRoutes: false })}
      streamUrl="ws://127.0.0.1:1/api/v1/stream"
      queryClient={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
    />
  );
}
afterEach(async () => {
  cleanup();
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});
const next = () => fireEvent.click(screen.getByRole('button', { name: 'Next' }));
async function select(label: string, name: string) {
  fireEvent.mouseDown(screen.getByRole('combobox', { name: label }));
  fireEvent.click(within(await screen.findByRole('listbox')).getByRole('option', { name }));
}

describe('setup wizard', () => {
  it('keeps all seven steps local until reviewed final commit and uses confirmation', async () => {
    const fake = installFakeApi();
    await signIn();
    fake.on('GET /api/v1/config', () => ({ body: doc, headers: { 'x-ngfw-revision': '1' } }));
    fake.on('POST /api/v1/config/setup/preview', {
      body: {
        changes: [{ op: 'replace', pointer: '/system/hostname', from: 'ngfw', to: 'router' }],
      },
    });
    fake.on('POST /api/v1/config/setup/stage', { body: { staged: true } });
    fake.on('POST /api/v1/config/validate', {
      body: { ok: true, warnings: [], plan: [], notApplied: [] },
    });
    fake.on('POST /api/v1/config/commit', {
      body: {
        status: 'pending',
        confirmDeadline: new Date(Date.now() + 120000).toISOString(),
        results: [],
        warnings: [],
        notApplied: [],
      },
    });
    render(app());
    await screen.findByRole('heading', { name: 'Setup wizard' });
    next();
    await screen.findByLabelText('Current password');
    fireEvent.change(screen.getByLabelText('Current password'), {
      target: { value: 'NGFW_TEST_PSK_setup_old' },
    });
    fireEvent.change(screen.getByLabelText('New administrator password'), {
      target: { value: 'NGFW_TEST_PSK_setup_new' },
    });
    next();
    await screen.findByLabelText('Hostname');
    fireEvent.change(screen.getByLabelText('Hostname'), { target: { value: 'router' } });
    next();
    await select('WAN interface', 'wan0');
    next();
    await select('LAN interface', 'lan0');
    next();
    await screen.findByText(/Enable outbound NAT/);
    next();
    await screen.findByRole('button', { name: 'Apply with 120-second confirmation' });
    expect(
      fake.calls
        .filter((r) => r.method === 'POST' && r.path.startsWith('/api/v1/config/'))
        .map((r) => r.path),
    ).toEqual(['/api/v1/config/setup/preview']);
    expect(screen.getByText(/"\/system\/hostname"/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Apply with 120-second confirmation' }));
    await screen.findByRole('button', { name: 'Confirm LAN management works' });
    expect(fake.calls.filter((r) => r.path === '/api/v1/config/commit')).toHaveLength(1);
    expect(fake.calls.find((r) => r.path === '/api/v1/config/commit')!.search).toContain(
      'confirm=120',
    );
    expect(fake.calls.find((r) => r.path === '/api/v1/config/setup/stage')!.auth).toBe(
      'Bearer h.p.s',
    );
  });
  it('refuses a weak password before advancing', async () => {
    const fake = installFakeApi();
    await signIn();
    fake.on('GET /api/v1/config', { body: doc });
    render(app());
    await screen.findByRole('heading', { name: 'Setup wizard' });
    next();
    await screen.findByLabelText('Current password');
    next();
    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent('at least 12 characters'),
    );
    expect(fake.calls.some((r) => r.path.includes('/setup/stage'))).toBe(false);
  });
  it('factory login redirects dashboard to setup', async () => {
    const fake = installFakeApi();
    await signIn();
    fake.on('GET /api/v1/config', { body: doc });
    render(app('/'));
    expect(await screen.findByRole('heading', { name: 'Setup wizard' })).toBeInTheDocument();
  });
});
