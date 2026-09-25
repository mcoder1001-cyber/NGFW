import type { paths } from '@ngfw/api-client';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { natSchema, type Session } from '../nat44-ed-sessions/model';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;
type Body<O> = O extends { requestBody: { content: { 'application/json': infer T } } } ? T : never;

/** i18n namespace of F-nat44-ei-64-66-nptv6 (locale namespace = task slug). */
export const NS = 'nat44-ei-64-66-nptv6';

/** `GET /api/v1/state/nat/ei/sessions` (the NAT44-ED page shape), generated from the OpenAPI document. */
export type EiSessionsPage = Ok<NonNullable<paths['/api/v1/state/nat/ei/sessions']['get']>>;
/** `POST /api/v1/actions/nat/ei/sessions/kill` body. */
export type EiKillBody = Body<NonNullable<paths['/api/v1/actions/nat/ei/sessions/kill']['post']>>;
/** `GET /api/v1/state/nat/nat64/sessions`. */
export type Nat64SessionsPage = Ok<NonNullable<paths['/api/v1/state/nat/nat64/sessions']['get']>>;
export type Nat64Session = Nat64SessionsPage['items'][number];
/** `GET /api/v1/state/nat/nptv6`. */
export type Nptv6State = Ok<NonNullable<paths['/api/v1/state/nat/nptv6']['get']>>;
/** `GET /api/v1/state/drift`. */
export type Drift = Ok<NonNullable<paths['/api/v1/state/drift']['get']>>;

/** The translators of this task that are edited as one subtree of `nat` each. */
export type Subtree = 'nat64' | 'nat66' | 'nptv6';

/**
 * The JSON Schema of `nat.<subtree>` (the one schema, rule 5) with the root's `$defs`, so references still resolve.
 */
export function subtreeSchema(key: Subtree): JsonSchema {
  const root = natSchema();
  const props = (root.properties ?? {}) as Record<string, JsonSchema>;
  const s = props[key];
  if (!s || typeof s !== 'object') throw new Error(`nat.${key} schema not found`);
  return { ...s, ...(root.$defs ? { $defs: root.$defs } : {}) } as JsonSchema;
}

type Translate = (key: string, opts?: Record<string, unknown>) => string;

/**
 * Titles and help texts of every property at every depth (objects and array items) from `field.<name>.title|help`,
 * the English schema title as the fallback; fieldset groups from `group.<name>`.
 */
export function localizeDeep(schema: JsonSchema, t: Translate): JsonSchema {
  const walk = (s: JsonSchema): JsonSchema => {
    const out: Record<string, unknown> = { ...s };
    if (s.properties && typeof s.properties === 'object') {
      const props: Record<string, JsonSchema> = {};
      for (const [name, raw] of Object.entries(s.properties as Record<string, JsonSchema>)) {
        const prop = walk(raw);
        const hints = (prop['x-vrx-ui'] ?? {}) as Record<string, unknown>;
        const help = t(`field.${name}.help`, { defaultValue: '' });
        props[name] = {
          ...prop,
          title: t(`field.${name}.title`, { defaultValue: prop.title ?? name }),
          'x-vrx-ui': { ...hints, ...(help ? { help } : {}) },
        } as JsonSchema;
      }
      out['properties'] = props;
    }
    const items = (s as { items?: unknown }).items;
    if (items && typeof items === 'object' && !Array.isArray(items)) {
      out['items'] = walk(items as JsonSchema);
    }
    return out as JsonSchema;
  };
  return walk(schema);
}

/** The EI kill for one session row: the inside endpoint and VRF (NAT44-EI keys sessions by it). */
export function eiKillBodyOf(s: Session): EiKillBody {
  const protocol =
    s.protocol === 'tcp' || s.protocol === 'udp' || s.protocol === 'icmp' ? s.protocol : 'tcp';
  return {
    protocol,
    insideAddress: s.insideAddress,
    insidePort: s.insidePort,
    externalAddress: s.externalAddress,
    externalPort: s.externalPort,
    vrf: s.vrf,
  };
}

/** Stable row id of a NAT64 session. */
export function nat64SessionId(s: Nat64Session): string {
  return `${s.tableId}|${s.protocol}|${s.client}:${s.clientPort}|${s.remote}:${s.remotePort}`;
}

/** `v` without any `description` member, at every depth (VPP never stores descriptions: Retrieve cannot return them). */
function withoutDescriptions(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(withoutDescriptions);
  if (v !== null && typeof v === 'object') {
    return Object.fromEntries(
      Object.entries(v as Record<string, unknown>)
        .filter(([k]) => k !== 'description')
        .map(([k, x]) => [k, withoutDescriptions(x)]),
    );
  }
  return v;
}

/**
 * Drift entries of `/nat/<subtree>` (the applied state differs from the running configuration there). The API compares
 * lists as a whole, so a list whose only difference is a `description` (configuration-only, never read back from VPP)
 * is not drift.
 */
export function driftUnder(drift: Drift | undefined, pointer: string): Drift['changes'] {
  return (drift?.changes ?? []).filter(
    (c) =>
      (c.pointer === pointer || c.pointer.startsWith(`${pointer}/`)) &&
      JSON.stringify(withoutDescriptions(c.from)) !== JSON.stringify(withoutDescriptions(c.to)),
  );
}
