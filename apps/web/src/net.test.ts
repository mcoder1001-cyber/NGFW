import { describe, expect, it } from 'vitest';
import * as net from './net';

/** The server's worst-case commit (apps/api/src/commit/budget.ts COMMIT_BUDGET_MAX_MS, TD-10a review 2.4a). */
const SERVER_BUDGET_MS = 111_000;

describe('net deadlines (TD-10a, review 2.4a)', () => {
  it('commit, rollback, confirm and validate wait longer than the server works', () => {
    for (const path of [
      '/api/v1/config/commit',
      '/api/v1/config/rollback/3',
      '/api/v1/config/commit/confirm',
      '/api/v1/config/validate',
    ]) {
      expect(
        net.timeoutFor(new Request(`http://localhost${path}`, { method: 'POST' })),
        path,
      ).toBeGreaterThan(SERVER_BUDGET_MS + 10_000);
    }
    expect(net.timeoutFor(new Request('http://localhost/api/v1/config'))).toBe(6_000);
  });
});

describe('applyOutcome: what became of a commit-like request that got no answer (TD-10a, review 2.4a)', () => {
  const sentAt = Date.parse('2026-09-24T20:00:00.000Z');
  const base: Record<string, unknown> = {
    sentAt,
    beforeRevision: 4,
    pending: null,
    sync: { state: 'in-sync', reason: '' },
    newest: { id: 4, kind: 'commit', createdAt: '2026-09-24T19:00:00.000Z' },
  };
  const outcome = (f: Record<string, unknown>) =>
    (net as unknown as { applyOutcome: (x: unknown) => unknown }).applyOutcome({ ...base, ...f });

  it('a pending commit: applied, waiting for confirmation', () => {
    expect(
      outcome({
        pending: {
          txnId: 't1',
          deadline: '2026-09-24T20:05:00.000Z',
          createdAt: '2026-09-24T20:00:01.000Z',
        },
      }),
    ).toEqual({
      kind: 'pending',
      txnId: 't1',
      deadlineMs: Date.parse('2026-09-24T20:05:00.000Z'),
    });
  });
  it('a newer revision than the one running before: applied', () => {
    expect(
      outcome({ newest: { id: 5, kind: 'rollback', createdAt: '2026-09-24T20:00:02.000Z' } }),
    ).toEqual({ kind: 'applied', revision: 5 });
  });
  it('running not known to match the data plane: unknown, with the reason', () => {
    expect(outcome({ sync: { state: 'unknown', reason: 'no answer to Apply' } })).toEqual({
      kind: 'unknown',
      reason: 'no answer to Apply',
    });
  });
  it('nothing new, in sync: not applied', () => {
    expect(outcome({})).toEqual({ kind: 'not-applied' });
  });
});
