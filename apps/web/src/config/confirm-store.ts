import { useSyncExternalStore } from 'react';

/**
 * The confirmed commit this browser started (docs/05 "Signature UX"): kept locally so the countdown keeps running when
 * the device stops answering — which is exactly what happens when a commit cuts the operator's own access. Persisted in
 * sessionStorage so a reload during the outage keeps it. The server's `GET /config/commit/pending` stays authoritative
 * whenever it answers.
 */
export interface TrackedCommit {
  txnId: string;
  /** Local-clock deadline (ms). Never later than request time + window, so the local countdown errs on the short side. */
  deadlineMs: number;
  kind: 'commit' | 'rollback';
  /** When tracking started; server answers older than this cannot resolve the commit. */
  trackedAt: number;
}

export type CommitOutcomeKind = 'confirmed' | 'confirmedElsewhere' | 'reverted';

export interface ConfirmState {
  tracked: TrackedCommit | null;
  /** Result of the last tracked commit, shown until dismissed. */
  outcome: { kind: CommitOutcomeKind; txnId: string; revision?: number | undefined } | null;
}

const KEY = 'vrx.confirm';
const listeners = new Set<() => void>();

function load(): ConfirmState {
  try {
    const raw = globalThis.sessionStorage?.getItem(KEY);
    if (raw) {
      const s = JSON.parse(raw) as Partial<ConfirmState>;
      return { tracked: s.tracked ?? null, outcome: s.outcome ?? null };
    }
  } catch {
    // unavailable storage: in-memory only
  }
  return { tracked: null, outcome: null };
}

let state: ConfirmState = load();

function set(next: ConfirmState): void {
  state = next;
  try {
    globalThis.sessionStorage?.setItem(KEY, JSON.stringify(state));
  } catch {
    // ignore
  }
  for (const l of listeners) l();
}

export const confirmStore = {
  get: (): ConfirmState => state,
  subscribe(l: () => void): () => void {
    listeners.add(l);
    return () => {
      listeners.delete(l);
    };
  },
  track(t: TrackedCommit): void {
    set({ tracked: t, outcome: null });
  },
  /** Align the local deadline with the server's (never extend it past what we already show). */
  adjustDeadline(txnId: string, deadlineMs: number): void {
    const t = state.tracked;
    if (t && t.txnId === txnId && deadlineMs < t.deadlineMs - 1000) set({ ...state, tracked: { ...t, deadlineMs } });
  },
  resolve(kind: CommitOutcomeKind, txnId: string, revision?: number): void {
    set({ tracked: state.tracked?.txnId === txnId ? null : state.tracked, outcome: { kind, txnId, revision } });
  },
  dismiss(): void {
    set({ ...state, outcome: null });
  },
  /** Tests / sign-out. */
  reset(): void {
    set({ tracked: null, outcome: null });
  },
};

export function useConfirmState(): ConfirmState {
  return useSyncExternalStore(confirmStore.subscribe, confirmStore.get, confirmStore.get);
}

/** `m:ss` of a remaining duration (never negative). */
export function formatCountdown(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000));
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, '0')}`;
}
