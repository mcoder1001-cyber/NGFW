import { Body, Controller, Get, HttpCode, Post, Query, Req } from '@nestjs/common';
import { ApiBody, ApiOkResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import type { z } from 'zod';
import type { VrxRequest } from '../../common/principal.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { MAX_PAGE_SIZE, SessionsOut, SessionsQuery } from '../nat44-ed-sessions/dto.js';
import { EiKillBody, EiKillOut, Nat64SessionsOut, Nat64SessionsQuery, Nptv6Out } from './dto.js';
import { Nat44Ei6466Nptv6Service } from './service.js';

/**
 * NAT44-EI / NAT64 live sessions, the NAT44-EI session kill and the NPTv6 bindings (F-nat44-ei-64-66-nptv6).
 * Configuration (`nat.mode: "ei"`, `nat.nat64`, `nat.nat66`, `nat.nptv6`) goes through the generic `/api/v1/config/nat`
 * pointer routes; this controller only reads state (`/state/**`, rule 8) and runs the one action, a static route under
 * `/actions/**` audited by the global AuditInterceptor with the inside endpoint as the resource.
 */
@ApiTags('nat')
@Controller('api/v1')
export class Nat44Ei6466Nptv6Controller {
  constructor(private readonly nat: Nat44Ei6466Nptv6Service) {}

  @Get('state/nat/ei/sessions')
  @Protected(400, 501, 502, 503)
  @ApiOperation({
    summary:
      'NAT44-EI sessions of this agent, server-side paged (pageSize ≤ 1000) and filtered by the agent (NatSessions, variant EI)',
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
  eiSessions(@Query(new ZodPipe(SessionsQuery)) q: z.output<typeof SessionsQuery>) {
    return this.nat.eiSessions(q);
  }

  @Post('actions/nat/ei/sessions/kill')
  @HttpCode(200)
  @Protected(400, 404, 501, 502, 503)
  @ApiOperation({
    summary:
      'Delete one NAT44-EI session by its inside endpoint (protocol, inside address and port) and inside VRF',
  })
  @ApiBody({ schema: openapi(EiKillBody) })
  @ApiOkResponse({ schema: openapi(EiKillOut, 'output') })
  async eiKill(
    @Body(new ZodPipe(EiKillBody)) body: z.output<typeof EiKillBody>,
    @Req() req: VrxRequest,
  ) {
    const vrf = body.vrf ?? 'default';
    req.audit = {
      resource: `nat/ei/sessions/${body.protocol}/${body.insideAddress}:${body.insidePort}/${vrf}`,
      before: { ...body, vrf },
    };
    const out = await this.nat.eiKill(body);
    req.audit.after = { deleted: true, summary: out.summary };
    return out;
  }

  @Get('state/nat/nat64/sessions')
  @Protected(400, 501, 502, 503)
  @ApiOperation({
    summary:
      'NAT64 sessions of this agent (IPv6 client ↔ IPv4 pool ↔ IPv4 remote), server-side paged (NatSessions, variant NAT64)',
  })
  @ApiQuery({ name: 'page', required: false, schema: { type: 'integer', minimum: 1 } })
  @ApiQuery({
    name: 'pageSize',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: MAX_PAGE_SIZE },
  })
  @ApiQuery({ name: 'protocol', required: false, schema: { type: 'string' } })
  @ApiOkResponse({ schema: openapi(Nat64SessionsOut, 'output') })
  nat64Sessions(@Query(new ZodPipe(Nat64SessionsQuery)) q: z.output<typeof Nat64SessionsQuery>) {
    return this.nat.nat64Sessions(q);
  }

  @Get('state/nat/nptv6')
  @Protected(503)
  @ApiOperation({
    summary:
      'NPTv6 bindings of the running configuration, marked write-only (VPP 26.06 has no npt66 dump; the agent re-applies them on every resync)',
  })
  @ApiOkResponse({ schema: openapi(Nptv6Out, 'output') })
  nptv6() {
    return this.nat.nptv6();
  }
}
