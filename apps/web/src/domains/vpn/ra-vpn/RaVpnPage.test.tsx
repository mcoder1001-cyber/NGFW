import { QueryClient } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../../App';
import i18n from '../../../i18n';
import { createTestRouter } from '../../../router';
import { installFakeApi, resetSession, signIn } from '../../../test-api';
import { activationAllowed, stepSchema } from './model';
import en from '../../../locales/en/ra-vpn.json';
import fa from '../../../locales/fa/ra-vpn.json';
const ID = 'a'.repeat(64);
const MAX = '18446744073709551615';
const profile = {
  enabled: false,
  localAddr: '192.0.2.19',
  vrf: 'default',
  underlayVrf: 'default',
  auth: 'eap-tls' as const,
  certificate: 'server',
  clientCa: 'client-ca',
  proposal: 'secure',
  pools: [{ name: 'clients', prefix: '10.44.0.0/24', dns: ['10.44.0.1'] }],
  splitTunnel: ['10.55.0.0/24'],
  users: [],
  transport: {
    outer: { vpp: '198.18.19.0/31', namespace: '198.18.19.1/31' },
    inner: { vpp: '198.18.19.2/31', namespace: '198.18.19.3/31' },
  },
  accessPolicy: { ingress: ['inside-in'], egress: ['inside-out'] },
  outerPolicy: { ingress: ['outside-in'], egress: ['outside-out'] },
};
afterEach(async () => {
  cleanup();
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});
function fixture(role: 'admin' | 'operator' | 'readonly' = 'admin', operational = true) {
  const api = installFakeApi(role);
  api.on('GET /api/v1/config/candidate/vpn', { body: { remoteAccess: { office: profile } } });
  api.on('GET /api/v1/state/vpn/remote-access/capabilities', {
    body: {
      engine: 'strongswan-ra',
      operational,
      supportedAuth: ['eap-tls'],
      reason: operational ? '' : 'engine-not-ready',
      editableDisabledDrafts: true,
    },
  });
  api.on('GET /api/v1/state/vpn/remote-access/sessions', {
    body: {
      items: [
        {
          id: ID,
          profile: 'office',
          identity: 'roadwarrior',
          addresses: ['10.44.0.2'],
          establishedSeconds: MAX,
          bytesIn: MAX,
          bytesOut: MAX,
        },
      ],
      nextCursor: '',
    },
  });
  api.on(`POST /api/v1/actions/vpn/remote-access/sessions/${ID}/disconnect`, {
    body: { disconnected: true },
  });
  return api;
}
async function mount() {
  await signIn();
  render(
    <App
      router={createTestRouter(['/vpn?tab=ra-vpn'], { devRoutes: false })}
      streamUrl="ws://127.0.0.1:1/api/v1/stream"
      queryClient={new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } })}
    />,
  );
  await screen.findByRole('heading', { name: i18n.t('ra-vpn:title') });
}
async function selectOffice() {
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Profile' }));
  fireEvent.click(await screen.findByRole('option', { name: 'office' }));
}
describe('RA model and structural wizard contracts', () => {
  it('extracts authentication, both transit ACL policies, pools/DNS and secret-reference wizard steps from the canonical schema', () => {
    expect(Object.keys(stepSchema(0).properties ?? {})).toContain('auth');
    expect(Object.keys(stepSchema(1).properties ?? {})).toEqual([
      'transport',
      'accessPolicy',
      'outerPolicy',
    ]);
    expect(Object.keys(stepSchema(2).properties ?? {})).toEqual(['pools', 'splitTunnel']);
    expect(Object.keys(stepSchema(3).properties ?? {})).toContain('users');
  });
  it('allows disabled drafts but refuses enabled profiles without actual capability and both explicit policies', () => {
    const cap = { operational: true, supportedAuth: ['eap-tls'] };
    expect(activationAllowed(profile, undefined)).toBe(true);
    expect(activationAllowed({ auth: profile.auth }, undefined)).toBe(false);
    expect(activationAllowed({ ...profile, enabled: true }, undefined)).toBe(false);
    expect(activationAllowed({ ...profile, enabled: true }, { ...cap, operational: false })).toBe(
      false,
    );
    expect(activationAllowed({ ...profile, enabled: true, auth: 'pubkey' }, cap)).toBe(false);
    expect(activationAllowed({ ...profile, enabled: true, outerPolicy: undefined }, cap)).toBe(
      false,
    );
    expect(
      activationAllowed(
        { ...profile, enabled: true, accessPolicy: { ingress: [], egress: ['out'] } },
        cap,
      ),
    ).toBe(false);
    expect(activationAllowed({ ...profile, enabled: true }, cap)).toBe(true);
  });
  it('provides matching en/fa translation keys', () => {
    const keys = (x: object, prefix = ''): string[] =>
      Object.entries(x).flatMap(([key, value]) =>
        typeof value === 'object' ? keys(value as object, `${prefix}${key}.`) : [`${prefix}${key}`],
      );
    expect(keys(fa).sort()).toEqual(keys(en).sort());
  });
});
describe('RA real UI transport consumers (scripted unit API)', () => {
  it('displays exact uint64 text and sends profile-scoped admin disconnect only after confirmation', async () => {
    const api = fixture();
    await mount();
    await selectOffice();
    await screen.findByText('roadwarrior');
    expect(screen.getAllByText('18,446,744,073,709,551,615')).toHaveLength(3);
    fireEvent.click(screen.getByRole('button', { name: 'Disconnect' }));
    expect(
      api.calls.filter((x) => x.method === 'POST' && x.path.includes('/disconnect')),
    ).toHaveLength(0);
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Disconnect' }));
    await screen.findByText('roadwarrior');
    expect(api.calls.find((x) => x.path.includes('/disconnect'))?.search).toBe('?profile=office');
  });
  it('localizes maximum uint64 counters with Persian digits without precision loss', async () => {
    fixture();
    localStorage.setItem('ngfw.ui.settings', JSON.stringify({ lang: 'fa', persianDigits: true }));
    await mount();
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'پروفایل' }));
    fireEvent.click(await screen.findByRole('option', { name: 'office' }));
    await screen.findByText('roadwarrior');
    expect(screen.getAllByText('۱۸,۴۴۶,۷۴۴,۰۷۳,۷۰۹,۵۵۱,۶۱۵')).toHaveLength(3);
    expect(screen.queryByText(MAX)).not.toBeInTheDocument();
  });
  it('shows session loading only during an eligible initial fetch, then the successful empty state', async () => {
    const api = fixture();
    api.on('GET /api/v1/state/vpn/remote-access/sessions', { body: { items: [], nextCursor: '' } });
    const original = globalThis.fetch;
    let release!: () => void;
    const waiting = new Promise<void>((resolve) => {
      release = resolve;
    });
    globalThis.fetch = async (...args) => {
      const input = args[0];
      const url = typeof input === 'string' ? input : 'url' in input ? input.url : input.href;
      if (url.includes('/remote-access/sessions')) await waiting;
      return original(...args);
    };
    try {
      await mount();
      expect(screen.queryByText('Loading observed sessions…')).not.toBeInTheDocument();
      await selectOffice();
      expect(await screen.findByRole('status')).toHaveTextContent('Loading observed sessions…');
      expect(screen.queryByText('No observed sessions for this profile.')).not.toBeInTheDocument();
      await act(async () => {
        release();
      });
      expect(await screen.findByText('No observed sessions for this profile.')).toBeInTheDocument();
      expect(screen.queryByText('Loading observed sessions…')).not.toBeInTheDocument();
    } finally {
      release();
      globalThis.fetch = original;
    }
  });
  it('saves a disabled draft through all four wizard steps with explicit existing policies and reference-only credentials', async () => {
    const api = fixture('admin', false);
    api.on('PATCH /api/v1/config/vpn', { body: { remoteAccess: { office: profile } } });
    await mount();
    await selectOffice();
    fireEvent.click(screen.getByRole('button', { name: 'Edit profile' }));
    for (const heading of [
      'Transit addresses and existing ACL policies',
      'Client pools, DNS and split routes',
      'Credential references, RADIUS and timers',
    ]) {
      fireEvent.click(
        within(await screen.findByRole('dialog')).getByRole('button', { name: 'Next' }),
      );
      await screen.findByText(heading);
    }
    fireEvent.click(
      within(await screen.findByRole('dialog')).getByRole('button', { name: 'Save candidate' }),
    );
    await screen.findByRole('heading', { name: 'Remote-access VPN' });
    const write = api.calls.find((x) => x.method === 'PATCH' && x.path === '/api/v1/config/vpn');
    expect(write?.body).toMatchObject({
      remoteAccess: {
        office: {
          enabled: false,
          transport: profile.transport,
          accessPolicy: profile.accessPolicy,
          outerPolicy: profile.outerPolicy,
        },
      },
    });
    expect(JSON.stringify(write?.body)).not.toContain('passwordValue');
  });
  it('refuses enabled candidate submission while the actual capability is unavailable', async () => {
    const api = fixture('admin', false);
    await mount();
    await selectOffice();
    fireEvent.click(screen.getByRole('button', { name: 'Edit profile' }));
    fireEvent.click(
      within(await screen.findByRole('dialog')).getByRole('switch', { name: 'Enabled' }),
    );
    for (const heading of [
      'Transit addresses and existing ACL policies',
      'Client pools, DNS and split routes',
      'Credential references, RADIUS and timers',
    ]) {
      fireEvent.click(
        within(await screen.findByRole('dialog')).getByRole('button', { name: 'Next' }),
      );
      await screen.findByText(heading);
    }
    fireEvent.click(
      within(await screen.findByRole('dialog')).getByRole('button', { name: 'Save candidate' }),
    );
    await screen.findByText(/Enabling requires an operational engine/);
    expect(api.calls.some((x) => x.method === 'PATCH')).toBe(false);
  });
  it('renders Persian profile and nested policy labels through the mounted VPN page', async () => {
    fixture('admin', false);
    await mount();
    await act(async () => {
      await i18n.changeLanguage('fa');
    });
    await screen.findByRole('heading', { name: 'VPN دسترسی از راه دور' });
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'پروفایل' }));
    fireEvent.click(await screen.findByRole('option', { name: 'office' }));
    fireEvent.click(screen.getByRole('button', { name: 'ویرایش پروفایل' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByRole('switch', { name: 'فعال' })).not.toBeChecked();
    expect(within(dialog).getByText('EAP-TLS (گواهی کلاینت)')).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole('button', { name: 'بعدی' }));
    await screen.findByText('آدرس‌های ترانزیت و سیاست‌های ACL موجود');
    expect(screen.getByText('ارجاع ACL ترانزیت عمومی')).toBeInTheDocument();
    expect(screen.getAllByText('ارجاع ACL ورودی').length).toBeGreaterThan(0);
  });
  it('readonly users observe sessions but cannot edit or disconnect', async () => {
    fixture('readonly');
    await mount();
    await selectOffice();
    await screen.findByText('roadwarrior');
    expect(screen.getByRole('button', { name: 'Add profile' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Disconnect' })).toBeDisabled();
  });
  it('nonoperational capabilities stop session polling and keep the disabled draft editor available', async () => {
    const api = fixture('admin', false);
    await mount();
    await selectOffice();
    expect(screen.getByText(/engine is not operational/)).toBeInTheDocument();
    expect(api.calls.some((x) => x.path.endsWith('/sessions'))).toBe(false);
    expect(screen.queryByText('Loading observed sessions…')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Edit profile' }));
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Enabled' })).not.toBeChecked();
  });
});
