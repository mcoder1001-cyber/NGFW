import { createPrivateKey, createPublicKey, diffieHellman } from 'node:crypto';
import { EventKind } from '@ngfw/proto';
import { describe, expect, it } from 'vitest';
import { eventTopic } from '../../telemetry/relay.service.js';
import { wireguardKeypair, wireguardNames, wireguardStateOf } from './wireguard.controller.js';

const jwk = (d: string | undefined, x: string) => ({
  kty: 'OKP',
  crv: 'X25519',
  x: Buffer.from(x, 'base64').toString('base64url'),
  ...(d === undefined ? {} : { d: Buffer.from(d, 'base64').toString('base64url') }),
});

describe('F-wireguard API helpers', () => {
  it('generates WireGuard key pairs (base64 of 32 bytes; the public key belongs to the private key)', () => {
    const a = wireguardKeypair();
    const b = wireguardKeypair();
    for (const k of [a.privateKey, a.publicKey]) expect(Buffer.from(k, 'base64')).toHaveLength(32);
    expect(a.privateKey).not.toBe(b.privateKey);
    const pa = createPrivateKey({ key: jwk(a.privateKey, a.publicKey), format: 'jwk' });
    const pb = createPrivateKey({ key: jwk(b.privateKey, b.publicKey), format: 'jwk' });
    expect(
      Buffer.from(createPublicKey(pa).export({ format: 'jwk' }).x!, 'base64url').toString('base64'),
    ).toBe(a.publicKey);
    // X25519 agreement works both ways
    const ab = diffieHellman({
      privateKey: pa,
      publicKey: createPublicKey({ key: jwk(undefined, b.publicKey), format: 'jwk' }),
    });
    const ba = diffieHellman({
      privateKey: pb,
      publicKey: createPublicKey({ key: jwk(undefined, a.publicKey), format: 'jwk' }),
    });
    expect(ab.equals(ba)).toBe(true);
  });

  it('joins the agent state with the running names', () => {
    const running = {
      vpn: {
        wireguard: { interfaces: { hq: { instance: 7, peers: { b1: { publicKey: 'P1=' } } } } },
      },
    };
    expect([...wireguardNames(running).itf]).toEqual([['wg7', 'hq']]);
    const out = wireguardStateOf(
      {
        owner: 'w1',
        eventsActive: false,
        retrievedAt: new Date('2026-09-25T00:00:00Z'),
        interfaces: [
          {
            name: 'wg7',
            instance: 7,
            swIfIndex: 3,
            publicKey: 'K=',
            listenPort: 1,
            listenAddress: '10.1.0.1',
            adminUp: true,
            linkUp: true,
            rxPackets: '1',
            rxBytes: '2',
            txPackets: '3',
            txBytes: '4',
            peers: [
              {
                publicKey: 'P1=',
                peerIndex: 0,
                established: false,
                dead: true,
                endpoint: '',
                endpointPort: 0,
                lastHandshake: undefined,
                persistentKeepaliveSec: 0,
                allowedIps: [],
              },
              {
                publicKey: 'P2=',
                peerIndex: 1,
                established: false,
                dead: false,
                endpoint: '10.1.0.2',
                endpointPort: 5,
                lastHandshake: undefined,
                persistentKeepaliveSec: 0,
                allowedIps: [],
              },
            ],
          },
        ],
      },
      running,
    );
    expect(out.interfaces[0]).toMatchObject({ name: 'hq', vppName: 'wg7', rxBytes: 2, txBytes: 4 });
    expect(out.interfaces[0]!.peers.map((p) => [p.name, p.status, p.endpoint])).toEqual([
      ['b1', 'dead', null],
      [null, 'down', '10.1.0.2'],
    ]);
  });

  it('relays peer events on wireguard.events', () => {
    expect(eventTopic(EventKind.EVENT_KIND_WIREGUARD_PEER_CHANGED)).toBe('wireguard.events');
  });
});
