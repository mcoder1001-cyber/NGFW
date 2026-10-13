import { RootConfig } from '@ngfw/schema';
import { QueryClient } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';

function app() {
  return (
    <App
      router={createTestRouter(['/system/setup'], { devRoutes: false })}
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
const next = () => fireEvent.click(screen.getByRole('button', { name: i18n.t('setup:next') }));
async function reachWan() {
  await screen.findByRole('heading', { name: i18n.t('setup:title') });
  next();
  await screen.findByLabelText(i18n.t('setup:current'));
  fireEvent.change(screen.getByLabelText(i18n.t('setup:current')), {
    target: { value: 'NGFW_TEST_PSK_regression_old' },
  });
  fireEvent.change(screen.getByLabelText(i18n.t('setup:newPassword')), {
    target: { value: 'NGFW_TEST_PSK_regression_new' },
  });
  next();
  await screen.findByLabelText(i18n.t('setup:hostname'));
  next();
  fireEvent.click(await screen.findByRole('checkbox', { name: i18n.t('setup:configureWan') }));
  await screen.findByRole('combobox', { name: i18n.t('setup:wan') });
}
async function select(label: string, name: string) {
  await waitFor(() =>
    expect(screen.getByRole('combobox', { name: label })).not.toHaveAttribute(
      'aria-disabled',
      'true',
    ),
  );
  fireEvent.mouseDown(screen.getByRole('combobox', { name: label }));
  fireEvent.click(within(await screen.findByRole('listbox')).getByRole('option', { name }));
}

describe('setup wizard independent network-picker regression', () => {
  it.each(['en', 'fa'])(
    'offers reference-only PPPoE in %s and clears credentials when changing modes',
    async (lang) => {
      const fake = installFakeApi();
      await signIn();
      fake.on('GET /api/v1/config', {
        body: RootConfig.parse({ interfaces: { wan0: {}, lan0: {} } }),
      });
      render(app());
      await screen.findByRole('heading', { name: 'Setup wizard' });
      if (lang === 'fa') {
        await select('Language', 'Persian');
        await waitFor(() => expect(i18n.language).toBe('fa'));
      }
      await reachWan();
      await select(i18n.t('setup:wan'), 'wan0');
      await select(i18n.t('setup:wanMode'), i18n.t('setup:pppoe'));
      next();
      expect(await screen.findByText(i18n.t('setup:pppoeRequired'))).toBeInTheDocument();
      fireEvent.change(screen.getByLabelText(i18n.t('setup:pppoeUsername')), {
        target: { value: 'isp-user' },
      });
      fireEvent.change(screen.getByLabelText(i18n.t('setup:pppoePasswordRef')), {
        target: { value: 'plaintext-password' },
      });
      next();
      expect(await screen.findByText(i18n.t('setup:pppoeRequired'))).toBeInTheDocument();
      fireEvent.change(screen.getByLabelText(i18n.t('setup:pppoePasswordRef')), {
        target: { value: 'password/isp' },
      });
      await select(i18n.t('setup:wanMode'), i18n.t('setup:dhcp'));
      expect(screen.queryByLabelText(i18n.t('setup:pppoePasswordRef'))).not.toBeInTheDocument();
      await select(i18n.t('setup:wanMode'), i18n.t('setup:pppoe'));
      expect(screen.getByLabelText(i18n.t('setup:pppoePasswordRef'))).toHaveValue('');
      expect(
        fake.calls.some((r) => r.method === 'POST' && r.path.startsWith('/api/v1/config/')),
      ).toBe(false);
    },
  );

  it('prefills an unconfigured WAN address and gateway from live observations', async () => {
    const fake = installFakeApi();
    await signIn();
    fake.on('GET /api/v1/config', { body: RootConfig.parse({}) });
    fake.on('GET /api/v1/state/interfaces', {
      body: {
        items: [
          {
            name: 'uplink',
            kind: 'interface',
            state: {
              swIfIndex: 4,
              managed: false,
              vrf: 'default',
              type: 'dpdk',
              ipv4: ['192.0.2.2/24'],
            },
            physical: null,
          },
          {
            name: 'internal',
            kind: 'interface',
            state: { swIfIndex: 5, managed: false, vrf: 'default', type: 'dpdk', ipv4: [] },
            physical: null,
          },
        ],
      },
    });
    fake.on('GET /api/v1/state/routes', {
      body: {
        items: [
          {
            prefix: '0.0.0.0/0',
            vrf: 'default',
            nextHops: [{ interface: 'uplink', address: '192.0.2.1' }],
          },
        ],
      },
    });
    render(app());
    await reachWan();
    await waitFor(() =>
      expect(screen.getByLabelText('WAN address/prefix')).toHaveValue('192.0.2.2/24'),
    );
    expect(screen.getByLabelText('WAN gateway')).toHaveValue('192.0.2.1');
    expect(screen.getByRole('combobox', { name: 'WAN interface' })).toHaveTextContent('uplink');
  });

  it('prefills static WAN and retains administrator edits after returning to the step', async () => {
    const fake = installFakeApi();
    await signIn();
    fake.on('GET /api/v1/config', {
      body: RootConfig.parse({
        interfaces: { uplink: { ipv4: ['203.0.113.2/24'] }, lan0: {} },
        routing: {
          static: [
            { prefix: '0.0.0.0/0', nextHops: [{ interface: 'uplink', address: '203.0.113.1' }] },
          ],
        },
      }),
    });
    fake.on('GET /api/v1/state/interfaces', { body: { items: [] } });
    fake.on('GET /api/v1/state/routes', { body: { items: [] } });
    render(app());
    await reachWan();
    await waitFor(() =>
      expect(screen.getByLabelText('WAN address/prefix')).toHaveValue('203.0.113.2/24'),
    );
    expect(screen.getByRole('combobox', { name: 'WAN interface' })).toHaveTextContent('uplink');
    expect(screen.getByLabelText('WAN gateway')).toHaveValue('203.0.113.1');
    fireEvent.change(screen.getByLabelText('WAN gateway'), { target: { value: '203.0.113.254' } });
    next();
    await screen.findByText(i18n.t('setup:defaultsHelp'));
    fireEvent.click(screen.getByRole('button', { name: 'Back' }));
    expect(await screen.findByLabelText('WAN gateway')).toHaveValue('203.0.113.254');
    expect(screen.queryByText('LAN interface')).not.toBeInTheDocument();
  });

  it('retains configured choices when live state fails and retry loads new interfaces', async () => {
    const fake = installFakeApi();
    await signIn();
    fake.on('GET /api/v1/config', {
      body: RootConfig.parse({ interfaces: { configuredWan: {}, configuredLan: {} } }),
    });
    fake.on('GET /api/v1/state/interfaces', {
      status: 503,
      body: { title: 'Unavailable', status: 503 },
    });
    render(app());
    await reachWan();
    expect(await screen.findByText(i18n.t('setup:interfacesFailed'))).toBeInTheDocument();
    await select(i18n.t('setup:wan'), 'configuredWan');
    fake.on('GET /api/v1/state/interfaces', {
      body: {
        items: [
          {
            name: 'discoveredLan',
            kind: 'interface',
            state: { swIfIndex: 8, managed: false, vrf: 'default', type: 'dpdk' },
            physical: null,
          },
        ],
      },
    });
    fireEvent.click(screen.getByRole('button', { name: i18n.t('setup:retryInterfaces') }));
    await waitFor(() =>
      expect(screen.queryByText(i18n.t('setup:interfacesFailed'))).not.toBeInTheDocument(),
    );
    fireEvent.mouseDown(screen.getByRole('combobox', { name: i18n.t('setup:wan') }));
    const list = await screen.findByRole('listbox');
    expect(
      within(list)
        .getAllByRole('option')
        .map((option) => option.textContent),
    ).toEqual(['configuredLan', 'configuredWan', 'discoveredLan']);
    fireEvent.click(within(list).getByRole('option', { name: 'discoveredLan' }));
    expect(
      fake.calls.filter((r) => r.path === '/api/v1/state/interfaces').length,
    ).toBeGreaterThanOrEqual(2);
    expect(
      fake.calls.some((r) => r.method === 'POST' && r.path.startsWith('/api/v1/config/')),
    ).toBe(false);
  });
  it('warns on partial observation failure and retry preserves configured choices', async () => {
    const fake = installFakeApi();
    await signIn();
    fake.on('GET /api/v1/config', {
      body: RootConfig.parse({ interfaces: { configuredWan: {}, configuredLan: {} } }),
    });
    fake.on('GET /api/v1/state/interfaces', {
      body: {
        items: [],
        dataplaneStatus: 'unavailable',
        observationErrors: [{ source: 'live', message: 'Agent unavailable' }],
      },
    });
    render(app());
    await reachWan();
    expect(await screen.findByText(i18n.t('setup:interfacesFailed'))).toBeInTheDocument();
    await select(i18n.t('setup:wan'), 'configuredWan');
    fake.on('GET /api/v1/state/interfaces', { body: { items: [], observationErrors: [] } });
    fireEvent.click(screen.getByRole('button', { name: i18n.t('setup:retryInterfaces') }));
    await waitFor(() =>
      expect(screen.queryByText(i18n.t('setup:interfacesFailed'))).not.toBeInTheDocument(),
    );
    expect(screen.getByRole('combobox', { name: i18n.t('setup:wan') })).toHaveTextContent(
      'configuredWan',
    );
    next();
    await screen.findByText(i18n.t('setup:defaultsHelp'));
    expect(screen.queryByRole('combobox', { name: 'LAN interface' })).not.toBeInTheDocument();
    expect(
      fake.calls.filter((r) => r.path === '/api/v1/state/interfaces').length,
    ).toBeGreaterThanOrEqual(2);
    expect(
      fake.calls.some((r) => r.method === 'POST' && r.path.startsWith('/api/v1/config/')),
    ).toBe(false);
  });
  it('shows empty-state guidance without exposing undefined schema validation', async () => {
    const fake = installFakeApi();
    await signIn();
    fake.on('GET /api/v1/config', { body: RootConfig.parse({}) });
    fake.on('GET /api/v1/state/interfaces', { body: { items: [] } });
    render(app());
    await reachWan();
    expect(await screen.findByText(i18n.t('setup:noInterfaces'))).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole('button', { name: i18n.t('setup:next') })).toBeEnabled(),
    );
    next();
    expect(await screen.findByText(i18n.t('setup:selectInterface'))).toBeInTheDocument();
    expect(
      screen.queryByText(/expected string|received undefined|invalid_type/),
    ).not.toBeInTheDocument();
    expect(
      fake.calls.some((r) => r.method === 'POST' && r.path.startsWith('/api/v1/config/')),
    ).toBe(false);
  });
  it('renders Persian WAN selection errors and proceeds without a LAN step', async () => {
    const fake = installFakeApi();
    await signIn();
    fake.on('GET /api/v1/config', { body: RootConfig.parse({}) });
    fake.on('GET /api/v1/state/interfaces', {
      body: {
        items: [
          {
            name: 'wanNic',
            kind: 'interface',
            state: { swIfIndex: 4, managed: false, vrf: 'default', type: 'dpdk' },
            physical: null,
          },
          {
            name: 'lanNic',
            kind: 'interface',
            state: { swIfIndex: 5, managed: false, vrf: 'default', type: 'dpdk' },
            physical: null,
          },
        ],
      },
    });
    render(app());
    await screen.findByRole('heading', { name: 'Setup wizard' });
    await select('Language', 'Persian');
    await waitFor(() => expect(i18n.language).toBe('fa'));
    await reachWan();
    await waitFor(() =>
      expect(screen.getByRole('button', { name: i18n.t('setup:next') })).toBeEnabled(),
    );
    next();
    expect(await screen.findByText('پیش از ادامه یک واسط شبکه انتخاب کنید.')).toBeInTheDocument();
    await select('رابط WAN', 'wanNic');
    next();
    await screen.findByText(i18n.t('setup:defaultsHelp'));
    expect(screen.queryByRole('combobox', { name: 'رابط LAN' })).not.toBeInTheDocument();
    expect(document.documentElement).toHaveAttribute('dir', 'rtl');
    expect(
      fake.calls.some((r) => r.method === 'POST' && r.path.startsWith('/api/v1/config/')),
    ).toBe(false);
  });
});
