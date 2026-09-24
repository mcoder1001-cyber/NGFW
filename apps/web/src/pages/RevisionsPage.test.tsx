import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { App } from '../App';
import i18n from '../i18n';
import { createTestRouter } from '../router';
import { installFakeApi, resetSession, signIn, type FakeApi } from '../test-api';

/**
 * TD-10a (review 2.4a): a rollback that gets no answer in time is not reported as a failure — the page asks the API
 * what became of it (GET /state/system: pending commit + sync; GET /config/revisions?limit=1) and says so.
 */
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

const meta = (id: number, kind = 'commit') => ({
  id,
  createdAt: new Date(Date.now() - (10 - id) * 60_000).toISOString(),
  authorId: 1,
  author: 'admin',
  comment: `r${id}`,
  parentId: id > 1 ? id - 1 : null,
  hash: 'a'.repeat(64),
  txnId: `txn-${id}`,
  kind,
  secretChanges: [],
});

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

function revisionsApi(after: {
  newest?: ReturnType<typeof meta>;
  pending?: unknown;
  sync?: { state: string; reason: string };
}): FakeApi {
  const api = installFakeApi();
  let sent = false;
  api.on('GET /api/v1/config/diff', { body: { baseRevision: 2, changes: [] } });
  api.on('GET /api/v1/config', { body: { system: { hostname: 'two' } } });
  api.on('GET /api/v1/config/revisions/1', {
    body: { ...meta(1), payload: { system: { hostname: 'one' } } },
  });
  api.on('GET /api/v1/config/revisions', () => ({
    body:
      sent && after.newest
        ? { items: [after.newest, meta(2), meta(1)], total: 3 }
        : { items: [meta(2), meta(1)], total: 2 },
  }));
  api.on('GET /api/v1/state/system', () => ({
    body: {
      api: { version: 't', startedAt: '', wsClients: 0 },
      agent: { reachable: true },
      runningRevision: sent && after.newest ? after.newest.id : 2,
      pendingCommit: sent ? (after.pending ?? null) : null,
      sync: {
        txnId: null,
        since: '',
        ...(sent && after.sync ? after.sync : { state: 'in-sync', reason: '' }),
      },
    },
  }));
  api.on('POST /api/v1/config/rollback/1', () => {
    sent = true; // the server goes on and finishes; the answer never arrives
    return 'network-error';
  });
  return api;
}

async function rollBackToOne() {
  await signIn();
  render(app('/system/revisions'));
  const buttons = await screen.findAllByRole('button', { name: 'Roll back…' });
  fireEvent.click(buttons.at(-1)!);
  const dialog = await screen.findByRole('dialog', { name: 'Roll back to revision 1' });
  const submit = within(dialog).getByRole('button', { name: 'Roll back to revision 1' });
  await screen.findByText(/hostname/);
  fireEvent.click(submit);
  return dialog;
}

describe('RevisionsPage: rollback without an answer (TD-10a)', () => {
  it('reports the rollback as applied when a new revision exists', async () => {
    revisionsApi({ newest: meta(3, 'rollback') });
    const dialog = await rollBackToOne();
    expect(
      await within(dialog).findByText(/revision 3 was created: the rollback was applied/i),
    ).toBeInTheDocument();
  });

  it('reports a pending rollback (applied, waiting for confirmation)', async () => {
    revisionsApi({
      pending: {
        txnId: 'txn-p',
        author: 1,
        comment: '',
        kind: 'rollback',
        deadline: new Date(Date.now() + 600_000).toISOString(),
        createdAt: new Date().toISOString(),
      },
    });
    const dialog = await rollBackToOne();
    expect(
      await within(dialog).findByText(/waits for confirmation \(transaction txn-p\)/i),
    ).toBeInTheDocument();
  });

  it('reports "not applied" when nothing changed', async () => {
    revisionsApi({});
    const dialog = await rollBackToOne();
    expect(await within(dialog).findByText(/the rollback was not applied/i)).toBeInTheDocument();
  });
});
