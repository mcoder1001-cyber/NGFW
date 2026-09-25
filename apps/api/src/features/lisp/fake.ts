import type { handleUnaryCall } from '@grpc/grpc-js';
import type { LispStateRequest, LispStateResponse } from '@ngfw/proto';

type Json = Record<string, unknown>;

/** The fake agent's view needed here: its owner and the applied state (protobuf JSON). */
export interface LispFakeHost {
  readonly owner: string;
  current(): Json;
  record(method: string, request: unknown): void;
  /** gRPC status every call fails with (FakeAgent.failAllWith). */
  failWith(): number | undefined;
}

const ACTIONS = new Set(['no-action', 'natively-forward', 'send-map-request', 'drop']);
const arr = (v: unknown): Json[] => (Array.isArray(v) ? (v as Json[]) : []);
const obj = (v: unknown): Json => (v !== null && typeof v === 'object' ? (v as Json) : {});
const num = (v: unknown): number => (typeof v === 'number' ? v : 0);
const str = (v: unknown): string => (typeof v === 'string' ? v : '');

/**
 * Fake LispState (F-lisp, wave-A-hotspots P5): derived from the applied `tunnels.lisp` the way VPP would report it —
 * local EIDs and static remote mappings in the EID table, adjacencies, EID-table maps (VRF names become table ids from
 * `vrfs`), resolvers, the GPE VNIs. Interfaces are reported by name with sw_if_index 0 (the fake has no VPP).
 */
export function lispStateFake(
  host: LispFakeHost,
): handleUnaryCall<LispStateRequest, LispStateResponse> {
  return (call, cb) => {
    host.record('LispState', call.request);
    const code = host.failWith();
    if (code !== undefined) {
      cb(Object.assign(new Error('fake agent failure'), { code }), null);
      return;
    }
    if (call.request.owner !== host.owner) {
      cb(
        Object.assign(new Error(`owner "${call.request.owner}" does not match`), { code: 3 }),
        null,
      );
      return;
    }
    const cur = host.current();
    const l = obj(obj(cur['tunnels'])['lisp']);
    const vrfs = obj(cur['vrfs']);
    const tableOf = (name: string) => (name === 'default' ? 0 : num(obj(vrfs[name])['id']));
    const enabled = l['enabled'] === true;
    const gpe = enabled || l['gpe'] === true; // VPP enables GPE together with LISP
    const res: LispStateResponse = {
      owner: host.owner,
      enabled,
      gpeEnabled: gpe,
      pitr: enabled ? str(l['pitr']) : '',
      locatorSets: [],
      mappings: [],
      adjacencies: [],
      eidTables: [],
      mapResolvers: [],
      mapServers: [],
      gpeVnis: [],
      retrievedAt: new Date(),
    };
    if (enabled) {
      for (const [name, set] of Object.entries(obj(l['locatorSets'])).sort()) {
        res.locatorSets.push({
          name,
          locators: arr(obj(set)['locators']).map((x) => ({
            interface: str(x['interface']),
            swIfIndex: 0,
            priority: num(x['priority']),
            weight: num(x['weight']),
          })),
        });
      }
      for (const e of arr(l['localEids'])) {
        res.mappings.push({
          vni: num(e['vni']),
          eid: str(e['eid']),
          local: true,
          locatorSet: str(e['locatorSet']),
          rlocs: [],
          action: 'no-action',
          authoritative: false,
          ttl: 0,
        });
      }
      for (const m of arr(l['remoteMappings'])) {
        const action = str(m['action']);
        res.mappings.push({
          vni: num(m['vni']),
          eid: str(m['eid']),
          local: false,
          locatorSet: '',
          rlocs: arr(m['rlocs']).map((r) => str(r['address'])),
          action: ACTIONS.has(action) ? action : 'no-action',
          authoritative: true,
          ttl: 0,
        });
      }
      for (const a of arr(l['adjacencies'])) {
        res.adjacencies.push({ vni: num(a['vni']), reid: str(a['reid']), leid: str(a['leid']) });
      }
      for (const [vni, t] of Object.entries(obj(l['eidTables']))) {
        const bd = obj(t)['bridgeDomain'];
        res.eidTables.push(
          typeof bd === 'number'
            ? { vni: Number(vni), dpTable: bd, isL2: true }
            : { vni: Number(vni), dpTable: tableOf(str(obj(t)['vrf'])), isL2: false },
        );
      }
      res.eidTables.sort((a, b) => a.vni - b.vni);
      res.mapResolvers = arr(l['mapResolvers']).map(String).sort();
      res.mapServers = arr(l['mapServers']).map(String).sort();
    }
    if (gpe) {
      res.gpeVnis = [...new Set(arr(l['gpeEntries']).map((g) => num(g['vni'])))].sort(
        (a, b) => a - b,
      );
    }
    cb(null, res);
  };
}
