import type { handleUnaryCall } from '@grpc/grpc-js';
import type { TunnelStateRequest, TunnelStateResponse, TunnelStateTunnel } from '@ngfw/proto';
import { TUNNEL_KINDS } from '@ngfw/schema';

type Json = Record<string, unknown>;

/** The fake agent's view needed here: its owner and the applied state (protobuf JSON). */
export interface TunnelsFakeHost {
  readonly owner: string;
  current(): Json;
  record(method: string, request: unknown): void;
  /** gRPC status every call fails with (FakeAgent.failAllWith). */
  failWith(): number | undefined;
}

const obj = (v: unknown): Json => (v !== null && typeof v === 'object' ? (v as Json) : {});

/** VPP 26.06 device classes of the tunnel interfaces (as the agent's descriptors map them). */
const DEVICE_CLASS: Record<string, string> = {
  gre: 'GRE tunnel device',
  ipip: 'IPIP tunnel device',
  vxlan: 'VXLAN',
  vxlanGpe: 'VXLAN_GPE',
  gtpu: 'GTPU',
  l2tpv3: 'L2TPv3',
  pppoe: 'PPPoE',
};

/**
 * Fake TunnelState (S-tunnels-contract, wave-A-hotspots P5): derived from the applied `tunnels` the way the agent reports
 * it — one interface per tunnel, named `<prefix><instance>` for the instance-keyed kinds and `<prefix><n>` (allocation
 * order) for the kinds VPP names; admin state follows `enabled`, link follows admin (the fake has no VPP).
 */
export function tunnelStateFake(
  host: TunnelsFakeHost,
): handleUnaryCall<TunnelStateRequest, TunnelStateResponse> {
  return (call, cb) => {
    host.record('TunnelState', call.request);
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
    const t = obj(host.current()['tunnels']);
    const tunnels: TunnelStateTunnel[] = [];
    let swIfIndex = 100;
    let allocated = 0;
    for (const k of TUNNEL_KINDS) {
      for (const [name, v] of Object.entries(obj(t[k.key])).sort()) {
        const item = obj(v);
        const instance = item['instance'];
        const sixrd = k.key === 'ipip' && item['sixrd'] !== undefined;
        const iface =
          k.instance && !sixrd && typeof instance === 'number'
            ? `${k.vppPrefix}${instance}`
            : `${sixrd ? 'ipip' : k.vppPrefix}${allocated++}`;
        const up = item['enabled'] !== false;
        tunnels.push({
          name,
          kind: k.key,
          interface: iface,
          swIfIndex: swIfIndex++,
          adminUp: up,
          linkUp: up,
          mtu: typeof item['mtu'] === 'number' ? item['mtu'] : 9000,
          notes: [],
          counters: undefined,
          deviceClass: sixrd ? 'ip6ip-6rd' : (DEVICE_CLASS[k.key] ?? ''),
        });
      }
    }
    tunnels.sort((a, b) => a.kind.localeCompare(b.kind) || a.name.localeCompare(b.name));
    cb(null, { owner: host.owner, retrievedAt: new Date(), tunnels });
  };
}
