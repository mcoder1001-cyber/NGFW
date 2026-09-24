import { useSyncExternalStore } from 'react';

/**
 * The confirmed commit this browser started (docs/05 "Signature UX"): kept locally so the countdown keeps running when
 * the device stops answering — which is exactly what happens when a commit cuts the operator's own access. Persisted in
 * sessionStorage so a reload during the outage keeps it. The server's `GET /config/commit/pending` stays authoritative
 * whenever it answers.
 */
/**
 * What the agent reported when it applied the commit that now waits for confirmation (review M1). The confirm answer
 * itself carries no apply details, so this is kept and shown in the countdown and again in the confirmed outcome.
 */
export interface ApplySummary {
  status: 'applied' | 'partially-applied' | 'not-applied';
  notApplied: string[];
  warnings: { pointer: string; message: string; rule?: string }[];
  results: { key: string; op: string; code: string; message: string; pointer: string; subsystem: string }[];
  sync?: { state: 'in-sync' | 'unknown' | 'degraded'; reason: string; txnId: string | null; since: string } | undefined;
}

/**
 * P06's rule (D-P06-15) applied to a pending answer: nothing listed in `notApplied` → applied; every changed domain
 * listed → not-applied; otherwise partially-applied. `notApplied` already names only CHANGED unimplemented domains.
 */
export function applyStatus(notApplied: readonly string[], changedDomains: readonly string[]): ApplySummary['status'] {
  if (notApplied.length === 0) return 'applied';
  const changed = changedDomains.filter((d) => d !== '');
  return changed.length > 0 && changed.every((d) => notApplied.includes(d)) ? 'not-applied' : 'partially-applied';
}

export interface TrackedCommit {
  txnId: string;
  /** Local-clock deadline (ms). Never later than request time + window, so the local countdown errs on the short side. */
  deadlineMs: number;
  kind: 'commit' | 'rollback';
  /** When tracking started; server answers older than this cannot resolve the commit. */
  trackedAt: number;
  /** Apply result of the pending answer (absent for a commit started in another session). */
  summary?: ApplySummary | undefined;
}

export type CommitOutcomeKind = 'confirmed' | 'confirmedElsewhere' | 'reverted';

export interface ConfirmState {
  tracked: TrackedCommit | null;
  /** Result of the last tracked commit, shown until dismissed. */
  outcome: { kind: CommitOutcomeKind; txnId: string; revision?: number | undefined; summary?: ApplySummary | undefined } | null;
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
    // first answer wins: a late poll-based resolution must not overwrite this session's own confirm (or vice versa)
    if (state.tracked?.txnId !== txnId && state.outcome?.txnId === txnId) return;
    const summary = state.tracked?.txnId === txnId ? state.tracked.summary : undefined;
    set({ tracked: state.tracked?.txnId === txnId ? null : state.tracked, outcome: { kind, txnId, revision, summary } });
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
