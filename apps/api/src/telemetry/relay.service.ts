import { Inject, Injectable, Logger, type OnApplicationShutdown } from '@nestjs/common';
import type { ClientReadableStream } from '@grpc/grpc-js';
import { EventKind, type Event, type StatsBatch } from '@ngfw/proto';
import type { WebSocket } from 'ws';
import { AgentClient } from '../agent/agent.client.js';
import { SystemEventsService } from '../audit/system-events.service.js';
import { ENV, type Env } from '../config.js';
import { ROLE_RANK, type Principal } from '../common/principal.js';
import { DB, type Db } from '../db/db.js';
import { appUser } from '../db/schema.js';
import { Bus, TOPICS, type SessionsEvent, type Topic } from '../infra/bus.js';

const STATS_TOPICS: readonly Topic[] = ['iface.counters', 'worker.cpu'];
const MAX_CLIENT_MESSAGE = 4096;
const MAX_BUFFERED = 1024 * 1024;

export function eventTopic(kind: EventKind): Topic {
  switch (kind) {
    case EventKind.EVENT_KIND_LINK_UP:
    case EventKind.EVENT_KIND_LINK_DOWN:
      return 'link.events';
    case EventKind.EVENT_KIND_RECONCILE_START:
    case EventKind.EVENT_KIND_RECONCILE_DONE:
      return 'reconcile.events';
    case EventKind.EVENT_KIND_CONFIRM_REVERTED:
      return 'commit.events';
    // Feature event kinds: one `case …: return '<topic>';` under the feature's anchor (wave-A-hotspots P6).
    // wave-BC: F-vrrp-config-sync
    // wave-BC: F-ospf
    // wave-BC: F-isis-rip
    // wave-BC: F-bfd-redistribution
    // wave-BC: F-ra-vpn
    // wave-BC: F-mpls-ldp
    // wave-BC: F-igmp-mfib
    // wave-A: F-neighbors-ra
    case EventKind.EVENT_KIND_NEIGHBOR_CHANGED:
      return 'neighbor.events';
    // wave-A: F-object-model
    // wave-A: F-acl
    // wave-A: P11
    // wave-A: F-wireguard
    // wave-A: P12
    default:
      return 'agent.events';
  }
}

export function eventJson(e: Event) {
  return {
    kind: EventKind[e.kind]?.replace('EVENT_KIND_', '').toLowerCase() ?? String(e.kind),
    seq: e.seq,
    ts: e.ts?.toISOString(),
    interface: e.interface,
    message: e.message,
    txnId: e.txnId || undefined,
    summary: e.summary,
    attributes: e.attributes,
  };
}

interface Client {
  socket: WebSocket;
  principal: Principal;
  topics: Set<Topic>;
  alive: boolean;
  expiry?: NodeJS.Timeout;
}

/** Close codes (application range 4000–4999). */
export const WS_CLOSE = { expired: 4401, revoked: 4403 } as const;

/**
 * Telemetry relay (P06 §8): one upstream StreamEvents (always, with reconnect/backoff) and one StreamStats (only while
 * some client wants counters) from the agent, fanned out on `WS /api/v1/stream`. Clients send `{subscribe:[topics]}` /
 * `{unsubscribe:[topics]}`; each connection gets only its topics; heartbeats every VRX_WS_HEARTBEAT_MS (JSON message +
 * WebSocket ping; a client that misses a pong is dropped). Agent events also go to the in-process bus (the commit
 * engine listens for CONFIRM_REVERTED).
 */
@Injectable()
export class RelayService implements OnApplicationShutdown {
  private readonly log = new Logger('Relay');
  private readonly clients = new Set<Client>();
  private events: ClientReadableStream<Event> | undefined;
  private stats: ClientReadableStream<StatsBatch> | undefined;
  private eventsBackoff = 1000;
  private statsBackoff = 1000;
  private reconnectTimer: NodeJS.Timeout | undefined;
  private statsTimer: NodeJS.Timeout | undefined;
  private heartbeat: NodeJS.Timeout | undefined;
  private started = false;
  private stopped = false;
  private readonly unsubscribe: () => void;
  /** Last batch seen (GET /state/interfaces uses it while a stats stream runs). */
  lastStats: StatsBatch | undefined;

