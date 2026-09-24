import type { paths } from '@ngfw/api-client';
import {
  LldpSchema,
  MirrorSessionSchema,
  NsimSchema,
  type InterfaceConfig,
  type LldpConfig,
  type MirrorSessionConfig,
  type NsimConfig,
} from '@ngfw/schema';
import type { VrxStatus } from '@ngfw/ui-kit';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { z } from 'zod';
import type { InterfaceItem } from '../model';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;

/** `GET /api/v1/state/lldp/neighbors` as generated from the OpenAPI document (never hand-written). */
export type NeighborsPage = Ok<NonNullable<paths['/api/v1/state/lldp/neighbors']['get']>>;
export type NeighborRow = NeighborsPage['items'][number] & { id: string };

export type { InterfaceConfig, LldpConfig, MirrorSessionConfig, NsimConfig };

const OPTIONS = { target: 'draft-2020-12', io: 'input' } as const;

/** Form schemas — the one Zod schema of `packages/schema` as JSON Schema (00-CONTEXT rule 5). */
export const formSchemas = {
  lldp: () => z.toJSONSchema(LldpSchema, OPTIONS) as JsonSchema,
  nsim: () => z.toJSONSchema(NsimSchema, OPTIONS) as JsonSchema,
  mirror: () => z.toJSONSchema(MirrorSessionSchema, OPTIONS) as JsonSchema,
};

/** One mirror session of the candidate: `interfaces.<source>.mirror[index]`. */
export interface SessionRow {
  id: string;
  source: string;
  index: number;
  destination: string;
  direction: string;
  level: string;
  /** the agent reports it (Retrieve view of `/state/interfaces`), i.e. VPP mirrors it */
  active: boolean;
  /** differs from running (the candidate is pending) */
  pending: boolean;
}

const sessionKey = (m: { destination?: string; level?: string }) =>
  `${m.destination ?? ''}\u0000${m.level ?? 'device'}`;

/**
 * Every mirror session of the candidate interfaces, sorted by source, marked with the agent's view (`config` of the
 * `/state/interfaces` item: what Retrieve reports) and with a pending change (candidate ≠ running).
 */
export function sessionsOf(
  candidate: Record<string, InterfaceConfig> | undefined,
  state: InterfaceItem[] | undefined,
): SessionRow[] {
  const byName = new Map((state ?? []).map((i) => [i.name, i]));
  const out: SessionRow[] = [];
  for (const [source, itf] of Object.entries(candidate ?? {})) {
    const item = byName.get(source);
    const actual = new Set(
      ((item?.config?.['mirror'] ?? []) as { destination?: string; level?: string }[]).map(
        sessionKey,
      ),
    );
    const running = ((item?.running?.['mirror'] ?? []) as MirrorSessionConfig[]).map(
      (m) => `${sessionKey(m)}\u0000${m.direction ?? 'both'}`,
    );
    (itf.mirror ?? []).forEach((m, index) => {
      out.push({
        id: `${source}#${index}`,
        source,
        index,
        destination: m.destination,
        direction: m.direction ?? 'both',
        level: m.level ?? 'device',
        active: actual.has(sessionKey(m)),
        pending: !running.includes(`${sessionKey(m)}\u0000${m.direction ?? 'both'}`),
      });
    });
  }
  return out.sort(
    (a, b) => a.source.localeCompare(b.source, undefined, { numeric: true }) || a.index - b.index,
  );
}

/**
 * Merge patch (for PATCH /config/interfaces) that replaces the session list of `source` (RFC 7396 replaces arrays
 * whole; an empty list removes the key).
 */
export function mirrorPatch(
  source: string,
  sessions: MirrorSessionConfig[],
): Record<string, unknown> {
  return { [source]: { mirror: sessions.length ? sessions : null } };
}

/** `sessions` with entry `index` replaced by `next` (null removes it; index −1 appends). */
export function withSession(
  sessions: MirrorSessionConfig[] | undefined,
  index: number,
  next: MirrorSessionConfig | null,
): MirrorSessionConfig[] {
  const list = [...(sessions ?? [])];
  if (index < 0) {
    if (next) list.push(next);
    return list;
  }
  if (next) list[index] = next;
  else list.splice(index, 1);
  return list;
}

/** "12 s" / "3 min" / "2 h" age text (seconds, VPP clock estimate); "" for 0 = never. */
export function ageText(
  sec: number,
  t: (k: string, o?: Record<string, unknown>) => string,
): string {
  if (!(sec > 0)) return '';
  if (sec < 90) return t('age.sec', { n: Math.round(sec) });
  if (sec < 5400) return t('age.min', { n: Math.round(sec / 60) });
  return t('age.hour', { n: Math.round(sec / 3600) });
}

/** Rows of a neighbours page with a stable grid id. */
export function neighborRows(p: NeighborsPage): NeighborRow[] {
  return p.items.map((n) => ({ ...n, id: `${n.interface}#${n.swIfIndex}` }));
}

/** Table cells aligned to the end of the row (logical, RTL-safe). */
export const END_CELL = { textAlign: 'end' } as const;
export const MONO = { fontFamily: 'monospace' } as const;

/** Parent interfaces of the candidate (mirror sources and nsim / LLDP interfaces are parents), sorted. */
export function parentNames(ifs: Record<string, InterfaceConfig> | undefined): string[] {
  return Object.keys(ifs ?? {}).sort((a, b) => a.localeCompare(b, undefined, { numeric: true }));
}

/** Every interface name of the candidate: parents and `<parent>.<id>` sub-interfaces (mirror destinations). */
export function allNames(ifs: Record<string, InterfaceConfig> | undefined): string[] {
  const out: string[] = [];
  for (const [p, itf] of Object.entries(ifs ?? {})) {
    out.push(p);
    for (const id of Object.keys(itf.subinterfaces ?? {})) out.push(`${p}.${id}`);
  }
  return out.sort((a, b) => a.localeCompare(b, undefined, { numeric: true }));
}

type Translate = (key: string, opts?: Record<string, unknown>) => string;

/**
 * `localizeSchema` (P08, top-level properties only) applied to nested objects and array items as well, so the LLDP
 * interface rows and the nsim cross-connect are translated too (`loopback-bvi-gso-lldp-span:field.<name>.title|help`).
 */
export function localizeAll(
  schema: JsonSchema,
  t: Translate,
  localize: (s: JsonSchema, t: Translate) => JsonSchema,
): JsonSchema {
  const top = localize(schema, t);
  const props = (top.properties ?? {}) as Record<string, JsonSchema>;
  const out: Record<string, JsonSchema> = {};
  for (const [name, p] of Object.entries(props)) {
    let q = p;
    if (q.properties) q = localizeAll(q, t, localize);
    const items = q.items as JsonSchema | undefined;
    if (items && typeof items === 'object' && !Array.isArray(items) && items.properties) {
      q = { ...q, items: localizeAll(items, t, localize) } as JsonSchema;
    }
    out[name] = q;
  }
  return { ...top, properties: out } as JsonSchema;
}

/** Chip status of "present / heard": up when true. */
export function presence(on: boolean): VrxStatus {
  return on ? 'up' : 'down';
}

/** Pointers of the two `services` forms (server problems are mapped relative to them). */
export const LLDP_POINTER = '/services/lldp';
export const NSIM_POINTER = '/services/nsim';
