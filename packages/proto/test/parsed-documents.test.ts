// Drift guard, document side (P03b). The Go guard (apps/agent/internal/contracttest drift_test.go)
// compares the JSON Schema of RootConfig with the DesiredState descriptor; this test runs what the API
// actually sends through the TS stubs: every valid example and fixture is `RootConfig.parse()`d (Zod
// fills every default and prefaults all 13 domains), secret-flagged leaves are stripped with
// `redactSecrets` (D-040, exactly as P06 does before `fromJSON`), and `toJSON(fromJSON(parsed))` must
// reproduce the parsed document key-for-key and value-for-value. Because ts-proto `fromJSON` drops
// unknown keys (see desired-state.test.ts), any schema leaf without a proto field — including leaves
// that only appear once defaults are filled — shows up as a missing key.
//
// Two intended differences: (1) 64-bit leaves are JSON strings in protobuf JSON (D-039, forceLong=string)
// while Zod keeps numbers, so for the known uint64 fields a number equals its decimal string; (2) proto3
// `repeated` and `map` fields have no presence, so an empty `[]` / `{}` that Zod filled as a default is
// not emitted by `toJSON` (docs/contracts/proto.md §1: absent and empty are the same thing). An empty
// *message* that is dropped would be masked by (2) here — the Go guard catches such a missing field
// structurally, which is why both guards exist.
import { readdirSync, readFileSync } from 'node:fs';
import { RootConfig, redactSecrets } from '@ngfw/schema';
import { describe, expect, it } from 'vitest';
import { DesiredState } from '../gen/ts/vrx/v1/dataplane.js';

/** JSON names of the uint64 leaves under DesiredState (the Go drift guard pins their proto type). */
const UINT64_KEYS = new Set(['espBytes', 'espPackets', 'cb', 'eb', 'burstBytes']);

const corpora = {
  examples: new URL('../../schema/examples/', import.meta.url),
  fixtures: new URL('./fixtures/', import.meta.url),
} as const;

function documents(): { name: string; doc: unknown }[] {
  const out: { name: string; doc: unknown }[] = [
    { name: '{} (every default, all domains prefaulted)', doc: {} },
  ];
  for (const [label, dir] of Object.entries(corpora)) {
    for (const f of readdirSync(dir).sort()) {
      if (f.endsWith('.json') && !f.startsWith('invalid-')) {
        const doc: unknown = JSON.parse(readFileSync(new URL(f, dir), 'utf8'));
        out.push({ name: `${label}/${f}`, doc: label === 'fixtures' ? uint64AsNumbers(doc) : doc });
      }
    }
  }
  return out;
}

/**
 * The proto fixtures carry 64-bit leaves in their protobuf-JSON form (decimal strings); the configuration
 * document (Zod) has numbers there. Converts them back so the fixture is a document again.
 */
function uint64AsNumbers(v: unknown, key = ''): unknown {
  if (typeof v === 'string' && UINT64_KEYS.has(key) && /^\d+$/.test(v)) {
    const n = Number(v);
    if (!Number.isSafeInteger(n)) throw new Error(`fixture ${key}=${v} exceeds 2^53`);
    return n;
  }
  if (Array.isArray(v)) return v.map((x) => uint64AsNumbers(x, key));
  if (v !== null && typeof v === 'object') {
    return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, uint64AsNumbers(x, k)]));
  }
  return v;
}

/** `[]` or `{}` — what proto3 repeated/map fields cannot distinguish from absent. */
function isEmptyContainer(v: unknown): boolean {
  return (
    (Array.isArray(v) && v.length === 0) ||
    (v !== null && typeof v === 'object' && Object.keys(v).length === 0)
  );
}

/** Differences between the parsed document and its proto round trip, as JSON-pointer-ish paths. */
function diff(want: unknown, got: unknown, path = '', key = ''): string[] {
  if (typeof want === 'number' && typeof got === 'string' && UINT64_KEYS.has(key)) {
    return String(want) === got ? [] : [`${path}: ${want} ≠ "${got}"`];
  }
  if (Array.isArray(want) && Array.isArray(got)) {
    if (want.length !== got.length) return [`${path}: array length ${want.length} ≠ ${got.length}`];
    return want.flatMap((w, i) => diff(w, got[i], `${path}/${i}`, key));
  }
  if (
    want !== null &&
    got !== null &&
    typeof want === 'object' &&
    typeof got === 'object' &&
    !Array.isArray(want)
  ) {
    const w = want as Record<string, unknown>;
    const g = got as Record<string, unknown>;
    const out: string[] = [];
    for (const k of Object.keys(w)) {
      if (!(k in g)) {
        if (!isEmptyContainer(w[k])) out.push(`${path}/${k}: dropped by the proto (no field?)`);
      } else out.push(...diff(w[k], g[k], `${path}/${k}`, k));
    }
    for (const k of Object.keys(g)) if (!(k in w)) out.push(`${path}/${k}: invented by the proto`);
    return out;
  }
  return Object.is(want, got) ? [] : [`${path}: ${JSON.stringify(want)} ≠ ${JSON.stringify(got)}`];
}

describe('parsed documents survive the DesiredState projection', () => {
  const docs = documents();

  it('corpus: {} plus every valid example and fixture', () => {
    expect(docs.length).toBeGreaterThan(20);
  });

  for (const { name, doc } of docs) {
    it(name, () => {
      const parsed = redactSecrets(RootConfig.parse(doc));
      const back = DesiredState.toJSON(DesiredState.fromJSON(parsed));
      expect(diff(parsed, back)).toEqual([]);
    });
  }

  it('the comparison is not vacuous: a leaf without a proto field is reported', () => {
    const parsed = redactSecrets(RootConfig.parse({})) as Record<string, Record<string, unknown>>;
    const broken = { ...parsed, system: { ...parsed['system'], bogusLeaf: 'x' } };
    expect(diff(broken, DesiredState.toJSON(DesiredState.fromJSON(broken)))).toEqual([
      '/system/bogusLeaf: dropped by the proto (no field?)',
    ]);
  });

  it('secret-flagged leaves are stripped before fromJSON (D-040)', () => {
    const doc = {
      management: { users: [{ username: 'a', role: 'admin', scope: '*', passwordHash: '$6$x' }] },
    };
    const parsed = redactSecrets(RootConfig.parse(doc)) as {
      management: { users: Record<string, unknown>[] };
    };
    expect('passwordHash' in (parsed.management.users[0] ?? {})).toBe(false);
    expect(diff(parsed, DesiredState.toJSON(DesiredState.fromJSON(parsed)))).toEqual([]);
  });
});
