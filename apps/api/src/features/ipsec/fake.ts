import { status, type handleUnaryCall } from '@grpc/grpc-js';
import type {
  IpsecConnState,
  IpsecIkeSa,
  IpsecStateRequest,
  IpsecStateResponse,
} from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';

type Json = Record<string, unknown>;

/** Tunnels a test forced down (no live IKE_SA), per fake agent. Default = each enabled strongSwan tunnel is up. */
const downTunnels = new WeakMap<FakeAgent, Set<string>>();

/** Force a fake IPsec tunnel down (no live SA) or back up. */
export function setFakeIpsecTunnelDown(agent: FakeAgent, tunnel: string, down = true): void {
  let s = downTunnels.get(agent);
  if (s === undefined) downTunnels.set(agent, (s = new Set()));
  if (down) s.add(tunnel);
  else s.delete(tunnel);
}

/** Enabled strongSwan tunnels of what the fake applied (`agent.current`, protobuf JSON). */
function strongswanTunnels(current: Json): [string, Json][] {
  const tuns = ((current['vpn'] as Json | undefined)?.['ipsec'] as Json | undefined)?.['tunnels'];
  const out: [string, Json][] = [];
  for (const [name, raw] of Object.entries((tuns ?? {}) as Json)) {
    const t = raw as Json;
    if (t['enabled'] === false) continue;
    const engine = t['engine'];
    if (engine !== undefined && engine !== 'strongswan') continue;
    out.push([name, t]);
  }
  return out;
}

function connOf(name: string, t: Json): IpsecConnState {
  const rekey = (t['rekey'] as Json | undefined) ?? {};
  return {
    name,
    tunnel: name,
    version: String((t['ikeVersion'] as number | undefined) ?? 2),
    localAddrs: [String(t['localAddr'] ?? '')],
    remoteAddrs: [String(t['remoteAddr'] ?? '')],
    localId: String(t['localId'] ?? t['localAddr'] ?? ''),
    remoteId: String(t['remoteId'] ?? ''),
    localAuth: (t['auth'] as Json | undefined)?.['method'] === 'cert' ? 'pubkey' : 'psk',
    remoteAuth: (t['auth'] as Json | undefined)?.['method'] === 'cert' ? 'pubkey' : 'psk',
    rekeySec: String((rekey['ikeSec'] as number | undefined) ?? 14400),
    reauthSec: '0',
    children: [
      {
        name,
        mode: String(t['mode'] ?? 'tunnel'),
        rekeySec: String((rekey['espSec'] as number | undefined) ?? 3600),
        localTs: [...((t['localTs'] as string[] | undefined) ?? [])],
        remoteTs: [...((t['remoteTs'] as string[] | undefined) ?? [])],
      },
    ],
  };
}

function establishedSaOf(name: string, t: Json, i: number): IpsecIkeSa {
  return {
    name,
    tunnel: name,
    uniqueId: String(i + 1),
    version: String((t['ikeVersion'] as number | undefined) ?? 2),
    state: 'ESTABLISHED',
    localHost: String(t['localAddr'] ?? ''),
    localPort: 500,
    localId: String(t['localId'] ?? t['localAddr'] ?? ''),
    remoteHost: String(t['remoteAddr'] ?? ''),
    remotePort: 500,
    remoteId: String(t['remoteId'] ?? t['remoteAddr'] ?? ''),
    initiator: true,
    natAny: false,
    encrAlg: 'AES_GCM_16',
    encrKeysize: 256,
    integAlg: '',
    prfAlg: 'PRF_HMAC_SHA2_256',
    dhGroup: 'CURVE_25519',
    establishedSec: '42',
    rekeySec: '13800',
    reauthSec: '0',
    children: [
      {
        name,
        uniqueId: String(i + 1),
        reqId: i + 1,
        state: 'INSTALLED',
        mode: String(t['mode'] ?? 'tunnel'),
        protocol: (t['protocol'] as string | undefined) === 'ah' ? 'AH' : 'ESP',
        encap: false,
        spiIn: (0xc0000000 + i).toString(16),
        spiOut: (0xd0000000 + i).toString(16),
        encrAlg: 'AES_GCM_16',
        encrKeysize: 256,
        integAlg: '',
        dhGroup: '',
        esn: false,
        bytesIn: '4096',
        packetsIn: '32',
        bytesOut: '4096',
        packetsOut: '32',
        rekeySec: '3300',
        lifeSec: '3600',
        installSec: '42',
        localTs: [...((t['localTs'] as string[] | undefined) ?? [])],
        remoteTs: [...((t['remoteTs'] as string[] | undefined) ?? [])],
        ifIdIn: '',
        ifIdOut: '',
      },
    ],
  };
}

/**
 * The fake agent's IpsecState (wave-A-hotspots P5): one loaded connection per enabled strongSwan tunnel of
 * `agent.current`, each with an ESTABLISHED IKE_SA + INSTALLED CHILD_SA (byte counters set, no keys) unless a
 * test forced it down with setFakeIpsecTunnelDown. Never returns key material.
 */
export function ipsecFakeState(
  agent: FakeAgent,
): handleUnaryCall<IpsecStateRequest, IpsecStateResponse> {
  return (call, cb) => {
    agent.calls.push({ method: 'IpsecState', request: call.request });
    if (call.request.owner && call.request.owner !== agent.owner) {
      cb({
        code: status.INVALID_ARGUMENT,
        details: `owner '${call.request.owner}' ≠ agent owner '${agent.owner}'`,
      });
      return;
    }
    const want = call.request.tunnels;
    const down = downTunnels.get(agent) ?? new Set<string>();
    const conns: IpsecConnState[] = [];
    const sas: IpsecIkeSa[] = [];
    let i = 0;
    for (const [name, t] of strongswanTunnels(agent.current as Json)) {
      if (want.length > 0 && !want.includes(name)) continue;
      conns.push(connOf(name, t));
      if (!down.has(name)) sas.push(establishedSaOf(name, t, i));
      i++;
    }
    conns.sort((a, b) => a.name.localeCompare(b.name));
    sas.sort((a, b) => a.name.localeCompare(b.name));
    const limit = call.request.limit === 0 ? 1000 : Math.min(call.request.limit, 1000);
    const page = sas.slice(call.request.offset, call.request.offset + limit);
    cb(null, {
      conns,
      sas: page,
      total: sas.length,
      pendingAction: '',
      owner: agent.owner,
      retrievedAt: new Date(),
      eventsActive: true,
      charonRestarted: false,
      daemonVersion: '5.9.13-fake',
      unlistedSas: '0',
    });
  };
}
