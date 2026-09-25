import { QueryClient } from '@tanstack/react-query';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import * as net from '../net';
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

/** Shorten the outcome follow-up (net.OUTCOME_WAIT) for a test; returns the restore function. */
function shortWait(untilMs: number, everyMs: number): () => void {
  const wait = (net as { OUTCOME_WAIT?: { untilMs: number; everyMs: number } }).OUTCOME_WAIT;
  if (!wait) return () => undefined;
  const saved = { ...wait };
  Object.assign(wait, { untilMs, everyMs });
  return () => Object.assign(wait, saved);
}

afterEach(async () => {
  await resetSession();
  localStorage.clear();
  await i18n.changeLanguage('en');
});

function revisionsApi(after: {
  newest?: ReturnType<typeof meta>;
  pending?: unknown;
  sync?: { state: string; reason: string };
  /** The server finishes the rollback this long after the request (review M1). */
  landsAfterMs?: number;
}): FakeApi {
  const api = installFakeApi();
  let sentAt = 0;
  const landed = () => sentAt > 0 && Date.now() - sentAt >= (after.landsAfterMs ?? 0);
  api.on('GET /api/v1/config/diff', { body: { baseRevision: 2, changes: [] } });
  api.on('GET /api/v1/config', { body: { system: { hostname: 'two' } } });
  api.on('GET /api/v1/config/revisions/1', {
    body: { ...meta(1), payload: { system: { hostname: 'one' } } },
  });
  api.on('GET /api/v1/config/revisions', () => ({
    body:
      landed() && after.newest
        ? { items: [after.newest, meta(2), meta(1)], total: 3 }
        : { items: [meta(2), meta(1)], total: 2 },
  }));
  api.on('GET /api/v1/state/system', () => ({
    body: {
      api: { version: 't', startedAt: '', wsClients: 0 },
      agent: { reachable: true },
      runningRevision: landed() && after.newest ? after.newest.id : 2,
      pendingCommit:
        landed() && after.pending
          ? { ...(after.pending as object), createdAt: new Date(sentAt).toISOString() } // created by this request
          : null,
      sync: {
        txnId: null,
        since: '',
        ...(landed() && after.sync ? after.sync : { state: 'in-sync', reason: '' }),
      },
    },
  }));
  api.on('POST /api/v1/config/rollback/1', () => {
    sentAt = Date.now(); // the server goes on and finishes; the answer never arrives
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

  it('reports "not applied" when nothing changed — only after the server budget has passed', async () => {
    const restore = shortWait(2_500, 200);
    try {
      revisionsApi({});
      const dialog = await rollBackToOne();
      expect(
        await within(dialog).findByText(/following up what became of the rollback/i),
      ).toBeInTheDocument();
      expect(within(dialog).queryByText(/the rollback was not applied/i)).toBeNull();
      expect(
        await within(dialog).findByText(/the rollback was not applied/i, {}, { timeout: 6_000 }),
      ).toBeInTheDocument();
    } finally {
      restore();
    }
  });

  it('M1: an early network error while the rollback still runs → never "not applied"; the late revision is reported', async () => {
    const restore = shortWait(8_000, 200);
    try {
      revisionsApi({ newest: meta(3, 'rollback'), landsAfterMs: 1_500 });
      const dialog = await rollBackToOne();
      const applied = await within(dialog).findByText(
        /revision 3 was created: the rollback was applied/i,
        {},
        { timeout: 6_000 },
      );
      expect(applied).toBeInTheDocument();
      expect(within(dialog).queryByText(/the rollback was not applied/i)).toBeNull();
    } finally {
      restore();
    }
  });
});
