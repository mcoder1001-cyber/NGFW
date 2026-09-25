import type { paths } from '@ngfw/api-client';
import {
  QOS_SOURCE_MAX,
  QOS_SOURCES,
  type QosConfig,
  type QosInterfaceAttachment,
  type QosMap,
  type QosPolicer,
  type QosSource,
} from '@ngfw/schema';
import type { JsonSchema, ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { ApiError } from '../../../api-problem';
import { domainSchemas } from '../../../schema/registry';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;

/** `GET /api/v1/state/services/qos/policers` as generated from the OpenAPI document (never hand-written). */
export type QosState = Ok<NonNullable<paths['/api/v1/state/services/qos/policers']['get']>>;
export type PolicerItem = QosState['items'][number];
export type Counter = PolicerItem['conform'];
/** `POST /api/v1/actions/qos/policers/{name}/reset`. */
export type ResetResult = Ok<NonNullable<paths['/api/v1/actions/qos/policers/{name}/reset']['post']>>;

/**
 * A candidate node as the API stores it: merge patches keep what was sent, so every member may be absent (a record's
 * entries are present when their key is).
 */
type Loose<T> = T extends readonly (infer U)[]
  ? Loose<U>[]
  : T extends object
    ? string extends keyof T
      ? { [K in keyof T]: Loose<T[K]> }
      : { [K in keyof T]?: Loose<T[K]> }
    : T;

/** `services.qos` of the candidate, typed from the configuration schema (`@ngfw/schema`). */
export type QosCfg = Loose<QosConfig>;
export type PolicerCfg = Loose<QosPolicer>;
export type ShaperCfg = Loose<QosConfig['shapers'][string]>;
export type MapCfg = Loose<QosMap>;
export type AttachmentCfg = Loose<QosInterfaceAttachment>;
export interface ServicesCfg {
  qos?: QosCfg;
  [key: string]: unknown;
}

export { QOS_SOURCES, QOS_SOURCE_MAX, type QosSource };

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

function prop(schema: JsonSchema | undefined, name: string): JsonSchema {
  const p = (schema?.properties as Record<string, JsonSchema> | undefined)?.[name];
  if (!p || typeof p !== 'object') throw new Error(`schema property ${name} not found`);
  return p;
}

function recordValue(schema: JsonSchema): JsonSchema {
  const v = (schema as { additionalProperties?: unknown }).additionalProperties;
  if (!isObject(v)) throw new Error('record value schema not found');
  return v as JsonSchema;
}

/** The one schema (00-CONTEXT rule 5): `services.qos` of the generated domain JSON Schema. */
function qosSchema(): JsonSchema {
  return prop(domainSchemas.services, 'qos');
}
export const policerItemSchema = (): JsonSchema => recordValue(prop(qosSchema(), 'policers'));
export const shaperItemSchema = (): JsonSchema => recordValue(prop(qosSchema(), 'shapers'));

type Translate = (key: string, opts?: Record<string, unknown>) => string;

/**
 * Titles and help in the UI language, recursively: `qos-flat:label.<base>_<path>` / `help.<base>_<path>` (path segments
 * joined with `_`). The schema's English text is the fallback. `base` names the form: policer, shaper.
 */
export function localize(schema: JsonSchema, t: Translate, base: string): JsonSchema {
  return localizeNode(schema, t, base, true);
}

function localizeNode(schema: JsonSchema, t: Translate, path: string, root: boolean): JsonSchema {
  const out: Record<string, unknown> = { ...schema };
  if (!root) {
    const hints = isObject(schema['x-vrx-ui']) ? schema['x-vrx-ui'] : {};
    const help = t(`help.${path}`, {
      defaultValue: typeof hints['help'] === 'string' ? hints['help'] : '',
    });
    out['title'] = t(`label.${path}`, { defaultValue: schema.title ?? path });
    out['x-vrx-ui'] = { ...hints, ...(help ? { help } : {}) };
  }
  if (isObject(schema.properties)) {
    out['properties'] = Object.fromEntries(
      Object.entries(schema.properties as Record<string, JsonSchema>).map(([k, v]) => [
        k,
        localizeNode(v, t, `${path}_${k}`, false),
      ]),
    );
  }
  return out as JsonSchema;
}

/** Server pointers under `prefix` (e.g. `/services/qos/policers/gold`) → pointers relative to the edited form. */
export function problemFor(error: unknown, prefix: string): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  return {
    ...p,
    errors: (p.errors ?? []).map((e) => ({
      ...e,
      pointer: e.pointer.startsWith(prefix) ? e.pointer.slice(prefix.length) : e.pointer,
    })),
  };
}