  constructor(
    private readonly agent: AgentClient,
    private readonly bus: Bus,
    private readonly sysEvents: SystemEventsService,
    @Inject(ENV) private readonly env: Env,
    @Inject(DB) private readonly db: Db,
  ) {
    const offPublish = this.bus.onPublish((m) => this.fanout(m.topic, m.data));
    const offSessions = this.bus.onSessions((e) => void this.onSessions(e));
    this.unsubscribe = () => {
      offPublish();
      offSessions();
    };
  }

  /** Start the upstream event stream and the heartbeat (main.ts / tests; not during OpenAPI generation). */
  start(): void {
    if (this.started) return;
    this.started = true;
    this.stopped = false;
    this.openEvents();
    this.heartbeat = setInterval(() => this.beat(), this.env.VRX_WS_HEARTBEAT_MS);
    this.heartbeat.unref();
  }

  onApplicationShutdown(): void {
    this.stopped = true;
    this.unsubscribe();
    for (const t of [this.reconnectTimer, this.statsTimer]) if (t) clearTimeout(t);
    if (this.heartbeat) clearInterval(this.heartbeat);
    this.events?.cancel();
    this.stats?.cancel();
    for (const c of this.clients) c.socket.close(1001, 'server shutting down');
    this.clients.clear();
  }

  get clientCount(): number {
    return this.clients.size;
  }

  private openEvents(): void {
    if (this.stopped) return;
    const call = this.agent.streamEvents({ kinds: [], interfaces: [] });
    this.events = call;
    call.on('data', (e: Event) => {
      this.eventsBackoff = 1000;
      this.bus.agentEvent(e);
      this.fanout(eventTopic(e.kind), eventJson(e));
      if (
        e.kind === EventKind.EVENT_KIND_DEGRADED ||
        e.kind === EventKind.EVENT_KIND_VPP_DISCONNECTED ||
        e.kind === EventKind.EVENT_KIND_VPP_CONNECTED
      ) {
        void this.sysEvents.record(
          e.kind === EventKind.EVENT_KIND_VPP_CONNECTED ? 'info' : 'error',
          'agent',
          EventKind[e.kind]?.replace('EVENT_KIND_', '') ?? 'EVENT',
          e.message || 'agent event',
        );
      }
    });
    const retry = () => {
      if (this.events !== call || this.stopped) return;
      this.events = undefined;
      const delay = this.eventsBackoff;
      this.eventsBackoff = Math.min(this.eventsBackoff * 2, 30_000);
      this.reconnectTimer = setTimeout(() => this.openEvents(), delay);
      this.reconnectTimer.unref();
    };
    call.on('error', retry);
    call.on('end', retry);
  }

  private wantsStats(): boolean {
    for (const c of this.clients) for (const t of STATS_TOPICS) if (c.topics.has(t)) return true;
    return false;
  }

  /** Open StreamStats while somebody listens, cancel it when nobody does. */
  private syncStats(): void {
    const want = this.wantsStats() && !this.stopped;
    if (want && this.stats === undefined && this.statsTimer === undefined) this.openStats();
    if (!want && this.stats !== undefined) {
      const s = this.stats;
      this.stats = undefined;
      s.cancel();
    }
  }

  private openStats(): void {
    const call = this.agent.streamStats({
      intervalMs: 1000,
      interfaces: [],
      includeWorkerCpu: true,
    });
    this.stats = call;
    call.on('data', (b: StatsBatch) => {
      this.statsBackoff = 1000;
      this.lastStats = b;
      const base = { seq: b.seq, ts: b.ts?.toISOString(), intervalMs: b.intervalMs };
      this.fanout('iface.counters', { ...base, interfaces: b.interfaceCounters });
      if (b.workerCpu.length > 0) this.fanout('worker.cpu', { ...base, workers: b.workerCpu });
    });
    const retry = () => {
      if (this.stats !== call) return; // cancelled on purpose
      this.stats = undefined;
      const delay = this.statsBackoff;
      this.statsBackoff = Math.min(this.statsBackoff * 2, 30_000);
      this.statsTimer = setTimeout(() => {
        this.statsTimer = undefined;
        this.syncStats();
      }, delay);
      this.statsTimer.unref();
    };
    call.on('error', retry);
    call.on('end', retry);
  }

  private send(c: Client, msg: unknown): void {
    if (c.socket.readyState !== c.socket.OPEN) return;
    // a slow client loses messages instead of growing the server's memory (counters are absolute, nothing is lost)
    if (c.socket.bufferedAmount > MAX_BUFFERED) return;
    c.socket.send(JSON.stringify(msg));
  }

