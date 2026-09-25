import { Controller, Get, Query } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import type {
  MplsStateFibEntry,
  MplsStatePath,
  MplsStateResponse,
  MplsStateTunnel,
} from '@ngfw/proto';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { ProblemError, problems } from '../../common/problem.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';

/** The agent's MplsState window (offset + limit of one listing, apps/agent/internal/agent/rpc_mpls_srmpls.go). */
export const MPLS_FIB_WINDOW = 100_000;

/** `GET /api/v1/state/routing/mpls/fib` query. */
export const MplsFibQuery = z
  .object({
    table: z.coerce
      .number()
      .int()
      .min(0)
      .max(4294967295)
      .default(0)
      .describe('MPLS table: 0 (the default table) or one of this system’s MPLS tables'),
    label: z.coerce
      .number()
      .int()
      .min(0)
      .max(1048575)
      .optional()
      .describe('only the entries of this local label'),
    page: z.coerce.number().int().min(1).default(1),
    pageSize: z.coerce.number().int().min(1).max(1000).default(100),
  })
  .superRefine((q, ctx) => {
    // bounded before it becomes the agent's uint32 offset (no wrap-around, no window beyond the agent's)
    if (q.page * q.pageSize > MPLS_FIB_WINDOW)
      ctx.addIssue({
        code: 'custom',
        path: ['page'],
        message: `page × pageSize must be ≤ ${MPLS_FIB_WINDOW}: filter by label`,
      });
  });

const PathOut = z.object({
  type: z.string().describe('normal, local, drop, icmp-unreach, … (fib_api_path_type)'),
  proto: z.string().describe('next-hop protocol: ip4, ip6, mpls, ethernet'),
  nextHop: z.string().optional(),
  interface: z.string().optional().describe('logical interface name, else VPP’s'),
  tableId: z.number().int().describe('table the next hop or the payload is looked up in'),
  outLabels: z.array(z.number().int()).describe('pushed labels, outermost first'),
  weight: z.number().int(),
  preference: z.number().int(),
});
export type MplsPathOut = z.infer<typeof PathOut>;

export const MplsFibEntryOut = z.object({
  label: z.number().int(),
  eos: z.boolean().describe('end of stack'),
  payload: z.string().optional().describe('EOS payload: ip4, ip6, ethernet, mpls'),
  paths: z.array(PathOut),
});
export type MplsFibEntryOut = z.infer<typeof MplsFibEntryOut>;

const TableOut = z.object({ tableId: z.number().int(), name: z.string() });

export const MplsFibOut = z.object({
  page: z.number().int(),
  pageSize: z.number().int(),
  total: z.number().int().describe('entries that matched (the agent reads the whole table)'),
  tableId: z.number().int(),
  tables: z.array(TableOut).describe('the MPLS tables this system can read (0 when it exists)'),
  retrievedAt: z.string().optional(),
  items: z.array(MplsFibEntryOut),
});

export const MplsTunnelOut = z.object({
  name: z
    .string()
    .describe('configuration name (routing.mpls.tunnels key), else VPP’s interface name'),
  interface: z.string().describe('VPP interface name (mpls-tunnel<N>)'),
  swIfIndex: z.number().int(),
  tunnelIndex: z.number().int(),
  l2Only: z.boolean(),
  multicast: z.boolean(),
  owned: z.boolean().describe('created by this system (routing.mpls.tunnels)'),
  paths: z.array(PathOut),
});
export type MplsTunnelOut = z.infer<typeof MplsTunnelOut>;

export const MplsTunnelsOut = z.object({
  tables: z.array(TableOut),
  retrievedAt: z.string().optional(),
  items: z.array(MplsTunnelOut),
});

export function pathOut(p: MplsStatePath): MplsPathOut {
  return {
    type: p.type,
    proto: p.proto,
    ...(p.nextHop ? { nextHop: p.nextHop } : {}),
    ...(p.interface ? { interface: p.interface } : {}),
    tableId: p.tableId,
    outLabels: [...p.outLabels],
    weight: p.weight,
    preference: p.preference,
  };
}

export function fibEntryOut(e: MplsStateFibEntry): MplsFibEntryOut {
  return {
    label: e.label,
    eos: e.eos,
    ...(e.payload ? { payload: e.payload } : {}),
    paths: e.paths.map(pathOut),
  };
}