/** Server issues whose pointer is at or below `prefix` ("" = none). */
export function issuesUnder(error: unknown, prefix: string): { pointer: string; message: string }[] {
  if (!(error instanceof ApiError)) return [];
  return (error.body.errors ?? []).filter(
    (e) => e.pointer === prefix || e.pointer.startsWith(`${prefix}/`),
  );
}

/** RFC 6901 escaping of one pointer segment. */
export const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');

/** A merge patch of `services` that sets `value` (or `null` = remove) at `path` below it. */
export function nestPatch(path: readonly string[], value: unknown): Record<string, unknown> {
  let out: unknown = value;
  for (let i = path.length - 1; i >= 0; i--) out = { [path[i]!]: out };
  return out as Record<string, unknown>;
}

// ─── Rates, bursts, shapers ─────────────────────────────────────────────────

/** Minimum derived shaper burst: two full-size Ethernet frames (a bucket below one MTU drops every such frame). */
export const SHAPER_MIN_BURST_BYTES = 3000;

/**
 * The burst the agent derives for a shaper without `burstBytes` (≈ 10 ms of traffic at the rate, at least
 * {@link SHAPER_MIN_BURST_BYTES}); the API and the agent use the same formula.
 */
export function shaperBurstBytes(rateKbps: number): number {
  return Math.max(Math.ceil((rateKbps * 5) / 4), SHAPER_MIN_BURST_BYTES);
}

/**
 * The rate/burst helper of the policer form: the burst that holds `windowMs` of traffic at `rate` — bytes for kbit/s
 * (rate × window / 8), packets for packets/s (rate × window / 1000). Rounded up; 0 for invalid input.
 */
export function burstFor(rate: number, unit: 'kbps' | 'pps', windowMs: number): number {
  if (!Number.isFinite(rate) || !Number.isFinite(windowMs) || rate <= 0 || windowMs <= 0) return 0;
  return unit === 'pps' ? Math.ceil((rate * windowMs) / 1000) : Math.ceil((rate * windowMs) / 8);
}

// ─── Marking maps: 256-entry rows ↔ compact grids ───────────────────────────

/** One grid per source: cell i = the output value for recorded value i; null = not listed (VPP writes 0). */
export type Grid = (number | null)[];
export type Grids = Record<QosSource, Grid>;

/** The cells a source can carry (DSCP 0–63, PCP / EXP 0–7, ext 0–255). */
export function cellsOf(source: QosSource): number {
  return QOS_SOURCE_MAX[source] + 1;
}

/** Row entries `[{from, to}]` → a grid of `cellsOf(source)` cells (entries outside the source's range are dropped). */
export function gridFromRow(
  source: QosSource,
  row: readonly { from?: number; to?: number }[] | undefined,
): Grid {
  const grid: Grid = Array.from({ length: cellsOf(source) }, () => null);
  for (const e of row ?? []) {
    if (typeof e.from !== 'number' || typeof e.to !== 'number') continue;
    if (e.from >= 0 && e.from < grid.length) grid[e.from] = e.to;
  }
  return grid;
}

/** A grid → the schema's row form: one `{from, to}` per listed cell, sorted by `from`. */
export function rowFromGrid(grid: Grid): { from: number; to: number }[] {
  const out: { from: number; to: number }[] = [];
  grid.forEach((to, from) => {
    if (to !== null) out.push({ from, to });
  });
  return out;
}

export function gridsFromMap(map: MapCfg | undefined): Grids {
  return Object.fromEntries(
    QOS_SOURCES.map((s) => [s, gridFromRow(s, map?.rows?.[s] as { from?: number; to?: number }[])]),
  ) as Grids;
}

