import { status, type handleUnaryCall } from '@grpc/grpc-js';
import type {
  Srv6StateLocalSid,
  Srv6StatePolicy,
  Srv6StateRequest,
  Srv6StateResponse,
  Srv6StateSteering,
} from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';

type Json = Record<string, unknown>;

export interface FakeSrv6Counters {
  goodPackets: number;
  goodBytes: number;
  badPackets: number;
  badBytes: number;
}

/** Counters the tests set per SID, per fake agent (default: all 0). */
const counters = new WeakMap<FakeAgent, Map<string, FakeSrv6Counters>>();

/** Set a fake local SID's counters (what sr_localsids_with_packet_stats_dump would report). */
export function setFakeSrv6Counters(agent: FakeAgent, sid: string, c: FakeSrv6Counters): void {
  let m = counters.get(agent);
  if (m === undefined) counters.set(agent, (m = new Map()));
  m.set(sid, c);
}

const obj = (v: unknown): Json =>
  v !== null && typeof v === 'object' && !Array.isArray(v) ? (v as Json) : {};
const str = (v: unknown, dflt = ''): string => (typeof v === 'string' ? v : dflt);
/** Plain code-unit order, as the agent's Go string comparison (never localeCompare). */
const cmp = (a: string, b: string): number => (a < b ? -1 : a > b ? 1 : 0);

/**
 * The fake agent's Srv6State (wave-A-hotspots P5: the real fake behaviour lives in the feature): the objects of
 * `routing.srv6` in what the fake applied (`agent.current`, protobuf JSON) as the real agent reports them — table ids
 * from `vrfs.<name>.id` (default = 0), an encapsulating policy's source resolved as policy `encapSource`, else
 * `routing.srv6.encapSource` ('' for insert policies), steering traffic type from the prefix family. Counters are 0
 * unless a test set them with setFakeSrv6Counters. Sorted like the agent: SIDs by SID, policies by BSID, steering L3 by
 * table then prefix, then L2 by interface. The write-only globals are never reported.
 */
export function srv6FakeState(
  agent: FakeAgent,
): handleUnaryCall<Srv6StateRequest, Srv6StateResponse> {
  return (call, cb) => {
    agent.calls.push({ method: 'Srv6State', request: call.request });
    if (call.request.owner && call.request.owner !== agent.owner) {
      cb({
        code: status.INVALID_ARGUMENT,
        details: `owner '${call.request.owner}' ≠ agent owner '${agent.owner}'`,
      });
      return;
    }
    const vrfs = obj(agent.current['vrfs']);
    const table = (name: unknown): number => {
      const n = str(name, 'default');
      const id = obj(vrfs[n])['id'];
      return typeof id === 'number' ? id : 0;
    };
    const srv6 = obj(obj(agent.current['routing'])['srv6']);
    const set = counters.get(agent) ?? new Map<string, FakeSrv6Counters>();
    const localSids: Srv6StateLocalSid[] = Object.entries(obj(srv6['localSids'])).map(
      ([sid, raw]) => {
        const l = obj(raw);
        const behavior = str(l['behavior']);
        const c = set.get(sid);
        return {
          sid,
          behavior,
          psp: l['psp'] === true,
          fibTable: table(l['vrf']),
          interface: str(l['interface']),
          nextHop: str(l['nextHop']),
          lookupTable: l['lookupVrf'] === undefined ? 0 : table(l['lookupVrf']),
          goodPackets: String(c?.goodPackets ?? 0),
          goodBytes: String(c?.goodBytes ?? 0),
          badPackets: String(c?.badPackets ?? 0),
          badBytes: String(c?.badBytes ?? 0),
        };
      },
    );
    localSids.sort((a, b) => cmp(a.sid, b.sid));
    const globalSrc = str(srv6['encapSource']);
    const policies: Srv6StatePolicy[] = Object.entries(obj(srv6['policies'])).map(([bsid, raw]) => {
      const p = obj(raw);
      const encap = p['encap'] !== false;
      const lists = Array.isArray(p['sidLists']) ? (p['sidLists'] as unknown[]) : [];
      return {
        bsid,
        type: str(p['type'], 'default'),
        encap,
        fibTable: table(p['vrf']),
        encapSource: encap ? str(p['encapSource'], globalSrc) : '',
        sidLists: lists.map((x) => {
          const l = obj(x);
          return {
            sids: Array.isArray(l['sids']) ? (l['sids'] as unknown[]).map((s) => str(s)) : [],
            weight: typeof l['weight'] === 'number' ? l['weight'] : 1,
          };
        }),
      };
    });
    policies.sort((a, b) => cmp(a.bsid, b.bsid));
    const raw = Array.isArray(srv6['steering']) ? (srv6['steering'] as unknown[]) : [];
    const steering: Srv6StateSteering[] = raw.map((x) => {
      const s = obj(x);
      if (s['type'] === 'l2')
        return {
          trafficType: 'l2',
          prefix: '',
          fibTable: 0,
          interface: str(s['interface']),
          bsid: str(s['bsid']),
        };
      const prefix = str(s['prefix']);
      return {
        trafficType: prefix.includes(':') ? 'ipv6' : 'ipv4',
        prefix,
        fibTable: table(s['vrf']),
        interface: '',
        bsid: str(s['bsid']),
      };
    });
    steering.sort((a, b) => {
      const l2a = a.trafficType === 'l2' ? 1 : 0;
      const l2b = b.trafficType === 'l2' ? 1 : 0;
      if (l2a !== l2b) return l2a - l2b;
      if (l2a === 1) return cmp(a.interface, b.interface);
      return a.fibTable - b.fibTable || cmp(a.prefix, b.prefix);
    });
    cb(null, { localSids, policies, steering, owner: agent.owner, retrievedAt: new Date() });
  };
}
