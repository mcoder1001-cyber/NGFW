import 'reflect-metadata';
import { describe, expect, it, vi } from 'vitest';
import { HaSyncOp } from '@ngfw/proto';
import type { AgentClient } from '../../agent/agent.client.js';
import type { NgfwRequest } from '../../common/principal.js';
import { ROLE_KEY } from '../../auth/decorators.js';
import { HaStateSyncController } from './controller.js';
import { syncState, toState } from './dto.js';
describe('HA sync REST boundary', () => {
  it('requires admin for the mutating action', () => {
    expect(Reflect.getMetadata(ROLE_KEY, HaStateSyncController.prototype.resync)).toBe('admin');
  });
  it('preserves absent counters and unsupported kinds', () => {
    const out = toState({
      owner: 'w18',
      listener: undefined,
      failover: undefined,
      lastResync: undefined,
      kinds: [
        {
          kind: 'ipsec',
          supported: false,
          configured: true,
          active: false,
          reason: 'native rekey',
        },
      ],
      resyncCount: '0',
      packetCountersAvailable: false,
      actionsAllowed: false,
      retrievedAt: new Date(0),
      observationError: 'disconnected',
    });
    expect(syncState.parse(out).lastMissedCount).toBeNull();
    expect(out.lastResync).toBeNull();
    expect(out.kinds[0]?.active).toBe(false);
  });
  it('waits for completion and records no cluster secret', async () => {
    const runAction = vi.fn().mockResolvedValue({ done: { exitCode: 0, summary: 'done' } });
    const c = new HaStateSyncController({ runAction } as unknown as AgentClient);
    const req = {} as NgfwRequest;
    expect(await c.resync(req)).toEqual({ summary: 'done' });
    expect(runAction).toHaveBeenCalledWith({ haSync: { op: HaSyncOp.HA_SYNC_OP_RESYNC } }, 20000);
    expect(req.audit).toEqual({
      resource: 'ha/state-sync/resync',
      before: { operation: 'resync' },
      after: { operation: 'resync', completed: true },
    });
  });
  it('rejects an ended stream without native completion', async () => {
    const c = new HaStateSyncController({
      runAction: vi.fn().mockResolvedValue({}),
    } as unknown as AgentClient);
    await expect(c.resync({} as NgfwRequest)).rejects.toThrow(
      'Agent stream ended without completion',
    );
  });
});
