import 'reflect-metadata';
import { Reflector } from '@nestjs/core';
import { FastifyAdapter, type NestFastifyApplication } from '@nestjs/platform-fastify';
import { Test } from '@nestjs/testing';
import { RoutingStateResponse } from '@ngfw/proto';
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { AgentClient } from '../../agent/agent.client.js';
import { AuditService } from '../../audit/audit.service.js';
import { AuthGuard } from '../../auth/auth.guard.js';
import { AuthService } from '../../auth/auth.service.js';
import { ProblemError, ProblemFilter } from '../../common/problem.js';
import { OspfController } from './ospf.controller.js';
const agent = { routingState: vi.fn() };
const response = () =>
  RoutingStateResponse.fromPartial({ frrRunning: true, readers: { ospfNeighbors: '{}' } });

describe('OSPF read-only controller contract', () => {
  beforeEach(() => agent.routingState.mockReset());
  it('uses a fixed registered reader and never asks for a RIB lookup', async () => {
    agent.routingState.mockResolvedValue(response());
    expect(await new OspfController(agent as unknown as AgentClient).state()).toMatchObject({
      neighbors: [],
      unavailable: null,
    });
    expect(agent.routingState).toHaveBeenCalledExactlyOnceWith({
      readers: ['ospfNeighbors'],
      ribPrefixes: [],
      ribVrf: '',
    });
  });
  it('preserves mapped agent unavailable errors instead of fabricating healthy empty observations', async () => {
    const error = new ProblemError(503, 'unavailable', 'Service unavailable', 'agent unreachable');
    const unavailable = {
      routingState: async () => {
        throw error;
      },
    };
    const observed = await new OspfController(unavailable as unknown as AgentClient)
      .state()
      .catch((reason: unknown) => reason);
    expect(observed).toBeInstanceOf(ProblemError);
    expect((observed as ProblemError).getStatus()).toBe(503);
    expect((observed as ProblemError).body()).toEqual({
      type: 'https://vrx.dev/problems/unavailable',
      title: 'Service unavailable',
      status: 503,
      detail: 'agent unreachable',
    });
  });
});

describe('OSPF GET registration and existing global authentication guard', () => {
  let app: NestFastifyApplication;
  let rpcFailure: ProblemError | null = null;
  beforeAll(async () => {
    const module = await Test.createTestingModule({
      controllers: [OspfController],
      providers: [
        {
          provide: AgentClient,
          useValue: {
            routingState: async (request: Parameters<AgentClient['routingState']>[0]) => {
              if (rpcFailure) throw rpcFailure;
              return agent.routingState(request);
            },
          },
        },
      ],
    }).compile();
    app = module.createNestApplication<NestFastifyApplication>(new FastifyAdapter(), {
      logger: false,
    });
    const auth = {
      authenticate: vi.fn(async (authorization: string | undefined) =>
        authorization === 'Bearer readonly-fixture'
          ? { id: 1, username: 'reader', role: 'readonly', via: 'jwt' }
          : null,
      ),
    };
    app.useGlobalFilters(new ProblemFilter());
    app.useGlobalGuards(
      new AuthGuard(new Reflector(), auth as unknown as AuthService, {} as AuditService),
    );
    await app.init();
    await app.getHttpAdapter().getInstance().ready();
  });
  afterAll(async () => {
    await app.close();
  });
  beforeEach(() => {
    agent.routingState.mockReset();
    rpcFailure = null;
  });
  it('rejects unauthenticated reads before calling the agent', async () => {
    const result = await app.inject({ method: 'GET', url: '/api/v1/state/ospf' });
    expect(result.statusCode).toBe(401);
    expect(agent.routingState).not.toHaveBeenCalled();
  });
  it('permits readonly observation and ignores caller-supplied reader/RIB selectors', async () => {
    agent.routingState.mockResolvedValue(response());
    const result = await app.inject({
      method: 'GET',
      url: '/api/v1/state/ospf?readers=untrusted&ribPrefixes=0.0.0.0/0',
      headers: { authorization: 'Bearer readonly-fixture' },
    });
    expect(result.statusCode).toBe(200);
    expect(result.json()).toMatchObject({ neighbors: [], unavailable: null });
    expect(agent.routingState).toHaveBeenCalledExactlyOnceWith({
      readers: ['ospfNeighbors'],
      ribPrefixes: [],
      ribVrf: '',
    });
  });
  it('returns mapped RPC unavailability through the actual problem filter as HTTP503', async () => {
    rpcFailure = new ProblemError(503, 'unavailable', 'Service unavailable', 'agent unreachable');
    const result = await app.inject({
      method: 'GET',
      url: '/api/v1/state/ospf',
      headers: { authorization: 'Bearer readonly-fixture' },
    });
    expect(result.statusCode).toBe(503);
    expect(result.headers['content-type']).toContain('application/problem+json');
    expect(result.json()).toEqual({
      type: 'https://vrx.dev/problems/unavailable',
      title: 'Service unavailable',
      status: 503,
      detail: 'agent unreachable',
      instance: '/api/v1/state/ospf',
    });
    expect(agent.routingState).not.toHaveBeenCalled();
  });
});
