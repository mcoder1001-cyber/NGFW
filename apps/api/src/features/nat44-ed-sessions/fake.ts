import { status, type handleUnaryCall } from '@grpc/grpc-js';
import type {
  NatPoolUsage,
  NatSession,
  NatSessionsRequest,
  NatSessionsResponse,
  NatSummaryRequest,
  NatSummaryResponse,
} from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';

/**
 * The fake agent's NatSessions / NatSummary (wave-A-hotspots P5: wired into `FakeAgent.impl()` by one spread line). It
 * follows the agent's contract (proto.md §11): owner check, limit 0 = 100, > 1000 INVALID_ARGUMENT, AND filter, page
 * order = users by (table, address) then session order, never more than `limit` sessions; the summary counts the
 * seeded sessions per pool of the applied document (`fake.current.nat.pools`). Tests seed sessions with
 * `natFake(fake).sessions`. The kill runs through the fake's generic Action handler (UNIMPLEMENTED until the A4
 * dispatch owner adds per-feature cases there).
 */
export interface NatFakeState {
  enabled: boolean;
  sessionLimit: number;
  sessions: NatSession[];
}

const states = new WeakMap<FakeAgent, NatFakeState>();

/** The NAT state of one fake agent (created on first use: enabled, VPP's default session limit, no sessions). */
export function natFake(fake: FakeAgent): NatFakeState {
  let s = states.get(fake);
  if (s === undefined) {
    s = { enabled: true, sessionLimit: 63 * 1024, sessions: [] };
    states.set(fake, s);
  }
  return s;
}

/** A session row with the defaults a test does not care about. */
export function fakeSession(p: Partial<NatSession>): NatSession {
  return {
    insideAddress: '10.1.1.2',
    insidePort: 40000,
    outsideAddress: '10.1.2.100',
    outsidePort: 1024,
    externalAddress: '10.1.2.2',
    externalPort: 80,
    externalNatAddress: '0.0.0.0', // VPP fills it only for twice-NAT sessions
    externalNatPort: 0,
    protocol: 'tcp',
    vrf: 'default',
    tableId: 0,
    static: false,
    twiceNat: false,
    timedOut: false,
    idleSeconds: '1',
    bytes: '120',
    packets: '2',
    ...p,
  };
}

const ipNum = (a: string): number => a.split('.').reduce((n, o) => n * 256 + Number(o), 0);

type Json = Record<string, unknown>;

export function nat44EdSessionsFake(fake: FakeAgent): {
  natSessions: handleUnaryCall<NatSessionsRequest, NatSessionsResponse>;
  natSummary: handleUnaryCall<NatSummaryRequest, NatSummaryResponse>;
} {
  const ownerErr = (owner: string) =>
    owner && owner !== fake.owner
      ? { code: status.INVALID_ARGUMENT, details: `owner '${owner}' ≠ agent owner '${fake.owner}'` }
      : null;
  return {
    natSessions: (call, cb) => {
      const r = call.request;
      fake.calls.push({ method: 'NatSessions', request: r });
      const bad = ownerErr(r.owner);
      if (bad) return cb(bad);
      const limit = r.limit === 0 ? 100 : r.limit;
      if (limit > 1000)
        return cb({ code: status.INVALID_ARGUMENT, details: `limit ${limit} > 1000` });
      const f = r.filter ?? {};
      const rows = [...natFake(fake).sessions]
        .sort((a, b) => a.tableId - b.tableId || ipNum(a.insideAddress) - ipNum(b.insideAddress))
        .filter(
          (s) =>
            (f.insideAddress === undefined || s.insideAddress === f.insideAddress) &&
            (f.outsideAddress === undefined || s.outsideAddress === f.outsideAddress) &&
            (f.externalAddress === undefined || s.externalAddress === f.externalAddress) &&
            (f.protocol === undefined || s.protocol === f.protocol) &&
            (f.vrf === undefined || s.vrf === f.vrf) &&
            (f.port === undefined ||
              [s.insidePort, s.outsidePort, s.externalPort].includes(f.port)),
        );
      const page = rows.slice(r.offset, r.offset + limit);
      const users = new Set(rows.map((s) => `${s.tableId}|${s.insideAddress}`));
      const end = r.offset + page.length;
      cb(null, {
        sessions: page,
        ...(end < rows.length ? { nextOffset: end } : {}),
        totalUsers: String(users.size),
        totalSessions: String(rows.length),
        truncated: false,
        owner: fake.owner,
        retrievedAt: new Date(),
      });
    },
    natSummary: (call, cb) => {
      fake.calls.push({ method: 'NatSummary', request: call.request });
      const bad = ownerErr(call.request.owner);
      if (bad) return cb(bad);
      const st = natFake(fake);
      const nat = (fake.current['nat'] ?? {}) as Json;
      const pools: NatPoolUsage[] = [];
      for (const p of (nat['pools'] as Json[] | undefined) ?? []) {
        if (typeof p['range'] === 'string') {
          const [first = '', last = first] = p['range'].split('-');
          pools.push({
            firstAddress: first,
            lastAddress: last,
            interface: '',
            vrf: typeof p['vrf'] === 'string' ? p['vrf'] : 'default',
            twiceNat: p['twiceNat'] === true,
            addresses: ipNum(last) - ipNum(first) + 1,
            sessions: String(
              st.sessions.filter(
                (s) =>
                  ipNum(s.outsideAddress) >= ipNum(first) && ipNum(s.outsideAddress) <= ipNum(last),
              ).length,
            ),
          });
        } else if (typeof p['interface'] === 'string') {
          pools.push({
            firstAddress: '',
            lastAddress: '',
            interface: p['interface'],
            vrf: '',
            twiceNat: p['twiceNat'] === true,
            addresses: 0,
            sessions: '0',
          });
        }
      }
      const byProtocol: Record<string, string> = {};
      for (const s of st.sessions)
        byProtocol[s.protocol] = String(Number(byProtocol[s.protocol] ?? 0) + 1);
      cb(null, {
        enabled: st.enabled,
        sessionLimit: st.enabled ? st.sessionLimit : 0,
        totalUsers: String(new Set(st.sessions.map((s) => `${s.tableId}|${s.insideAddress}`)).size),
        totalSessions: String(st.sessions.length),
        staticSessions: String(st.sessions.filter((s) => s.static).length),
        pools,
        sessionsByProtocol: byProtocol,
        truncated: false,
        owner: fake.owner,
        retrievedAt: new Date(),
      });
    },
  };
}