/** Every source with at least one listed cell (a source without entries is omitted: VPP writes 0 for it). */
export function rowsFromGrids(grids: Grids): Partial<Record<QosSource, { from: number; to: number }[]>> {
  const out: Partial<Record<QosSource, { from: number; to: number }[]>> = {};
  for (const s of QOS_SOURCES) {
    const row = rowFromGrid(grids[s]);
    if (row.length > 0) out[s] = row;
  }
  return out;
}

/** Parses one grid cell as typed: '' = not listed, otherwise an integer 0–255; undefined = invalid input. */
export function parseCell(text: string): number | null | undefined {
  const s = text.trim();
  if (s === '') return null;
  if (!/^\d{1,3}$/.test(s)) return undefined;
  const n = Number(s);
  return n <= 255 ? n : undefined;
}

// ─── Rows of the lists ──────────────────────────────────────────────────────

/** applied: running and VPP agree it exists · missing: configured, VPP has not got it · pending: only in the candidate
 * (not committed) · unmanaged: in VPP, not in the running configuration. */
export type RowStatus = 'applied' | 'missing' | 'pending' | 'unmanaged';

function statusOf(item: PolicerItem | undefined, inCandidate: boolean): RowStatus {
  if (!item) return 'pending';
  if (item.configured && item.present) return 'applied';
  if (item.configured) return 'missing';
  if (item.present) return inCandidate ? 'pending' : 'unmanaged';
  return 'pending';
}

export interface PolicerRow {
  name: string;
  type: string;
  rateUnit: string;
  cir: number | undefined;
  eir: number | undefined;
  cb: number | undefined;
  eb: number | undefined;
  status: RowStatus;
  state: PolicerItem | undefined;
  cfg: PolicerCfg | undefined;
}

/** The candidate's policers plus the ones VPP reports that no configuration names (`unmanaged`), sorted by name. */
export function policerRows(qos: QosCfg | undefined, items: readonly PolicerItem[] | undefined): PolicerRow[] {
  const cand = qos?.policers ?? {};
  const byName = new Map(
    (items ?? []).filter((i) => i.kind === 'policer').map((i) => [i.name, i] as const),
  );
  const names = [...new Set([...Object.keys(cand), ...byName.keys()])].sort((a, b) =>
    a.localeCompare(b),
  );
  return names.map((name) => {
    const cfg = cand[name];
    const st = byName.get(name);
    return {
      name,
      type: cfg?.type ?? st?.type ?? '1r2c',
      rateUnit: cfg?.rateUnit ?? st?.rateUnit ?? 'kbps',
      cir: cfg?.cir ?? st?.cir,
      eir: cfg ? cfg.eir : st?.eir || undefined,
      cb: cfg?.cb ?? st?.cb,
      eb: cfg ? cfg.eb : st?.eb || undefined,
      status: statusOf(st, cfg !== undefined),
      state: st,
      cfg,
    };
  });
}

export interface ShaperRow {
  name: string;
  rateKbps: number | undefined;
  /** Configured burst, or undefined = derived by the agent (see `derivedBurst`). */
  burstBytes: number | undefined;
  derivedBurst: number | undefined;
  status: RowStatus;
  state: PolicerItem | undefined;
  cfg: ShaperCfg | undefined;
}

/** Shapers ("rate limits") of the candidate plus VPP's `shaper:<name>` policers no configuration names. */
export function shaperRows(qos: QosCfg | undefined, items: readonly PolicerItem[] | undefined): ShaperRow[] {
  const cand = qos?.shapers ?? {};
  const byName = new Map(
    (items ?? []).filter((i) => i.kind === 'shaper').map((i) => [i.name, i] as const),
  );
  const names = [...new Set([...Object.keys(cand), ...byName.keys()])].sort((a, b) =>
    a.localeCompare(b),
  );
  return names.map((name) => {
    const cfg = cand[name];
    const st = byName.get(name);
    const rate = cfg?.rateKbps ?? st?.cir;
    return {
      name,
      rateKbps: rate,
      burstBytes: cfg ? cfg.burstBytes : st?.cb,
      derivedBurst: rate !== undefined ? shaperBurstBytes(rate) : undefined,
      status: statusOf(st, cfg !== undefined),
      state: st,
      cfg,
    };
  });
}

