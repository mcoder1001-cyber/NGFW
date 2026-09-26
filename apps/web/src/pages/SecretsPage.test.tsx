import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { App } from '../App';
import i18n from '../i18n';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../test-api';
import { SecretsPage, validRef } from './SecretsPage';

/** System › Secrets in jsdom against a scripted stand-in of the P06 secrets API (unit level). */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';
const LONG = { timeout: 60_000 };
const WAIT = { timeout: 15_000 };
// the fixture value (00-CONTEXT: test fixtures use the literal VRX_TEST_PSK_<id>)
const VALUE = 'VRX_TEST_PSK_WEB2';

const LIST = [
  { ref: 'cert/web', kind: 'cert', name: 'web', createdAt: '2026-09-24T09:00:00.000Z' },
  { ref: 'psk/branch-1', kind: 'psk', name: 'branch-1', createdAt: '2026-09-24T10:00:00.000Z' },
];

function setup(role: 'admin' | 'operator' | 'readonly' = 'admin') {
  const api = installFakeApi(role, role === 'admin' ? 'admin' : 'viewer');
  api.on('GET /api/v1/secrets', { body: LIST });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
  const ui = <App router={createMemoryRouter([{ path: '/', element: <SecretsPage /> }])} streamUrl={STREAM} queryClient={queryClient} />;
  return { api, queryClient, ui };
}

function posts(api: FakeApi) {
  return api.calls.filter((c) => c.method === 'POST' && c.path === '/api/v1/secrets');
}

/** Everything the query cache holds (queries and mutations: data, variables, errors) as text. */
function cacheText(qc: QueryClient): string {
  const queries = qc.getQueryCache().getAll().map((q) => ({ key: q.queryKey, state: q.state }));
  const mutations = qc.getMutationCache().getAll().map((m) => m.state);
  return JSON.stringify({ queries, mutations }, (_k, v: unknown) => (v instanceof Error ? { ...v, message: v.message } : v));
}

