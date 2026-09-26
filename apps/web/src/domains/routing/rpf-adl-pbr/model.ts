import type { paths } from '@ngfw/api-client';
import {
  AdlSchema,
  AutoSdlSchema,
  PbrAttachmentSchema,
  PbrPolicySchema,
  UrpfSchema,
  type AdlConfig,
  type AutoSdlConfig,
  type PbrAttachmentConfig,
  type PbrPolicyConfig,
  type UrpfConfig,
} from '@ngfw/schema';
import type { JsonSchema, ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { z } from 'zod';
import { ApiError } from '../../../api-problem';

/**
 * F-rpf-adl-pbr screens: the forms are the contract's own sub-schemas (packages/schema ext/rpf-adl-pbr.ts) turned into
 * JSON Schema in the browser, exactly like the domain schemas (schema/registry.ts) — never hand-written (00-CONTEXT
 * rule 5). References that must exist (ACL, VRF, policy, interface) become the choices of a select: the enum is taken
 * from the candidate, so the picker offers what the semantic rules accept.
 */

export const NS = 'rpf-adl-pbr';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;
/** `GET /api/v1/state/pbr` as generated from the OpenAPI document. */
export type PbrState = Ok<NonNullable<paths['/api/v1/state/pbr']['get']>>;
export type PbrPolicyRow = PbrState['policies'][number];
export type PbrAttachmentRow = PbrState['attachments']['items'][number];
export type PbrStatus = PbrPolicyRow['status'];

export type { AdlConfig, AutoSdlConfig, PbrAttachmentConfig, PbrPolicyConfig, UrpfConfig };

/** `routing.pbr` as the candidate holds it. */
export interface PbrConfig {
  policies?: Record<string, PbrPolicyConfig>;
  attachments?: PbrAttachmentConfig[];
}

const OPTIONS = { target: 'draft-2020-12', io: 'input' } as const;
const toJson = (s: z.ZodType): JsonSchema => z.toJSONSchema(s, OPTIONS) as JsonSchema;

export const schemas = {
  policy: () => toJson(PbrPolicySchema),
  attachment: () => toJson(PbrAttachmentSchema),
  urpf: () => toJson(UrpfSchema),
  adl: () => toJson(AdlSchema),
  autoSdl: () => toJson(AutoSdlSchema),
  /** The ADL page's per-interface dialog: both groups at once, defaults = off. */
  security: () => toJson(z.object({ urpf: UrpfSchema.prefault({}), adl: AdlSchema.prefault({}) })),
};

/** The dialog's value → the merge patch of `interfaces.<name>`: an object that checks nothing is removed (absent = off). */
export function securityPatch(value: { urpf?: UrpfConfig; adl?: AdlConfig }): {
  urpf: UrpfConfig | null;
  adl: AdlConfig | null;
} {
  const u = value.urpf;
  const a = value.adl;
  return {
    urpf: u && (u.ipv4 !== undefined || u.ipv6 !== undefined) ? u : null,
    adl: a && (a.ipv4 || a.ipv6) ? a : null,
  };
}

type Translate = (key: string, opts?: Record<string, unknown>) => string;

/** Deep copy of a JSON schema with `enum` choices set on the properties at the given paths (`paths.items.vrf`). */
export function withChoices(
  schema: JsonSchema,
  choices: Record<string, readonly string[]>,
): JsonSchema {
  const copy = structuredClone(schema) as Record<string, unknown>;
  for (const [path, values] of Object.entries(choices)) {
    let node: Record<string, unknown> | undefined = copy;
    for (const seg of path.split('.')) {
      const props = node?.['properties'] as Record<string, Record<string, unknown>> | undefined;
      node =
        seg === 'items' ? (node?.['items'] as Record<string, unknown> | undefined) : props?.[seg];
    }
    if (node && values.length > 0) {
      node['enum'] = [...values];
      delete node['pattern'];
      delete node['minLength'];
      delete node['maxLength'];
    }
  }
  return copy as JsonSchema;
}

/**
 * Titles, help and enum labels in the UI language: `<prefix>.<prop>.title|help|enum.<value>` in this namespace, the
 * schema's English text as fallback; nested objects and array items recurse (`paths.items.vrf` → `<prefix>.paths.vrf`).
 */
export function localize(schema: JsonSchema, t: Translate, prefix: string): JsonSchema {
  const s = schema as Record<string, unknown>;
  const out: Record<string, unknown> = { ...s };
  const props = s['properties'] as Record<string, Record<string, unknown>> | undefined;
  if (props) {
    const next: Record<string, unknown> = {};
    for (const [name, prop] of Object.entries(props)) {
      const key = `${prefix}.${name}`;
      const hints = (prop['x-vrx-ui'] ?? {}) as Record<string, unknown>;
      const help = t(`${key}.help`, {
        defaultValue: typeof hints['help'] === 'string' ? hints['help'] : '',
      });
      const inner = prop['anyOf'] ? prop : localize(prop as JsonSchema, t, key);
      const values = Array.isArray(prop['enum']) ? (prop['enum'] as unknown[]) : undefined;
      const enumLabels = values
        ? Object.fromEntries(
            values.map((v) => [
              String(v),
              t(`${key}.enum.${String(v)}`, { defaultValue: String(v) }),
            ]),
          )
        : undefined;
      next[name] = {
        ...(inner as Record<string, unknown>),
        title: t(`${key}.title`, {
          defaultValue: typeof prop['title'] === 'string' ? prop['title'] : name,
        }),
        'x-vrx-ui': { ...hints, ...(help ? { help } : {}), ...(enumLabels ? { enumLabels } : {}) },
      };
    }
    out['properties'] = next;
  }
  const items = s['items'] as JsonSchema | undefined;
  if (items && typeof items === 'object') out['items'] = localize(items, t, prefix);
  return out as JsonSchema;
}

/** Server pointers under `base` → pointers relative to the edited form (`/routing/pbr/policies/x/acl` → `/acl`). */
export function problemUnder(error: unknown, base: string): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  return {
    ...p,
    errors: (p.errors ?? []).map((e) => ({
      ...e,
      pointer: e.pointer.startsWith(base) ? e.pointer.slice(base.length) : e.pointer,
    })),
  };
}

