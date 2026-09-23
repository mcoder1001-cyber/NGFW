import { backoffDelay, DEFAULT_BACKOFF, type BackoffOptions } from './backoff.js';

/**
 * Wire protocol (docs/04-api-datamodel.md: `WSS /api/v1/stream { subscribe: [...] }`):
 *   client → server  { subscribe: string[] } | { unsubscribe: string[] }
 *   server → client  { topic: string, data: unknown, ts?: number }   (one message per sample)
 *                    { error: { title?: string, detail?: string } }  (non-fatal, surfaced via onError)
 * Recorded for P06 in docs/status/tasks/P07a-questions.md; the shape is isolated in `encode/decode`.
 */
export interface TopicMessage<T = unknown> {
  topic: string;
  data: T;
  /** Server timestamp (ms since epoch) when provided, else client receive time. */
  ts: number;
}

export type WsStatus = 'idle' | 'connecting' | 'open' | 'reconnecting' | 'closed';

export type TopicHandler<T = unknown> = (batch: TopicMessage<T>[]) => void;

/** Minimal WebSocket surface used by the client (matches the DOM type and `ws`). */
export interface WebSocketLike {
  readonly readyState: number;
  onopen: ((ev: unknown) => void) | null;
  onclose: ((ev: unknown) => void) | null;
  onerror: ((ev: unknown) => void) | null;
  onmessage: ((ev: { data: unknown }) => void) | null;
  send(data: string): void;
  close(code?: number, reason?: string): void;
}

export type WebSocketFactory = (url: string) => WebSocketLike;

export interface VrxWsClientOptions {
  url: string;
  /** Injected for tests / Node; defaults to `globalThis.WebSocket`. */
  factory?: WebSocketFactory;
  /** Buffered delivery period. Consumers never see more than one batch per topic per period. */
  flushIntervalMs?: number;
  backoff?: Partial<BackoffOptions>;
  /** Close the socket when the last topic is unsubscribed (default true). */
  closeWhenIdle?: boolean;
  onError?: (err: unknown) => void;
  /** Injected clock/timers for deterministic tests. */
  now?: () => number;
}

const OPEN = 1;

/**
 * ONE multiplexed connection for the whole SPA. Components never open sockets; they call
 * `useTopic()` which delegates here. Features: ref-counted topic subscriptions, lazy connect on
 * first subscriber, exponential backoff with jitter, automatic re-subscribe after reconnect, and
 * per-topic buffering flushed at 1 Hz so bursty counters do not re-render the UI on every sample.
 */
export class VrxWsClient {
  readonly url: string;
  private readonly factory: WebSocketFactory;
  private readonly flushIntervalMs: number;
  private readonly backoff: BackoffOptions;
  private readonly closeWhenIdle: boolean;
  private readonly onError: ((err: unknown) => void) | undefined;
  private readonly now: () => number;

  private socket: WebSocketLike | null = null;
  private statusValue: WsStatus = 'idle';
  private attempt = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private flushTimer: ReturnType<typeof setInterval> | null = null;
  private closedByUser = false;

  private readonly handlers = new Map<string, Set<TopicHandler>>();
  private readonly buffer = new Map<string, TopicMessage[]>();
  private readonly statusListeners = new Set<(s: WsStatus) => void>();

  constructor(opts: VrxWsClientOptions) {
    this.url = opts.url;
    this.factory =
      opts.factory ??
      ((url) => {
        const Ctor = (globalThis as { WebSocket?: new (u: string) => WebSocketLike }).WebSocket;
        if (!Ctor) throw new Error('WebSocket is not available in this environment');
        return new Ctor(url);
      });
    this.flushIntervalMs = opts.flushIntervalMs ?? 1000;
    this.backoff = { ...DEFAULT_BACKOFF, ...opts.backoff };
    this.closeWhenIdle = opts.closeWhenIdle ?? true;
    this.onError = opts.onError;
    this.now = opts.now ?? (() => Date.now());
  }

  get status(): WsStatus {
    return this.statusValue;
  }

  get topics(): string[] {
    return [...this.handlers.keys()];
  }

  onStatus(listener: (s: WsStatus) => void): () => void {
    this.statusListeners.add(listener);
    return () => {
      this.statusListeners.delete(listener);
    };
  }

  /** Subscribe to a topic. Returns the unsubscribe function. Connects lazily on the first subscriber. */
  subscribe<T = unknown>(topic: string, handler: TopicHandler<T>): () => void {
    let set = this.handlers.get(topic);
    const isNewTopic = !set;
    if (!set) this.handlers.set(topic, (set = new Set()));
    set.add(handler as TopicHandler);
    if (isNewTopic && this.statusValue === 'open') this.send({ subscribe: [topic] });
    if (this.socket === null) this.connect();
    return () => {
      const s = this.handlers.get(topic);
      if (!s) return;
      s.delete(handler as TopicHandler);
      if (s.size === 0) {
        this.handlers.delete(topic);
        this.buffer.delete(topic);
        if (this.statusValue === 'open') this.send({ unsubscribe: [topic] });
        if (this.handlers.size === 0 && this.closeWhenIdle) this.close();
      }
    };
  }

