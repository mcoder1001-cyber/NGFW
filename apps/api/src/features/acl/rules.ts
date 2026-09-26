import { isPlainObject } from '@ngfw/schema';

/**
 * F-acl: pure functions over the configuration document's ACL lists — the rule editor's server-side paging, search,
 * pending marks and bulk operations. Rules are evaluated in `sequence` order (unique within a list, semantic rule
 * `acl.rule-sequences-unique`); `index` is the rule's position in the document's `rules` array, i.e. the pointer
 * `/acl/lists/<name>/rules/<index>` the generic config routes edit.
 */

export type Json = Record<string, unknown>;

export interface IndexedRule {
  index: number;
  sequence: number;
  rule: Json;
}

export function aclOf(doc: Json): Json {
  const acl = doc['acl'];
  return isPlainObject(acl) ? acl : {};
}

export function listsOf(doc: Json, kind: 'lists' | 'macip' = 'lists'): Record<string, Json> {
  const l = aclOf(doc)[kind];
  return isPlainObject(l) ? (l as Record<string, Json>) : {};
}

export function rulesOf(list: Json | undefined): Json[] {
  const r = list?.['rules'];
  return Array.isArray(r) ? (r.filter(isPlainObject) as Json[]) : [];
}

/** Rules with their document index, in sequence order (ties by index). */
export function sortedRules(list: Json | undefined): IndexedRule[] {
  const out = rulesOf(list).map((rule, index) => ({
    index,
    sequence: typeof rule['sequence'] === 'number' ? rule['sequence'] : 0,
    rule,
  }));
  out.sort((a, b) => a.sequence - b.sequence || a.index - b.index);
  return out;
}

function matchText(m: unknown): string {
  if (!isPlainObject(m)) return 'any';
  switch (m['kind']) {
    case 'prefix':
      return String(m['prefix'] ?? '');
    case 'object':
      return String(m['name'] ?? '');
    case 'inline':
      return serviceText(m['spec']);
    default:
      return 'any';
  }
}

export function serviceText(spec: unknown): string {
  if (!isPlainObject(spec)) return 'any';
  const p = String(spec['protocol'] ?? 'any');
  const parts = [p];
  const dst = spec['destinationPorts'];
  if (Array.isArray(dst) && dst.length > 0) parts.push(dst.join('|'));
  if (typeof spec['type'] === 'number') parts.push(String(spec['type']));
  if (typeof spec['number'] === 'number') parts.push(String(spec['number']));
  return parts.join(':');
}

/** The text a quick filter searches: sequence, action, addresses, service, schedule, description. */
export function ruleSearchText(rule: Json): string {
  return [
    String(rule['sequence'] ?? ''),
    String(rule['action'] ?? ''),
    String(rule['ipVersion'] ?? ''),
    matchText(rule['source']),
    matchText(rule['destination']),
    matchText(rule['service']),
    String(rule['schedule'] ?? ''),
    String(rule['description'] ?? ''),
    rule['enabled'] === false ? 'disabled' : 'enabled',
    rule['log'] === true ? 'log' : '',
  ]
    .join(' ')
    .toLowerCase();
}

/** Quick-filter terms: every term must occur (case-insensitive). */
export function matchesFilter(rule: Json, terms: readonly string[]): boolean {
  if (terms.length === 0) return true;
  const text = ruleSearchText(rule);
  return terms.every((t) => text.includes(t.toLowerCase()));
}

export function filterTerms(filter: string | undefined): string[] {
  return (filter ?? '')
    .split(/\s+/)
    .map((t) => t.trim())
    .filter((t) => t.length > 0)
    .slice(0, 16);
}

function stable(v: unknown): string {
  if (Array.isArray(v)) return `[${v.map(stable).join(',')}]`;
  if (isPlainObject(v)) {
    return `{${Object.keys(v)
      .sort()
      .map((k) => `${JSON.stringify(k)}:${stable(v[k])}`)
      .join(',')}}`;
  }
  return JSON.stringify(v) ?? 'null';
}

export function sameJson(a: unknown, b: unknown): boolean {
  // fast path: documents from the datastore (jsonb) share one key order; the canonical form only when they differ
  return a === b || JSON.stringify(a) === JSON.stringify(b) || stable(a) === stable(b);
}

export type Pending = 'added' | 'changed' | null;