export function tunnelOut(t: MplsStateTunnel): MplsTunnelOut {
  return {
    name: t.name,
    interface: t.interface,
    swIfIndex: t.swIfIndex,
    tunnelIndex: t.tunnelIndex,
    l2Only: t.l2Only,
    multicast: t.multicast,
    owned: t.owned,
    paths: t.paths.map(pathOut),
  };
}

const tablesOut = (r: MplsStateResponse) =>
  r.tables.map((t) => ({ tableId: t.tableId, name: t.name }));

/**
 * F-mpls-srmpls live state (proto.md "F-mpls-srmpls: MplsState"): the MPLS FIB, paged by the agent (only the page
 * crosses the gRPC boundary), and the MPLS tunnels. `routing.mpls` itself is configured through the generic
 * `/api/v1/config/**` pointer routes. Every FIB read is one full walk of the table in VPP (the agent runs one at a
 * time and answers 503 while another is in progress): clients refresh on demand, never poll faster than 30 s (D-132).
 */
@ApiTags('state')
@Controller('api/v1/state/routing/mpls')
export class MplsSrmplsController {
  constructor(private readonly agent: AgentClient) {}

  @Get('fib')
  @Protected(400, 404, 502, 503)
  @ApiQuery({ name: 'table', required: false, schema: { type: 'integer', minimum: 0 } })
  @ApiQuery({
    name: 'label',
    required: false,
    schema: { type: 'integer', minimum: 0, maximum: 1048575 },
  })
  @ApiQuery({
    name: 'page',
    required: false,
    schema: { type: 'integer', minimum: 1 },
    description: `page × pageSize ≤ ${MPLS_FIB_WINDOW} (the agent's listing window)`,
  })
  @ApiQuery({
    name: 'pageSize',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 1000 },
  })
  @ApiOperation({
    summary: 'Live MPLS FIB of one MPLS table, paged by the agent, with each entry’s paths',
    description:
      'Table 0 is the default MPLS table (VPP-global: every entry, VPP’s reserved labels 0–15 included); other tables must be this system’s. One full walk of the table in VPP per call: refresh on demand, do not poll.',
  })
  @ApiOkResponse({ schema: openapi(MplsFibOut, 'output') })
  async fib(@Query(new ZodPipe(MplsFibQuery)) q: z.output<typeof MplsFibQuery>) {
    const r = await this.state({
      view: 'fib',
      tableId: q.table,
      label: q.label ?? 0,
      offset: (q.page - 1) * q.pageSize,
      limit: q.pageSize,
    });
    return {
      page: q.page,
      pageSize: q.pageSize,
      total: r.total,
      tableId: r.tableId,
      tables: tablesOut(r),
      ...(r.retrievedAt ? { retrievedAt: r.retrievedAt.toISOString() } : {}),
      items: r.entries.map(fibEntryOut),
    };
  }

  @Get('tunnels')
  @Protected(502, 503)
  @ApiOperation({
    summary: 'Live MPLS tunnels (this system’s and untagged ones) with their paths and out labels',
  })
  @ApiOkResponse({ schema: openapi(MplsTunnelsOut, 'output') })
  async tunnels() {
    const r = await this.state({ view: 'tunnels', tableId: 0, label: 0, offset: 0, limit: 0 });
    return {
      tables: tablesOut(r),
      ...(r.retrievedAt ? { retrievedAt: r.retrievedAt.toISOString() } : {}),
      items: r.tunnels.map(tunnelOut),
    };
  }

  /** Agent INVALID_ARGUMENT → 400, NOT_FOUND (a table that is not readable) → 404; the rest as agentProblem made it. */
  private async state(req: Parameters<AgentClient['mplsState']>[0]): Promise<MplsStateResponse> {
    try {
      return await this.agent.mplsState(req);
    } catch (e) {
      if (!(e instanceof ProblemError)) throw e;
      const code = e.extra['grpcCode'];
      if (code === 'INVALID_ARGUMENT') throw problems.badRequest(e.detail ?? 'invalid request');
      if (code === 'NOT_FOUND') throw problems.notFound(e.detail ?? 'unknown MPLS table');
      throw e;
    }
  }
}
