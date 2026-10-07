import type { handleUnaryCall } from '@grpc/grpc-js';
import type {
  BfdStateRequest,
  BfdStateResponse,
  RedistributionMatrixRequest,
  RedistributionMatrixResponse,
} from '@ngfw/proto';
import type { LispFakeHost } from '../lisp/fake.js';
const obj = (v: unknown): Record<string, unknown> =>
  v && typeof v === 'object' ? (v as Record<string, unknown>) : {};
const num = (v: unknown, fallback: number) => (typeof v === 'number' ? v : fallback);
const str = (v: unknown) => (typeof v === 'string' ? v : '');
export function bfdStateFake(
  host: LispFakeHost,
): handleUnaryCall<BfdStateRequest, BfdStateResponse> {
  return (call, cb) => {
    host.record('BfdState', call.request);
    const code = host.failWith() ?? (call.request.owner !== host.owner ? 3 : undefined);
    if (code !== undefined) {
      cb(Object.assign(new Error('fake agent failure'), { code }), null);
      return;
    }
    const b = obj(obj(host.current()['routing'])['bfd']);
    const sessions = Array.isArray(b['sessions']) ? b['sessions'].map(obj) : [];
    cb(null, {
      owner: host.owner,
      retrievedAt: new Date(),
      error: '',
      sessions: sessions.map((s) => ({
        engine: 'vpp',
        lastFlap: undefined,
        interface: str(s['interface']),
        localAddress: str(s['localAddress']),
        peerAddress: str(s['peerAddress']),
        state: s['enabled'] === false ? 'admin-down' : 'down',
        desiredMinTxUs: num(s['desiredMinTxUs'], 300000),
        requiredMinRxUs: num(s['requiredMinRxUs'], 300000),
        detectMultiplier: num(s['detectMultiplier'], 3),
        multihop: s['multihop'] === true,
      })),
    });
  };
}
export function redistributionMatrixFake(
  host: LispFakeHost,
): handleUnaryCall<RedistributionMatrixRequest, RedistributionMatrixResponse> {
  return (call, cb) => {
    host.record('RedistributionMatrix', call.request);
    const code = host.failWith() ?? (call.request.owner !== host.owner ? 3 : undefined);
    if (code !== undefined) {
      cb(Object.assign(new Error('fake agent failure'), { code }), null);
      return;
    }
    const r = obj(host.current()['routing']);
    const edges: RedistributionMatrixResponse['edges'] = [];
    for (const target of ['bgp', 'ospf', 'isis', 'rip', 'ospf6', 'ripng']) {
      const c = obj(r[target]);
      for (const [source, raw] of Object.entries(obj(c['redistribute']))) {
        const opt = obj(raw);
        edges.push({
          source,
          target,
          vrf: str(c['vrf']) || 'default',
          routeMap: str(opt['routeMap']),
          metric: typeof opt['metric'] === 'number' ? opt['metric'] : undefined,
          readOnly: target === 'ospf6' || target === 'ripng',
        });
      }
    }
    cb(null, { owner: host.owner, retrievedAt: new Date(), edges, error: '' });
  };
}
