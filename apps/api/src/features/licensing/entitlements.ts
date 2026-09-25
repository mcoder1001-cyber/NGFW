import type { ProblemIssue } from '../../common/problem.js';

/**
 * Entitlements are predicates over the configuration document (JSON-pointer patterns, `*` = every key / index).
 *
 * SAMPLE TABLE — the split between "community" and paid features is a product-owner decision (open question in
 * docs/status/tasks/F-licensing-questions.md). Everything not listed here is never gated.
 */
export interface FeatureDef {
  feature: string;
  /** The feature is "used" when any of these patterns matches a node in the document. */
  pointers: readonly string[];
  description: string;
}

export interface LimitDef {
  limit: string;
  /** Counted nodes. */
  pointer: string;
  description: string;
}

export const FEATURES: readonly FeatureDef[] = [
  {
    feature: 'ipsec',
    pointers: ['/vpn/ipsec/tunnels/*'],
    description: 'IPsec site-to-site tunnels',
  },
  { feature: 'wireguard', pointers: ['/vpn/wireguard/interfaces/*'], description: 'WireGuard' },
  { feature: 'bgp', pointers: ['/routing/bgp'], description: 'BGP' },
  { feature: 'ospf', pointers: ['/routing/ospf'], description: 'OSPF' },
  { feature: 'isis', pointers: ['/routing/isis'], description: 'IS-IS' },
  {
    feature: 'ha',
    pointers: ['/ha/vrrp/*', '/ha/cluster'],
    description: 'High availability (VRRP, cluster)',
  },
];

export const LIMITS: readonly LimitDef[] = [
  { limit: 'ipsecTunnels', pointer: '/vpn/ipsec/tunnels/*', description: 'IPsec tunnels' },
  {
    limit: 'wireguardInterfaces',
    pointer: '/vpn/wireguard/interfaces/*',
    description: 'WireGuard interfaces',
  },
];

export interface Entitlements {
  features: string[];
  /** Missing limit = unlimited. */
  limits: Record<string, number>;
}

/** No licence (or expired past grace / invalid): the community set. SAMPLE — product-owner decision. */
export const COMMUNITY: Entitlements = {
  features: ['wireguard', 'ospf'],
  limits: { wireguardInterfaces: 2 },
};

function escape(seg: string): string {
  return seg.replace(/~/g, '~0').replace(/\//g, '~1');
}

/** Concrete pointers of every node matching `pattern` (document order). */
export function matchPointers(doc: unknown, pattern: string): string[] {
  const segs = pattern.split('/').slice(1);
  const out: string[] = [];
  const walk = (node: unknown, i: number, path: string) => {
    if (node === undefined || node === null) return;
    if (i === segs.length) {
      out.push(path);
      return;
    }
    if (typeof node !== 'object') return;
    const seg = segs[i]!;
    const entries: [string, unknown][] = Array.isArray(node)
      ? node.map((v, idx) => [String(idx), v])
      : Object.entries(node as Record<string, unknown>);
    for (const [k, v] of entries)
      if (seg === '*' || seg === k) walk(v, i + 1, `${path}/${escape(k)}`);
  };
  walk(doc, 0, '');
  return out;
}

/**
 * Nodes of `candidate` that the entitlements do not cover. Grandfathering: a node that already exists in `running`
 * never offends, so an expired licence rejects only NEW unlicensed use and never forces removal of the running
 * configuration (the data plane is never degraded by licensing — D-040).
 */
export function entitlementIssues(
  candidate: unknown,
  running: unknown,
  ent: Entitlements,
): ProblemIssue[] {
  const issues: ProblemIssue[] = [];
  const have = new Set(ent.features);
  for (const f of FEATURES) {
    if (have.has(f.feature)) continue;
    for (const p of f.pointers) {
      const inRunning = new Set(matchPointers(running, p));
      const fresh = matchPointers(candidate, p).find((ptr) => !inRunning.has(ptr));
      if (fresh !== undefined) {
        issues.push({
          pointer: fresh,
          message: `${f.description} is not covered by the licence (feature "${f.feature}")`,
          rule: `license.feature.${f.feature}`,
        });
        break;
      }
    }
  }
  for (const l of LIMITS) {
    const max = ent.limits[l.limit];
    if (max === undefined) continue;
    const cand = matchPointers(candidate, l.pointer);
    const run = matchPointers(running, l.pointer);
    if (cand.length <= max || cand.length <= run.length) continue;
    const inRunning = new Set(run);
    const fresh = cand.find((ptr) => !inRunning.has(ptr)) ?? cand[cand.length - 1]!;
    issues.push({
      pointer: fresh,
      message: `${l.description}: ${cand.length} configured, the licence allows ${max} (limit "${l.limit}")`,
      rule: `license.limit.${l.limit}`,
    });
  }
  return issues;
}
