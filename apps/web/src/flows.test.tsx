import { QueryClient } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from './App';
import i18n from './i18n';
import { createTestRouter } from './router';
import { installFakeApi, resetSession, signIn, type FakeApi } from './test-api';

/**
 * P07b flows in jsdom against a scripted stand-in of the P06 API (unit level). The same flows run against the real API
 * and agent in test/e2e/flow.e2e.mjs.
 */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';

function app(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
  return <App router={createTestRouter([path], { devRoutes: false })} streamUrl={STREAM} queryClient={queryClient} />;
}

const user = (username: string, role: string, extra: Record<string, unknown> = {}) => ({ username, role, scope: '*', sshKeys: [], disabled: false, ...extra });
const deadlineIn = (ms: number) => new Date(Date.now() + ms).toISOString();

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('login and protected routes', () => {
  it('redirects to /login?next=…, shows a wrong password, then lands on the page it came from', async () => {
    const api = installFakeApi();
    api.on('POST /api/v1/auth/refresh', { status: 401, body: { detail: 'no refresh token' } });
    let attempts = 0;
    api.on('POST /api/v1/auth/login', (_req, body) =>
      (body as { password: string }).password === 'right'
        ? { body: { accessToken: 'h.p.s', tokenType: 'Bearer', expiresIn: 900, user: { id: 1, username: 'admin', role: 'admin' } } }
        : { status: 401, body: { detail: `bad ${++attempts}` } },
    );
    api.on('GET /api/v1/config/revisions', { body: { items: [], total: 0 } });
    render(app('/system/revisions'));
    expect(await screen.findByRole('heading', { level: 1, name: 'Sign in to VRX' })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText(/Username/), { target: { value: 'admin' } });
    fireEvent.change(screen.getByLabelText(/Password/), { target: { value: 'wrong' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Wrong username or password');
    fireEvent.change(screen.getByLabelText(/Password/), { target: { value: 'right' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
    expect(await screen.findByRole('heading', { level: 2, name: 'Revisions' })).toBeInTheDocument();
    expect(api.calls.some((c) => c.path === '/api/v1/config/revisions' && c.auth === 'Bearer h.p.s')).toBe(true);
  });
});

function withChanges(api: FakeApi, lockOwner = 'admin') {
  api.on('GET /api/v1/config/diff', {
    body: {
      baseRevision: 1,
      changes: [{ op: 'replace', pointer: '/management/users', from: [user('admin', 'admin')], to: [user('admin', 'admin'), user('w1ro', 'readonly')] }],
    },
  });
  api.on('GET /api/v1/config/lock', { body: { locked: true, owner: lockOwner, ownerId: 1, lockedAt: deadlineIn(0), lastActivity: deadlineIn(0), expiresAt: deadlineIn(600_000) } });
}

describe('pending-change bar → commit dialog → confirm countdown', () => {
  it('commits with auto-revert, counts down, confirms and reports the revision', async () => {
    const api = installFakeApi();
    await signIn();
    withChanges(api);
    api.on('POST /api/v1/config/validate', { body: { ok: true, warnings: [], plan: [], notApplied: [] } });
    let pending: unknown = null;
    api.on('GET /api/v1/config/commit/pending', () => ({ body: { pending } }));
    api.on('POST /api/v1/config/commit', () => {
      pending = { txnId: 'txn-1', author: 1, comment: 'add w1ro', kind: 'commit', deadline: deadlineIn(120_000), createdAt: deadlineIn(0) };
      return { body: { status: 'pending', txnId: 'txn-1', confirmDeadline: deadlineIn(120_000), results: [], warnings: [], notApplied: [] } };
    });
    api.on('POST /api/v1/config/commit/confirm', () => {
      pending = null;
      return { body: { status: 'confirmed', txnId: 'txn-1', revision: { id: 2 }, results: [], warnings: [], notApplied: [] } };
    });
    render(app('/'));
    const bar = await screen.findByTestId('pending-bar');
    expect(within(bar).getByTestId('pending-count')).toHaveTextContent('1 uncommitted change'); // one user added
    expect(within(bar).getByTestId('lock-owner')).toHaveTextContent('Locked by: you');
    fireEvent.click(within(bar).getByRole('button', { name: 'Commit…' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('/management/users/1')).toBeInTheDocument();
    expect(await within(dialog).findByTestId('validate-ok')).toHaveTextContent('Validation passed.');
    expect(within(dialog).getByRole('checkbox')).toBeChecked();
    fireEvent.change(within(dialog).getByLabelText('Comment'), { target: { value: 'add w1ro' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Commit with auto-revert' }));
    const banner = await screen.findByTestId('confirm-banner');
    expect(banner).toHaveTextContent(/auto-revert in [12]:[0-5]\d/);
    const commit = api.calls.find((c) => c.path === '/api/v1/config/commit');
    expect(commit?.search).toBe('?comment=add%20w1ro&confirm=120');
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull()); // the closing dialog hides the page from role queries
    fireEvent.click(await within(banner).findByRole('button', { name: 'Confirm' }));
    expect(await screen.findByTestId('confirm-outcome')).toHaveTextContent('The changes are kept and saved as revision 2.');
  });

  it('keeps counting down with "Reconnecting…" while the device does not answer', async () => {
    const api = installFakeApi();
    await signIn();
    let down = false;
    api.on('GET /api/v1/config/commit/pending', () =>
      down ? 'network-error' : { body: { pending: { txnId: 'txn-9', author: 1, comment: '', kind: 'commit', deadline: deadlineIn(90_000), createdAt: deadlineIn(0) } } },
    );
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<App router={createTestRouter(['/'], { devRoutes: false })} streamUrl={STREAM} queryClient={qc} />);
    const banner = await screen.findByTestId('confirm-banner');
    expect(banner).toHaveTextContent('A commit is waiting for confirmation');
    down = true;
    await act(async () => {
      await qc.refetchQueries({ queryKey: ['config', 'commit', 'pending'] });
    });
    await waitFor(() => expect(screen.getByTestId('confirm-banner')).toHaveTextContent('Reconnecting to the device…'));
    expect(screen.getByTestId('confirm-banner')).toHaveTextContent(/auto-revert in 1:[0-5]\d/);
    expect(within(screen.getByTestId('confirm-banner')).getByRole('button', { name: 'Confirm' })).toBeDisabled();
  });

  it('shows the lock owner of another user and disables Commit/Discard', async () => {
    const api = installFakeApi('operator', 'op1');
    await signIn();
    withChanges(api, 'admin');
    render(app('/'));
    const bar = await screen.findByTestId('pending-bar');
    await waitFor(() => expect(within(bar).getByTestId('lock-owner')).toHaveTextContent('Locked by: admin'));
    expect(within(bar).getByRole('button', { name: 'Commit…' })).toBeDisabled();
    expect(within(bar).getByRole('button', { name: 'Discard' })).toBeDisabled();
  });

  it('shows a server problem with its pointers when the commit fails validation', async () => {
    const api = installFakeApi();
    await signIn();
    withChanges(api);
    api.on('POST /api/v1/config/validate', {
      status: 400,
      body: {
        type: 'https://vrx.dev/problems/validation',
        title: 'Validation failed',
        status: 400,
        detail: 'semantic validation failed',
        errors: [{ pointer: '/management/users', message: 'at least one enabled admin user with a password or an SSH key is required' }],
      },
    });
    render(app('/'));
    fireEvent.click(within(await screen.findByTestId('pending-bar')).getByRole('button', { name: 'Commit…' }));
    const problem = await within(await screen.findByRole('dialog')).findByTestId('problem');
    expect(problem).toHaveTextContent('Validation failed (400)');
    expect(problem).toHaveTextContent('/management/users — at least one enabled admin user');
    expect(within(screen.getByRole('dialog')).getByRole('button', { name: 'Commit with auto-revert' })).toBeDisabled();
  });
});

describe('readonly user', () => {
  it('sees users and revisions with every action disabled', async () => {
    const api = installFakeApi('readonly', 'w1ro');
    await signIn();
    api.on('GET /api/v1/config/candidate/management', { body: { users: [user('admin', 'admin'), user('w1ro', 'readonly')] } });
    api.on('GET /api/v1/config/management', { body: { users: [user('admin', 'admin'), user('w1ro', 'readonly')] } });
    render(app('/system/users'));
    expect(await screen.findByTestId('users-readonly')).toHaveTextContent('Signed in as Read-only: only administrators can add, change or remove users.');
    expect(screen.getByRole('button', { name: 'Add user' })).toBeDisabled();
    expect(await screen.findByRole('button', { name: 'Edit w1ro' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Remove admin' })).toBeDisabled();
  });
});