export interface MapRow {
  name: string;
  id: number | undefined;
  /** Listed entries per source (sources without entries omitted). */
  entries: Partial<Record<QosSource, number>>;
  usedBy: string[];
  cfg: MapCfg;
}

export function mapRows(qos: QosCfg | undefined): MapRow[] {
  const usedBy = new Map<string, string[]>();
  for (const [ifName, a] of Object.entries(qos?.interfaces ?? {})) {
    const m = a.mark?.map;
    if (m) usedBy.set(m, [...(usedBy.get(m) ?? []), ifName]);
  }
  return Object.entries(qos?.maps ?? {})
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([name, cfg]) => {
      const entries: Partial<Record<QosSource, number>> = {};
      for (const s of QOS_SOURCES) {
        const n = cfg.rows?.[s]?.length ?? 0;
        if (n > 0) entries[s] = n;
      }
      return { name, id: cfg.id, entries, usedBy: (usedBy.get(name) ?? []).sort(), cfg };
    });
}

export interface AttachmentRow {
  name: string;
  input: string | undefined;
  output: string | undefined;
  shaper: string | undefined;
  record: string | undefined;
  store: { source?: string; value?: number } | undefined;
  mark: { map?: string; output?: string } | undefined;
  description: string | undefined;
  cfg: AttachmentCfg;
}

export function attachmentRows(qos: QosCfg | undefined): AttachmentRow[] {
  return Object.entries(qos?.interfaces ?? {})
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([name, a]) => ({
      name,
      input: a.policer?.input,
      output: a.policer?.output,
      shaper: a.shaper,
      record: a.record,
      store: a.store,
      mark: a.mark,
      description: a.description,
      cfg: a,
    }));
}

// ─── Interface attachment form ──────────────────────────────────────────────

/** The attachment dialog's state: plain strings for selects ('' = none). */
export interface AttachmentForm {
  description: string;
  input: string;
  egress: 'none' | 'policer' | 'shaper';
  output: string;
  shaper: string;
  record: string;
  storeOn: boolean;
  storeSource: string;
  storeValue: string;
  markOn: boolean;
  markMap: string;
  markOutput: string;
}

export function attachmentForm(a: AttachmentCfg | undefined): AttachmentForm {
  return {
    description: a?.description ?? '',
    input: a?.policer?.input ?? '',
    egress: a?.shaper ? 'shaper' : a?.policer?.output ? 'policer' : 'none',
    output: a?.policer?.output ?? '',
    shaper: a?.shaper ?? '',
    record: a?.record ?? '',
    storeOn: a?.store !== undefined,
    storeSource: a?.store?.source ?? 'ip',
    storeValue: a?.store?.value !== undefined ? String(a.store.value) : '0',
    markOn: a?.mark !== undefined,
    markMap: a?.mark?.map ?? '',
    markOutput: a?.mark?.output ?? 'ip',
  };
}

/**
 * The attachment document of a form (only what is set; the server's schema and semantic rules decide the rest, e.g.
 * that an attachment needs at least one action). `storeValue` that is not an integer is sent as typed, so the server
 * answers with a pointer instead of the UI silently dropping it.
 */
export function attachmentValue(f: AttachmentForm): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  if (f.description.trim()) out['description'] = f.description.trim();
  const policer: Record<string, string> = {};
  if (f.input) policer['input'] = f.input;
  if (f.egress === 'policer' && f.output) policer['output'] = f.output;
  if (Object.keys(policer).length > 0) out['policer'] = policer;
  if (f.egress === 'shaper' && f.shaper) out['shaper'] = f.shaper;
  if (f.record) out['record'] = f.record;
  if (f.storeOn) {
    const v = /^\d+$/.test(f.storeValue.trim()) ? Number(f.storeValue.trim()) : f.storeValue;
    out['store'] = { source: f.storeSource, value: v };
  }
  if (f.markOn) out['mark'] = { map: f.markMap, output: f.markOutput };
  return out;
}

/** Decimal counter strings (u64) → a display-friendly number when safe, else the string itself. */
export function counterText(c: Counter | undefined, fmt: (n: number) => string): string {
  if (!c) return '';
  const n = Number(c.packets);
  return Number.isSafeInteger(n) ? fmt(n) : c.packets;
}
