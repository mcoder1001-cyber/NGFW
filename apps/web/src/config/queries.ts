import type { paths } from '@ngfw/api-client';
import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query';
import { api } from '../api';
import { call } from '../api-problem';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } } ? T : never;
/** JSON body of the 200 answer of `M P` in the generated OpenAPI types (never hand-written). */
type Json<P extends keyof paths, M extends 'get' | 'post' | 'patch'> = Ok<NonNullable<paths[P][M]>>;

export type ConfigDiff = Json<'/api/v1/config/diff', 'get'>;
export type Change = ConfigDiff['changes'][number];
export type LockInfo = Json<'/api/v1/config/lock', 'get'>;
export type PendingInfo = NonNullable<Json<'/api/v1/config/commit/pending', 'get'>['pending']>;
export type CommitResult = Json<'/api/v1/config/commit', 'post'>;
export type ValidateResult = Json<'/api/v1/config/validate', 'post'>;
export type SystemState = Json<'/api/v1/state/system', 'get'>;
export type RevisionPage = Json<'/api/v1/config/revisions', 'get'>;
export type RevisionMeta = RevisionPage['items'][number];
export type Revision = Json<'/api/v1/config/revisions/{rev}', 'get'>;
export type SyncState = NonNullable<CommitResult['sync']>;

/** Query keys mirror API paths (docs/05 "Query keys mirror API paths"). */
export const qk = {
  config: ['config'] as const,
  diff: ['config', 'diff'] as const,
  lock: ['config', 'lock'] as const,
  pending: ['config', 'commit', 'pending'] as const,
  revisions: (limit: number, offset: number) => ['config', 'revisions', { limit, offset }] as const,
  revision: (rev: number) => ['config', 'revisions', rev] as const,
  candidate: (path: string) => ['config', 'candidate', path] as const,
  system: ['state', 'system'] as const,
};

/** Poll period of the pending-change bar (the WS `commit.events` topic invalidates earlier). */
export const POLL_MS = 5_000;

export function useDiff() {
  return useQuery({
    queryKey: qk.diff,
    queryFn: async ({ signal }) => (await call(api.GET('/api/v1/config/diff', { signal }))).data,
    refetchInterval: POLL_MS,
  });
}

export function useLock() {
  return useQuery({
    queryKey: qk.lock,
    queryFn: async ({ signal }) => (await call(api.GET('/api/v1/config/lock', { signal }))).data,
    refetchInterval: POLL_MS,
  });
}

/** The unconfirmed commit plus the server clock offset (from the `Date` header) for the local countdown. */
export function usePending() {
  return useQuery({
    queryKey: qk.pending,
    queryFn: async ({ signal }) => {
      const t0 = Date.now();
      const { data, response } = await call(api.GET('/api/v1/config/commit/pending', { signal }));
      return { pending: data.pending, response, t0 };
    },
    refetchInterval: POLL_MS,
    // keep the last answer while the server is unreachable: the countdown continues locally
    retry: false,
  });
}

export function useSystemState() {
  return useQuery({
    queryKey: qk.system,
    queryFn: async ({ signal }) => (await call(api.GET('/api/v1/state/system', { signal }))).data,
    refetchInterval: POLL_MS,
  });
}

export function invalidateConfig(qc: QueryClient) {
  return Promise.all([qc.invalidateQueries({ queryKey: qk.config }), qc.invalidateQueries({ queryKey: qk.system })]);
}

export function useValidate() {
  return useMutation({
    mutationFn: async () => (await call(api.POST('/api/v1/config/validate'))).data,
  });
}

export interface CommitArgs {
  comment: string;
  /** Auto-revert window in seconds; undefined = commit without confirmation. */
  confirmSec?: number | undefined;
}

export interface CommitOutcome {
  result: CommitResult;
  /** Client time when the request was sent (lower bound for the local countdown). */
  sentAt: number;
  response: Response;
}

function commitQuery({ comment, confirmSec }: CommitArgs) {
  return {
    ...(comment.trim() ? { comment: comment.trim() } : {}),
    ...(confirmSec !== undefined ? { confirm: confirmSec } : {}),
  };
}

export function useCommit() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (args: CommitArgs): Promise<CommitOutcome> => {
      const sentAt = Date.now();
      const { data, response } = await call(api.POST('/api/v1/config/commit', { params: { query: commitQuery(args) } }));
      return { result: data, sentAt, response };
    },
    // not awaited: TanStack runs the caller's mutate() callbacks only after this settles (the countdown must start at once)
    onSettled: () => {
      void invalidateConfig(qc);
    },
  });
}

export function useRollback() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ rev, ...args }: CommitArgs & { rev: number }): Promise<CommitOutcome> => {
      const sentAt = Date.now();
      const { data, response } = await call(
        api.POST('/api/v1/config/rollback/{rev}', { params: { path: { rev }, query: commitQuery(args) } }),
      );
      return { result: data, sentAt, response };
    },
    // not awaited: TanStack runs the caller's mutate() callbacks only after this settles (the countdown must start at once)
    onSettled: () => {
      void invalidateConfig(qc);
    },
  });
}

export function useConfirm() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await call(api.POST('/api/v1/config/commit/confirm'))).data,
    // not awaited: TanStack runs the caller's mutate() callbacks only after this settles (the countdown must start at once)
    onSettled: () => {
      void invalidateConfig(qc);
    },
  });
}

export function useDiscard() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await call(api.POST('/api/v1/config/discard'))).data,
    // not awaited: TanStack runs the caller's mutate() callbacks only after this settles (the countdown must start at once)
    onSettled: () => {
      void invalidateConfig(qc);
    },
  });
}

export function useBreakLock() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => (await call(api.DELETE('/api/v1/config/lock'))).data,
    // not awaited: TanStack runs the caller's mutate() callbacks only after this settles (the countdown must start at once)
    onSettled: () => {
      void invalidateConfig(qc);
    },
  });
}

export function useRevisions(limit: number, offset: number) {
  return useQuery({
    queryKey: qk.revisions(limit, offset),
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/config/revisions', { params: { query: { limit, offset } }, signal }))).data,
  });
}

export function fetchRevision(rev: number, signal?: AbortSignal) {
  return call(api.GET('/api/v1/config/revisions/{rev}', { params: { path: { rev } }, ...(signal ? { signal } : {}) })).then((r) => r.data);
}

export function useRevision(rev: number | null) {
  return useQuery({
    queryKey: qk.revision(rev ?? 0),
    queryFn: ({ signal }) => fetchRevision(rev!, signal),
    enabled: rev !== null && rev > 0,
    staleTime: Infinity, // revisions are immutable
  });
}

/** The running document (redacted), for "compare with running". */
export function useRunning(enabled: boolean) {
  return useQuery({
    queryKey: ['config', 'running'] as const,
    queryFn: async ({ signal }) => (await call(api.GET('/api/v1/config', { signal }))).data as unknown,
    enabled,
  });
}
