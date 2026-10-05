import 'reflect-metadata';
import { Server, ServerCredentials, status } from '@grpc/grpc-js';
import { Reflector } from '@nestjs/core';
import { Test } from '@nestjs/testing';
import { FastifyAdapter, type NestFastifyApplication } from '@nestjs/platform-fastify';
import { DataplaneService, type DataplaneServer } from '@ngfw/proto';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { AgentClient } from '../../agent/agent.client.js';
import { AuditInterceptor } from '../../audit/audit.interceptor.js';
import type { AuditService } from '../../audit/audit.service.js';
import { AuthGuard } from '../../auth/auth.guard.js';
import type { AuthService } from '../../auth/auth.service.js';
import { ProblemFilter } from '../../common/problem.js';
import { testEnv } from '../../testing/fixtures.js';
import { RaVpnController } from './ra-vpn.controller.js';

const ID = 'a'.repeat(64);
const MAX = '18446744073709551615';
describe('remote-access actual Unix gRPC → AgentClient → HTTP contract', () => {
  const dir = mkdtempSync(join(tmpdir(), 'ra-'));
  const socket = join(dir, 'agent.sock');
  const grpc = new Server();
  const audit = {
    write: vi.fn(async () => undefined),
    writeAggregated: vi.fn(async () => undefined),
  };
  const agent = new AgentClient(testEnv({ NGFW_AGENT_SOCKET: socket, NGFW_AGENT_OWNER: 'w1' }));
  const calls: unknown[] = [];
  let failure: number | null = null;
  let app: NestFastifyApplication;
  const handlers: Pick<
    DataplaneServer,
    'remoteAccessCapabilities' | 'remoteAccessSessions' | 'remoteAccessDisconnect'
  > = {
    remoteAccessCapabilities: (_call, callback) =>
      callback(null, {
        engine: 'strongswan-ra',
        operational: false,
        supportedAuth: ['eap-tls'],
        reason: 'engine-not-ready',
        editableDisabledDrafts: true,
      }),
    remoteAccessSessions: (call, callback) => {
      calls.push(call.request);
      callback(null, {
        sessions: [
          {
            id: ID,
            profile: call.request.profile,
            identity: 'user@example.test',
            addresses: ['10.44.0.2'],
            establishedSeconds: MAX,
            bytesIn: MAX,
            bytesOut: MAX,
          },
        ],
        nextCursor: '',
      });
    },
    remoteAccessDisconnect: (call, callback) => {
      calls.push(call.request);
      if (failure !== null) {
        callback({ code: failure, details: 'sensitive-daemon-detail' });
        return;
      }
      if (call.request.id !== ID || call.request.profile !== 'office') {
        callback({ code: status.FAILED_PRECONDITION, details: 'session is stale or foreign' });
        return;
      }
      callback(null, { disconnected: true });
    },
  };
  beforeAll(async () => {
    grpc.addService(DataplaneService, handlers);
    await new Promise<void>((resolve, reject) =>
      grpc.bindAsync(`unix:${socket}`, ServerCredentials.createInsecure(), (error) =>
        error ? reject(error) : resolve(),
      ),
    );
    const module = await Test.createTestingModule({
      controllers: [RaVpnController],
      providers: [{ provide: AgentClient, useValue: agent }],
    }).compile();
    app = module.createNestApplication<NestFastifyApplication>(new FastifyAdapter(), {
      logger: false,
    });
    const auth = {
      authenticate: async (authorization?: string) => {
        const role = authorization?.replace('Bearer ', '');
        return role === 'admin' || role === 'operator' || role === 'readonly'
          ? { id: 1, username: role, role, via: 'jwt', sid: 'fixture' }
          : null;
      },
    };
    app.useGlobalGuards(
      new AuthGuard(
        new Reflector(),
        auth as unknown as AuthService,
        audit as unknown as AuditService,
      ),
    );
    app.useGlobalInterceptors(
      new AuditInterceptor(new Reflector(), audit as unknown as AuditService),
    );
    app.useGlobalFilters(new ProblemFilter());
    await app.init();
    await app.getHttpAdapter().getInstance().ready();
  });
  afterAll(async () => {
    await app.close();
    agent.close();
    grpc.forceShutdown();
    rmSync(dir, { recursive: true, force: true });
  });
  beforeEach(() => {
    calls.length = 0;
    failure = null;
    audit.write.mockClear();
    audit.writeAggregated.mockClear();
  });
  const action = (id = ID, profile = 'office', role = 'admin') =>
    app.inject({
      method: 'POST',
      url: `/api/v1/actions/vpn/remote-access/sessions/${id}/disconnect?profile=${profile}`,
      headers: { authorization: `Bearer ${role}` },
    });
  it('reports nonoperational observed capability without implying sessions are active', async () => {
    const response = await app.inject({
      method: 'GET',
      url: '/api/v1/state/vpn/remote-access/capabilities',
      headers: { authorization: 'Bearer readonly' },
    });
    expect(response.statusCode).toBe(200);
    expect(response.json()).toMatchObject({
      operational: false,
      engine: 'strongswan-ra',
      reason: 'engine-not-ready',
    });
  });
  it('preserves maximum uint64 decimal values through actual gRPC and HTTP', async () => {
    const response = await app.inject({
      method: 'GET',
      url: '/api/v1/state/vpn/remote-access/sessions?profile=office&limit=100',
      headers: { authorization: 'Bearer readonly' },
    });
    expect(response.statusCode).toBe(200);
    expect(response.json().items[0]).toMatchObject({
      establishedSeconds: MAX,
      bytesIn: MAX,
      bytesOut: MAX,
    });
    expect(calls).toEqual([{ profile: 'office', cursor: '', limit: 100 }]);
  });
  it('rejects invalid page limits/cursors and unknown query keys before transport', async () => {
    for (const query of [
      'profile=office&limit=101',
      'profile=office&limit=0',
      'profile=office&cursor=%2Fetc',
      'profile=office&owner=foreign',
      'limit=1',
    ]) {
      const response = await app.inject({
        method: 'GET',
        url: `/api/v1/state/vpn/remote-access/sessions?${query}`,
        headers: { authorization: 'Bearer readonly' },
      });
      expect(response.statusCode).toBe(400);
    }
    expect(calls).toHaveLength(0);
  });
  it('rejects unauthenticated observation before transport', async () => {
    expect(
      (
        await app.inject({
          method: 'GET',
          url: '/api/v1/state/vpn/remote-access/sessions?profile=office',
        })
      ).statusCode,
    ).toBe(401);
    expect(calls).toHaveLength(0);
  });
  it('admin disconnect is verified and audited with safe profile-scoped resource', async () => {
    const response = await action();
    expect(response.statusCode).toBe(200);
    expect(response.json()).toEqual({ disconnected: true });
    expect(audit.write).toHaveBeenCalledWith(
      expect.objectContaining({
        result: 'success',
        status: 200,
        resource: `vpn/remoteAccess/office/sessions/${ID}`,
        after: { action: 'disconnect' },
      }),
    );
  });
  it('operator and readonly disconnects are denied and failure-audited before agent calls', async () => {
    for (const role of ['operator', 'readonly'])
      expect((await action(ID, 'office', role)).statusCode).toBe(403);
    expect(calls).toHaveLength(0);
    expect(audit.write).toHaveBeenCalledTimes(2);
    expect(audit.write).toHaveBeenCalledWith(
      expect.objectContaining({ result: 'failure', status: 403 }),
    );
  });
  it('malformed identifiers never reach transport and failures are audited', async () => {
    expect((await action('not-an-id')).statusCode).toBe(400);
    expect(calls).toHaveLength(0);
    expect(audit.write).toHaveBeenCalledWith(
      expect.objectContaining({ result: 'failure', status: 400 }),
    );
  });
  it('foreign profile and stale session refusal remain conflict and failure-audited', async () => {
    expect((await action(ID, 'foreign')).statusCode).toBe(409);
    expect((await action('b'.repeat(64))).statusCode).toBe(409);
    expect(audit.write).toHaveBeenCalledTimes(2);
    expect(audit.write).toHaveBeenCalledWith(
      expect.objectContaining({ result: 'failure', status: 409 }),
    );
  });
  it('agent permission denial is HTTP403 with withheld remote detail and failure audit', async () => {
    failure = status.PERMISSION_DENIED;
    const response = await action();
    expect(response.statusCode).toBe(403);
    expect(response.body).not.toContain('sensitive-daemon-detail');
    expect(audit.write).toHaveBeenCalledWith(
      expect.objectContaining({ result: 'failure', status: 403 }),
    );
  });
  it('agent transport unavailability is not a successful disconnect', async () => {
    failure = status.UNAVAILABLE;
    const response = await action();
    expect(response.statusCode).toBe(503);
    expect(audit.write).toHaveBeenCalledWith(
      expect.objectContaining({ result: 'failure', status: 503 }),
    );
  });
});
