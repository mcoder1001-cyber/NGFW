import { status, type handleUnaryCall, type ServiceError } from '@grpc/grpc-js';
import type {
  MplsStateFibEntry,
  MplsStatePath,
  MplsStateRequest,
  MplsStateResponse,
  MplsStateTable,
  MplsStateTunnel,
} from '@ngfw/proto';
import { ipFamily, isPlainObject } from '@ngfw/schema';
import type { FakeAgent } from '../../testing/fake-agent.js';

/**
 * F-mpls-srmpls behaviour of the in-process fake agent (wave-A-hotspots P5; wired by one line in
 * testing/fake-agent.ts): `MplsState` over the applied document, with the agent's rules — readable tables are 0 (present
 * as soon as the MPLS configuration needs it, as on a box whose globals owner created it) and this owner's tables
 * (`<owner>:<id>`); the FIB of a table holds VPP's reserved entries (labels 0, 1, 2), the label routes of that table and,
 * in table 0, the bound labels and the SR-MPLS binding SIDs; paging, the label filter and the request checks are the
 * agent's (limit ≤ 1000, offset + limit ≤ 100 000, unknown view → INVALID_ARGUMENT, unreadable table → NOT_FOUND).
 * Tunnels: one `mpls-tunnel<N>` interface per configured tunnel, owned.
 */
type Json = Record<string, unknown>;

const err = (code: status, details: string): Partial<ServiceError> => ({ code, details });

const MAX_LIMIT = 1000;
const MAX_WINDOW = 100_000;

interface Mpls {
  interfaces?: string[];
  tables?: Record<string, unknown>;
  labelRoutes?: Json[];
  ipBindings?: Json[];
  tunnels?: Record<string, Json>;
  sr?: { policies?: Record<string, Json>; steering?: Json[] };
}

function mplsOf(doc: Json): Mpls | undefined {
  const routing = doc['routing'];
  if (!isPlainObject(routing) || !isPlainObject(routing['mpls'])) return undefined;
  return routing['mpls'] as Mpls;
}

function needsTableZero(m: Mpls): boolean {
  return (
    (m.interfaces?.length ?? 0) > 0 ||
    (m.ipBindings?.length ?? 0) > 0 ||
    Object.keys(m.sr?.policies ?? {}).length > 0 ||
    (m.labelRoutes ?? []).some((r) => Number(r['table'] ?? 0) === 0)
  );
}

function vrfId(doc: Json, name: unknown): number {
  if (name === undefined || name === 'default') return 0;
  const vrfs = (doc['vrfs'] ?? {}) as Record<string, Json>;
  return Number(vrfs[String(name)]?.['id'] ?? 0);
}

function path(p: Partial<MplsStatePath>): MplsStatePath {
  return {
    type: 'normal',
    proto: 'ip4',
    nextHop: '',
    interface: '',
    tableId: 0,
    outLabels: [],
    weight: 1,
    preference: 0,
    ...p,
  };
}

/** A document path as the agent reports it (the proto follows the next hop, else the payload). */
function docPath(doc: Json, p: Json, payload: string): MplsStatePath {
  const nh = typeof p['nextHop'] === 'string' ? p['nextHop'] : '';
  const proto = nh ? (ipFamily(nh) === 6 ? 'ip6' : 'ip4') : payload === 'ip6' ? 'ip6' : 'ip4';
  return path({
    proto,
    nextHop: nh,
    interface: typeof p['interface'] === 'string' ? p['interface'] : '',
    tableId: p['vrf'] !== undefined ? vrfId(doc, p['vrf']) : 0,
    outLabels: ((p['outLabels'] as number[] | undefined) ?? []).map(Number),
    weight: typeof p['weight'] === 'number' ? p['weight'] : 1,
  });
}

function tables(agent: FakeAgent, m: Mpls | undefined): MplsStateTable[] {
  if (m === undefined) return [];
  const out: MplsStateTable[] = needsTableZero(m) ? [{ tableId: 0, name: 'vrx:0' }] : [];
  for (const id of Object.keys(m.tables ?? {}))
    out.push({ tableId: Number(id), name: `${agent.owner}:${id}` });
  return out.sort((a, b) => a.tableId - b.tableId);
}

