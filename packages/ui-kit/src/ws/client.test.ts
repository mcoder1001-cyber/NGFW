// @vitest-environment node
import type { AddressInfo } from 'node:net';
import { once } from 'node:events';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { WebSocketServer, type WebSocket as ServerSocket } from 'ws';
import { VrxWsClient, type TopicMessage, type WebSocketLike, type WsStatus } from './client.js';

/**
 * Runs the real client against a real `ws` server on an ephemeral loopback port (no slot port needed;
 * nothing is left running — the server is closed in afterEach).
 */
describe('VrxWsClient against a mock WebSocket server', () => {
  let wss: WebSocketServer;
  let url: string;
  const serverSockets: ServerSocket[] = [];
  const received: unknown[] = [];

  beforeEach(async () => {
    wss = new WebSocketServer({ host: '127.0.0.1', port: 0 });
    await once(wss, 'listening');
    url = `ws://127.0.0.1:${(wss.address() as AddressInfo).port}/api/v1/stream`;
    wss.on('connection', (socket) => {
      serverSockets.push(socket);
      socket.on('message', (data) => received.push(JSON.parse(data.toString())));
    });
  });

  afterEach(async () => {
    for (const s of serverSockets) s.terminate();
    serverSockets.length = 0;
    received.length = 0;
    await new Promise<void>((resolve) => wss.close(() => resolve()));
  });

  const factory = (u: string): WebSocketLike => new WebSocket(u) as unknown as WebSocketLike;

  it('multiplexes topics on one socket, buffers at the flush rate and unsubscribes on release', async () => {
    const client = new VrxWsClient({ url, factory, flushIntervalMs: 50, backoff: { baseMs: 10, maxMs: 50 }, idleCloseDelayMs: 0 });
    const statuses: WsStatus[] = [];
    client.onStatus((s) => statuses.push(s));
    const batches: TopicMessage<{ rx: number }>[][] = [];
    const off1 = client.subscribe<{ rx: number }>('iface.counters', (b) => batches.push(b));
    const off2 = client.subscribe('bgp.events', () => {});

    await vi.waitFor(() => expect(client.status).toBe('open'));
    expect(serverSockets).toHaveLength(1);
    await vi.waitFor(() => expect(received).toContainEqual({ subscribe: ['iface.counters', 'bgp.events'] }));

    for (let i = 0; i < 5; i++) {
      serverSockets[0]!.send(JSON.stringify({ topic: 'iface.counters', data: { rx: i }, ts: 1000 + i }));
    }
    serverSockets[0]!.send(JSON.stringify({ topic: 'not.subscribed', data: 1 }));
    await vi.waitFor(() => expect(batches.length).toBeGreaterThan(0));
    expect(batches).toHaveLength(1); // one delivery for five samples
    expect(batches[0]!.map((m) => m.data.rx)).toEqual([0, 1, 2, 3, 4]);
    expect(batches[0]![4]!.ts).toBe(1004);

    off2();
    await vi.waitFor(() => expect(received).toContainEqual({ unsubscribe: ['bgp.events'] }));
    expect(client.status).toBe('open');
    off1();
    await vi.waitFor(() => expect(client.status).toBe('idle'));
    expect(statuses).toEqual(['connecting', 'open', 'idle']);
  });

  it('reconnects with backoff after the server drops the connection and re-subscribes', async () => {
    const client = new VrxWsClient({ url, factory, flushIntervalMs: 0, backoff: { baseMs: 10, maxMs: 40, jitter: 0 }, idleCloseDelayMs: 0 });
    const statuses: WsStatus[] = [];
    client.onStatus((s) => statuses.push(s));
    const data: unknown[] = [];
    const off = client.subscribe('iface.counters', (b) => data.push(...b.map((m) => m.data)));
    await vi.waitFor(() => expect(client.status).toBe('open'));
    await vi.waitFor(() => expect(received).toHaveLength(1));

    serverSockets[0]!.terminate();
    await vi.waitFor(() => expect(statuses).toContain('reconnecting'));
    await vi.waitFor(() => expect(serverSockets).toHaveLength(2), { timeout: 3000 });
    await vi.waitFor(() => expect(client.status).toBe('open'));
    await vi.waitFor(() => expect(received).toHaveLength(2));
    expect(received[1]).toEqual({ subscribe: ['iface.counters'] });

    serverSockets[1]!.send(JSON.stringify({ topic: 'iface.counters', data: 'after-reconnect' }));
    await vi.waitFor(() => expect(data).toEqual(['after-reconnect']));
    expect(statuses).toEqual(['connecting', 'open', 'reconnecting', 'open']);
    off();
    await vi.waitFor(() => expect(client.status).toBe('idle'));
  });

  it('keeps retrying while the server is unreachable and stops when closed', async () => {
    const client = new VrxWsClient({ url: 'ws://127.0.0.1:1/api/v1/stream', factory, backoff: { baseMs: 5, maxMs: 20, jitter: 0 }, onError: () => {} });
    const off = client.subscribe('iface.counters', () => {});
    await vi.waitFor(() => expect(client.status).toBe('reconnecting'));
    client.close();
    expect(client.status).toBe('closed');
    off();
  });
});
