import { Injectable } from '@nestjs/common';
import type { Event } from '@ngfw/proto';
import { EventEmitter } from 'node:events';

/** Topics of the telemetry relay (`WS /api/v1/stream`, `{subscribe: [topics]}`). */
export const TOPICS = [
  'iface.counters',
  'worker.cpu',
  'link.events',
  'reconcile.events',
  'commit.events',
  'agent.events',
  // Feature topics: one line under the feature's anchor (wave-A-hotspots P6).
  // wave-A: F-neighbors-ra
  // wave-A: F-object-model
  // wave-A: F-acl
  // wave-A: P11
  // wave-A: F-wireguard
  // wave-A: P12
] as const;
export type Topic = (typeof TOPICS)[number];

export interface BusMessage {
  topic: Topic;
  data: unknown;
}

/**
 * In-process event bus between the telemetry relay (agent streams) and the commit engine (confirm reverts), so
 * neither depends on the other. `publish` fans out to WebSocket clients; `agentEvent` carries raw agent events.
 */
@Injectable()
export class Bus {
  private readonly ee = new EventEmitter();

  constructor() {
    this.ee.setMaxListeners(1000);
  }

  publish(topic: Topic, data: unknown): void {
    this.ee.emit('publish', { topic, data } satisfies BusMessage);
  }
  onPublish(fn: (m: BusMessage) => void): () => void {
    this.ee.on('publish', fn);
    return () => this.ee.off('publish', fn);
  }

  /** Sessions ended: a login session (sid), all sessions of a user (userId), or users changed by a commit. */
  sessions(e: { sid?: string; userId?: number; usersChanged?: boolean }): void {
    this.ee.emit('sessions', e);
  }
  onSessions(
    fn: (e: { sid?: string; userId?: number; usersChanged?: boolean }) => void,
  ): () => void {
    this.ee.on('sessions', fn);
    return () => this.ee.off('sessions', fn);
  }

  agentEvent(e: Event): void {
    this.ee.emit('agent-event', e);
  }
  onAgentEvent(fn: (e: Event) => void): () => void {
    this.ee.on('agent-event', fn);
    return () => this.ee.off('agent-event', fn);
  }
}