  /** Open the connection (idempotent). */
  connect(): void {
    if (this.socket) return;
    this.closedByUser = false;
    this.setStatus(this.attempt === 0 ? 'connecting' : 'reconnecting');
    let ws: WebSocketLike;
    try {
      ws = this.factory(this.url);
    } catch (err) {
      this.onError?.(err);
      this.scheduleReconnect();
      return;
    }
    this.socket = ws;
    let dead = false;
    const onDead = () => {
      if (dead || this.socket !== ws) return;
      dead = true;
      this.socket = null;
      this.stopFlush();
      if (this.closedByUser || this.handlers.size === 0) {
        this.setStatus(this.closedByUser ? 'closed' : 'idle');
        return;
      }
      this.scheduleReconnect();
    };
    ws.onopen = () => {
      this.attempt = 0;
      this.setStatus('open');
      const topics = this.topics;
      if (topics.length > 0) this.send({ subscribe: topics });
      this.startFlush();
    };
    ws.onmessage = (ev) => this.handleMessage(ev.data);
    ws.onerror = (ev) => {
      this.onError?.(ev);
      // A failed connection attempt fires `error` and, in some runtimes (Node's WebSocket), no `close`.
      if (ws.readyState !== OPEN) onDead();
    };
    ws.onclose = onDead;
  }

  /** Close intentionally; no reconnect until `subscribe()`/`connect()` is called again. */
  close(): void {
    this.closedByUser = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.stopFlush();
    this.attempt = 0;
    const ws = this.socket;
    this.socket = null;
    if (ws) {
      try {
        ws.close(1000, 'client closed');
      } catch (err) {
        this.onError?.(err);
      }
    }
    this.setStatus(this.handlers.size === 0 ? 'idle' : 'closed');
  }

  /** Deliver everything buffered right now (also used by tests). */
  flush(): void {
    if (this.buffer.size === 0) return;
    const pending = [...this.buffer.entries()];
    this.buffer.clear();
    for (const [topic, batch] of pending) {
      const set = this.handlers.get(topic);
      if (!set) continue;
      for (const h of set) {
        try {
          h(batch);
        } catch (err) {
          this.onError?.(err);
        }
      }
    }
  }

  // ---- internals ----

  private scheduleReconnect(): void {
    if (this.reconnectTimer) return;
    const delay = backoffDelay(this.attempt, this.backoff);
    this.attempt += 1;
    this.setStatus('reconnecting');
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      if (!this.closedByUser && this.handlers.size > 0) this.connect();
      else this.setStatus(this.closedByUser ? 'closed' : 'idle');
    }, delay);
  }

  private handleMessage(raw: unknown): void {
    let msg: unknown;
    try {
      const text = typeof raw === 'string' ? raw : raw instanceof Uint8Array ? new TextDecoder().decode(raw) : null;
      msg = text === null ? raw : JSON.parse(text);
    } catch (err) {
      this.onError?.(err);
      return;
    }
    if (!msg || typeof msg !== 'object') return;
    const m = msg as { topic?: unknown; data?: unknown; ts?: unknown; error?: unknown };
    if (m.error !== undefined) {
      this.onError?.(m.error);
      return;
    }
    if (typeof m.topic !== 'string') return;
    if (!this.handlers.has(m.topic)) return;
    const entry: TopicMessage = {
      topic: m.topic,
      data: m.data,
      ts: typeof m.ts === 'number' ? m.ts : this.now(),
    };
    const list = this.buffer.get(m.topic);
    if (list) list.push(entry);
    else this.buffer.set(m.topic, [entry]);
    if (this.flushIntervalMs <= 0) this.flush();
  }

  private send(payload: { subscribe: string[] } | { unsubscribe: string[] }): void {
    const ws = this.socket;
    if (!ws || ws.readyState !== OPEN) return;
    try {
      ws.send(JSON.stringify(payload));
    } catch (err) {
      this.onError?.(err);
    }
  }

  private startFlush(): void {
    this.stopFlush();
    if (this.flushIntervalMs > 0) this.flushTimer = setInterval(() => this.flush(), this.flushIntervalMs);
  }

  private stopFlush(): void {
    if (this.flushTimer) {
      clearInterval(this.flushTimer);
      this.flushTimer = null;
    }
    this.flush();
  }

  private setStatus(s: WsStatus): void {
    if (s === this.statusValue) return;
    this.statusValue = s;
    for (const l of this.statusListeners) l(s);
  }
}
