import type { paths } from '@ngfw/api-client';
import type { WireguardInterface, WireguardPeer } from '@ngfw/schema';
import type { VrxStatus } from '@ngfw/ui-kit';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { domainSchemas } from '../../../schema/registry';

type Ok<O> = O extends { responses: { 200: { content: { 'application/json': infer T } } } }
  ? T
  : never;

/** `GET /api/v1/state/vpn/wireguard` as generated from the OpenAPI document (never hand-written). */
export type WireguardState = Ok<NonNullable<paths['/api/v1/state/vpn/wireguard']['get']>>;
export type WgInterfaceState = WireguardState['interfaces'][number];
export type WgPeerState = WgInterfaceState['peers'][number];
export type WgPeerStatus = WgPeerState['status'];
/** `POST /api/v1/actions/vpn/wireguard/keypair`. */
export type Keypair = Ok<NonNullable<paths['/api/v1/actions/vpn/wireguard/keypair']['post']>>;

/** `vpn.wireguard.interfaces` of the candidate. */
export type WgInterfacesConfig = Record<string, WireguardInterface>;
export type { WireguardInterface, WireguardPeer };

function prop(s: JsonSchema | undefined, name: string): JsonSchema {
  const p = ((s?.properties ?? {}) as Record<string, JsonSchema>)[name];
  if (p === undefined) throw new Error(`schema property ${name} not found`);
  return p;
}

function recordItem(s: JsonSchema): JsonSchema {
  const a = (s as { additionalProperties?: JsonSchema }).additionalProperties;
  if (!a || typeof a !== 'object') throw new Error('record item schema not found');
  return a;
}

/** `vpn.wireguard.interfaces.<name>` item schema — the one schema (00-CONTEXT rule 5), without `peers` (own table). */
export function interfaceFormSchema(): JsonSchema {
  const item = recordItem(prop(prop(domainSchemas.vpn as JsonSchema, 'wireguard'), 'interfaces'));
  const props = { ...((item.properties ?? {}) as Record<string, JsonSchema>) };
  delete props['peers'];
  const required = Array.isArray(item.required)
    ? item.required.filter((r) => r !== 'peers')
    : undefined;
  return { ...item, properties: props, ...(required ? { required } : {}) } as JsonSchema;
}

/** `vpn.wireguard.interfaces.<name>.peers.<peer>` item schema. */
export function peerFormSchema(): JsonSchema {
  const item = recordItem(prop(prop(domainSchemas.vpn as JsonSchema, 'wireguard'), 'interfaces'));
  return recordItem(prop(item, 'peers'));
}

/** Semantic chip status of an interface (missing from the data plane = down). */
export function ifaceChip(st: WgInterfaceState | undefined): VrxStatus {
  if (!st) return 'down';
  if (!st.adminUp) return 'adminDown';
  return st.linkUp ? 'up' : 'down';
}

/** Semantic chip status of a peer. */
export function peerChip(s: WgPeerStatus | undefined): VrxStatus {
  switch (s) {
    case 'established':
      return 'up';
    case 'dead':
      return 'down';
    default:
      return 'adminDown';
  }
}

/** A live peer change from the WS topic `wireguard.events` (EVENT_KIND_WIREGUARD_PEER_CHANGED). */
export interface PeerEventData {
  kind?: string;
  interface?: string;
  ts?: string;
  attributes?: Record<string, string>;
}

export interface LivePeer {
  status: WgPeerStatus;
  at: string | undefined;
}

/** Folds peer events into `<wg>|<public key>` → latest status (newer events win). */
export function foldPeerEvents(
  prev: ReadonlyMap<string, LivePeer>,
  events: readonly PeerEventData[],
): Map<string, LivePeer> {
  const next = new Map(prev);
  for (const e of events) {
    const a = e.attributes ?? {};
    if (e.kind !== 'wireguard_peer_changed' || !e.interface || !a['public_key']) continue;
    const status: WgPeerStatus =
      a['established'] === 'true' ? 'established' : a['dead'] === 'true' ? 'dead' : 'down';
    next.set(`${e.interface}|${a['public_key']}`, { status, at: e.ts });
  }
  return next;
}

