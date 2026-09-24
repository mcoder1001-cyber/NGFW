import { Controller, Get, Query } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import type { ListRoutesEntry, ListRoutesResponse } from '@ngfw/proto';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { ProblemError, problems } from '../../common/problem.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { DatastoreService } from '../../datastore/datastore.service.js';

/** `GET /api/v1/state/routes` query (the P06 parameters plus the FIB browser's filters). */
export const RoutesQuery = z.object({
  vrf: z
    .string()
    .min(1)
    .max(63)
    .optional()
    .describe('VRF name; absent = every VRF (default first, then the configured ones)'),
  page: z.coerce.number().int().min(1).default(1),
  pageSize: z.coerce.number().int().min(1).max(1000).default(100),
  family: z.enum(['ipv4', 'ipv6']).optional(),
  prefix: z
    .string()
    .max(64)
    .optional()
    .describe('only routes equal to or more specific than this prefix (CIDR)'),
  source: z
    .string()
    .max(64)
    .optional()
    .describe(
      'only routes that carry this FIB source (VPP name: API, interface, adjacency, svs, …)',
    ),
});

const PathOut = z.object({
  type: z
    .string()
    .describe('normal, local, drop, udp-encap, icmp-unreach, icmp-prohibit, source-lookup, …'),
  nextHop: z.string().optional(),
  interface: z.string().optional(),
  tableId: z.number().int().describe('table the next hop is resolved in'),
  weight: z.number().int(),
  preference: z.number().int(),
  flags: z.array(z.string()),
});

export const RouteOut = z.object({
  vrf: z.string(),
  prefix: z.string(),
  origin: z
    .string()
    .describe('static (API source), connected (interface source), else the FIB source name'),
  source: z.string().describe('best FIB source as VPP names it'),
  nextHops: z.array(z.object({ address: z.string().optional(), interface: z.string().optional() })),
  distance: z.number().int().optional().describe('administrative distance of a static route'),
  paths: z.array(PathOut).describe('the entry’s paths and what they resolve to (DPO kind)'),
  statsIndex: z.number().int(),
});
export type RouteOut = z.infer<typeof RouteOut>;

export const RoutesOut = z.object({
  page: z.number().int(),
  pageSize: z.number().int(),
  total: z.number().int(),
  vrf: z.string().optional().describe('the VRF read (absent when every VRF was listed)'),
  tableId: z.number().int().optional(),
  retrievedAt: z.string().optional(),
  items: z.array(RouteOut),
});

/** One FIB entry of the agent's ListRoutes as a `/state/routes` item. */
export function routeOut(vrf: string, r: ListRoutesEntry): RouteOut {
  const origin = r.source === 'API' ? 'static' : r.source === 'interface' ? 'connected' : r.source;
  const hops = r.paths.filter((p) => p.nextHop !== '' || p.interface !== '');
  const pref = r.paths.find((p) => p.preference > 0)?.preference;
  return {
    vrf,
    prefix: r.prefix,
    origin,
    source: r.source,
    nextHops: hops.map((p) => ({
      ...(p.nextHop ? { address: p.nextHop } : {}),
      ...(p.interface ? { interface: p.interface } : {}),
    })),
    ...(origin === 'static' && pref !== undefined ? { distance: pref } : {}),
    paths: r.paths.map((p) => ({
      type: p.type,
      ...(p.nextHop ? { nextHop: p.nextHop } : {}),
      ...(p.interface ? { interface: p.interface } : {}),
      tableId: p.tableId,
      weight: p.weight,
      preference: p.preference,
      flags: [...p.flags],
    })),
    statsIndex: r.statsIndex,
  };
}

/**
 * F-vrf-static-ecmp live state: the FIB browser. `GET /api/v1/state/routes` moved here from the StateController (P06/P08
 * paged a full Retrieve in the API); it now asks the agent's ListRoutes RPC for exactly one page of one VRF, so the API
 * never holds the table (proto.md §11). The operationId stays `State_routes` (the CLI binds by it).
 */
@ApiTags('state')
@Controller('api/v1/state')
export class VrfStaticEcmpController {
  constructor(
    private readonly agent: AgentClient,
    private readonly ds: DatastoreService,
  ) {}

  @Get('routes')
  @Protected(400, 404, 502, 503)
  @ApiQuery({ name: 'vrf', required: false, schema: { type: 'string' } })
  @ApiQuery({ name: 'page', required: false, schema: { type: 'integer', minimum: 1 } })
  @ApiQuery({
    name: 'pageSize',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 1000 },
  })
  @ApiQuery({ name: 'family', required: false, schema: { type: 'string', enum: ['ipv4', 'ipv6'] } })
  @ApiQuery({ name: 'prefix', required: false, schema: { type: 'string' } })
  @ApiQuery({ name: 'source', required: false, schema: { type: 'string' } })
  @ApiOperation({
    operationId: 'State_routes',
    summary:
      'Live FIB of a VRF (every VRF when none is given), paged and filtered by the agent, with each entry’s paths',
  })
  @ApiOkResponse({ schema: openapi(RoutesOut, 'output') })
  async routes(@Query(new ZodPipe(RoutesQuery)) q: z.output<typeof RoutesQuery>) {
    const names = q.vrf !== undefined ? [q.vrf] : await this.vrfNames();
    let offset = (q.page - 1) * q.pageSize;
    let need = q.pageSize;
    let total = 0;
    let last: ListRoutesResponse | undefined;
    const items: RouteOut[] = [];
    for (const vrf of names) {
      let r: ListRoutesResponse;
      try {
        r = await this.agent.listRoutes({
          vrf,
          family: q.family ?? '',
          prefix: q.prefix ?? '',
          source: q.source ?? '',
          offset: need > 0 ? offset : 0,
          limit: need > 0 ? need : 1,
        });
      } catch (e) {
        // a configured VRF the agent does not know yet (commit pending) is skipped in the all-VRF listing
        if (q.vrf === undefined && e instanceof ProblemError && e.extra['grpcCode'] === 'NOT_FOUND')
          continue;
        throw this.mapError(e, q.vrf !== undefined);
      }
      last = r;
      total += r.total;
      if (need === 0) continue;
      if (offset >= r.total) {
        offset -= r.total;
        continue;
      }
      items.push(...r.routes.map((x) => routeOut(r.vrf, x)));
      need -= r.routes.length;
      offset = 0;
    }
    return {
      page: q.page,
      pageSize: q.pageSize,
      total,
      ...(q.vrf !== undefined && last ? { vrf: last.vrf, tableId: last.tableId } : {}),
      ...(last?.retrievedAt ? { retrievedAt: last.retrievedAt.toISOString() } : {}),
      items,
    };
  }

  /** `default` and every VRF of the running configuration. */
  private async vrfNames(): Promise<string[]> {
    const running = await this.ds.getRunning();
    const vrfs = running.doc['vrfs'];
    const names =
      vrfs !== null && typeof vrfs === 'object'
        ? Object.keys(vrfs).filter((n) => n !== 'default')
        : [];
    return ['default', ...names.sort()];
  }

  /** Agent INVALID_ARGUMENT → 400, NOT_FOUND (unknown VRF) → 404; the rest as agentProblem made it. */
  private mapError(e: unknown, single: boolean): unknown {
    if (!(e instanceof ProblemError)) return e;
    const code = e.extra['grpcCode'];
    if (code === 'INVALID_ARGUMENT') return problems.badRequest(e.detail ?? 'invalid filter');
    if (code === 'NOT_FOUND' && single) return problems.notFound(e.detail ?? 'unknown VRF');
    return e;
  }
}
