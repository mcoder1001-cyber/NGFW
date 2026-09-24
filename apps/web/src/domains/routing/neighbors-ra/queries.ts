import type { paths } from '@ngfw/api-client';
import type { ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;

/** `GET /api/v1/state/neighbors` and `POST /api/v1/actions/arp-flush` as generated from the OpenAPI document. */
export type NeighborsPage = Ok<NonNullable<paths['/api/v1/state/neighbors']['get']>>;
export type Neighbor = NeighborsPage['items'][number];
export type NeighborsQuery = NonNullable<
  NonNullable<paths['/api/v1/state/neighbors']['get']['parameters']['query']>
>;
export type ArpFlushResult = Ok<NonNullable<paths['/api/v1/actions/arp-flush']['post']>>;

/** Query keys mirror API paths (docs/05). */
export const neighborKeys = {
  all: ['state', 'neighbors'] as const,
  page: (q: NeighborsQuery) => ['state', 'neighbors', q] as const,
};

/**
 * Live refresh: the table polls; the WS topic `neighbor.events` (agent EVENT_KIND_NEIGHBOR_CHANGED, at most 1 Hz per
 * interface) invalidates earlier once the agent publishes events (TD-8 wires the agent's event seam).
 */
export const NEIGHBORS_POLL_MS = 5_000;

const SORTABLE = new Set(['interface', 'ip', 'mac', 'age', 'vrf', 'state']);

/** Table filters outside the grid (the grid's quick filter becomes `search`). */
export interface NeighborFilters {
  interface?: string | undefined;
  family?: 'ipv4' | 'ipv6' | undefined;
  state?: 'static' | 'dynamic' | undefined;
  vrf?: string | undefined;
}

/** Grid page request (0-based page) + filters → API query (1-based page, server-side sort/filter/paging). */
export function toQuery(req: ServerPageRequest, f: NeighborFilters): NeighborsQuery {
  const q: NeighborsQuery = { page: req.page + 1, pageSize: req.pageSize };
  const sort = req.sort[0];
  if (sort && SORTABLE.has(sort.field === 'ageSec' ? 'age' : sort.field)) {
    q.sort = (sort.field === 'ageSec' ? 'age' : sort.field) as NonNullable<NeighborsQuery['sort']>;
    q.dir = sort.dir;
  }
  const search = req.quickFilter.join(' ').trim();
  if (search) q.search = search.slice(0, 64);
  if (f.interface) q.interface = f.interface;
  if (f.family) q.family = f.family;
  if (f.state) q.state = f.state;
  if (f.vrf) q.vrf = f.vrf;
  return q;
}

export async function fetchNeighbors(
  q: NeighborsQuery,
  signal?: AbortSignal,
): Promise<NeighborsPage> {
  return (
    await call(
      api.GET('/api/v1/state/neighbors', { params: { query: q }, ...(signal ? { signal } : {}) }),
    )
  ).data;
}

/** Interface names for the filter and the flush dialog (the live table of P08). */
export function useInterfaceNames() {
  return useQuery({
    queryKey: ['state', 'interfaces', 'names'],
    queryFn: async ({ signal }) =>
      (await call(api.GET('/api/v1/state/interfaces', { signal }))).data.items
        .filter((i) => i.state !== null)
        .map((i) => i.name),
    staleTime: 10_000,
  });
}

export function useArpFlush() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: { interface?: string; family?: 'ipv4' | 'ipv6' }) =>
      (await call(api.POST('/api/v1/actions/arp-flush', { body }))).data,
    onSettled: () => qc.invalidateQueries({ queryKey: neighborKeys.all }),
  });
}

/**
 * `routing.neighbors` of the candidate; null when the document has none. Read through `routing` (one path segment):
 * the generated client percent-encodes a `/` inside a path parameter, so a nested pointer cannot go in the URL.
 */
export function useCandidateNeighbors() {
  return useQuery({
    queryKey: qk.candidate('routing'),
    queryFn: async ({ signal }) => {
      const r = await call(api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: 'routing' } }, signal }));
      const routing = (r.data ?? {}) as { neighbors?: unknown };
      return routing.neighbors ?? null;
    },
  });
}

/** Write `routing.neighbors` as a merge patch of `routing` (removed members become null; the generic pointer route). */
export function usePatchNeighbors() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (neighbors: unknown) =>
      call(api.PATCH('/api/v1/config/{path}', { params: { path: { path: 'routing' } }, body: { neighbors } as never })),
    onSettled: () => invalidateConfig(qc),
  });
}
