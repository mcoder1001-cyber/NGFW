import { act, renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { VrxWsClient, type TopicMessage, type VrxWsClientOptions, type WebSocketLike } from './client.js';
import { useTopic } from './useTopic.js';
import { WsProvider } from './WsProvider.js';

/** Review P07a L6: buffer cap, no backoff bypass, idle grace period, topic switch resets state. */
class FakeSocket implements WebSocketLike {
  readyState = 0;
  onopen: ((ev: unknown) => void) | null = null;
  onclose: ((ev: unknown) => void) | null = null;
  onerror: ((ev: unknown) => void) | null = null;
  onmessage: ((ev: { data: unknown }) => void) | null = null;
  sent: unknown[] = [];
  closed = false;
  send(data: string): void {
    this.sent.push(JSON.parse(data));
  }
  close(): void {
    this.closed = true;
    this.readyState = 3;
    this.onclose?.({});
  }
  open(): void {
    this.readyState = 1;
    this.onopen?.({});
  }
  drop(): void {
    this.readyState = 3;
    this.onclose?.({});
  }
  push(topic: string, data: unknown): void {
    this.onmessage?.({ data: JSON.stringify({ topic, data }) });
  }
}

function make(opts: Partial<VrxWsClientOptions> = {}) {
  const sockets: FakeSocket[] = [];
  const client = new VrxWsClient({
    url: 'ws://unit/api/v1/stream',
    factory: () => {
      const s = new FakeSocket();
      sockets.push(s);
      return s;
    },
    flushIntervalMs: 1000,
    backoff: { baseMs: 1000, maxMs: 1000, jitter: 0 },
    ...opts,
  });
  return { sockets, client };
}

describe('VrxWsClient edge cases (review L6)', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('(a) caps the per-topic buffer, keeping the newest samples', () => {
    const { sockets, client } = make({ maxBufferPerTopic: 3 });
    const batches: TopicMessage[][] = [];
    client.subscribe('iface.counters', (b) => batches.push(b));
    sockets[0]!.open();
    for (let i = 0; i < 10; i++) sockets[0]!.push('iface.counters', i);
    vi.advanceTimersByTime(1000);
    expect(batches).toHaveLength(1);
    expect(batches[0]!.map((m) => m.data)).toEqual([7, 8, 9]);
    client.close();
  });

  it('(b) a subscribe during backoff waits for the scheduled reconnect', () => {
    const { sockets, client } = make();
    const off1 = client.subscribe('a', () => {});
    sockets[0]!.open();
    sockets[0]!.drop();
    expect(client.status).toBe('reconnecting');
    const off2 = client.subscribe('b', () => {});
    expect(sockets).toHaveLength(1); // no immediate reconnect
    vi.advanceTimersByTime(999);
    expect(sockets).toHaveLength(1);
    vi.advanceTimersByTime(1);
    expect(sockets).toHaveLength(2);
    sockets[1]!.open();
    expect(sockets[1]!.sent).toEqual([{ subscribe: ['a', 'b'] }]);
    off1();
    off2();
    client.close();
  });

  it('(c) keeps the socket through a short idle gap, closes after the grace period', () => {
    const { sockets, client } = make({ idleCloseDelayMs: 500 });
    const off1 = client.subscribe('a', () => {});
    sockets[0]!.open();
    off1(); // page A unmounts
    vi.advanceTimersByTime(100);
    const off2 = client.subscribe('b', () => {}); // page B mounts
    vi.advanceTimersByTime(1000);
    expect(sockets).toHaveLength(1);
    expect(sockets[0]!.closed).toBe(false);
    expect(client.status).toBe('open');
    off2();
    vi.advanceTimersByTime(499);
    expect(sockets[0]!.closed).toBe(false);
    vi.advanceTimersByTime(1);
    expect(sockets[0]!.closed).toBe(true);
    expect(client.status).toBe('idle');
  });
});

describe('useTopic topic switch (review L6d)', () => {
  it('drops the previous topic data when the topic changes', () => {
    const { sockets, client } = make({ flushIntervalMs: 0, idleCloseDelayMs: 0 });
    const wrapper = ({ children }: { children: ReactNode }) => <WsProvider client={client}>{children}</WsProvider>;
    const hook = renderHook(({ topic }) => useTopic(topic), { wrapper, initialProps: { topic: 'a' } });
    act(() => sockets[0]!.open());
    act(() => sockets[0]!.push('a', 'from-a'));
    expect(hook.result.current.data).toBe('from-a');
    hook.rerender({ topic: 'b' });
    expect(hook.result.current.data).toBeUndefined();
    expect(hook.result.current.updatedAt).toBeUndefined();
    act(() => sockets[0]!.push('b', 'from-b'));
    expect(hook.result.current.data).toBe('from-b');
    hook.unmount();
    client.close();
  });
});