/** Semantic colour of a status: in sync = up, missing = down, drift / unmanaged = degraded. */
export function statusColour(s: PbrStatus): 'up' | 'down' | 'degraded' {
  if (s === 'in-sync') return 'up';
  return s === 'missing' ? 'down' : 'degraded';
}

/** One path in one line: `10.0.0.1 via Gi0/8/0`, `lookup in wan2`, `via Gi0/8/0 ×2`. */
export function pathText(
  p: {
    address?: string | undefined;
    interface?: string | undefined;
    vrf?: string | undefined;
    weight?: number | undefined;
  },
  t: Translate,
): string {
  const parts: string[] = [];
  if (p.address) parts.push(p.address);
  if (p.interface) parts.push(t('path.via', { interface: p.interface }));
  if (!p.address && !p.interface) parts.push(t('path.lookup', { vrf: p.vrf ?? 'default' }));
  else if (p.vrf && p.vrf !== 'default' && !p.interface)
    parts.push(t('path.inVrf', { vrf: p.vrf }));
  if (p.weight !== undefined && p.weight !== 1) parts.push(`×${p.weight}`);
  return parts.join(' ');
}

/** The candidate's policies merged with the live view: every name once, candidate values first. */
export interface PolicyView {
  name: string;
  config: PbrPolicyConfig | undefined;
  live: PbrPolicyRow | undefined;
  /** the candidate differs from the live (running-derived) row, or only one side has it */
  pending: 'new' | 'changed' | 'removed' | undefined;
}

export function policyViews(
  candidate: PbrConfig | undefined,
  state: PbrState | undefined,
  running: PbrConfig | undefined,
): PolicyView[] {
  const cand = candidate?.policies ?? {};
  const run = running?.policies ?? {};
  const live = new Map((state?.policies ?? []).map((p) => [p.name, p]));
  const names = [...new Set([...Object.keys(cand), ...Object.keys(run), ...live.keys()])].sort();
  return names.map((name) => {
    const c = cand[name];
    const r = run[name];
    let pending: PolicyView['pending'];
    if (c && !r) pending = 'new';
    else if (!c && r) pending = 'removed';
    else if (c && r && JSON.stringify(c) !== JSON.stringify(r)) pending = 'changed';
    return { name, config: c, live: live.get(name), pending };
  });
}

/** Attachment key (policy, interface, family). */
export const attachmentKey = (a: {
  policy: string;
  interface: string;
  family?: string | undefined;
}) => `${a.policy}\u0000${a.interface}\u0000${a.family ?? 'ipv4'}`;

/** Interfaces with uRPF or ADL in the candidate (the ADL page's table). */
export interface SecurityRow {
  name: string;
  urpf: UrpfConfig | undefined;
  adl: AdlConfig | undefined;
}

export function securityRows(
  interfaces: Record<string, { urpf?: UrpfConfig; adl?: AdlConfig }> | undefined,
): SecurityRow[] {
  return Object.entries(interfaces ?? {})
    .map(([name, itf]) => ({ name, urpf: itf.urpf, adl: itf.adl }))
    .sort((a, b) => a.name.localeCompare(b.name));
}

/** "ipv4 strict · ipv6 loose (rx)" / "off". */
export function urpfText(u: UrpfConfig | undefined, t: Translate): string {
  if (!u || (!u.ipv4 && !u.ipv6)) return t('off');
  const fam = [
    u.ipv4 ? `IPv4 ${t(`mode.${u.ipv4}`)}` : '',
    u.ipv6 ? `IPv6 ${t(`mode.${u.ipv6}`)}` : '',
  ].filter(Boolean);
  return `${fam.join(' · ')} (${t(`direction.${u.direction ?? 'rx'}`)})`;
}

/** "IPv4 · IPv6 → allow" / "off". */
export function adlText(a: AdlConfig | undefined, t: Translate): string {
  if (!a || (!a.ipv4 && !a.ipv6)) return t('off');
  const fam = [a.ipv4 ? 'IPv4' : '', a.ipv6 ? 'IPv6' : ''].filter(Boolean);
  return t('adl.summary', { families: fam.join(' · '), vrf: a.allowVrf ?? '' });
}
