import { status, type ServiceError } from '@grpc/grpc-js';
import type { NestFastifyApplication } from '@nestjs/platform-fastify';
import type { MplsStateRequest, MplsStateResponse } from '@ngfw/proto';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { AgentClient } from '../../agent/agent.client.js';
import { buildOpenApi, createApp } from '../../app.js';
import { ProblemError } from '../../common/problem.js';
import { loadEnv } from '../../config.js';
import { FakeAgent } from '../../testing/fake-agent.js';
import { mplsSrmplsFake } from './fake.js';
import {
  MplsFibOut,
  MplsFibQuery,
  MplsSrmplsController,
  MplsTunnelsOut,
} from './mpls-srmpls.controller.js';

const MPLS = {
  interfaces: ['loop5001'],
  tables: { '5001': {} },
  labelRoutes: [
    {
      table: 5001,
      label: 50016,
      eos: true,
      paths: [{ nextHop: '10.5.1.2', interface: 'loop5001', outLabels: [50017], weight: 1 }],
    },
    {
      table: 0,
      label: 50020,
      eos: false,
      paths: [{ interface: 't1', outLabels: [50021], weight: 2 }],
    },
    { table: 0, label: 50030, eos: true, payload: 'ip6', paths: [{ vrf: 'red', weight: 1 }] },
  ],
  ipBindings: [{ label: 50040, vrf: 'red', prefix: '10.5.40.0/24' }],
  tunnels: {
    t1: {
      paths: [{ nextHop: '10.5.1.2', interface: 'loop5001', outLabels: [50050], weight: 1 }],
      l2Only: false,
    },
  },
  sr: { policies: { '50100': { segmentLists: [{ labels: [50101], weight: 1 }], spray: false } } },
};

/** Calls the fake's MplsState like grpc-js would. */
function state(agent: FakeAgent, req: Partial<MplsStateRequest>) {
  const { mplsState } = mplsSrmplsFake(agent);
  const request: MplsStateRequest = {
    owner: '',
    view: '',
    tableId: 0,
    label: 0,
    offset: 0,
    limit: 0,
    ...req,
  };
  return new Promise<{ err: Partial<ServiceError> | null; res?: MplsStateResponse | undefined }>((resolve) => {
    mplsState(
      { request } as never,
      ((err: Partial<ServiceError> | null, res?: MplsStateResponse) =>
        resolve({ err, res })) as never,
    );
  });
}

describe('F-mpls-srmpls fake agent: MplsState', () => {
  const agent = new FakeAgent({ owner: 'w5' });
  agent.current = { vrfs: { red: { id: 5010 } }, routing: { mpls: MPLS } };

  it('lists the readable tables and one table’s FIB with VPP’s reserved entries, paged', async () => {
    const { err, res } = await state(agent, { view: 'fib', tableId: 0 });
    expect(err).toBeNull();
    expect(res?.tables.map((t) => [t.tableId, t.name])).toEqual([
      [0, 'vrx:0'],
      [5001, 'w5:5001'],
    ]);
    expect(res?.entries.map((e) => `${e.label}/${e.eos ? 'eos' : 'neos'}`)).toEqual([
      '0/eos',
      '1/neos',
      '2/eos',
      '50020/neos',
      '50030/eos',
      '50040/eos',
      '50100/eos',
    ]);
    const lookup = res?.entries.find((e) => e.label === 50030);
    expect(lookup?.payload).toBe('ip6');
    expect(lookup?.paths[0]).toMatchObject({ proto: 'ip6', tableId: 5010 });
    const page = await state(agent, { view: 'fib', tableId: 0, offset: 3, limit: 2 });
    expect(page.res?.entries.map((e) => e.label)).toEqual([50020, 50030]);
    expect(page.res?.total).toBe(7);
    const one = await state(agent, { view: 'fib', tableId: 5001, label: 50016 });
    expect(one.res?.entries).toHaveLength(1);
    expect(one.res?.entries[0]?.paths[0]).toMatchObject({
      nextHop: '10.5.1.2',
      outLabels: [50017],
    });
  });

  it('refuses what the agent refuses', async () => {
    expect((await state(agent, { view: 'fib', tableId: 3001 })).err?.code).toBe(status.NOT_FOUND);
    expect((await state(agent, { view: 'nope' })).err?.code).toBe(status.INVALID_ARGUMENT);
    expect((await state(agent, { view: 'fib', limit: 1001 })).err?.code).toBe(
      status.INVALID_ARGUMENT,
    );
    expect((await state(agent, { view: 'fib', owner: 'w3' })).err?.code).toBe(
      status.INVALID_ARGUMENT,
    );
  });

  it('lists the configured tunnels as owned mpls-tunnel interfaces', async () => {
    const { res } = await state(agent, { view: 'tunnels' });
    expect(res?.tunnels).toEqual([
      expect.objectContaining({
        name: 't1',
        interface: 'mpls-tunnel0',
        owned: true,
        l2Only: false,
      }),
    ]);
    expect(res?.tunnels[0]?.paths[0]?.outLabels).toEqual([50050]);
  });
});

