import { Injectable } from '@nestjs/common';
import type {
  ActionDone,
  NatPoolUsage,
  NatSession,
  NatSessionFilter,
  NatSummaryResponse,
} from '@ngfw/proto';
import { isPlainObject } from '@ngfw/schema';
import { AgentClient } from '../../agent/agent.client.js';
import { ProblemError, problems } from '../../common/problem.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import type { KillBody, PoolUsageOut, SessionOut, SessionsQuery } from './dto.js';

/** Ports VPP's ED can allocate per outside address (1024–65535). */
export const PORTS_PER_ADDRESS = 64_512;

type Json = Record<string, unknown>;

export function sessionJson(s: NatSession): SessionOut {
  return {
    insideAddress: s.insideAddress,
    insidePort: s.insidePort,
    outsideAddress: s.outsideAddress,
    outsidePort: s.outsidePort,
    externalAddress: s.externalAddress,
    externalPort: s.externalPort,
    externalNatAddress: s.externalNatAddress,
    externalNatPort: s.externalNatPort,
    protocol: s.protocol,
    vrf: s.vrf,
    tableId: s.tableId,
    static: s.static,
    twiceNat: s.twiceNat,
    timedOut: s.timedOut,
    idleSeconds: Number(s.idleSeconds),
    bytes: Number(s.bytes),
    packets: Number(s.packets),
  };
}

/** `a.b.c.d-a.b.c.e` or one address → its ends (undefined when it is not that shape). */
function rangeEnds(range: string): { first: string; last: string } | undefined {
  const m = /^(\d{1,3}(?:\.\d{1,3}){3})(?:-(\d{1,3}(?:\.\d{1,3}){3}))?$/.exec(range.trim());
  if (m === null || m[1] === undefined) return undefined;
  return { first: m[1], last: m[2] ?? m[1] };
}

const vrfKey = (v: unknown): string => (typeof v === 'string' && v !== '' ? v : 'default');

function poolKey(p: {
  range?: string | null;
  interface?: string | null;
  vrf?: unknown;
  twiceNat: boolean;
}): string {
  if (p.interface) return `if|${p.interface}|${p.twiceNat}`;
  const e = rangeEnds(p.range ?? '');
  return `range|${e?.first ?? p.range}|${e?.last ?? p.range}|${vrfKey(p.vrf)}|${p.twiceNat}`;
}

/**
 * The agent's per-pool usage joined with the running configuration's pools (their names live only in the document:
 * VPP pools carry no tag): matched by identity (range + VRF + twice-NAT class, or interface + class). A pool the
 * agent reports that the running configuration lacks is `configured: false`; a configured pool the agent does not
 * report is `applied: false` (drift, or NAT44 not enabled).
 */
export function mergePools(
  agentPools: readonly NatPoolUsage[],
  runningNat: unknown,
): PoolUsageOut[] {
  const out: PoolUsageOut[] = [];
  const byKey = new Map<string, PoolUsageOut>();
  for (const p of agentPools) {
    const isIf = p.interface !== '';
    const range =
      p.firstAddress === ''
        ? null
        : p.firstAddress === p.lastAddress
          ? p.firstAddress
          : `${p.firstAddress}-${p.lastAddress}`;
    const row: PoolUsageOut = {
      name: null,
      kind: isIf ? 'interface' : 'range',
      range: isIf ? null : range,
      interface: isIf ? p.interface : null,
      vrf: isIf ? null : p.vrf || 'default',
      twiceNat: p.twiceNat,
      addresses: p.addresses,
      sessions: Number(p.sessions),
      utilisation:
        p.addresses > 0 ? Math.min(1, Number(p.sessions) / (p.addresses * PORTS_PER_ADDRESS)) : 0,
      applied: true,
      configured: false,
    };
    out.push(row);
    byKey.set(
      poolKey({ range: row.range, interface: row.interface, vrf: row.vrf, twiceNat: row.twiceNat }),
      row,
    );
  }
  const pools =
    isPlainObject(runningNat) && Array.isArray(runningNat['pools']) ? runningNat['pools'] : [];
  for (const raw of pools) {
    if (!isPlainObject(raw)) continue;
    const p = raw as Json;
    const twiceNat = p['twiceNat'] === true;
    const iface = typeof p['interface'] === 'string' ? p['interface'] : null;
    const range = typeof p['range'] === 'string' ? p['range'] : null;
    const name = typeof p['name'] === 'string' ? p['name'] : null;
    const hit = byKey.get(poolKey({ range, interface: iface, vrf: p['vrf'], twiceNat }));
    if (hit !== undefined && hit.name === null) {
      hit.name = name;
      hit.configured = true;
      continue;
    }
    const ends = range === null ? undefined : rangeEnds(range);
    out.push({
      name,
      kind: iface === null ? 'range' : 'interface',
      range,
      interface: iface,
      vrf: iface === null ? vrfKey(p['vrf']) : null,
      twiceNat,
      addresses: ends === undefined ? 0 : ipv4Count(ends.first, ends.last),
      sessions: 0,
      utilisation: 0,
      applied: false,
      configured: true,
    });
  }
  return out;
}

