import { act, renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it } from 'vitest';
import { VrxWsClient, type WebSocketLike } from './client.js';
import { useTopic } from './useTopic.js';
import { WsProvider, useWsStatus } from './WsProvider.js';

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
  push(topic: string, data: unknown): void {
    this.onmessage?.({ data: JSON.stringify({ topic, data }) });
  }
}

function setup() {
  const sockets: FakeSocket[] = [];
  const client = new VrxWsClient({
    url: 'ws://unit/api/v1/stream',
    factory: () => {
      const s = new FakeSocket();
      sockets.push(s);
      return s;
    },
    flushIntervalMs: 0,
  });
  const wrapper = ({ children }: { children: ReactNode }) => <WsProvider client={client}>{children}</WsProvider>;
  return { sockets, client, wrapper };
}

describe('useTopic', () => {
  it('subscribes on mount through the shared client, receives data, unsubscribes on unmount', () => {
    const { sockets, wrapper } = setup();
    const hook = renderHook(() => ({ topic: useTopic<{ rx: number }>('iface.counters'), status: useWsStatus() }), { wrapper });
    expect(sockets).toHaveLength(1);
    expect(hook.result.current.status).toBe('connecting');

    act(() => sockets[0]!.open());
    expect(hook.result.current.status).toBe('open');
    expect(sockets[0]!.sent).toEqual([{ subscribe: ['iface.counters'] }]);

    act(() => sockets[0]!.push('iface.counters', { rx: 42 }));
    expect(hook.result.current.topic.data).toEqual({ rx: 42 });
    expect(hook.result.current.topic.batchSize).toBe(1);

    hook.unmount();
    expect(sockets[0]!.sent).toContainEqual({ unsubscribe: ['iface.counters'] });
    expect(sockets[0]!.closed).toBe(true); // last subscriber gone → socket released
  });

  it('two components share one socket and one server subscription per topic', () => {
    const { sockets, wrapper } = setup();
    const a = renderHook(() => useTopic('iface.counters'), { wrapper });
    const b = renderHook(() => useTopic('iface.counters'), { wrapper });
    const c = renderHook(() => useTopic('bgp.events'), { wrapper });
    expect(sockets).toHaveLength(1);
    act(() => sockets[0]!.open());
    expect(sockets[0]!.sent).toEqual([{ subscribe: ['iface.counters', 'bgp.events'] }]);
    act(() => sockets[0]!.push('iface.counters', 7));
    expect(a.result.current.data).toBe(7);
    expect(b.result.current.data).toBe(7);
    expect(c.result.current.data).toBeUndefined();
    a.unmount();
    expect(sockets[0]!.sent).toHaveLength(1); // b still listens → no unsubscribe yet
    b.unmount();
    expect(sockets[0]!.sent).toContainEqual({ unsubscribe: ['iface.counters'] });
    c.unmount();
  });
});
