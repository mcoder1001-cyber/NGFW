import type { ProblemIssue } from '../../common/problem.js';

/**
 * F-rule-expiry: the rules of a configuration document that carry `expiresAt` — ACL rules (`acl.lists.<n>.rules[]`),
 * host rules (`acl.host.<n>.rules[]`) and NAT44-ED static mappings (`nat.staticMappings[]`) — identified across
 * documents by list name + sequence (rules) or mapping name, so a commit can tell a new/changed expiry from an
 * unchanged one.
 */
export interface ExpiringRule {
  kind: 'acl' | 'host' | 'nat';
  /** Stable identity across documents: `acl/<list>/<sequence>`, `host/<list>/<sequence>`, `nat/<name>`. */
  id: string;
  /** JSON pointer of the rule in THIS document. */
  pointer: string;
  expiresAt: string;
  owner?: string;
  ticket?: string;
}

type Json = Record<string, unknown>;
const obj = (v: unknown): Json =>
  v !== null && typeof v === 'object' && !Array.isArray(v) ? (v as Json) : {};
const arr = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);
const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');

export function expiringRules(doc: unknown): ExpiringRule[] {
  const out: ExpiringRule[] = [];
  const acl = obj(obj(doc)['acl']);
  const meta = (r: Json) => ({
    expiresAt: r['expiresAt'] as string,
    ...(typeof r['owner'] === 'string' ? { owner: r['owner'] } : {}),
    ...(typeof r['ticket'] === 'string' ? { ticket: r['ticket'] } : {}),
  });
  for (const [kind, leaf] of [
    ['acl', 'lists'],
    ['host', 'host'],
  ] as const) {
    for (const [name, list] of Object.entries(obj(acl[leaf]))) {
      arr(obj(list)['rules']).forEach((r, i) => {
        const rule = obj(r);
        if (typeof rule['expiresAt'] !== 'string') return;
        out.push({
          kind,
          id: `${kind}/${name}/${String(rule['sequence'])}`,
          pointer: `/acl/${leaf}/${esc(name)}/rules/${i}`,
          ...meta(rule),
        });
      });
    }
  }
  arr(obj(obj(doc)['nat'])['staticMappings']).forEach((m, i) => {
    const mapping = obj(m);
    if (typeof mapping['expiresAt'] !== 'string') return;
    out.push({
      kind: 'nat',
      id: `nat/${String(mapping['name'])}`,
      pointer: `/nat/staticMappings/${i}`,
      ...meta(mapping),
    });
  });
  return out;
}

/**
 * Commit rule `rule.expires-in-past`: an expiry at or before `now` is refused on a NEW rule or a CHANGED expiry; the
 * same rule with the same expiresAt as in running stays valid (an expired rule is kept until it is extended or
 * deleted). Unparsable dates are the schema's.
 */
export function expiryInPastIssues(doc: unknown, running: unknown, now: Date): ProblemIssue[] {
  const before = new Map(expiringRules(running).map((r) => [r.id, r.expiresAt]));
  const issues: ProblemIssue[] = [];
  for (const r of expiringRules(doc)) {
    const t = Date.parse(r.expiresAt);
    if (!Number.isFinite(t) || t > now.getTime() || before.get(r.id) === r.expiresAt) continue;
    issues.push({
      pointer: `${r.pointer}/expiresAt`,
      message: `expiresAt ${r.expiresAt} is already in the past; set a future time (an expired rule is kept only while its expiry is unchanged)`,
      rule: 'rule.expires-in-past',
    });
  }
  return issues;
}

/** What a warning scan finds: rules expiring within `warnDays` (not yet expired) and expired ones. */
export function expiryStates(
  doc: unknown,
  now: Date,
  warnDays: number,
): { expiring: ExpiringRule[]; expired: ExpiringRule[] } {
  const expiring: ExpiringRule[] = [];
  const expired: ExpiringRule[] = [];
  const horizon = now.getTime() + warnDays * 86_400_000;
  for (const r of expiringRules(doc)) {
    const t = Date.parse(r.expiresAt);
    if (!Number.isFinite(t)) continue;
    if (t <= now.getTime()) expired.push(r);
    else if (t <= horizon) expiring.push(r);
  }
  return { expiring, expired };
}
