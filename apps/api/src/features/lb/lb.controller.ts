import { Controller, Get, HttpCode, Param, Post, Query, Req } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiParam, ApiQuery, ApiTags } from '@nestjs/swagger';
import type { LbVipState } from '@ngfw/proto';
import { isPlainObject } from '@ngfw/schema';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { problems } from '../../common/problem.js';
import type { VrxRequest } from '../../common/principal.js';
import { Protected } from '../../common/responses.js';
import { openapi, SafeParamPipe, ZodPipe } from '../../common/zod.js';
import { DatastoreService } from '../../datastore/datastore.service.js';

type Json = Record<string, unknown>;

const ServerOut = z.object({
  address: z.string(),
  inUse: z
    .boolean()
    .describe('LB_AS_FLAGS_USED; false = removed, waiting for the lb garbage collection (V20)'),
  inUseSince: z.number().int().describe('VPP clock (s) of the last use or removal'),
  configured: z.boolean().describe('the running configuration lists this server for the VIP'),
});
const VipOut = z.object({
  name: z.string().describe('services.lb.vips key'),
  prefix: z.string(),
  protocol: z.string().describe('any | tcp | udp (as configured; VPP 26.06 does not report it)'),
  port: z.number().int().describe('0 = all ports'),
  encap: z.string().describe('configured encapsulation'),
  status: z
    .enum(['active', 'not-applied', 'missing', 'no-servers'])
    .describe(
      'active: in VPP with a server in use; not-applied: the agent has no record of creating it on this VPP instance; missing: recorded but VPP does not list it; no-servers: in VPP without a server in use',
    ),
  applied: z
    .boolean()
    .describe('the agent created the VIP on the running VPP instance (D-080 boot record)'),
  vppEntries: z
    .number()
    .int()
    .describe(
      'lb_vip_dump entries with this prefix and port; > 1 = "removed" copies until the garbage collection',
    ),
  vppEncap: z.string().describe('encapsulation VPP reports (from the VIP type); "" = not in VPP'),
  dscp: z.number().int(),
  targetPort: z.number().int(),
  servers: z.array(ServerOut),
});
const VipsOut = z.object({
  retrievedAt: z.string().optional(),
  totalVppVips: z
    .number()
    .int()
    .describe('lb_vip_dump entries in the whole VPP (every owner, removed ones included)'),
  items: z.array(VipOut),
});
const VipsQuery = z.object({
  name: z.string().max(63).optional().describe('only this VIP'),
});
const FlushOut = z.object({
  vip: z.string().describe('the flushed VIP (lb.vip/<prefix>/<protocol>/<port>)'),
});

/** The running `services.lb.vips` by name (empty when absent). */
export function lbVipsOf(doc: Json): Map<string, Json> {
  const services = doc['services'];
  const lb = isPlainObject(services) ? services['lb'] : undefined;
  const vips = isPlainObject(lb) ? lb['vips'] : undefined;
  const out = new Map<string, Json>();
  if (!isPlainObject(vips)) return out;
  for (const [name, v] of Object.entries(vips)) if (isPlainObject(v)) out.set(name, v);
  return out;
}

export function vipJson(s: LbVipState, cfg: Json | undefined): z.output<typeof VipOut> {
  const configured = new Set(
    (Array.isArray(cfg?.['servers']) ? (cfg['servers'] as unknown[]) : [])
      .map((x) => (isPlainObject(x) ? x['address'] : undefined))
      .filter((a): a is string => typeof a === 'string'),
  );
  const inUse = s.servers.some((x) => x.inUse);
  const status: z.output<typeof VipOut>['status'] = !s.applied
    ? 'not-applied'
    : s.vppEntries === 0
      ? 'missing'
      : inUse
        ? 'active'
        : 'no-servers';
  return {
    name: s.name,
    prefix: s.prefix,
    protocol: s.protocol,
    port: s.port,
    encap: typeof cfg?.['encap'] === 'string' ? cfg['encap'] : '',
    status,
    applied: s.applied,
    vppEntries: s.vppEntries,
    vppEncap: s.encap,
    dscp: s.dscp,
    targetPort: s.targetPort,
    servers: s.servers.map((x) => ({
      address: x.address,
      inUse: x.inUse,
      inUseSince: x.inUseSince,
      configured: x.inUse && configured.has(x.address),
    })),
  };
}

/**
 * F-lb (VPP lb plugin): the live view `GET /api/v1/state/lb/vips` (agent LbState — read-only, never a Retrieve source:
 * every lb object is write-only, V20/D-063) and the action `POST /api/v1/actions/lb/vips/{name}/flush` (agent
 * LbFlushVip; audited like every mutation). The configuration itself goes through the generic pointer routes
 * (`/api/v1/config/services/lb`). The agent walks VPP's lb tables one at a time (D-132): clients refresh at most every
 * 30 s.
 */
@Controller('api/v1')
export class LbController {
  constructor(
    private readonly agent: AgentClient,
    private readonly ds: DatastoreService,
  ) {}

  @Get('state/lb/vips')
  @ApiTags('state')
  @Protected(400, 501, 502, 503)
  @ApiQuery({ name: 'name', required: false, schema: { type: 'string' } })
  @ApiOperation({
    summary:
      'Load-balancer VIPs of the running configuration as VPP holds them: VPP entries (removed copies included), VIP type, servers in use or removed (agent LbState)',
  })
  @ApiOkResponse({ schema: openapi(VipsOut, 'output') })
  async vips(@Query(new ZodPipe(VipsQuery)) q: z.output<typeof VipsQuery>) {
    const [r, running] = await Promise.all([
      this.agent.lbState(q.name !== undefined ? [q.name] : []),
      this.ds.getRunning(),
    ]);
    const cfg = lbVipsOf(running.doc);
    return {
      retrievedAt: r.retrievedAt?.toISOString(),
      totalVppVips: r.totalVppVips,
      items: r.vips.map((s) => vipJson(s, cfg.get(s.name))),
    };
  }

  @Post('actions/lb/vips/:name/flush')
  @HttpCode(200)
  @ApiTags('actions')
  @Protected(404, 409, 501, 502, 503)
  @ApiParam({ name: 'name', schema: { type: 'string' }, description: 'services.lb.vips key' })
  @ApiOperation({
    summary:
      'Flush the sticky flow table of a load-balancer VIP (lb_flush_vip): established flows are re-hashed over the current servers. 409 unless the VIP is applied and has a server in use',
  })
  @ApiOkResponse({ schema: openapi(FlushOut, 'output') })
  async flush(@Param('name', new SafeParamPipe('name', 63)) name: string, @Req() req: VrxRequest) {
    req.audit = { resource: `services/lb/vips/${name}`, after: { action: 'flush' } };
    const running = await this.ds.getRunning();
    if (!lbVipsOf(running.doc).has(name))
      throw problems.notFound(`VIP '${name}' is not in the running configuration`);
    const r = await this.agent.lbFlushVip(name);
    return { vip: r.vip };
  }
}
