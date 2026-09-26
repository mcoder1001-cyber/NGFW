import { deepEqual, isPlainObject } from '@ngfw/schema';
import { z } from 'zod';

/**
 * `GET /api/v1/state/pbr` (F-rpf-adl-pbr): the policy-based routing view — every PBR policy and attachment of the
 * running configuration next to what the agent retrieves from VPP (Retrieve `routing`, ABF policies named by the
 * agent's pbr.policy records), with a per-row status. Pure: the controller passes running, candidate and actual.
 *
 * Status: `in-sync` (running = actual) · `drift` (both, different) · `missing` (running only: not applied — e.g. its
 * ACL does not exist in VPP yet) · `unmanaged` (actual only: a leftover the next commit deletes; unnamed policies
 * are reported as `#<id>`).
 */

type Json = Record<string, unknown>;

export const PbrPathOut = z.object({
  address: z.string().optional(),
  interface: z.string().optional(),
  vrf: z.string(),
  weight: z.number().int(),
});

export const PbrStatus = z.enum(['in-sync', 'drift', 'missing', 'unmanaged']);
export type PbrStatus = z.infer<typeof PbrStatus>;

export const PbrPolicyOut = z.object({
  name: z.string(),
  acl: z.string(),
  priority: z.number().int(),
  paths: z.array(PbrPathOut),
  status: PbrStatus,
  attachments: z.number().int().describe('attachments of this policy in the running configuration'),
});

export const PbrAttachmentOut = z.object({
  policy: z.string(),
  interface: z.string(),
  family: z.enum(['ipv4', 'ipv6']),
  status: PbrStatus,
});

export const PbrStateOut = z.object({
  retrievedAt: z.string().optional(),
  pendingChange: z.boolean().describe('the candidate differs from running in routing.pbr'),
  policies: z.array(PbrPolicyOut),
  attachments: z.object({
    page: z.number().int(),
    pageSize: z.number().int(),
    total: z.number().int(),
    items: z.array(PbrAttachmentOut),
  }),
  counters: z
    .object({ available: z.literal(false), reason: z.string() })
    .describe('per-policy ACL hit counters: not available in this build'),
});
export type PbrStateOut = z.infer<typeof PbrStateOut>;

export const PbrStateQuery = z.object({
  page: z.coerce.number().int().min(1).default(1),
  pageSize: z.coerce.number().int().min(1).max(1000).default(100),
});

const COUNTERS_REASON =
  'ACL hit counters need a counters RPC in the agent contract and acl-plugin statistics switched on by the globals owner (F-rpf-adl-pbr-questions)';

function pbrOf(doc: Json | undefined): Json {
  const routing = doc?.['routing'];
  const pbr = isPlainObject(routing) ? routing['pbr'] : undefined;
  return isPlainObject(pbr) ? pbr : {};
}

function policiesOf(pbr: Json): Record<string, Json> {
  const p = pbr['policies'];
  return isPlainObject(p) ? (p as Record<string, Json>) : {};
}

interface Attachment {
  policy: string;
  interface: string;
  family: 'ipv4' | 'ipv6';
}

function attachmentsOf(pbr: Json): Attachment[] {
  const a = pbr['attachments'];
  if (!Array.isArray(a)) return [];
  return a.filter(isPlainObject).map((x) => ({
    policy: String(x['policy'] ?? ''),
    interface: String(x['interface'] ?? ''),
    family: x['family'] === 'ipv6' ? 'ipv6' : 'ipv4',
  }));
}

/** A policy in its canonical form (the defaults Retrieve reports), so running and actual compare. */
function canonicalPolicy(p: Json): Json {
  const paths = Array.isArray(p['paths']) ? p['paths'].filter(isPlainObject) : [];
  return {
    acl: String(p['acl'] ?? ''),
    priority: typeof p['priority'] === 'number' ? p['priority'] : 100,
    paths: paths
      .map((x) => ({
        ...(x['address'] !== undefined ? { address: String(x['address']) } : {}),
        ...(x['interface'] !== undefined ? { interface: String(x['interface']) } : {}),
        vrf: typeof x['vrf'] === 'string' ? x['vrf'] : 'default',
        weight: typeof x['weight'] === 'number' ? x['weight'] : 1,
      }))
      .sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b))),
  };
}

const attachmentKey = (a: Attachment) => `${a.policy}\u0000${a.interface}\u0000${a.family}`;

export function pbrState(
  running: Json,
  candidate: Json,
  actual: Json,
  retrievedAt: Date | undefined,
  page: { page: number; pageSize: number },
): PbrStateOut {
  const want = pbrOf(running);
  const have = pbrOf(actual);
  const wantPolicies = policiesOf(want);
  const havePolicies = policiesOf(have);
  const wantAttach = attachmentsOf(want);
  const haveAttach = attachmentsOf(have);

  const policies: z.infer<typeof PbrPolicyOut>[] = [];
  const names = [...new Set([...Object.keys(wantPolicies), ...Object.keys(havePolicies)])].sort();
  for (const name of names) {
    const w = wantPolicies[name];
    const h = havePolicies[name];
    const shown = canonicalPolicy((w ?? h) as Json);
    let status: PbrStatus;
    if (w && h) status = deepEqual(canonicalPolicy(w), canonicalPolicy(h)) ? 'in-sync' : 'drift';
    else status = w ? 'missing' : 'unmanaged';
    policies.push({
      name,
      acl: shown['acl'] as string,
      priority: shown['priority'] as number,
      paths: shown['paths'] as z.infer<typeof PbrPathOut>[],
      status,
      attachments: wantAttach.filter((a) => a.policy === name).length,
    });
  }

  const haveKeys = new Set(haveAttach.map(attachmentKey));
  const wantKeys = new Set(wantAttach.map(attachmentKey));
  const rows = [
    ...wantAttach.map((a) => ({
      ...a,
      status: (haveKeys.has(attachmentKey(a)) ? 'in-sync' : 'missing') as PbrStatus,
    })),
    ...haveAttach
      .filter((a) => !wantKeys.has(attachmentKey(a)))
      .map((a) => ({ ...a, status: 'unmanaged' as PbrStatus })),
  ];
  const start = (page.page - 1) * page.pageSize;
  return {
    ...(retrievedAt ? { retrievedAt: retrievedAt.toISOString() } : {}),
    pendingChange: !deepEqual(pbrOf(running), pbrOf(candidate)),
    policies,
    attachments: {
      page: page.page,
      pageSize: page.pageSize,
      total: rows.length,
      items: rows.slice(start, start + page.pageSize),
    },
    counters: { available: false, reason: COUNTERS_REASON },
  };
}
