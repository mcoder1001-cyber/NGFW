import type { ActionDone, ListNeighborsRequest, ListNeighborsResponse } from '@ngfw/proto';
import { describe, expect, it } from 'vitest';
import type { AgentClient } from '../../agent/agent.client.js';
import type { VrxRequest } from '../../common/principal.js';
import { ProblemError } from '../../common/problem.js';
import { EventKind } from '@ngfw/proto';
import { eventTopic } from '../../telemetry/relay.service.js';
import { TOPICS } from '../../infra/bus.js';
import { ArpFlushBody, NeighborsQuery, NeighborsRaController } from './neighbors-ra.controller.js';

function stub(opts: {
  list?: ListNeighborsResponse;
  flush?: { lines: string[]; done: ActionDone | undefined };
}) {
  const calls: { list: Omit<ListNeighborsRequest, 'owner'>[]; flush: unknown[] } = {
    list: [],
    flush: [],
  };
  const agent = {
    listNeighbors: (r: Omit<ListNeighborsRequest, 'owner'>) => {
      calls.list.push(r);
      return Promise.resolve(
        opts.list ?? { neighbors: [], total: 0, owner: 'w9', retrievedAt: undefined },
      );
    },
    arpFlush: (r: unknown) => {
      calls.flush.push(r);
      return Promise.resolve(opts.flush ?? { lines: [], done: undefined });
    },
  } as unknown as AgentClient;
  return { ctl: new NeighborsRaController(agent), calls };
}

describe('NeighborsRaController', () => {
  it('maps page/pageSize/dir onto the agent’s offset/limit/descending and every row field', async () => {
    const at = new Date('2026-09-24T18:00:00Z');
    const { ctl, calls } = stub({
      list: {
        neighbors: [
          {
            interface: 'host-w9l0',
            ip: '10.9.1.7',
            mac: '02:00:00:00:09:07',
            family: 'ipv4',
            state: 'dynamic',
            noFibEntry: false,
            ageSec: 3.25,
            vrf: 'default',
            tableId: 0,
          },
        ],
        total: 31,
        owner: 'w9',
        retrievedAt: at,
      },
    });
    const out = await ctl.neighbors(
      NeighborsQuery.parse({ page: '3', pageSize: '10', dir: 'desc', sort: 'mac', vrf: 'w9red' }),
    );
    expect(calls.list[0]).toEqual({
      vrf: 'w9red',
      interface: '',
      family: '',
      state: '',
      search: '',
      sort: 'mac',
      descending: true,
      offset: 20,
      limit: 10,
    });
    expect(out).toEqual({
      page: 3,
      pageSize: 10,
      total: 31,
      retrievedAt: at.toISOString(),
      items: [
        {
          interface: 'host-w9l0',
          ip: '10.9.1.7',
          mac: '02:00:00:00:09:07',
          family: 'ipv4',
          state: 'dynamic',
          noFibEntry: false,
          ageSec: 3.25,
          vrf: 'default',
          tableId: 0,
        },
      ],
    });
  });

  it('returns the flush result and records it for the audit row', async () => {
    const { ctl, calls } = stub({
      flush: {
        lines: ['host-w9l0 ipv4: deleted 2 learned entries'],
        done: {
          summary: 'deleted 2 learned entries on 1 interfaces',
          exitCode: 0,
          stats: { deleted: '2', interfaces: '1' },
        },
      },
    });
    const req = {} as VrxRequest;
    const out = await ctl.arpFlush(req, ArpFlushBody.parse({ interface: 'host-w9l0' }));
    expect(calls.flush[0]).toEqual({ interface: 'host-w9l0', family: '' });
    expect(out).toEqual({
      deleted: 2,
      interfaces: 1,
      summary: 'deleted 2 learned entries on 1 interfaces',
      lines: ['host-w9l0 ipv4: deleted 2 learned entries'],
    });
    expect(req.audit).toEqual({
      resource: 'arp-flush/host-w9l0',
      before: { interface: 'host-w9l0', family: '' },
      after: { deleted: 2, interfaces: 1, exitCode: 0 },
    });
  });

  it('a failed or cut-short flush is a 502 problem (and still audited)', async () => {
    for (const done of [
      { summary: 'ip_neighbor_add_del: VPPApiError', exitCode: 1, stats: { deleted: '3' } },
      undefined,
    ]) {
      const { ctl } = stub({ flush: { lines: [], done } });
      const req = {} as VrxRequest;
      const err = await ctl.arpFlush(req, ArpFlushBody.parse(undefined)).catch((e: unknown) => e);
      expect(err).toBeInstanceOf(ProblemError);
      expect((err as ProblemError).getStatus()).toBe(502);
      expect(req.audit?.resource).toBe('arp-flush/*');
    }
  });

  it("an interface the agent refuses (INVALID_ARGUMENT) is the caller's 400 with a pointer", async () => {
    const agent = {
      arpFlush: () =>
        Promise.reject(
          new ProblemError(
            502,
            'agent-rejected',
            'Agent rejected the request',
            'agent: invalid request: interface "loop301": tagged w3',
            undefined,
            {
              grpcCode: 'INVALID_ARGUMENT',
            },
          ),
        ),
    } as unknown as AgentClient;
    const err = (await new NeighborsRaController(agent)
      .arpFlush({} as VrxRequest, ArpFlushBody.parse({ interface: 'loop301' }))
      .catch((e: unknown) => e)) as ProblemError;
    expect(err.getStatus()).toBe(400);
    expect(err.errors).toEqual([
      { pointer: '/interface', message: 'agent: invalid request: interface "loop301": tagged w3' },
    ]);
  });

  it("vrf and search are terminal-safe single-line text (main's safeText, review L5)", () => {
    expect(NeighborsQuery.safeParse({ search: '02:00', vrf: 'w9red' }).success).toBe(true);
    for (const bad of ['a\u001b[31mb', 'x\ny', 'a\u202eb', 'x'.repeat(65)]) {
      expect(NeighborsQuery.safeParse({ search: bad }).success).toBe(false);
      expect(NeighborsQuery.safeParse({ vrf: bad }).success).toBe(false);
    }
  });

  it('rejects unknown body members and families', () => {
    expect(ArpFlushBody.safeParse({ family: 'ipx' }).success).toBe(false);
    expect(ArpFlushBody.safeParse({ interfaces: 'x' }).success).toBe(false);
    expect(ArpFlushBody.parse(undefined)).toEqual({});
  });

  it('routes EVENT_KIND_NEIGHBOR_CHANGED to the neighbor.events topic', () => {
    expect(TOPICS).toContain('neighbor.events');
    expect(eventTopic(EventKind.EVENT_KIND_NEIGHBOR_CHANGED)).toBe('neighbor.events');
  });
});
