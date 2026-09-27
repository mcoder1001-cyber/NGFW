import type { paths } from '@ngfw/api-client';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { natSchema } from '../nat44-ed-sessions/model';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;

/** i18n namespace of F-nat46 (locale namespace = task slug). */
export const NS = 'nat46';

export type Nat46Client = Ok<NonNullable<paths['/api/v1/state/nat/nat46/client']['get']>>;

/** The JSON Schema of `nat.nat46` with the root's `$defs` (the one schema, rule 5). */
export function nat46Schema(): JsonSchema {
  const root = natSchema();
  const props = (root.properties ?? {}) as Record<string, JsonSchema>;
  const s = props['nat46'];
  if (!s || typeof s !== 'object') throw new Error('nat.nat46 schema not found');
  return { ...s, ...(root.$defs ? { $defs: root.$defs } : {}) } as JsonSchema;
}
