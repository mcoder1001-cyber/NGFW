import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Server, ServerCredentials, status, type handleServerStreamingCall } from '@grpc/grpc-js';
import { ActionRequest, type ActionOutput, DataplaneService, HaSyncOp } from '@ngfw/proto';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { testEnv } from '../testing/fixtures.js';
import { AgentClient } from './agent.client.js';
import { ProblemError } from '../common/problem.js';
import { HaStateSyncController } from '../features/ha-state-sync/controller.js';
import type { NgfwRequest } from '../common/principal.js';

describe('AgentClient action error transport', () => {
  const dir = mkdtempSync(join(tmpdir(), 'ha-rpc-'));
  const socket = join(dir, 'agent.sock');
  const server = new Server();
  const agent = new AgentClient(testEnv({ NGFW_AGENT_SOCKET: socket }));
  let errorCode: number | undefined;
  const requests: ActionRequest[] = [];
  const action: handleServerStreamingCall<ActionRequest, ActionOutput> = (call) => {
    requests.push(call.request);
    if (errorCode !== undefined) {
      // A partial stream followed by an error must never resolve as successful output.
      call.write({ line: 'partial', pcapChunk: undefined, done: undefined });
      call.emit(
        'error',
        Object.assign(new Error('private agent details'), {
          code: errorCode,
          details: 'private agent details',
        }),
      );
      return;
    }
    call.end();
  };
  beforeAll(async () => {
    server.addService(DataplaneService, { action });
    await new Promise<void>((resolve, reject) => {
      server.bindAsync(`unix:${socket}`, ServerCredentials.createInsecure(), (err) => {
        if (err) reject(err);
        else resolve();
      });
    });
  });
  afterAll(async () => {
    agent.close();
    await new Promise<void>((resolve) => server.tryShutdown(() => resolve()));
    rmSync(dir, { recursive: true, force: true });
  });

  it.each([
    [status.PERMISSION_DENIED, 403, 'agent-permission-denied', 'PERMISSION_DENIED'],
    [status.FAILED_PRECONDITION, 409, 'agent-precondition', 'FAILED_PRECONDITION'],
    [status.ABORTED, 409, 'agent-aborted', 'ABORTED'],
    [status.UNAVAILABLE, 503, 'agent-unavailable', 'UNAVAILABLE'],
    [status.INTERNAL, 502, 'agent-error', 'INTERNAL'],
  ])(
    'preserves gRPC %s as HTTP %s without accepting partial output',
    async (code, http, slug, grpcCode) => {
      errorCode = code as number;
      const request = ActionRequest.fromPartial({ haSync: { op: HaSyncOp.HA_SYNC_OP_RESYNC } });
      const result = await agent.runAction(request).catch((err: unknown) => err);
      expect(result).toBeInstanceOf(ProblemError);
      const body = (result as ProblemError).body('/api/v1/actions/ha/sync/resync');
      expect(body).toMatchObject({
        status: http,
        type: `https://ngfw.dev/problems/${slug}`,
        grpcCode,
      });
      expect(result).not.toHaveProperty('done');
      expect(requests.at(-1)).toEqual(request);
      if (code === status.PERMISSION_DENIED) {
        expect(JSON.stringify(body)).not.toContain('private agent details');
      }
    },
  );

  it('keeps HA failure audit incomplete when the agent refuses permission', async () => {
    errorCode = status.PERMISSION_DENIED;
    const req = {} as NgfwRequest;
    const controller = new HaStateSyncController(agent);
    await expect(controller.resync(req)).rejects.toMatchObject({ slug: 'agent-permission-denied' });
    expect(req.audit).toEqual({
      resource: 'ha/state-sync/resync',
      before: { operation: 'resync' },
    });
  });

  it('does not invent completion for an ended stream without done', async () => {
    errorCode = undefined;
    const result = await agent.runAction(
      ActionRequest.fromPartial({ haSync: { op: HaSyncOp.HA_SYNC_OP_RESYNC } }),
    );
    expect(result).toEqual({ lines: [], pcapBytes: 0, done: undefined });
  });
});