function ipv4ToNumber(a: string): number {
  return a.split('.').reduce((n, o) => n * 256 + Number(o), 0);
}

function ipv4Count(first: string, last: string): number {
  const n = ipv4ToNumber(last) - ipv4ToNumber(first) + 1;
  return n > 0 ? n : 0;
}

export function summaryJson(s: NatSummaryResponse, runningNat: unknown) {
  return {
    enabled: s.enabled,
    sessionLimit: s.sessionLimit,
    totalUsers: Number(s.totalUsers),
    totalSessions: Number(s.totalSessions),
    staticSessions: Number(s.staticSessions),
    truncated: s.truncated,
    byProtocol: Object.fromEntries(
      Object.entries(s.sessionsByProtocol).map(([k, v]) => [k, Number(v)]),
    ),
    pools: mergePools(s.pools, runningNat),
    retrievedAt: s.retrievedAt?.toISOString(),
  };
}

/** The kill's `done` → the HTTP outcome: 0 deleted, 1 not found (404), anything else 502. */
export function killOutcome(done: ActionDone) {
  if (done.exitCode === 0)
    return { deleted: true as const, summary: done.summary, stats: done.stats };
  if (done.exitCode === 1) throw problems.notFound(`agent: ${done.summary}`);
  throw new ProblemError(502, 'agent-error', 'Agent error', `agent: ${done.summary}`);
}

export function filterOf(q: SessionsQuery): NatSessionFilter {
  const f: NatSessionFilter = {};
  if (q.inside !== undefined) f.insideAddress = q.inside;
  if (q.outside !== undefined) f.outsideAddress = q.outside;
  if (q.external !== undefined) f.externalAddress = q.external;
  if (q.port !== undefined) f.port = q.port;
  if (q.protocol !== undefined) f.protocol = q.protocol.toLowerCase();
  if (q.vrf !== undefined) f.vrf = q.vrf;
  return f;
}

/** `/state/nat/**` and the kill action: everything from the agent (NatSessions, NatSummary, Action), never VPP. */
@Injectable()
export class Nat44EdSessionsService {
  constructor(
    private readonly agent: AgentClient,
    private readonly ds: DatastoreService,
  ) {}

  async sessions(q: SessionsQuery) {
    const r = await this.agent.natSessions({
      offset: (q.page - 1) * q.pageSize,
      limit: q.pageSize,
      filter: filterOf(q),
    });
    return {
      page: q.page,
      pageSize: q.pageSize,
      total: Number(r.totalSessions),
      totalUsers: Number(r.totalUsers),
      truncated: r.truncated,
      retrievedAt: r.retrievedAt?.toISOString(),
      items: r.sessions.slice(0, q.pageSize).map(sessionJson),
    };
  }

  async summary() {
    const [s, running] = await Promise.all([this.agent.natSummary(), this.ds.getRunning()]);
    return summaryJson(s, running.doc['nat']);
  }

  async kill(body: KillBody) {
    const done = await this.agent.natSessionKill({
      protocol: body.protocol,
      insideAddress: body.insideAddress,
      insidePort: body.insidePort,
      externalAddress: body.externalAddress,
      externalPort: body.externalPort,
      vrf: body.vrf ?? '',
    });
    return killOutcome(done);
  }
}
