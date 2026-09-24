import { Body, Controller, Get, HttpCode, Post, Query, Req } from '@nestjs/common';
import { ApiBody, ApiOkResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import type { NeighborEntry } from '@ngfw/proto';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import type { VrxRequest } from '../../common/principal.js';
import { ProblemError } from '../../common/problem.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';

/** Logical interface name as the agent reports it (sub-interfaces "<parent>.<id>"). */
const InterfaceName = z
  .string()
  .min(1)
  .max(80)
  .regex(/^[A-Za-z][A-Za-z0-9_./-]*$/, 'expected an interface name');

const SORT_FIELDS = ['interface', 'ip', 'mac', 'age', 'vrf', 'state'] as const;

export const NeighborsQuery = z.object({
  vrf: z.string().min(1).max(64).optional(),
  interface: InterfaceName.optional(),
  family: z.enum(['ipv4', 'ipv6']).optional(),
  state: z.enum(['static', 'dynamic']).optional(),
  search: z
    .string()
    .min(1)
    .max(64)
    .optional()
    .describe('substring of the IP, MAC or interface name'),
  sort: z.enum(SORT_FIELDS).default('interface'),
  dir: z.enum(['asc', 'desc']).default('asc'),
  page: z.coerce.number().int().min(1).default(1),
  pageSize: z.coerce.number().int().min(1).max(1000).default(100),
});

export const NeighborOut = z.object({
  interface: z.string(),
  ip: z.string(),
  mac: z.string(),
  family: z.enum(['ipv4', 'ipv6']),
  state: z.enum(['static', 'dynamic']),
  noFibEntry: z.boolean(),
  ageSec: z.number().describe('seconds since the entry was last confirmed; 0 for static entries'),
  vrf: z.string(),
  tableId: z.number().int(),
});

export const NeighborsOut = z.object({
  page: z.number().int(),
  pageSize: z.number().int(),
  total: z.number().int(),
  retrievedAt: z.string().optional(),
  items: z.array(NeighborOut),
});

export const ArpFlushBody = z
  .strictObject({
    interface: InterfaceName.optional().describe(
      'flush this interface only; absent = every configured interface',
    ),
    family: z.enum(['ipv4', 'ipv6']).optional().describe('absent = ARP and ND'),
  })
  .default({});

export const ArpFlushOut = z.object({
  deleted: z
    .number()
    .int()
    .describe('learned entries deleted; static neighbours are never removed'),
  interfaces: z.number().int(),
  summary: z.string(),
  lines: z.array(z.string()),
});

/** NeighborEntry → API row (every field present: ts-proto's toJSON would drop defaults such as false/0). */
export function neighborRow(n: NeighborEntry): z.output<typeof NeighborOut> {
  return {
    interface: n.interface,
    ip: n.ip,
    mac: n.mac,
    family: n.family === 'ipv6' ? 'ipv6' : 'ipv4',
    state: n.state === 'static' ? 'static' : 'dynamic',
    noFibEntry: n.noFibEntry,
    ageSec: n.ageSec,
    vrf: n.vrf,
    tableId: n.tableId,
  };
}

/**
 * F-neighbors-ra (WBS D2.3, D2.6): the live ARP/ND table (`GET /api/v1/state/neighbors`, served by the agent's
 * ListNeighbors RPC and paged there) and the ARP flush action (`POST /api/v1/actions/arp-flush`, the agent's
 * `arp_flush` Action; a static route, so it wins over the generic `POST /api/v1/actions/:action`). The flush is a
 * mutation: the global AuditInterceptor writes one audit row, with the interface, family and outcome recorded here.
 */
@Controller()
export class NeighborsRaController {
  constructor(private readonly agent: AgentClient) {}

  @Get('api/v1/state/neighbors')
  @ApiTags('state')
  @Protected(400, 502, 503)
  @ApiQuery({ name: 'vrf', required: false, schema: { type: 'string' } })
  @ApiQuery({ name: 'interface', required: false, schema: { type: 'string' } })
  @ApiQuery({ name: 'family', required: false, schema: { type: 'string', enum: ['ipv4', 'ipv6'] } })
  @ApiQuery({
    name: 'state',
    required: false,
    schema: { type: 'string', enum: ['static', 'dynamic'] },
  })
  @ApiQuery({ name: 'search', required: false, schema: { type: 'string' } })
  @ApiQuery({ name: 'sort', required: false, schema: { type: 'string', enum: [...SORT_FIELDS] } })
  @ApiQuery({ name: 'dir', required: false, schema: { type: 'string', enum: ['asc', 'desc'] } })
  @ApiQuery({ name: 'page', required: false, schema: { type: 'integer', minimum: 1 } })
  @ApiQuery({
    name: 'pageSize',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 1000 },
  })
  @ApiOperation({
    summary:
      'Live ARP/ND table (learned and static neighbours) of the interfaces the agent can name, filtered and paged by the agent',
  })
  @ApiOkResponse({ schema: openapi(NeighborsOut, 'output') })
  async neighbors(@Query(new ZodPipe(NeighborsQuery)) q: z.output<typeof NeighborsQuery>) {
    const r = await this.agent.listNeighbors({
      vrf: q.vrf ?? '',
      interface: q.interface ?? '',
      family: q.family ?? '',
      state: q.state ?? '',
      search: q.search ?? '',
      sort: q.sort,
      descending: q.dir === 'desc',
      offset: (q.page - 1) * q.pageSize,
      limit: q.pageSize,
    });
    return {
      page: q.page,
      pageSize: q.pageSize,
      total: r.total,
      retrievedAt: r.retrievedAt?.toISOString(),
      items: r.neighbors.map(neighborRow),
    };
  }

  @Post('api/v1/actions/arp-flush')
  @HttpCode(200)
  @ApiTags('actions')
  @Protected(400, 501, 502, 503)
  @ApiBody({ schema: openapi(ArpFlushBody) })
  @ApiOperation({
    summary:
      'Flush learned ARP/ND entries of one interface or of every configured interface; static neighbours stay',
  })
  @ApiOkResponse({ schema: openapi(ArpFlushOut, 'output') })
  async arpFlush(
    @Req() req: VrxRequest,
    @Body(new ZodPipe(ArpFlushBody)) body: z.output<typeof ArpFlushBody>,
  ) {
    const target = { interface: body.interface ?? '', family: body.family ?? '' };
    req.audit = { resource: `arp-flush/${target.interface || '*'}`, before: target };
    const r = await this.agent.arpFlush(target);
    if (!r.done || r.done.exitCode !== 0) {
      req.audit = { ...req.audit, after: { exitCode: r.done?.exitCode ?? null, lines: r.lines } };
      throw new ProblemError(
        502,
        'agent-error',
        'ARP flush failed',
        `agent: ${r.done?.summary ?? 'the action ended without a result'}`,
        undefined,
        {
          deleted: Number(r.done?.stats['deleted'] ?? 0),
        },
      );
    }
    const out = {
      deleted: Number(r.done.stats['deleted'] ?? 0),
      interfaces: Number(r.done.stats['interfaces'] ?? 0),
      summary: r.done.summary,
      lines: r.lines,
    };
    req.audit = {
      ...req.audit,
      after: { deleted: out.deleted, interfaces: out.interfaces, exitCode: r.done.exitCode },
    };
    return out;
  }
}
