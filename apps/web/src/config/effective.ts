import { useQuery } from '@tanstack/react-query';
import { useMemo, useSyncExternalStore } from 'react';
import { api } from '../api';
import { call } from '../api-problem';
import { useAuth } from '../auth/AuthProvider';
import type { DiffChange } from './DiffView';
import { qk, useDiff, useLock, type LockInfo } from './queries';
import { refineChanges } from './refine';

/**
 * Write-only members (password hashes) are redacted everywhere (D-046/D-070), so P06's `/config/diff` does not show an
 * edit that only sets a hash (review H1). TD-2 adds `{op, pointer, redacted: true}` entries to the diff; until it lands
 * (and as a fallback) the UI derives them itself:
 *   1. usernames whose hash was set through the Users form in this candidate are remembered (sessionStorage, no
 *      value — never the hash) and become `…/users/<i>/passwordHash` redacted entries;
 *   2. the candidate lock held by me with no visible change means a hidden (write-only) change exists.
 * Both are marked `synthetic`. Server-reported redacted entries win over synthetic ones for the same pointer.
 */
const KEY = 'vrx.secretEdits';
const listeners = new Set<() => void>();

function load(): string[] {
  try {
    const raw = globalThis.sessionStorage?.getItem(KEY);
    const v = raw ? (JSON.parse(raw) as unknown) : [];
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [];
  } catch {
    return [];
  }
}

let edited: string[] = load();

function save(next: string[]): void {
  edited = next;
  try {
    globalThis.sessionStorage?.setItem(KEY, JSON.stringify(next));
  } catch {
    // in-memory only
  }
  for (const l of listeners) l();
}

export const secretEdits = {
  get: (): string[] => edited,
  subscribe(l: () => void): () => void {
    listeners.add(l);
    return () => {
      listeners.delete(l);
    };
  },
  /** A new password hash for `username` was written into the candidate. */
  markPassword(username: string): void {
    if (!edited.includes(username)) save([...edited, username]);
  },
  clear(): void {
    if (edited.length > 0) save([]);
  },
};

export function useSecretEdits(): string[] {
  return useSyncExternalStore(secretEdits.subscribe, secretEdits.get, secretEdits.get);
}

export function isStale(lock: LockInfo): boolean {
  return lock.expiresAt !== null && Date.parse(lock.expiresAt) <= Date.now();
}

/** One rule for "someone else holds the candidate" (review L2): a stale lock can be taken over, so it does not count. */
export function lockedByOther(lock: LockInfo | undefined, me: string | undefined): boolean {
  return !!lock?.locked && lock.owner !== me && !isStale(lock);
}

/**
 * Server diff + redacted entries (server or synthetic), not yet refined. Pure, for unit tests.
 */
export function effectiveChanges(
  server: readonly DiffChange[],
  opts: { editedUsers: readonly string[]; candidateUsers: readonly { username: string }[] | undefined; lockMine: boolean },
): DiffChange[] {
  const out: DiffChange[] = [...server];
  const have = new Set(server.map((c) => c.pointer));
  opts.editedUsers.forEach((name) => {
    const i = opts.candidateUsers?.findIndex((u) => u.username === name) ?? -1;
    const pointer = `/management/users/${i}/passwordHash`;
    if (i >= 0 && !have.has(pointer)) out.push({ op: 'replace', pointer, redacted: true, synthetic: true });
  });
  if (out.length === 0 && opts.lockMine) out.push({ op: 'replace', pointer: '', redacted: true, synthetic: true });
  return out;
}

export interface EffectiveChanges {
  /** Server changes plus redacted entries; pass to `<DiffView>` / the commit dialog. */
  changes: DiffChange[];
  /** What the bar counts (per-item refined). */
  count: number;
  dirty: boolean;
  lockMine: boolean;
  lockedByOther: boolean;
  lock: LockInfo | undefined;
}

export function useEffectiveChanges(): EffectiveChanges & { diff: ReturnType<typeof useDiff> } {
  const diff = useDiff();
  const lock = useLock();
  const { state } = useAuth();
  const me = state.user?.username;
  const editedUsers = useSecretEdits();
  const lockMine = !!lock.data?.locked && lock.data.owner === me;
  // the candidate users are needed only to place remembered password edits
  const users = useQuery({
    queryKey: qk.candidate('management'),
    queryFn: async ({ signal }) =>
      ((await call(api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: 'management' } }, signal }))).data as { users?: { username: string }[] })
        .users ?? [],
    enabled: editedUsers.length > 0,
  });
  // the lock is released by commit/discard: remembered edits are then either committed or gone
  if (lock.data && !lock.data.locked && editedUsers.length > 0) queueMicrotask(() => secretEdits.clear());
  const server = diff.data?.changes;
  const changes = useMemo(
    () => effectiveChanges((server ?? []) as DiffChange[], { editedUsers, candidateUsers: users.data, lockMine }),
    [server, editedUsers, users.data, lockMine],
  );
  const count = useMemo(() => refineChanges(changes).length, [changes]);
  return {
    diff,
    changes,
    count,
    dirty: count > 0,
    lockMine,
    lockedByOther: lockedByOther(lock.data, me),
    lock: lock.data,
  };
}
