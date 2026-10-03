import { type ApplyRequest, DesiredState, type DryRunRequest } from '@ngfw/proto';
import { randomBytes } from 'node:crypto';
import { mkdtempSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import type { SecretDeliveryService } from '../secrets/secret-delivery.service.js';
import { FakeAgent } from '../testing/fake-agent.js';
import { testEnv } from '../testing/fixtures.js';
import { AgentClient } from './agent.client.js';

describe('socket-only secret bundle', () => {
  const dir = mkdtempSync(join(tmpdir(), 'ngfw-secret-socket-test-'));
  const socket = join(dir, 'agent.sock');
  const fake = new FakeAgent({ owner: 'w1' });
  const bundle = { values: { 'psk/native': randomBytes(32) } };
  const resolve = vi.fn(async () => bundle);
  const agent = new AgentClient(testEnv({ NGFW_AGENT_SOCKET: socket, NGFW_AGENT_OWNER: 'w1' }), {
    resolve,
  } as unknown as SecretDeliveryService);
  beforeAll(async () => fake.start(socket));
  afterAll(async () => {
    agent.close();
    await fake.stop();
    rmSync(dir, { recursive: true, force: true });
  });

  it('attaches material to DryRun and reuses caller-pinned material for Apply', async () => {
    const ds = DesiredState.fromPartial({});
    await agent.dryRun({ txnId: 'dry', desiredState: ds, subsystems: [] });
    const pinned = { values: { 'psk/native': randomBytes(32) } };
    await agent.apply({
      txnId: 'apply',
      desiredState: ds,
      subsystems: [],
      confirmTimeoutSec: 0,
      confirmTxnId: '',
      secretBundle: pinned,
    });
    expect(resolve).toHaveBeenCalledTimes(1);
    const dry = fake.calls.find((c) => c.method === 'DryRun')?.request as DryRunRequest;
    const apply = fake.calls.find((c) => c.method === 'Apply')?.request as ApplyRequest;
    expect(dry.secretBundle?.values).toEqual(bundle.values);
    expect(apply.secretBundle?.values).toEqual(pinned.values);
  });

  it('never resolves secrets for a confirmation-only request', async () => {
    resolve.mockClear();
    await agent
      .apply({
        txnId: '',
        desiredState: undefined,
        subsystems: [],
        confirmTimeoutSec: 0,
        confirmTxnId: 'absent',
      })
      .catch(() => undefined);
    expect(resolve).not.toHaveBeenCalled();
  });
});