function fib(doc: Json, m: Mpls, table: number): MplsStateFibEntry[] {
  const special: MplsStateFibEntry[] = [
    { label: 0, eos: true, payload: 'ip4', paths: [path({ type: 'local' })] },
    { label: 1, eos: false, payload: '', paths: [path({ type: 'local' })] },
    { label: 2, eos: true, payload: 'ip4', paths: [path({ type: 'local' })] },
  ];
  const out = [...special];
  for (const r of m.labelRoutes ?? []) {
    if (Number(r['table'] ?? 0) !== table) continue;
    const eos = r['eos'] !== false;
    const paths = (r['paths'] as Json[] | undefined) ?? [];
    const v6 = paths.some((p) => typeof p['nextHop'] === 'string' && ipFamily(p['nextHop']) === 6);
    const payload = eos ? String(r['payload'] ?? (v6 ? 'ip6' : 'ip4')) : '';
    out.push({
      label: Number(r['label']),
      eos,
      payload,
      paths: paths.map((p) => docPath(doc, p, payload)),
    });
  }
  if (table === 0) {
    for (const b of m.ipBindings ?? []) {
      out.push({
        label: Number(b['label']),
        eos: true,
        payload: 'ip4',
        paths: [path({ tableId: vrfId(doc, b['vrf']) })],
      });
    }
    for (const [bsid, p] of Object.entries(m.sr?.policies ?? {})) {
      const lists = (p['segmentLists'] as Json[] | undefined) ?? [];
      out.push({
        label: Number(bsid),
        eos: true,
        payload: 'mpls',
        paths: lists.map((l) =>
          path({ proto: 'mpls', weight: typeof l['weight'] === 'number' ? l['weight'] : 1 }),
        ),
      });
    }
  }
  return out.sort((a, b) => a.label - b.label || (a.eos === b.eos ? 0 : a.eos ? -1 : 1));
}

function tunnels(doc: Json, m: Mpls | undefined): MplsStateTunnel[] {
  return Object.entries(m?.tunnels ?? {})
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([name, t], i) => ({
      name,
      interface: `mpls-tunnel${i}`,
      swIfIndex: 100 + i,
      tunnelIndex: i,
      l2Only: t['l2Only'] === true,
      multicast: false,
      owned: true,
      paths: ((t['paths'] as Json[] | undefined) ?? []).map((p) => docPath(doc, p, '')),
    }));
}

export function mplsSrmplsFake(agent: FakeAgent): {
  mplsState: handleUnaryCall<MplsStateRequest, MplsStateResponse>;
} {
  return {
    mplsState: (call, cb) => {
      const r = call.request;
      agent.calls.push({ method: 'MplsState', request: r });
      if (r.owner && r.owner !== agent.owner)
        return cb(err(status.INVALID_ARGUMENT, `owner '${r.owner}' ≠ '${agent.owner}'`));
      if (r.view !== 'fib' && r.view !== 'tunnels')
        return cb(err(status.INVALID_ARGUMENT, `view "${r.view}": "fib" or "tunnels"`));
      const doc = agent.current;
      const m = mplsOf(doc);
      const readable = tables(agent, m);
      const base: MplsStateResponse = {
        owner: agent.owner,
        view: r.view,
        tableId: 0,
        total: 0,
        entries: [],
        tunnels: [],
        tables: readable,
        retrievedAt: new Date(),
      };
      if (r.view === 'tunnels') return cb(null, { ...base, tunnels: tunnels(doc, m) });
      const limit = r.limit || 100;
      if (limit > MAX_LIMIT)
        return cb(err(status.INVALID_ARGUMENT, `limit ${limit} > ${MAX_LIMIT}`));
      if (r.offset + limit > MAX_WINDOW)
        return cb(err(status.INVALID_ARGUMENT, `offset + limit > ${MAX_WINDOW}: filter by label`));
      if (m === undefined || !readable.some((t) => t.tableId === r.tableId))
        return cb(
          err(status.NOT_FOUND, `MPLS table ${r.tableId} does not exist or is not this owner's`),
        );
      const all = fib(doc, m, r.tableId).filter((e) => r.label === 0 || e.label === r.label);
      cb(null, {
        ...base,
        tableId: r.tableId,
        total: all.length,
        entries: all.slice(r.offset, r.offset + limit),
      });
    },
  };
}
