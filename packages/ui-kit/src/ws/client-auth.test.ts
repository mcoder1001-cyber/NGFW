// @vitest-environment node
import type { AddressInfo } from 'node:net';
import { once } from 'node:events';
import type { IncomingMessage } from 'node:http';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { WebSocketServer, type WebSocket as ServerSocket } from 'ws';
import { VrxWsClient, type WebSocketLike } from './client.js';

/**
 * P07b: the browser authenticates the stream with subprotocols (`vrx.v1`, `bearer.<jwt>`, P06 D-P06-9). The
 * credential is read on every (re)connect, and without one the client stays idle instead of retrying 401s.
 */
describe('VrxWsClient credentials (subprotocols)', () => {
  let wss: WebSocketServer;
  let url: string;
  const offered: string[][] = [];
  const sockets: ServerSocket[] = [];

  beforeEach(async () => {
    wss = new WebSocketServer({
      host: '127.0.0.1',
      port: 0,
      handleProtocols: (protocols) => (protocols.has('vrx.v1') ? 'vrx.v1' : false),
    });
    await once(wss, 'listening');
    url = `ws://127.0.0.1:${(wss.address() as AddressInfo).port}/api/v1/stream`;
    wss.on('connection', (socket, req: IncomingMessage) => {
      sockets.push(socket);
      offered.push(String(req.headers['sec-websocket-protocol'] ?? '').split(',').map((s) => s.trim()));
    });
  });

  afterEach(async () => {
    for (const s of sockets) s.terminate();
    sockets.length = 0;
    offered.length = 0;
    await new Promise<void>((resolve) => wss.close(() => resolve()));
  });

  const factory = (u: string, p?: string[]): WebSocketLike => new WebSocket(u, p) as unknown as WebSocketLike;

  it('offers the current credential on each connect and re-reads it after a reconnect', async () => {
    let token = 'aaa.bbb.ccc';
    const client = new VrxWsClient({
      url,
      factory,
      protocols: () => ['vrx.v1', `bearer.${token}`],
      backoff: { baseMs: 10, maxMs: 20 },
      idleCloseDelayMs: 0,
    });
    const off = client.subscribe('commit.events', () => {});
    await vi.waitFor(() => expect(client.status).toBe('open'));
    expect(offered[0]).toEqual(['vrx.v1', 'bearer.aaa.bbb.ccc']);

    token = 'ddd.eee.fff'; // refreshed access token
    sockets[0]!.close(4401, 'credential expired'); // what the relay does at token expiry
    await vi.waitFor(() => expect(offered).toHaveLength(2));
    await vi.waitFor(() => expect(client.status).toBe('open'));
    expect(offered[1]).toEqual(['vrx.v1', 'bearer.ddd.eee.fff']);
    off();
    client.close();
  });

  it('stays idle while there is no credential and connects once there is one', async () => {
    const credential: { protocols?: string[] } = {};
    const client = new VrxWsClient({ url, factory, protocols: () => credential.protocols, idleCloseDelayMs: 0 });
    const off = client.subscribe('commit.events', () => {});
    expect(client.status).toBe('idle');
    await new Promise((r) => setTimeout(r, 50));
    expect(offered).toHaveLength(0);

    credential.protocols = ['vrx.v1', 'bearer.aaa.bbb.ccc'];
    client.connect();
    await vi.waitFor(() => expect(client.status).toBe('open'));
    expect(offered).toEqual([['vrx.v1', 'bearer.aaa.bbb.ccc']]);
    off();
    client.close();
  });

  it('surfaces P06 relay error frames through onError and ignores control frames', async () => {
    const errors: unknown[] = [];
    const client = new VrxWsClient({
      url,
      factory,
      protocols: () => ['vrx.v1', 'bearer.aaa.bbb.ccc'],
      onError: (e) => errors.push(e),
      flushIntervalMs: 10,
      idleCloseDelayMs: 0,
    });
    const got: unknown[] = [];
    const off = client.subscribe('commit.events', (b) => got.push(...b.map((m) => m.data)));
    await vi.waitFor(() => expect(client.status).toBe('open'));
    sockets[0]!.send(JSON.stringify({ type: 'heartbeat', ts: '2026-09-24T00:00:00Z' }));
    sockets[0]!.send(JSON.stringify({ type: 'error', message: 'unknown topic' }));
    sockets[0]!.send(JSON.stringify({ type: 'data', topic: 'commit.events', data: { type: 'confirmed', revision: 3 } }));
    await vi.waitFor(() => expect(got).toEqual([{ type: 'confirmed', revision: 3 }]));
    expect(errors).toEqual([{ detail: 'unknown topic' }]);
    off();
    client.close();
  });
});
