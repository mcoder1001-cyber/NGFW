import { status, type handleUnaryCall } from '@grpc/grpc-js';
import type {
  QosPolicerCounter,
  QosPolicerResetRequest,
  QosPolicerResetResponse,
  QosPolicerStateRequest,
  QosPolicerStateResponse,
  QosPolicerStatus,
} from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';
import { SHAPER_PREFIX, shaperBurstBytes } from './model.js';

/**
 * The fake agent's F-qos-flat RPCs (wave-A-hotspots P5), wired by the one `...qosFlatFake(this)` line in
 * testing/fake-agent.ts. The policers are what the fake holds as applied (`agent.current.services.qos`, protobuf
 * JSON): each policer as configured (defaults 1r2c / kbps), each shaper as the agent realises it — an egress
 * policer `shaper:<name>` (1r2c, kbps, cir = rateKbps, cb = burstBytes or the derived burst). Indexes follow the
 * sorted names; buckets are full. Counters are what a test sets with {@link setFakeQosCounters}; resets are recorded
 * in {@link fakeQosResets}. Owner mismatch, forced failures and "services not implemented" behave like the real agent
 * (docs/contracts/proto.md §11).
 */
type Json = Record<string, unknown>;

export interface FakeQosCounters {
  conform?: { packets: number; bytes: number };
  exceed?: { packets: number; bytes: number };
  violate?: { packets: number; bytes: number };
}

interface FakeQosState {
  counters: Map<string, FakeQosCounters>;
  resets: string[];
}

const states = new WeakMap<FakeAgent, FakeQosState>();

function stateOf(agent: FakeAgent): FakeQosState {
  let s = states.get(agent);
  if (!s) {
    s = { counters: new Map(), resets: [] };
    states.set(agent, s);
  }
  return s;
}

/** Set the counters the fake reports for a policer (VPP-side name: `<policer>` or `shaper:<name>`). */
export function setFakeQosCounters(agent: FakeAgent, vppName: string, c: FakeQosCounters): void {
  stateOf(agent).counters.set(vppName, c);
}

/** Policer names the fake was asked to reset, in order. */
export function fakeQosResets(agent: FakeAgent): string[] {
  return stateOf(agent).resets;
}

const isObj = (v: unknown): v is Json => typeof v === 'object' && v !== null && !Array.isArray(v);
const n = (v: unknown): number =>
  typeof v === 'number' ? v : typeof v === 'string' && v !== '' ? Number(v) : 0;

function counter(c: { packets: number; bytes: number } | undefined): QosPolicerCounter {
  return { packets: String(c?.packets ?? 0), bytes: String(c?.bytes ?? 0) };
}

/** The policers the fake's applied state realises, sorted by name, indexed in that order. */
function policersOf(agent: FakeAgent): QosPolicerStatus[] {
  const services = agent.current['services'];
  const qos = isObj(services) && isObj(services['qos']) ? services['qos'] : {};
  const rows: Omit<QosPolicerStatus, 'index' | 'conform' | 'exceed' | 'violate'>[] = [];
  const bucket = (cb: number, eb: number) => ({
    currentBucket: cb,
    currentLimit: cb,
    extendedBucket: eb,
    extendedLimit: eb,
  });
  for (const [name, p] of Object.entries(isObj(qos['policers']) ? qos['policers'] : {})) {
    if (!isObj(p)) continue;
    const cb = n(p['cb']);
    const eb = n(p['eb']);
    rows.push({
      name,
      kind: 'policer',
      type: typeof p['type'] === 'string' ? p['type'] : '1r2c',
      rateUnit: typeof p['rateUnit'] === 'string' ? p['rateUnit'] : 'kbps',
      cir: n(p['cir']),
      eir: n(p['eir']),
      cb: String(cb),
      eb: String(eb),
      ...bucket(cb, eb),
    });
  }
  for (const [name, s] of Object.entries(isObj(qos['shapers']) ? qos['shapers'] : {})) {
    if (!isObj(s)) continue;
    const rate = n(s['rateKbps']);
    const cb = s['burstBytes'] !== undefined ? n(s['burstBytes']) : shaperBurstBytes(rate);
    rows.push({
      name: SHAPER_PREFIX + name,
      kind: 'shaper',
      type: '1r2c',
      rateUnit: 'kbps',
      cir: rate,
      eir: 0,
      cb: String(cb),
      eb: '0',
      ...bucket(cb, 0),
    });
  }
  rows.sort((a, b) => a.name.localeCompare(b.name));
  const counters = stateOf(agent).counters;
  return rows.map((r, index) => {
    const c = counters.get(r.name) ?? {};
    return {
      ...r,
      index,
      conform: counter(c.conform),
      exceed: counter(c.exceed),
      violate: counter(c.violate),
    };
  });
}

/** Common checks of both RPCs; false = already answered. */
function admit(
  agent: FakeAgent,
  method: string,
  req: { owner: string },
  cb: (err: { code: status; details: string } | null) => void,
): boolean {
  agent.calls.push({ method, request: req });
  if (agent.failAllWith !== undefined) {
    cb({ code: agent.failAllWith, details: `fake agent: forced ${status[agent.failAllWith]}` });
    return false;
  }
  if (req.owner && req.owner !== agent.owner) {
    cb({
      code: status.INVALID_ARGUMENT,
      details: `owner '${req.owner}' ≠ agent owner '${agent.owner}'`,
    });
    return false;
  }
  if (!agent.implemented.includes('services')) {
    cb({ code: status.UNIMPLEMENTED, details: 'fake agent: services not implemented' });
    return false;
  }
  return true;
}

export function qosFlatFake(agent: FakeAgent): {
  qosPolicerState: handleUnaryCall<QosPolicerStateRequest, QosPolicerStateResponse>;
  qosPolicerReset: handleUnaryCall<QosPolicerResetRequest, QosPolicerResetResponse>;
} {
  return {
    qosPolicerState: (call, cb) => {
      const req = call.request;
      if (!admit(agent, 'QosPolicerState', req, cb)) return;
      const want = new Set(req.names);
      cb(null, {
        policers: policersOf(agent).filter((p) => want.size === 0 || want.has(p.name)),
        owner: agent.owner,
        retrievedAt: new Date(),
        countersError: '',
      });
    },
    qosPolicerReset: (call, cb) => {
      const req = call.request;
      if (!admit(agent, 'QosPolicerReset', req, cb)) return;
      const p = policersOf(agent).find((x) => x.name === req.name);
      if (!p) {
        cb({
          code: status.NOT_FOUND,
          details: `no policer '${req.name}' of owner '${agent.owner}'`,
        });
        return;
      }
      stateOf(agent).resets.push(req.name);
      cb(null, { owner: agent.owner, name: req.name, index: p.index, resetAt: new Date() });
    },
  };
}
