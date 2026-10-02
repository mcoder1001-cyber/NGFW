import { describe, expect, it, vi } from 'vitest';
import { StateController } from './state.controller.js';
import type { AgentClient } from '../agent/agent.client.js';
import type { RelayService } from '../telemetry/relay.service.js';
import type { CommitService } from '../commit/commit.service.js';
import type { DatastoreService } from '../datastore/datastore.service.js';
import type { SystemEventsService } from '../audit/system-events.service.js';
import type { ValidationService } from '../commit/validation.service.js';

describe('system health and operational identity compatibility', () => {
  function fixture() {
    const agent = {
      health: async () => ({ version: 'test' }),
      systemIdentityState: vi
        .fn()
        .mockResolvedValue({
          hostname: 'actual',
          timezone: 'UTC',
          uptimeSeconds: 12,
          resolverStatus: 'unavailable',
          configuredNameServers: ['192.0.2.53'],
          configuredSearchDomains: [],
          observedNameServers: [],
          errors: ['resolver-runtime'],
          retrievedAt: new Date('2026-10-02T00:00:00Z'),
        }),
    };
    const controller = new StateController(
      agent as unknown as AgentClient,
      { clientCount: 2 } as RelayService,
      {
        pendingInfo: async () => null,
        syncStatus: async () => ({ state: 'in-sync' }),
      } as unknown as CommitService,
      { getRunning: async () => ({ revision: { id: 7 } }) } as unknown as DatastoreService,
      {} as SystemEventsService,
      {} as ValidationService,
    );
    return { agent, controller };
  }
  it('adds observed identity without replacing existing health fields', async () => {
    const { controller } = fixture();
    expect(await controller.system()).toMatchObject({
      api: { wsClients: 2 },
      agent: { reachable: true, version: 'test' },
      runningRevision: 7,
      pendingCommit: null,
      sync: { state: 'in-sync' },
      identity: {
        hostname: 'actual',
        configuredNameServers: ['192.0.2.53'],
        observedNameServers: [],
        resolverStatus: 'unavailable',
        retrievedAt: '2026-10-02T00:00:00.000Z',
      },
    });
  });
  it('preserves health when an older or unavailable agent has no identity RPC', async () => {
    const { agent, controller } = fixture();
    agent.systemIdentityState.mockRejectedValueOnce(new Error('UNIMPLEMENTED'));
    expect(await controller.system()).toMatchObject({
      agent: { reachable: true },
      identity: null,
      runningRevision: 7,
    });
  });
});
