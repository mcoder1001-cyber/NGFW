import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from './App';
import { Session } from './auth/session';
import { confirmStore } from './config/confirm-store';
import { effectiveChanges, secretEdits } from './config/effective';
import i18n from './i18n';
import { createTestRouter } from './router';
import { installFakeApi, resetSession, signIn, type FakeApi } from './test-api';

/** Review P07b fix round (H1, M1, M2, M5) at unit level; the real-stack E2E covers them too. */
const STREAM = 'ws://127.0.0.1:1/api/v1/stream';
const qc = () => new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 0 } } });
const app = (path: string, session?: Session) => (
  <App router={createTestRouter([path], { devRoutes: false })} streamUrl={STREAM} queryClient={qc()} {...(session ? { session } : {})} />
);
const user = (username: string, role: string) => ({ username, role, scope: '*', sshKeys: [], disabled: false });
const lockedBy = (api: FakeApi, owner: string) =>
  api.on('GET /api/v1/config/lock', {
    body: { locked: true, owner, ownerId: 1, lockedAt: new Date(Date.now() - 3_600_000).toISOString(), lastActivity: new Date(Date.now() - 600_000).toISOString(), expiresAt: new Date(Date.now() + 600_000).toISOString() },
  });

afterEach(async () => {
  await resetSession();
  secretEdits.clear();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

describe('H1 — write-only (password hash) edits are visible and committable', () => {
  it('effectiveChanges: TD-2 redacted entries pass through; remembered edits and a hidden locked candidate become synthetic entries', () => {
    const td2 = { op: 'replace' as const, pointer: '/management/users/1/passwordHash', redacted: true };
    const users = [user('admin', 'admin'), user('w1ro', 'readonly')];
    expect(effectiveChanges([td2], { editedUsers: ['w1ro'], candidateUsers: users, lockMine: true })).toEqual([td2]); // server wins
    expect(effectiveChanges([], { editedUsers: ['w1ro'], candidateUsers: users, lockMine: true })).toEqual([
      { op: 'replace', pointer: '/management/users/1/passwordHash', redacted: true, synthetic: true },
    ]);
    expect(effectiveChanges([], { editedUsers: [], candidateUsers: undefined, lockMine: true })).toEqual([
      { op: 'replace', pointer: '', redacted: true, synthetic: true },
    ]);
    expect(effectiveChanges([], { editedUsers: [], candidateUsers: undefined, lockMine: false })).toEqual([]);
  });

  it('a TD-2 diff with only a redacted entry shows the bar, "Password changed" without any value, and Commit enabled', async () => {
    const api = installFakeApi();
    await signIn();
    lockedBy(api, 'admin');
    api.on('GET /api/v1/config/diff', { body: { baseRevision: 3, changes: [{ op: 'replace', pointer: '/management/users/1/passwordHash', redacted: true }] } });
    api.on('POST /api/v1/config/validate', { body: { ok: true, warnings: [], plan: [], notApplied: [] } });
    render(app('/'));
    const bar = await screen.findByTestId('pending-bar');
    expect(within(bar).getByTestId('pending-count')).toHaveTextContent('1 uncommitted change');
    fireEvent.click(within(bar).getByRole('button', { name: 'Commit…' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByTestId('redacted-change')).toHaveTextContent('Password changed (the value is never shown)');
    expect(within(dialog).queryByText(/\$argon2id\$/)).toBeNull();
    await within(dialog).findByTestId('validate-ok');
    expect(within(dialog).getByRole('button', { name: 'Commit with auto-revert' })).toBeEnabled();
  });

  it('before TD-2: an empty diff with the lock held by me still shows the bar (hidden write-only change)', async () => {
    const api = installFakeApi();
    await signIn();
    lockedBy(api, 'admin');
    render(app('/'));
    const bar = await screen.findByTestId('pending-bar');
    fireEvent.click(within(bar).getByRole('button', { name: 'Review diff' }));
    expect(await within(await screen.findByRole('dialog')).findByTestId('redacted-change')).toHaveTextContent(
      'The candidate is locked by you but shows no visible difference',
    );
  });
});

describe('M1 — the apply result survives the confirmation', () => {
  it('shows "not enforced" in the countdown and the apply result after Confirm', async () => {
    const api = installFakeApi();
    await signIn();
    lockedBy(api, 'admin');
    api.on('GET /api/v1/config/diff', { body: { baseRevision: 1, changes: [{ op: 'add', pointer: '/nat/pools/p1', to: { prefix: '192.0.2.0/28' } }] } });
    api.on('POST /api/v1/config/validate', { body: { ok: true, warnings: [], plan: [], notApplied: ['nat'] } });
    let pending: unknown = null;
    api.on('GET /api/v1/config/commit/pending', () => ({ body: { pending } }));
    api.on('POST /api/v1/config/commit', () => {
      pending = { txnId: 't-1', author: 1, comment: '', kind: 'commit', deadline: new Date(Date.now() + 120_000).toISOString(), createdAt: new Date().toISOString() };
      return {
        body: {
          status: 'pending', txnId: 't-1', confirmDeadline: new Date(Date.now() + 120_000).toISOString(), notApplied: ['nat'],
          warnings: [{ pointer: '/nat', message: 'nat is not implemented by this agent build' }],
          results: [{ key: 'nat/pool/p1', op: 'create', code: 'skipped', message: 'not implemented', pointer: '/nat/pools/p1', subsystem: 'nat' }],
        },
      };
    });
    api.on('POST /api/v1/config/commit/confirm', () => {
      pending = null;
      return { body: { status: 'confirmed', txnId: 't-1', revision: { id: 4 }, results: [], warnings: [], notApplied: [] } };
    });
    render(app('/'));
    fireEvent.click(within(await screen.findByTestId('pending-bar')).getByRole('button', { name: 'Commit…' }));
    const dialog = await screen.findByRole('dialog');
    await within(dialog).findByTestId('validate-ok');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Commit with auto-revert' }));
    expect(await screen.findByTestId('confirm-not-applied')).toHaveTextContent('Stored but not enforced yet: NAT');
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    fireEvent.click(await within(screen.getByTestId('confirm-banner')).findByRole('button', { name: 'Confirm' }));
    const outcome = await screen.findByTestId('confirm-outcome');
    expect(outcome).toHaveTextContent('saved as revision 4');
    const result = within(outcome).getByTestId('commit-result');
    expect(result).toHaveTextContent('What the device applied');
    expect(result).toHaveTextContent('Committed, but not applied to the data plane.');
    expect(result).toHaveTextContent('nat/pool/p1 skipped: not implemented');
    expect(result).toHaveTextContent('/nat — nat is not implemented by this agent build');
  });
});

describe('M2 — a reload during an outage never lands on /login', () => {
  it('keeps the countdown with "Reconnecting…" on the offline screen', async () => {
    const api = installFakeApi();
    api.on('POST /api/v1/auth/refresh', () => 'network-error');
    api.on('GET /api/v1/config/commit/pending', () => 'network-error');
    confirmStore.track({ txnId: 't-9', deadlineMs: Date.now() + 90_000, kind: 'commit', trackedAt: Date.now() });
    const s = new Session(undefined, undefined, { channel: null, retryMs: 60_000 });
    render(app('/system/users', s));
    const offline = await screen.findByTestId('offline-screen');
    expect(within(offline).getByText('The device does not answer')).toBeInTheDocument();
    const banner = within(offline).getByTestId('confirm-banner');
    expect(banner).toHaveTextContent('Reconnecting to the device…');
    expect(banner).toHaveTextContent(/auto-revert in 1:[23]\d/);
    expect(screen.queryByRole('heading', { name: 'Sign in to VRX' })).toBeNull();
    s.dispose();
  });
});

describe('M5 — breaking a lock needs a confirmation naming the owner', () => {
  it('asks first, then DELETEs /config/lock', async () => {
    const api = installFakeApi();
    await signIn();
    lockedBy(api, 'bob');
    api.on('GET /api/v1/config/diff', { body: { baseRevision: 1, changes: [{ op: 'replace', pointer: '/system/hostname', from: 'a', to: 'b' }] } });
    api.on('DELETE /api/v1/config/lock', { body: { locked: false, owner: null, ownerId: null, lockedAt: null, lastActivity: null, expiresAt: null } });
    render(app('/'));
    fireEvent.click(within(await screen.findByTestId('pending-bar')).getByRole('button', { name: 'Break lock' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByText('Break the lock of bob?')).toBeInTheDocument();
    expect(within(dialog).getByTestId('break-lock-body')).toHaveTextContent(/bob locked the candidate .*1 hour ago.*10 minutes ago.*discards all of their uncommitted changes/);
    expect(api.calls.some((c) => c.method === 'DELETE')).toBe(false);
    fireEvent.click(within(dialog).getByRole('button', { name: /Discard bob.s changes and break the lock/ }));
    await waitFor(() => expect(api.calls.some((c) => c.method === 'DELETE' && c.path === '/api/v1/config/lock')).toBe(true));
  });
});