  private fanout(topic: Topic, data: unknown): void {
    for (const c of this.clients)
      if (c.topics.has(topic)) this.send(c, { type: 'data', topic, data });
  }

  private beat(): void {
    const ts = new Date().toISOString();
    for (const c of this.clients) {
      if (!c.alive) {
        c.socket.terminate();
        continue;
      }
      c.alive = false;
      c.socket.ping();
      this.send(c, { type: 'heartbeat', ts });
    }
  }

  /**
   * Logout (sid), password change (userId) or a commit that changed users: close the affected connections — a
   * deleted, disabled or demoted user keeps nothing open (review L3).
   */
  private async onSessions(e: SessionsEvent): Promise<void> {
    for (const c of this.clients) {
      if (
        (e.sid && c.principal.sid === e.sid) ||
        (e.userId !== undefined &&
          c.principal.id === e.userId &&
          !(e.exceptSid !== undefined && c.principal.sid === e.exceptSid))
      ) {
        c.socket.close(WS_CLOSE.revoked, 'session ended');
      }
    }
    if (!e.usersChanged || this.clients.size === 0) return;
    try {
      const rows = await this.db
        .select({ id: appUser.id, role: appUser.role, disabled: appUser.disabled })
        .from(appUser);
      const users = new Map(rows.map((r) => [r.id, r]));
      for (const c of this.clients) {
        const u = users.get(c.principal.id);
        if (u === undefined || u.disabled || ROLE_RANK[u.role] < ROLE_RANK[c.principal.role]) {
          c.socket.close(WS_CLOSE.revoked, 'user changed');
        }
      }
    } catch (err) {
      this.log.warn(`could not re-check WebSocket users: ${(err as Error).message}`);
    }
  }

  /** A new authenticated WebSocket connection. */
  attach(socket: WebSocket, principal: Principal): void {
    const client: Client = { socket, principal, topics: new Set(), alive: true };
    this.clients.add(client);
    if (principal.exp !== undefined) {
      // review L3: the connection lives no longer than the credential that opened it
      const ms = Math.max(0, principal.exp * 1000 - Date.now());
      client.expiry = setTimeout(
        () => socket.close(WS_CLOSE.expired, 'credential expired'),
        Math.min(ms, 2 ** 31 - 1),
      );
      client.expiry.unref();
    }
    socket.on('pong', () => (client.alive = true));
    socket.on('message', (raw, isBinary) => {
      client.alive = true;
      this.onMessage(client, raw as Buffer, isBinary);
    });
    socket.on('close', () => {
      if (client.expiry) clearTimeout(client.expiry);
      this.clients.delete(client);
      this.syncStats();
    });
    this.send(client, {
      type: 'welcome',
      user: principal.username,
      topics: TOPICS,
      heartbeatMs: this.env.VRX_WS_HEARTBEAT_MS,
    });
  }

  private onMessage(c: Client, raw: Buffer, isBinary: boolean): void {
    if (isBinary || raw.length > MAX_CLIENT_MESSAGE) {
      this.send(c, { type: 'error', message: 'expected a small JSON text message' });
      return;
    }
    let msg: unknown;
    try {
      msg = JSON.parse(raw.toString('utf8'));
    } catch {
      this.send(c, { type: 'error', message: 'invalid JSON' });
      return;
    }
    const m = msg as { subscribe?: unknown; unsubscribe?: unknown; type?: unknown };
    if (m.type === 'ping') {
      this.send(c, { type: 'pong', ts: new Date().toISOString() });
      return;
    }
    const lists: [unknown, boolean][] = [
      [m.subscribe, true],
      [m.unsubscribe, false],
    ];
    let handled = false;
    for (const [list, add] of lists) {
      if (list === undefined) continue;
      handled = true;
      if (!Array.isArray(list) || list.some((t) => !(TOPICS as readonly unknown[]).includes(t))) {
        this.send(c, { type: 'error', message: `unknown topic; known: ${TOPICS.join(', ')}` });
        return;
      }
      for (const t of list as Topic[]) {
        if (add) c.topics.add(t);
        else c.topics.delete(t);
      }
    }
    if (!handled) {
      this.send(c, {
        type: 'error',
        message: 'expected {subscribe:[topics]} or {unsubscribe:[topics]}',
      });
      return;
    }
    this.send(c, { type: 'subscribed', topics: [...c.topics] });
    this.syncStats();
  }
}
