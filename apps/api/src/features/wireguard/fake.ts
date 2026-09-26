import { status, type handleUnaryCall } from '@grpc/grpc-js';
import type {
  WireguardInterfaceState,
  WireguardPeerState,
  WireguardStateRequest,
  WireguardStateResponse,
} from '@ngfw/proto';
import { createHash } from 'node:crypto';
import type { FakeAgent } from '../../testing/fake-agent.js';

type Json = Record<string, unknown>;

/** Peer state the tests set (public key → flags), per fake agent. */
const peerFlags = new WeakMap<
  FakeAgent,
  Map<string, { established: boolean; dead: boolean; at?: Date }>
>();

/** Set a fake peer's handshake state (the WS event is the test's own `agent.emit`). */
export function setFakeWireguardPeer(
  agent: FakeAgent,
  publicKey: string,
  state: { established: boolean; dead: boolean; at?: Date },
): void {
  let m = peerFlags.get(agent);
  if (m === undefined) peerFlags.set(agent, (m = new Map()));
  m.set(publicKey, state);
}

/** A stand-in public key for an interface (the fake has no key material): a hash of its private key reference. */
export function fakeInterfacePublicKey(privateKeyRef: string): string {
  return createHash('sha256').update(`fake-wg:${privateKeyRef}`).digest('base64');
}

/**
 * The fake agent's WireguardState (wave-A-hotspots P5: the real fake behaviour lives in the feature): one entry per
 * `vpn.wireguard.interfaces` of what the fake applied (`agent.current`, protobuf JSON), peers in the "down" state unless a
 * test set them with setFakeWireguardPeer, counters 0.
 */
export function wireguardFakeState(
  agent: FakeAgent,
): handleUnaryCall<WireguardStateRequest, WireguardStateResponse> {
  return (call, cb) => {
    agent.calls.push({ method: 'WireguardState', request: call.request });
    if (call.request.owner && call.request.owner !== agent.owner) {
      cb({
        code: status.INVALID_ARGUMENT,
        details: `owner '${call.request.owner}' ≠ agent owner '${agent.owner}'`,
      });
      return;
    }
    const flags = peerFlags.get(agent) ?? new Map();
    const wg = ((agent.current['vpn'] as Json | undefined)?.['wireguard'] as Json | undefined)?.[
      'interfaces'
    ];
    const out: WireguardInterfaceState[] = [];
    for (const raw of Object.values((wg ?? {}) as Json)) {
      const w = raw as Json;
      const name = `wg${String(w['instance'])}`;
      if (call.request.interfaces.length > 0 && !call.request.interfaces.includes(name)) continue;
      const peers: WireguardPeerState[] = Object.values((w['peers'] ?? {}) as Json).map((praw) => {
        const p = praw as Json;
        const key = String(p['publicKey']);
        const f = flags.get(key);
        const ep = p['endpoint'] as Json | undefined;
        return {
          publicKey: key,
          peerIndex: 0,
          established: f?.established ?? false,
          dead: f?.dead ?? false,
          endpoint: typeof ep?.['address'] === 'string' ? ep['address'] : '',
          endpointPort: typeof ep?.['port'] === 'number' ? ep['port'] : 0,
          lastHandshake: f?.at,
          persistentKeepaliveSec:
            typeof p['persistentKeepaliveSec'] === 'number' ? p['persistentKeepaliveSec'] : 0,
          allowedIps: [...((p['allowedIps'] as string[] | undefined) ?? [])].sort(),
        };
      });
      peers.sort((a, b) => a.publicKey.localeCompare(b.publicKey));
      out.push({
        name,
        instance: Number(w['instance']),
        swIfIndex: 0,
        publicKey: fakeInterfacePublicKey(String(w['privateKeyRef'])),
        listenPort: typeof w['listenPort'] === 'number' ? w['listenPort'] : 51820,
        listenAddress: String(w['listenAddress'] ?? ''),
        adminUp: w['enabled'] !== false,
        linkUp: w['enabled'] !== false,
        rxPackets: '0',
        rxBytes: '0',
        txPackets: '0',
        txBytes: '0',
        peers,
      });
    }
    out.sort((a, b) => a.name.localeCompare(b.name));
    cb(null, { interfaces: out, owner: agent.owner, retrievedAt: new Date(), eventsActive: true });
  };
}
