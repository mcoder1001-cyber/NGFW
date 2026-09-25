import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../../App';
import i18n from '../../i18n';
import { createTestRouter } from '../../router';
import { installFakeApi, resetSession, signIn } from '../../test-api';

/**
 * The generic `/config/*` editor against a scripted API (unit level; UI honesty rule — the real stack runs in
 * test/e2e). Proof for the PATCH/DELETE-as-merge-patch behaviour lives here at the screen level; the pure
 * computations themselves are unit-tested directly in schemaPath.test.ts / subtree.test.ts.
 */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

/** A complete interface config (as the server always stores it, defaults filled) — a partial fixture would make
 * `createMergePatch` see every default-filled field as "added" against the form's own defaulted value. */
const eth0Config = { mtu: 1500, enabled: true, description: '', ipv4: ['10.1.1.1/24'], ipv6: [], vrf: 'default', promiscuous: false, subinterfaces: {} };

function app(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
  return <App router={createTestRouter([path], { devRoutes: false })} streamUrl={STREAM} queryClient={queryClient} />;
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('advanced editor — record domains (a map of named entries)', () => {
  it('lists the candidate keys, deletes one as a merge patch, and opens a new key not yet in the candidate', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/config/candidate/interfaces', { body: { eth0: { mtu: 1500, enabled: true } } });
    let patched: unknown;
    api.on('PATCH /api/v1/config/interfaces', (_r, body) => {
      patched = body;
      return { body: { pointer: '/interfaces', before: null, after: null } };
    });
    await signIn();
    render(app('/config/interfaces'));

    expect(await screen.findByRole('heading', { level: 2, name: 'Interfaces' }, { timeout: 15_000 })).toBeInTheDocument();
    expect(await screen.findByText('eth0')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Remove eth0' }));
    await waitFor(() => expect(patched).toEqual({ eth0: null }));

    // add a key the candidate does not have yet: it opens a fresh (defaulted) form, not the record list. Its own
    // GET (D-UDE-1: the exact pointer, not the whole domain again) answers 404 — read as "not created yet", not
    // an error.
    fireEvent.change(screen.getByLabelText('New key'), { target: { value: 'eth1' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    expect(await screen.findByRole('button', { name: 'Save to candidate' }, { timeout: 15_000 })).toBeInTheDocument();
    expect(screen.queryByText('eth0')).toBeNull();
    // a node that does not exist in the candidate has nothing to remove
    expect(screen.queryByRole('button', { name: 'Remove this node' })).toBeNull();
  });
});

describe('advanced editor — a record item (JSON-pointer subtree two levels deep)', () => {
  it('reads and saves at the pointer\'s real depth (D-UDE-1), not the whole domain', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    // the exact node, not the whole `interfaces` domain — proof that the generic route now addresses the real
    // depth (`/api/v1/config/candidate/interfaces/eth0`), not `/api/v1/config/candidate/interfaces` + a client-side
    // walk.
    api.on('GET /api/v1/config/candidate/interfaces/eth0', { body: eth0Config });
    let patched: unknown;
    api.on('PATCH /api/v1/config/interfaces/eth0', (_r, body) => {
      patched = body;
      return { body: { pointer: '/interfaces/eth0', before: null, after: null } };
    });
    api.on('PATCH /api/v1/config/interfaces', (_r, body) => {
      patched = body;
      return { body: { pointer: '/interfaces', before: null, after: null } };
    });
    await signIn();
    render(app('/config/interfaces/eth0'));

    const mtu = await screen.findByLabelText('MTU', {}, { timeout: 15_000 });
    fireEvent.change(mtu, { target: { value: '1400' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save to candidate' }));
    // exactly the item's own merge patch, at its own pointer — no more `{ eth0: { mtu: 1400 } }` wrapper
    await waitFor(() => expect(patched).toEqual({ mtu: 1400 }));

    // removing the whole item is still a merge patch of its PARENT (the record), `{ eth0: null }`
    fireEvent.click(screen.getByRole('button', { name: 'Remove this node' }));
    await waitFor(() => expect(patched).toEqual({ eth0: null }));
  });
});

describe('advanced editor — a fixed-shape domain root (nested containers become their own tree node)', () => {
  it('never nulls out a nested container it never touched when saving a sibling scalar field', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/config/candidate/system', {
      body: { hostname: 'vrx', timezone: 'UTC', banner: {}, dns: { servers: [], searchDomains: [], vrf: 'default' } },
    });
    let patched: unknown;
    api.on('PATCH /api/v1/config/system', (_r, body) => {
      patched = body;
      return { body: { pointer: '/system', before: null, after: null } };
    });
    await signIn();
    render(app('/config/system'));

    expect(await screen.findByRole('heading', { level: 2, name: 'System' }, { timeout: 15_000 })).toBeInTheDocument();
    // `banner` and `dns` are nested objects: they get their own tree node (a chip), not a field of this form
    expect(await screen.findByRole('button', { name: 'Banners' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'DNS client' })).toBeInTheDocument();

    const hostname = screen.getByLabelText('Hostname');
    fireEvent.change(hostname, { target: { value: 'vrx-b' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save to candidate' }));
    // exactly the touched field: banner/dns are absent from the form and must stay untouched by the merge patch
    await waitFor(() => expect(patched).toEqual({ hostname: 'vrx-b' }));
  });

  it('opening a nested container navigates the breadcrumb one level down and reads that node directly', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/config/candidate/system', {
      body: { hostname: 'vrx', timezone: 'UTC', banner: { login: 'welcome' }, dns: { servers: [], searchDomains: [], vrf: 'default' } },
    });
    // the banner sub-page reads `/system/banner` directly (D-UDE-1), not the whole `system` domain again
    api.on('GET /api/v1/config/candidate/system/banner', { body: { login: 'welcome' } });
    await signIn();
    render(app('/config/system'));

    fireEvent.click(await screen.findByRole('button', { name: 'Banners' }, { timeout: 15_000 }));
    const login = await screen.findByLabelText('Pre-login banner');
    expect(login).toHaveValue('welcome');
    const nav = screen.getByLabelText('Configuration path');
    expect(within(nav).getByText('System')).toBeInTheDocument();
    expect(within(nav).getByText('Banners')).toBeInTheDocument();
  });
});

describe('advanced editor — bad paths never invent a hand-written domain list', () => {
  it('an unknown domain lists the real ones from the schema', async () => {
    installFakeApi();
    await signIn();
    render(app('/config/not-a-domain'));
    expect(await screen.findByRole('heading', { level: 2, name: 'No such configuration path' })).toBeInTheDocument();
    const main = within(screen.getByRole('main'));
    expect(main.getByRole('link', { name: 'Interfaces' })).toHaveAttribute('href', '/config/interfaces');
  });
});

describe('advanced editor — Persian/RTL', () => {
  it('renders the chrome in Persian', { timeout: 60_000 }, async () => {
    const api = installFakeApi();
    api.on('GET /api/v1/config/candidate/interfaces', { body: {} });
    await signIn();
    render(app('/config/interfaces'));
    expect(await screen.findByRole('heading', { level: 2, name: 'Interfaces' }, { timeout: 15_000 })).toBeInTheDocument();
    // the settings provider re-asserts the stored language on mount (UiSettingsProvider), so switch after render
    // (the same pattern InterfacesPage.test.tsx "renders in Persian" uses)
    await act(async () => {
      await i18n.changeLanguage('fa');
    });
    expect(await screen.findByRole('heading', { level: 2, name: 'اینترفیس‌ها' }, { timeout: 15_000 })).toBeInTheDocument();
    expect(await screen.findByText('هنوز هیچ موردی نیست.')).toBeInTheDocument();
    expect(screen.getByLabelText('کلید جدید')).toBeInTheDocument();
  });
});