/** Pending mark of a candidate rule against the running rule with the same sequence. */
export function pendingBySequence(running: Json | undefined): (rule: Json) => Pending {
  const bySeq = new Map<number, Json>(); // compared lazily: only the rules of the page are serialised
  for (const r of rulesOf(running)) {
    if (typeof r['sequence'] === 'number') bySeq.set(r['sequence'], r);
  }
  return (rule) => {
    const prev = typeof rule['sequence'] === 'number' ? bySeq.get(rule['sequence']) : undefined;
    if (prev === undefined) return 'added';
    return prev === rule || stable(prev) === stable(rule) ? null : 'changed';
  };
}

// ---- bulk operations ---------------------------------------------------------------------------------------------

export type BulkOp =
  | { op: 'enable' | 'disable' | 'delete'; sequences: number[] }
  | { op: 'move'; sequences: number[]; to: number }
  | { op: 'renumber'; start: number; step: number };

export class BulkError extends Error {}

export const MAX_SEQUENCE = 2_147_483_647;

/**
 * Applies a bulk operation to a list's rules (returns a new array in sequence order):
 * - enable / disable / delete the rules with these sequences;
 * - move: the rules with these sequences (keeping their relative order) get `to`, `to+1`, …; every other rule whose
 *   sequence is ≥ `to` moves up by the number of moved rules when that range is taken, so the moved block is evaluated
 *   right before them ("move to sequence");
 * - renumber: every rule gets `start`, `start+step`, … in its current order.
 * Unknown sequences are an error (the editor sends what it shows).
 */
export function applyBulk(rules: Json[], op: BulkOp): { rules: Json[]; changed: number } {
  const sorted = [...rules].sort((a, b) => (a['sequence'] as number) - (b['sequence'] as number));
  if (op.op === 'renumber') {
    const last = op.start + op.step * Math.max(sorted.length - 1, 0);
    if (last > MAX_SEQUENCE)
      throw new BulkError(`renumbering ends at ${last}, above ${MAX_SEQUENCE}`);
    let changed = 0;
    const out = sorted.map((r, i) => {
      const seq = op.start + op.step * i;
      if (r['sequence'] !== seq) changed++;
      return { ...r, sequence: seq };
    });
    return { rules: out, changed };
  }
  const want = new Set(op.sequences);
  const present = new Set(sorted.map((r) => r['sequence'] as number));
  const missing = op.sequences.filter((s) => !present.has(s));
  if (missing.length > 0) {
    throw new BulkError(
      `no rule with sequence ${missing.slice(0, 5).join(', ')}${missing.length > 5 ? ', …' : ''}`,
    );
  }
  switch (op.op) {
    case 'enable':
    case 'disable': {
      const enabled = op.op === 'enable';
      let changed = 0;
      const out = sorted.map((r) => {
        if (!want.has(r['sequence'] as number) || (r['enabled'] ?? true) === enabled) return r;
        changed++;
        return { ...r, enabled };
      });
      return { rules: out, changed };
    }
    case 'delete': {
      const out = sorted.filter((r) => !want.has(r['sequence'] as number));
      return { rules: out, changed: sorted.length - out.length };
    }
    case 'move': {
      const moving = sorted.filter((r) => want.has(r['sequence'] as number));
      const rest = sorted.filter((r) => !want.has(r['sequence'] as number));
      const k = moving.length;
      if (op.to + k - 1 > MAX_SEQUENCE)
        throw new BulkError(`the moved rules would end above ${MAX_SEQUENCE}`);
      const clash = rest.some((r) => {
        const s = r['sequence'] as number;
        return s >= op.to && s < op.to + k;
      });
      let changed = 0;
      const shifted = rest.map((r) => {
        const s = r['sequence'] as number;
        if (!clash || s < op.to) return r;
        changed++;
        return { ...r, sequence: s + k };
      });
      if (shifted.some((r) => (r['sequence'] as number) > MAX_SEQUENCE)) {
        throw new BulkError(
          `shifting the following rules would pass ${MAX_SEQUENCE}; renumber the list first`,
        );
      }
      const moved = moving.map((r, i) => {
        if (r['sequence'] !== op.to + i) changed++;
        return { ...r, sequence: op.to + i };
      });
      const out = [...shifted, ...moved].sort(
        (a, b) => (a['sequence'] as number) - (b['sequence'] as number),
      );
      return { rules: out, changed };
    }
  }
}
