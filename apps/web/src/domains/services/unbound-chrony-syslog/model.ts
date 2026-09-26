import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { ApiError } from '../../../api-problem';
import type { ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { domainSchemas } from '../../../schema/registry';

/** i18n namespace of the feature (locale namespace = task slug, wave-A-hotspots §0 rule 5). */
export const NS = 'unbound-chrony-syslog';

type Obj = JsonSchema & {
  properties?: Record<string, JsonSchema>;
  $defs?: Record<string, JsonSchema>;
};

/** A sub-schema of a domain schema, carrying the domain's `$defs` so references still resolve. */
function sub(domain: 'services' | 'management', path: readonly string[]): JsonSchema {
  const root = domainSchemas[domain] as Obj;
  let cur: unknown = root;
  for (const seg of path) {
    cur = (cur as Record<string, unknown> | undefined)?.[seg];
    if (cur === undefined) throw new Error(`${domain} schema: ${path.join('.')} not found`);
  }
  const out = { ...(cur as Obj) };
  if (root.$defs && !out.$defs) out.$defs = root.$defs;
  return out;
}

/** `services.dns.resolvers.<name>` — one Unbound resolver (the one schema, 00-CONTEXT rule 5). */
export const resolverSchema = (): JsonSchema =>
  sub('services', ['properties', 'dns', 'properties', 'resolvers', 'additionalProperties']);

/** `services.dns.vppCache` — VPP's caching DNS plugin (a write-only VPP global). */
export const vppCacheSchema = (): JsonSchema =>
  sub('services', ['properties', 'dns', 'properties', 'vppCache']);

/** `services.ntp` — chrony client/server. */
export const ntpSchema = (): JsonSchema => sub('services', ['properties', 'ntp']);

/** `management.syslog[i]` — one remote-syslog target, with the D-086 keys (facilities, format, queueSize, tls). */
export const syslogTargetSchema = (): JsonSchema =>
  sub('management', ['properties', 'syslog', 'items']);

/** Server pointers under `base` → pointers relative to the edited object, for `<SchemaForm problem>`. */
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

/** Seconds as a short human value (chrony offsets): 0.000012 → "12 µs". */
export function seconds(v: number): string {
  const a = Math.abs(v);
  if (a >= 1) return `${v.toFixed(3)} s`;
  if (a >= 1e-3) return `${(v * 1e3).toFixed(3)} ms`;
  return `${(v * 1e6).toFixed(1)} µs`;
}

/** The chrony source state glyphs as keys of `ntp.sourceState.*`. */
export const SOURCE_STATES: Record<string, string> = {
  '*': 'selected',
  '+': 'combined',
  '-': 'notCombined',
  '?': 'unusable',
  x: 'falseticker',
  '~': 'variable',
};

export const SEVERITIES = [
  'emergency',
  'alert',
  'critical',
  'error',
  'warning',
  'notice',
  'info',
  'debug',
] as const;
export const FACILITIES = [
  'kern',
  'user',
  'mail',
  'daemon',
  'auth',
  'syslog',
  'lpr',
  'news',
  'uucp',
  'cron',
  'authpriv',
  'ftp',
  'local0',
  'local1',
  'local2',
  'local3',
  'local4',
  'local5',
  'local6',
  'local7',
] as const;

/** The log explorer's look-back choices (hours). */
export const WINDOWS_H = [1, 6, 24, 168] as const;

/** MUI chip colour of a syslog severity. */
export function severityColor(sev: string): 'error' | 'warning' | 'info' | 'default' {
  const i = SEVERITIES.indexOf(sev as (typeof SEVERITIES)[number]);
  if (i < 0) return 'default';
  if (i <= 3) return 'error';
  if (i === 4) return 'warning';
  return i === 5 ? 'info' : 'default';
}

/** i18n key of a form field label: `field.<prefix>.<prop_path>` (nested paths joined with "_", so a parent's label and its
 * children's never collide as i18next objects). */
export function fieldKey(prefix: string, propPath: string): string {
  return `field.${prefix}.${propPath.replaceAll('.', '_')}`;
}