describe('MplsSrmplsController', () => {
  const calls: Parameters<AgentClient['mplsState']>[0][] = [];
  let reply: (req: Parameters<AgentClient['mplsState']>[0]) => MplsStateResponse = () => {
    throw new Error('unset');
  };
  const agent = {
    mplsState: async (req: Parameters<AgentClient['mplsState']>[0]) => {
      calls.push(req);
      return reply(req);
    },
  } as unknown as AgentClient;
  const c = new MplsSrmplsController(agent);
  const base: MplsStateResponse = {
    owner: 'w5',
    view: 'fib',
    tableId: 0,
    total: 0,
    entries: [],
    tunnels: [],
    tables: [{ tableId: 0, name: 'vrx:0' }],
    retrievedAt: new Date('2026-09-25T10:00:00Z'),
  };

  it('asks the agent for exactly one page and maps it to the documented shape', async () => {
    reply = () => ({
      ...base,
      total: 250,
      entries: [
        {
          label: 50016,
          eos: true,
          payload: 'ip4',
          paths: [
            {
              type: 'normal',
              proto: 'ip4',
              nextHop: '10.5.1.2',
              interface: 'loop5001',
              tableId: 0,
              outLabels: [50017],
              weight: 1,
              preference: 0,
            },
          ],
        },
      ],
    });
    const q = MplsFibQuery.parse({ table: '0', label: '50016', page: '3', pageSize: '50' });
    const out = await c.fib(q);
    expect(calls.at(-1)).toEqual({ view: 'fib', tableId: 0, label: 50016, offset: 100, limit: 50 });
    expect(MplsFibOut.strict().safeParse(out).success).toBe(true);
    expect(out).toMatchObject({
      page: 3,
      pageSize: 50,
      total: 250,
      retrievedAt: '2026-09-25T10:00:00.000Z',
    });
    expect(out.items[0]?.paths[0]).toEqual({
      type: 'normal',
      proto: 'ip4',
      nextHop: '10.5.1.2',
      interface: 'loop5001',
      tableId: 0,
      outLabels: [50017],
      weight: 1,
      preference: 0,
    });
  });

  it('maps agent NOT_FOUND → 404 and INVALID_ARGUMENT → 400; the query window is bounded before the agent', async () => {
    const fail = (grpcCode: string) => () => {
      throw new ProblemError(502, 'agent-error', 'Agent error', `agent: ${grpcCode}`, undefined, {
        grpcCode,
      });
    };
    reply = fail('NOT_FOUND');
    await expect(c.fib(MplsFibQuery.parse({ table: '3001' }))).rejects.toMatchObject({
      status: 404,
    });
    reply = fail('INVALID_ARGUMENT');
    await expect(c.fib(MplsFibQuery.parse({}))).rejects.toMatchObject({ status: 400 });
    reply = fail('UNAVAILABLE');
    await expect(c.fib(MplsFibQuery.parse({}))).rejects.toMatchObject({ status: 502 });
    expect(MplsFibQuery.safeParse({ page: '101', pageSize: '1000' }).success).toBe(false);
    expect(MplsFibQuery.safeParse({ label: '1048576' }).success).toBe(false);
  });

  it('lists tunnels', async () => {
    reply = () => ({
      ...base,
      view: 'tunnels',
      tunnels: [
        {
          name: 't1',
          interface: 'mpls-tunnel0',
          swIfIndex: 9,
          tunnelIndex: 0,
          l2Only: false,
          multicast: false,
          owned: true,
          paths: [],
        },
      ],
    });
    const out = await c.tunnels();
    expect(MplsTunnelsOut.strict().safeParse(out).success).toBe(true);
    expect(out.items[0]).toMatchObject({ name: 't1', swIfIndex: 9, owned: true });
  });
});

describe('OpenAPI', () => {
  let app: NestFastifyApplication;
  beforeAll(async () => {
    app = await createApp({ env: loadEnv({}), logger: false });
    await app.init();
  });
  afterAll(async () => app.close());

  it('documents GET /state/routing/mpls/{fib,tunnels} with the MplsSrmpls operation ids', () => {
    const paths = buildOpenApi(app).paths;
    expect(paths['/api/v1/state/routing/mpls/fib']?.get?.operationId).toBe('MplsSrmpls_fib');
    expect(paths['/api/v1/state/routing/mpls/tunnels']?.get?.operationId).toBe(
      'MplsSrmpls_tunnels',
    );
  });
});
