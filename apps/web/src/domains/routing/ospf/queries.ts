import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { JsonSchema, ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { api } from '../../../api';
import { ApiError, call } from '../../../api-problem';
import { invalidateConfig, qk } from '../../../config/queries';
import { domainSchemas } from '../../../schema/registry';

/**
 * WEB-4a: queries shared by the unrouted OSPF, IS-IS/RIP and BFD screens. `routing` as the candidate / running
 * configuration holds it (packages/schema domains/routing.ts); only the parts these screens read are typed.
 */
export type RedistributeSource = 'connected' | 'static' | 'bgp' | 'ospf' | 'isis' | 'rip';
export const REDISTRIBUTE_SOURCES: RedistributeSource[] = [
  'connected',
  'static',
  'bgp',
  'ospf',
  'isis',
  'rip',
];
export type Redistribute = Partial<
  Record<RedistributeSource, { routeMap?: string; metric?: number }>
>;

export interface OspfConfig {
  routerId?: string;
  vrf?: string;
  areas?: Record<string, { type?: 'normal' | 'stub' | 'nssa'; noSummary?: boolean }>;
  interfaces?: Record<
    string,
    { area: string; cost?: number; passive?: boolean; networkType?: string; bfd?: boolean }
  >;
  redistribute?: Redistribute;
  defaultInformationOriginate?: 'off' | 'on' | 'always';
}
export interface IsisConfig {
  net: string;
  level?: string;
  vrf?: string;
  interfaces?: Record<
    string,
    { passive?: boolean; metric?: number; circuitType?: string; bfd?: boolean }
  >;
  redistribute?: Redistribute;
}
export interface RipConfig {
  vrf?: string;
  networks?: string[];
  interfaces?: Record<string, { passive?: boolean }>;
  redistribute?: Redistribute;
  defaultMetric?: number;
}
export interface BfdSession {
  interface: string;
  localAddress: string;
  peerAddress: string;
  desiredMinTxUs?: number;
  requiredMinRxUs?: number;
  detectMultiplier?: number;
  enabled?: boolean;
}
export interface RoutingIgp {
  bgp?: { redistribute?: Redistribute };
  ospf?: OspfConfig;
  isis?: IsisConfig;
  rip?: RipConfig;
  bfd?: { sessions?: BfdSession[] };
}
export type Protocol = 'ospf' | 'isis' | 'rip' | 'bfd';

const PATH = 'routing';
export const routingKeys = { running: ['config', 'running', PATH] as const };

export function useCandidateRouting() {
  return useQuery({
    queryKey: qk.candidate(PATH),
    queryFn: async ({ signal }) =>
      ((
        await call(
          api.GET('/api/v1/config/candidate/{path}', { params: { path: { path: PATH } }, signal }),
        )
      ).data ?? {}) as RoutingIgp,
  });
}

export function useRunningRouting() {
  return useQuery({
    queryKey: routingKeys.running,
    queryFn: async ({ signal }) =>
      ((await call(api.GET('/api/v1/config/{path}', { params: { path: { path: PATH } }, signal })))
        .data ?? {}) as RoutingIgp,
  });
}

/** A merge patch of the candidate's `routing` (`{ ospf: … }`, `{ rip: null }` removes); the pending bar commits it. */
export function usePatchRouting() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: Record<string, unknown>) =>
      call(
        api.PATCH('/api/v1/config/{path}', {
          params: { path: { path: PATH } },
          body: patch as never,
        }),
      ),
    onSettled: () => invalidateConfig(qc),
  });
}

const isObj = (v: unknown): v is Record<string, unknown> =>
  typeof v === 'object' && v !== null && !Array.isArray(v);

/**
 * RFC 7396 patch turning `before` into `after`: objects recurse, keys missing from `after` become null (records such
 * as areas / interfaces / redistribute lose entries this way), arrays and scalars are replaced whole.
 */
export function mergePatch(before: unknown, after: unknown): unknown {
  if (!isObj(before) || !isObj(after)) return after;
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(after)) out[k] = k in before ? mergePatch(before[k], v) : v;
  for (const k of Object.keys(before)) if (!(k in after)) out[k] = null;
  return out;
}

/** A protocol's sub-schema of the `routing` domain — the one schema, never a hand-written form. */
export function routingSubSchema(key: Protocol): JsonSchema {
  const r = domainSchemas.routing as { properties?: Record<string, JsonSchema> };
  const s = r.properties?.[key];
  if (!s) throw new Error(`routing.${key} schema not found`);
  return s;
}

/** Server problem with pointers made relative to a sub-form (`/routing/ospf/areas` → `/areas`). */
export function problemUnder(error: unknown, base: string): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  return {
    ...p,
    errors: (p.errors ?? []).map((e) => ({
      ...e,
      pointer: e.pointer.startsWith(base) ? e.pointer.slice(base.length) || '/' : e.pointer,
    })),
  };
}

export const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