afterEach(async () => {
  await resetSession();
  vi.restoreAllMocks();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('secret references', () => {
  it('validRef follows the one schema (secretRef of packages/schema)', () => {
    expect(validRef('psk', 'branch-1')).toBe(true);
    expect(validRef('key', 'a.b_c-d')).toBe(true);
    expect(validRef('psk', '')).toBe(false);
    expect(validRef('psk', '-x')).toBe(false);
    expect(validRef('psk', 'has space')).toBe(false);
    expect(validRef('psk', 'x'.repeat(64))).toBe(false);
  });
});

describe('secrets page', () => {
  it('lists references; the reference cell is a keyboard button that opens the details', LONG, async () => {
    const { ui } = setup();
    await signIn();
    render(ui);
    expect(await screen.findByRole('heading', { level: 2, name: 'Secrets' }, WAIT)).toBeInTheDocument();
    const grid = await screen.findByRole('grid', { name: 'Secrets' }, WAIT);
    const key = await within(grid).findByRole('button', { name: 'Open psk/branch-1' }, WAIT);
    expect(within(grid).getByText('Certificate')).toBeInTheDocument();
    key.focus();
    await userEvent.setup().keyboard('{Enter}');
    const drawer = await screen.findByRole('region', { name: 'Secret psk/branch-1' }, WAIT);
    expect(within(drawer).getByText(/Use the reference psk\/branch-1/)).toBeInTheDocument();
    expect(within(drawer).getByRole('button', { name: 'Rotate' })).toBeEnabled();
  });

  it('creates a secret: the value goes to the API once and never into the query cache, form state or the console', LONG, async () => {
    const { api, queryClient, ui } = setup();
    const logged: unknown[] = [];
    for (const m of ['log', 'info', 'warn', 'error', 'debug'] as const) vi.spyOn(console, m).mockImplementation((...a: unknown[]) => void logged.push(a));
    api.on('POST /api/v1/secrets', { body: { ref: 'psk/site-a', created: true, version: 1 } });
    await signIn();
    render(ui);
    await screen.findByRole('button', { name: 'Open psk/branch-1' }, WAIT);
    fireEvent.click(screen.getByRole('button', { name: 'Add secret' }));
    const dialog = await screen.findByRole('dialog', { name: 'Add a secret' }, WAIT);
    const store = within(dialog).getByRole('button', { name: 'Store secret' });
    fireEvent.change(within(dialog).getByLabelText(/^Name/), { target: { value: 'site-a' } });
    expect(store).toBeDisabled(); // no value yet
    const input = within(dialog).getByTestId('secret-value') as HTMLInputElement;
    // review M2: never a browser-visible password field, and outside any <form> (no "Save password?" offer)
    expect(input).toHaveAttribute('type', 'text');
    expect(input).toHaveAttribute('autocomplete', 'off');
    expect(input).toHaveAttribute('data-lpignore', 'true');
    expect(input.closest('form')).toBeNull();
    fireEvent.change(input, { target: { value: VALUE } });
    expect(store).toBeEnabled();
    fireEvent.click(store);
    expect(await screen.findByTestId('secret-notice', {}, WAIT)).toHaveTextContent('Stored psk/site-a (version 1).');
    // sent exactly once, as a create (no ?replace)
    expect(posts(api)).toHaveLength(1);
    expect(posts(api)[0]).toMatchObject({ search: '', body: { kind: 'psk', name: 'site-a', value: VALUE } });
    // cleared and closed; nowhere in the cache (mutation variables hold kind/name/replace only), never logged
    expect(input.value).toBe('');
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull(), WAIT);
    const cached = cacheText(queryClient);
    expect(cached).toContain('site-a');
    expect(cached).not.toContain(VALUE);
    expect(JSON.stringify(logged)).not.toContain(VALUE);
    expect(document.body.innerHTML).not.toContain(VALUE);
  });

  it('rotates: a new version of the same reference (?replace=true), kind and name fixed', LONG, async () => {
    const { api, queryClient, ui } = setup();
    api.on('POST /api/v1/secrets', { body: { ref: 'psk/branch-1', created: false, version: 2 } });
    await signIn();
    render(ui);
    const grid = await screen.findByRole('grid', { name: 'Secrets' }, WAIT);
    fireEvent.click(await within(grid).findByRole('button', { name: 'Rotate psk/branch-1' }, WAIT));
    const dialog = await screen.findByRole('dialog', { name: 'Rotate psk/branch-1' }, WAIT);
    expect(within(dialog).getByLabelText(/^Name/)).toBeDisabled();
    fireEvent.change(within(dialog).getByTestId('secret-value'), { target: { value: VALUE } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Store new version' }));
    expect(await screen.findByTestId('secret-notice', {}, WAIT)).toHaveTextContent('Stored a new version of psk/branch-1 (version 2).');
    expect(posts(api)[0]).toMatchObject({ search: '?replace=true', body: { kind: 'psk', name: 'branch-1', value: VALUE } });
    expect(cacheText(queryClient)).not.toContain(VALUE);
  });

  it('an existing reference is refused on the name field (409 secret-exists); a bad name cannot be sent', LONG, async () => {
    const { api, queryClient, ui } = setup();
    api.on('POST /api/v1/secrets', {
      status: 409,
      body: { type: 'https://vrx.dev/problems/secret-exists', title: 'Conflict', status: 409, detail: "secret 'psk/branch-1' exists; send ?replace=true to store a new version" },
    });
    await signIn();
    render(ui);
    await screen.findByRole('button', { name: 'Open psk/branch-1' }, WAIT);
    fireEvent.click(screen.getByRole('button', { name: 'Add secret' }));
    const dialog = await screen.findByRole('dialog', { name: 'Add a secret' }, WAIT);
    const name = within(dialog).getByLabelText(/^Name/);
    fireEvent.change(within(dialog).getByTestId('secret-value'), { target: { value: VALUE } });
    fireEvent.change(name, { target: { value: 'has space' } });
    expect(within(dialog).getByRole('button', { name: 'Store secret' })).toBeDisabled();
    expect(name).toHaveAttribute('aria-invalid', 'true');
    fireEvent.change(name, { target: { value: 'branch-1' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Store secret' }));
    expect(await within(dialog).findByText(/psk\/branch-1 already exists\. Use Rotate/, {}, WAIT)).toBeInTheDocument();
    expect(cacheText(queryClient)).not.toContain(VALUE);
  });

  it('a certificate is pasted into a multi-line field (a password input would drop the PEM line breaks)', LONG, async () => {
    const { ui } = setup();
    await signIn();
    render(ui);
    await screen.findByRole('button', { name: 'Open psk/branch-1' }, WAIT);
    fireEvent.click(screen.getByRole('button', { name: 'Add secret' }));
    const dialog = await screen.findByRole('dialog', { name: 'Add a secret' }, WAIT);
    fireEvent.mouseDown(within(dialog).getByRole('combobox', { name: /^Kind/ }));
    fireEvent.click(await screen.findByRole('option', { name: 'Certificate' }, WAIT));
    await waitFor(() => expect(within(dialog).getByTestId('secret-value').tagName).toBe('TEXTAREA'), WAIT);
  });

  it('delete shows where a reference is still used (409 with pointers), then deletes', LONG, async () => {
    const { api, ui } = setup();
    api.on('DELETE /api/v1/secrets/psk/branch-1', {
      status: 409,
      body: {
        type: 'https://vrx.dev/problems/secret-in-use',
        title: 'Conflict',
        status: 409,
        detail: "secret 'psk/branch-1' is referenced by the configuration",
        errors: [{ pointer: '/vpn/ipsec/tunnels/branch-1/psk', message: 'references psk/branch-1' }],
      },
    });
    await signIn();
    render(ui);
    const grid = await screen.findByRole('grid', { name: 'Secrets' }, WAIT);
    fireEvent.click(await within(grid).findByRole('button', { name: 'Delete psk/branch-1' }, WAIT));
    const dialog = await screen.findByRole('dialog', { name: 'Delete psk/branch-1?' }, WAIT);
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete secret' }));
    expect(await within(dialog).findByTestId('problem', {}, WAIT)).toHaveTextContent('/vpn/ipsec/tunnels/branch-1/psk');
    api.on('DELETE /api/v1/secrets/psk/branch-1', { status: 204 });
    const lists = api.calls.filter((c) => c.method === 'GET' && c.path === '/api/v1/secrets').length;
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete secret' }));
    expect(await screen.findByTestId('secret-notice', {}, WAIT)).toHaveTextContent('Deleted psk/branch-1.');
    await waitFor(() => expect(api.calls.filter((c) => c.method === 'GET' && c.path === '/api/v1/secrets').length).toBeGreaterThan(lists), WAIT);
  });

  it('non-admins see the references with every action disabled', LONG, async () => {
    const { ui } = setup('operator');
    await signIn();
    render(ui);
    const grid = await screen.findByRole('grid', { name: 'Secrets' }, WAIT);
    await within(grid).findByRole('button', { name: 'Open psk/branch-1' }, WAIT);
    expect(screen.getByRole('button', { name: 'Add secret' })).toBeDisabled();
    expect(within(grid).getByRole('button', { name: 'Rotate psk/branch-1' })).toBeDisabled();
    expect(within(grid).getByRole('button', { name: 'Delete psk/branch-1' })).toBeDisabled();
  });
});
