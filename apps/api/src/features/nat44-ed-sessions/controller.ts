import { Body, Controller, Get, HttpCode, Post, Query, Req } from '@nestjs/common';
import { ApiBody, ApiOkResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import type { z } from 'zod';
import type { VrxRequest } from '../../common/principal.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { KillBody, KillOut, MAX_PAGE_SIZE, SessionsOut, SessionsQuery, SummaryOut } from './dto.js';
import { Nat44EdSessionsService } from './service.js';

/**
 * NAT44-ED live state and the session kill (F-nat44-ed-sessions). Configuration goes through the generic
 * `/api/v1/config/nat` pointer routes; this controller only reads state (`/state/**`, rule 8) and runs the one
 * action. The kill is a static route under `/actions/**` (it wins over the generic `POST /actions/:action`) and is
 * audited by the global AuditInterceptor with the 5-tuple as the resource detail.
 */
@ApiTags('nat')
@Controller('api/v1')
export class Nat44EdSessionsController {
  constructor(private readonly nat: Nat44EdSessionsService) {}

  @Get('state/nat/sessions')
  @Protected(400, 501, 502, 503)
  @ApiOperation({
    summary:
      'NAT44-ED sessions of this agent, server-side paged (pageSize ≤ 1000) and filtered by the agent (NatSessions RPC)',
  })
  @ApiQuery({ name: 'page', required: false, schema: { type: 'integer', minimum: 1 } })
  @ApiQuery({
    name: 'pageSize',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: MAX_PAGE_SIZE },
  })
  @ApiQuery({ name: 'inside', required: false, schema: { type: 'string', format: 'ipv4' } })
  @ApiQuery({ name: 'outside', required: false, schema: { type: 'string', format: 'ipv4' } })
  @ApiQuery({ name: 'external', required: false, schema: { type: 'string', format: 'ipv4' } })
  @ApiQuery({
    name: 'port',
    required: false,
    schema: { type: 'integer', minimum: 0, maximum: 65535 },
  })
  @ApiQuery({ name: 'protocol', required: false, schema: { type: 'string' } })
  @ApiQuery({ name: 'vrf', required: false, schema: { type: 'string' } })
  @ApiOkResponse({ schema: openapi(SessionsOut, 'output') })
  sessions(@Query(new ZodPipe(SessionsQuery)) q: z.output<typeof SessionsQuery>) {
    return this.nat.sessions(q);
  }

  @Get('state/nat/summary')
  @Protected(501, 502, 503)
  @ApiOperation({
    summary:
      'NAT44-ED totals and per-pool utilisation (NatSummary RPC) joined with the running configuration pool names',
  })
  @ApiOkResponse({ schema: openapi(SummaryOut, 'output') })
  summary() {
    return this.nat.summary();
  }

  @Post('actions/nat/sessions/kill')
  @HttpCode(200)
  @Protected(400, 404, 501, 502, 503)
  @ApiOperation({
    summary:
      'Delete one NAT44-ED session by its 5-tuple (protocol, inside and external endpoint) and inside VRF',
  })
  @ApiBody({ schema: openapi(KillBody) })
  @ApiOkResponse({ schema: openapi(KillOut, 'output') })
  async kill(@Body(new ZodPipe(KillBody)) body: z.output<typeof KillBody>, @Req() req: VrxRequest) {
    const vrf = body.vrf ?? 'default';
    req.audit = {
      resource: `nat/sessions/${body.protocol}/${body.insideAddress}:${body.insidePort}/${body.externalAddress}:${body.externalPort}/${vrf}`,
      before: { ...body, vrf },
    };
    const out = await this.nat.kill(body);
    req.audit.after = { deleted: true, summary: out.summary };
    return out;
  }
}