/** The status to show: the live event when it is newer than the state snapshot, else the snapshot. */
export function peerStatus(
  iface: WgInterfaceState,
  p: WgPeerState,
  live: ReadonlyMap<string, LivePeer>,
  stateAt: string | null,
): WgPeerStatus {
  const ev = live.get(`${iface.vppName}|${p.publicKey}`);
  if (ev && (stateAt === null || ev.at === undefined || ev.at >= stateAt)) return ev.status;
  return p.status;
}

/** A browser-side WireGuard key pair (WebCrypto X25519): the client's keys never reach the server. */
export async function browserKeypair(
  subtle: SubtleCrypto = globalThis.crypto.subtle,
): Promise<{ privateKey: string; publicKey: string }> {
  const kp = (await subtle.generateKey({ name: 'X25519' }, true, ['deriveBits'])) as CryptoKeyPair;
  const jwk = await subtle.exportKey('jwk', kp.privateKey);
  const raw = new Uint8Array(await subtle.exportKey('raw', kp.publicKey));
  if (!jwk.d) throw new Error('X25519 export failed');
  return { privateKey: b64(fromB64url(jwk.d)), publicKey: b64(raw) };
}

function fromB64url(s: string): Uint8Array {
  const b = atob(s.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - (s.length % 4)) % 4));
  return Uint8Array.from(b, (c) => c.charCodeAt(0));
}

function b64(u: Uint8Array): string {
  return btoa(String.fromCharCode(...u));
}

/**
 * The client (road-warrior) `.conf` of a peer: [Interface] with the client's private key — only when it was generated in
 * this browser session, otherwise a placeholder — and its address (the peer's first allowed IP); [Peer] = this router's
 * WireGuard interface (public key from the live state, endpoint = listen address:port, allowed IPs = the interface's
 * networks). Never stored anywhere by the UI.
 */
export function clientConfig(opts: {
  clientPrivateKey: string | undefined;
  peer: WireguardPeer;
  iface: WireguardInterface;
  serverPublicKey: string | undefined;
  endpointHost?: string;
}): string {
  const { peer, iface } = opts;
  const lines = [
    '[Interface]',
    `PrivateKey = ${opts.clientPrivateKey ?? '<the client private key: generated on the client>'}`,
    `Address = ${peer.allowedIps[0] ?? ''}`,
    '',
    '[Peer]',
    `PublicKey = ${opts.serverPublicKey ?? '<this router’s public key>'}`,
  ];
  if (peer.presharedKeyRef) lines.push(`PresharedKey = <the secret ${peer.presharedKeyRef}>`);
  const host = opts.endpointHost ?? iface.listenAddress;
  lines.push(`Endpoint = ${host.includes(':') ? `[${host}]` : host}:${iface.listenPort}`);
  const nets = (iface.address ?? []).map(networkOf);
  lines.push(`AllowedIPs = ${nets.length > 0 ? nets.join(', ') : '0.0.0.0/0, ::/0'}`);
  if ((peer.persistentKeepaliveSec ?? 0) > 0)
    lines.push(`PersistentKeepalive = ${peer.persistentKeepaliveSec}`);
  return lines.join('\n') + '\n';
}

/** "10.0.0.1/24" → "10.0.0.0/24" (IPv4); IPv6 addresses are kept as written. */
export function networkOf(cidr: string): string {
  const [addr, len] = cidr.split('/');
  if (addr === undefined || len === undefined || addr.includes(':')) return cidr;
  const bits = Number(len);
  const n = addr.split('.').reduce((acc, o) => (acc << 8) + Number(o), 0) >>> 0;
  const mask = bits === 0 ? 0 : (0xffffffff << (32 - bits)) >>> 0;
  const net = (n & mask) >>> 0;
  return `${[24, 16, 8, 0].map((s) => (net >>> s) & 255).join('.')}/${bits}`;
}
