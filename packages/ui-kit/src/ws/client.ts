import { backoffDelay, DEFAULT_BACKOFF, type BackoffOptions } from './backoff.js';

/**
 * Wire protocol (docs/04-api-datamodel.md: `WSS /api/v1/stream { subscribe: [...] }`):
 *   client → server  { subscribe: string[] } | { unsubscribe: string[] }
 *   server → client  { topic: string, data: unknown, ts?: number }   (one message per sample)
 *                    { error: { title?: string, detail?: string } }  (non-fatal, surfaced via onError)
 * P06's relay (apps/api/src/telemetry) sends `{ type: 'data', topic, data }`, `{ type: 'error', message }` and
 * control frames (`heartbeat`, `pong`, `subscribed`) that carry no topic and are ignored here.
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

/** `protocols` is the subprotocol list offered in the handshake (browsers cannot set headers on a WebSocket). */
export type WebSocketFactory = (url: string, protocols?: string[]) => WebSocketLike;

export interface VrxWsClientOptions {
  url: string;
  /** Injected for tests / Node; defaults to `globalThis.WebSocket`. */
  factory?: WebSocketFactory;
  /** Buffered delivery period. Consumers never see more than one batch per topic per period. */
  flushIntervalMs?: number;
  backoff?: Partial<BackoffOptions>;
  /** Close the socket when the last topic is unsubscribed (default true). */
  closeWhenIdle?: boolean;
  /**
   * Grace period before an idle socket is closed (default 2000 ms), so navigating between two pages that both use
   * topics does not drop and reopen the connection (review P07a L6c). 0 closes immediately.
   */
  idleCloseDelayMs?: number;
  /**
   * Most samples kept per topic between flushes (default 600); older ones are dropped. Background tabs throttle
   * timers to about once a minute, so an unbounded buffer could grow without limit (review P07a L6a).
   */
  maxBufferPerTopic?: number;
  /**
   * Subprotocols offered on every (re)connect, read at connect time so a refreshed credential is used after a
   * reconnect (P06 D-P06-9: `['vrx.v1', 'bearer.<access-token>']`, never a token in the URL). Returning
   * `undefined` postpones the connection: the client stays `idle` until `connect()`/`subscribe()` is called again.
   */
  protocols?: () => string[] | undefined;
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
  private readonly idleCloseDelayMs: number;
  private readonly maxBufferPerTopic: number;
  private idleTimer: ReturnType<typeof setTimeout> | null = null;
  private readonly onError: ((err: unknown) => void) | undefined;
  private readonly protocols: (() => string[] | undefined) | undefined;
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
      ((url, protocols) => {
        const Ctor = (globalThis as { WebSocket?: new (u: string, p?: string[]) => WebSocketLike }).WebSocket;
        if (!Ctor) throw new Error('WebSocket is not available in this environment');
        return protocols ? new Ctor(url, protocols) : new Ctor(url);
      });
    this.flushIntervalMs = opts.flushIntervalMs ?? 1000;
    this.backoff = { ...DEFAULT_BACKOFF, ...opts.backoff };
    this.closeWhenIdle = opts.closeWhenIdle ?? true;
    this.idleCloseDelayMs = opts.idleCloseDelayMs ?? 2000;
    this.maxBufferPerTopic = Math.max(1, opts.maxBufferPerTopic ?? 600);
    this.onError = opts.onError;
    this.protocols = opts.protocols;
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
    this.cancelIdleClose();
    if (isNewTopic && this.statusValue === 'open') this.send({ subscribe: [topic] });
    // During backoff the pending reconnect timer connects; do not bypass the delay (review P07a L6b).
    if (this.socket === null && this.reconnectTimer === null) this.connect();
    return () => {
      const s = this.handlers.get(topic);
      if (!s) return;
      s.delete(handler as TopicHandler);
      if (s.size === 0) {
        this.handlers.delete(topic);
        this.buffer.delete(topic);
        if (this.statusValue === 'open') this.send({ unsubscribe: [topic] });
        if (this.handlers.size === 0 && this.closeWhenIdle) this.scheduleIdleClose();
      }
    };
  }

  /** Open the connection (idempotent). */
  connect(): void {
    if (this.socket) return;
    this.closedByUser = false;
    let protocols: string[] | undefined;
    if (this.protocols) {
      protocols = this.protocols();
      // no credential yet (signed out): stay idle instead of hammering the server with 401 upgrades
      if (protocols === undefined) {
        this.setStatus('idle');
        return;
      }
    }
    this.setStatus(this.attempt === 0 ? 'connecting' : 'reconnecting');
    let ws: WebSocketLike;
    try {
      ws = protocols ? this.factory(this.url, protocols) : this.factory(this.url);
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
    this.cancelIdleClose();
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

  private scheduleIdleClose(): void {
    if (this.idleCloseDelayMs <= 0) {
      this.close();
      return;
    }
    this.cancelIdleClose();
    this.idleTimer = setTimeout(() => {
      this.idleTimer = null;
      if (this.handlers.size === 0) this.close();
    }, this.idleCloseDelayMs);
  }

  private cancelIdleClose(): void {
    if (this.idleTimer) {
      clearTimeout(this.idleTimer);
      this.idleTimer = null;
    }
  }

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
    const m = msg as { type?: unknown; topic?: unknown; data?: unknown; ts?: unknown; error?: unknown; message?: unknown };
    if (m.error !== undefined) {
      this.onError?.(m.error);
      return;
    }
    // P06 relay frames: {type:'data',topic,data} | {type:'error',message} | heartbeat/pong/subscribed (ignored)
    if (m.type === 'error') {
      this.onError?.({ detail: typeof m.message === 'string' ? m.message : 'stream error' });
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
    if (list) {
      list.push(entry);
      if (list.length > this.maxBufferPerTopic) list.splice(0, list.length - this.maxBufferPerTopic);
    } else this.buffer.set(m.topic, [entry]);
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
