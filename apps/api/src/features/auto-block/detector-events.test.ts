import { describe, expect, it, vi } from 'vitest';
import { EventKind, Event } from '@ngfw/proto';
import type { AgentClient } from '../../agent/agent.client.js';
import type { AuditService } from '../../audit/audit.service.js';
import type { SystemEventsService } from '../../audit/system-events.service.js';
import type { DatastoreService } from '../../datastore/datastore.service.js';
import type { Db } from '../../db/db.js';
import { Bus } from '../../infra/bus.js';
import { AutoBlockService } from './auto-block.service.js';
import { compileAllowlist } from './engine.js';

function fixture() {
  const bus = new Bus();
  const svc = new AutoBlockService(
    {} as DatastoreService,
    { onEntry: () => () => {} } as unknown as AuditService,
    {} as SystemEventsService,
    bus,
    {} as Db,
    {} as AgentClient,
  );
  Reflect.set(svc, 'cfg', {
    enabled: true,
    maxEntries: 100,
    rules: ['ssh', 'vpnAuth', 'portScan'].map((source) => ({
      source,
      enabled: true,
      threshold: 2,
      windowSec: 10,
      blockSec: 60,
      maxBlockSec: 300,
      escalate: false,
    })),
  });
  Reflect.set(svc, 'allow', compileAllowlist(['198.51.100.0/24']));
  const block = vi.fn(async () => {});
  Reflect.set(svc, 'block', block);
  const emit = (detector: string, source = '192.0.2.7', port?: string) =>
    bus.agentEvent(
      Event.fromPartial({
        kind: EventKind.EVENT_KIND_AUTOBLOCK_OBSERVED,
        attributes: {
          detector,
          source_ip: source,
          ...(port === undefined ? {} : { destination_port: port }),
        },
      }),
    );
  return { svc, block, emit };
}

describe('host detector event ingestion', () => {
  it('refreshes a repeated port and blocks exactly when distinct ports reach the threshold', () => {
    const { svc, block, emit } = fixture();
    let now = 0;
    svc.now = () => now;
    emit('portScan', undefined, '22');
    now = 9000;
    emit('portScan', undefined, '22');
    expect(block).not.toHaveBeenCalled();
    now = 11000;
    emit('portScan', undefined, '443');
    expect(block).toHaveBeenCalledTimes(1);
    expect(block.mock.calls[0]).toEqual(expect.arrayContaining(['192.0.2.7/32', 'portScan', 2]));
    svc.onModuleDestroy();
  });
  it('ignores unknown, port-less and malformed observations and unsubscribes on shutdown', () => {
    const { svc, block, emit } = fixture();
    for (const port of [undefined, '0', '65536', '22x', '-1', '00022']) {
      emit('portScan', undefined, port);
      emit('portScan', undefined, port);
    }
    emit('unknown');
    emit('unknown');
    expect(block).not.toHaveBeenCalled();
    svc.onModuleDestroy();
    emit('ssh');
    emit('ssh');
    expect(block).not.toHaveBeenCalled();
  });
  it('protects allowlisted and mapped management sources for every host detector', () => {
    const { svc, block, emit } = fixture();
    for (const source of ['198.51.100.7', '::ffff:198.51.100.7', '127.0.0.1']) {
      for (const kind of ['ssh', 'vpnAuth', 'portScan']) {
        emit(kind, source, '22');
        emit(kind, source, '443');
      }
    }
    expect(block).not.toHaveBeenCalled();
    emit('ssh');
    emit('ssh');
    expect(block).toHaveBeenCalledTimes(1);
    svc.onModuleDestroy();
  });
});
