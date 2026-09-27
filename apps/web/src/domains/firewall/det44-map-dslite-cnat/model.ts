import type { paths } from '@ngfw/api-client';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { natSchema } from '../nat44-ed-sessions/model';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;

/** i18n namespace of F-det44-map-dslite-cnat (locale namespace = task slug). */
export const NS = 'det44-map-dslite-cnat';

export type Det44SessionsPage = Ok<NonNullable<paths['/api/v1/state/nat/det44/sessions']['get']>>;
export type Det44Lookup = Ok<NonNullable<paths['/api/v1/actions/nat/det44/lookup']['post']>>;
export type CnatSessionsPage = Ok<NonNullable<paths['/api/v1/state/nat/cnat/sessions']['get']>>;

/** The `nat` subtrees this task edits, one schema-driven form each. */
export type Subtree = 'det44' | 'dslite' | 'map' | 'cnat' | 'pnat';

/** The JSON Schema of `nat.<subtree>` with the root's `$defs` (the one schema, rule 5). */
export function subtreeSchema(key: Subtree): JsonSchema {
  const root = natSchema();
  const props = (root.properties ?? {}) as Record<string, JsonSchema>;
  const s = props[key];
  if (!s || typeof s !== 'object') throw new Error(`nat.${key} schema not found`);
  return { ...s, ...(root.$defs ? { $defs: root.$defs } : {}) } as JsonSchema;
}

/** VPP's DET44 port range: 1024–65535 is split between the inside hosts sharing one outside address. */
export const DET44_PORTS = 64_512;
export const DET44_FIRST_PORT = 1024;

export interface PortBlockPlan {
  insideHosts: number;
  outsideAddresses: number;
  /** Inside hosts per outside address (VPP sharing_ratio). */
  ratio: number;
  /** Ports of each inside host (VPP ports_per_host). */
  portsPerHost: number;
  /** The block of the inside host at `index` (0-based inside the prefix): outside address offset and port range. */
  blockOf: (index: number) => { outsideOffset: number; lo: number; hi: number };
}

const prefixLen = (cidr: string): number | undefined => {
  const m = /^(\d{1,3}(?:\.\d{1,3}){3})\/(\d{1,2})$/.exec(cidr.trim());
  if (m === null) return undefined;
  const len = Number(m[2]);
  return m[1]!.split('.').every((o) => Number(o) <= 255) && len <= 32 ? len : undefined;
};

export type PlanError = 'invalid' | 'outside-larger' | 'ratio-too-large';

/**
 * The DET44 port-block calculator (VPP det44_add_del_map / snat_det_forward): ratio = 2^(outLen − inLen), ports per
 * host = ⌊64512 / ratio⌋, host i gets outside address offset ⌊i / ratio⌋ and ports 1024 + ppH·(i mod ratio) …
 * + ppH − 1. The schema allows at most 15 bits of difference.
 */
export function det44Plan(inside: string, outside: string): PortBlockPlan | PlanError {
  const il = prefixLen(inside);
  const ol = prefixLen(outside);
  if (il === undefined || ol === undefined) return 'invalid';
  if (ol < il) return 'outside-larger';
  if (ol - il > 15) return 'ratio-too-large';
  const ratio = 2 ** (ol - il);
  const portsPerHost = Math.floor(DET44_PORTS / ratio);
  return {
    insideHosts: 2 ** (32 - il),
    outsideAddresses: 2 ** (32 - ol),
    ratio,
    portsPerHost,
    blockOf: (i) => {
      const lo = DET44_FIRST_PORT + portsPerHost * (i % ratio);
      return { outsideOffset: Math.floor(i / ratio), lo, hi: lo + portsPerHost - 1 };
    },
  };
}

/** Slot props of an address/prefix input: always left-to-right (also in Persian). */
export const LTR_INPUT = { htmlInput: { dir: 'ltr' } } as const;

/** The subtree ids as values (JSX attributes take them from here). */
export const SUBTREES = {
  det44: 'det44',
  dslite: 'dslite',
  map: 'map',
  cnat: 'cnat',
  pnat: 'pnat',
} as const satisfies Record<Subtree, Subtree>;
