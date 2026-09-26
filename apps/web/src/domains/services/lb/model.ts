import type { paths } from '@ngfw/api-client';
import type { LbConfig, LbVipConfig } from '@ngfw/schema';
import type { VrxStatus } from '@ngfw/ui-kit';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { domainSchemas } from '../../../schema/registry';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;

/** `GET /api/v1/state/lb/vips` as generated from the OpenAPI document (never hand-written). */
export type LbVipsState = Ok<NonNullable<paths['/api/v1/state/lb/vips']['get']>>;
export type LbVipItem = LbVipsState['items'][number];
export type LbVipStatus = LbVipItem['status'];
export type { LbConfig, LbVipConfig };

/** The candidate's `services.lb` as the config route returns it (members may be absent before defaults). */
export type LbCandidate = Partial<Omit<LbConfig, 'vips'>> & {
  vips?: Record<string, Partial<LbVipConfig>>;
};

type Props = Record<string, JsonSchema>;
const propsOf = (s: JsonSchema | undefined): Props => (s?.properties ?? {}) as Props;

/** `services.lb` — the one schema (00-CONTEXT rule 5), from the generated domain JSON Schema. */
export function lbSchema(): JsonSchema {
  const s = propsOf(domainSchemas.services)['lb'];
  if (s === undefined) throw new Error('services.lb schema not found');
  return s;
}

/** `services.lb.vips.<name>` item schema. */
export function vipSchema(): JsonSchema {
  const vips = propsOf(lbSchema())['vips'] as { additionalProperties?: JsonSchema } | undefined;
  if (!vips?.additionalProperties || typeof vips.additionalProperties !== 'object')
    throw new Error('lb VIP schema not found');
  return vips.additionalProperties;
}

/** `services.lb` without `vips` (edited in the VIP table): the global settings and the NAT interfaces. */
export function settingsFormSchema(): JsonSchema {
  const s = lbSchema();
  const props = { ...propsOf(s) };
  delete props['vips'];
  return { ...s, properties: props } as JsonSchema;
}

/** The status column: active = up, no server in use = degraded, missing in VPP = down, not applied = adminDown. */
export function statusOf(s: LbVipStatus): VrxStatus {
  switch (s) {
    case 'active':
      return 'up';
    case 'no-servers':
      return 'degraded';
    case 'missing':
      return 'down';
    default:
      return 'adminDown';
  }
}

/** A server of lb_as_dump: in use = up, removed (waiting for the garbage collection) = adminDown. */
export function serverStatus(inUse: boolean): VrxStatus {
  return inUse ? 'up' : 'adminDown';
}

/** `tcp/80`, `udp/53` or `any`. */
export function protoPort(v: { protocol?: string | undefined; port?: number | undefined }): string {
  return v.protocol === undefined || v.protocol === 'any' || !v.port
    ? 'any'
    : `${v.protocol}/${v.port}`;
}

/** Removed copies VPP still lists for a VIP (vppEntries − 1 when it exists; the servers of those are "removed"). */
export function removedCopies(it: LbVipItem): number {
  return Math.max(0, it.vppEntries - 1);
}
