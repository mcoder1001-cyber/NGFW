import { Body, Controller, Get, HttpCode, Post, Query, Req } from '@nestjs/common';
import { ApiBody, ApiOkResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import type { z } from 'zod';
import { MinRole } from '../../auth/decorators.js';
import type { VrxRequest } from '../../common/principal.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import {
  CnatSessionsOut,
  CnatSessionsQuery,
  Det44CloseBody,
  Det44LookupBody,
  Det44LookupOut,
  Det44SessionsOut,
  Det44SessionsQuery,
  DoneOut,
  MAX_PAGE_SIZE,
} from './dto.js';
import { Det44MapDsliteCnatService } from './service.js';

/**
 * DET44 CGNAT and CNAT live state + actions (F-det44-map-dslite-cnat). Configuration (nat.det44 / dslite / map / cnat /
 * pnat) goes through the generic `/api/v1/config/nat` pointer routes; this controller reads state (`/state/**`) and
 * runs the lookup, the DET44 session close and the CNAT purge (admin: the CNAT session table is a VPP global). The
 * actions are audited by the global AuditInterceptor with the resource set here.
 */
@ApiTags('nat')
@Controller('api/v1')
export class Det44MapDsliteCnatController {
  constructor(private readonly svc: Det44MapDsliteCnatService) {}

  @Get('state/nat/det44/sessions')
  @Protected(400, 403, 404, 501, 502, 503)
  @ApiOperation({
    summary:
      "One DET44 user's sessions, server-side paged, with the user's deterministic outside address and port block (Det44Sessions RPC)",
  })
  @ApiQuery({ name: 'user', required: true, schema: { type: 'string', format: 'ipv4' } })
  @ApiQuery({ name: 'page', required: false, schema: { type: 'integer', minimum: 1 } })
  @ApiQuery({
    name: 'pageSize',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: MAX_PAGE_SIZE },
  })
  @ApiOkResponse({ schema: openapi(Det44SessionsOut, 'output') })
  det44Sessions(@Query(new ZodPipe(Det44SessionsQuery)) q: z.output<typeof Det44SessionsQuery>) {
    return this.svc.det44Sessions(q);
  }

  @Post('actions/nat/det44/lookup')
  @HttpCode(200)
  @Protected(400, 403, 404, 501, 502, 503)
  @ApiOperation({
    summary:
      'DET44 lookup for CGNAT logging: inside → outside address + port block, or outside address + port → inside (read-only)',
  })
  @ApiBody({ schema: openapi(Det44LookupBody) })
  @ApiOkResponse({ schema: openapi(Det44LookupOut, 'output') })
  det44Lookup(@Body(new ZodPipe(Det44LookupBody)) b: z.output<typeof Det44LookupBody>) {
    return this.svc.det44Lookup(b);
  }

  @Post('actions/nat/det44/sessions/close')
  @HttpCode(200)
  @Protected(400, 403, 404, 501, 502, 503)
  @ApiOperation({
    summary: 'Close one DET44 session by its inside or outside endpoint and the external endpoint',
  })
  @ApiBody({ schema: openapi(Det44CloseBody) })
  @ApiOkResponse({ schema: openapi(DoneOut, 'output') })
  async det44Close(
    @Body(new ZodPipe(Det44CloseBody)) b: z.output<typeof Det44CloseBody>,
    @Req() req: VrxRequest,
  ) {
    req.audit = {
      resource: `nat/det44/sessions/${b.direction}/${b.address}:${b.port}/${b.externalAddress}:${b.externalPort}`,
      before: { ...b },
    };
    const out = await this.svc.det44Close(b);
    req.audit.after = { closed: true, summary: out.summary };
    return out;
  }

  @Get('state/nat/cnat/sessions')
  @Protected(400, 501, 502, 503)
  @ApiOperation({ summary: 'The CNAT session table, server-side paged (CnatSessions RPC)' })
  @ApiQuery({ name: 'page', required: false, schema: { type: 'integer', minimum: 1 } })
  @ApiQuery({
    name: 'pageSize',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: MAX_PAGE_SIZE },
  })
  @ApiOkResponse({ schema: openapi(CnatSessionsOut, 'output') })
  cnatSessions(@Query(new ZodPipe(CnatSessionsQuery)) q: z.output<typeof CnatSessionsQuery>) {
    return this.svc.cnatSessions(q);
  }

  @Post('actions/nat/cnat/sessions/purge')
  @HttpCode(200)
  @MinRole('admin')
  @Protected(403, 501, 502, 503)
  @ApiOperation({
    summary:
      'Purge the whole CNAT session table (admin; the table is a VPP global — the agent refuses unless it is the globals owner)',
  })
  @ApiOkResponse({ schema: openapi(DoneOut, 'output') })
  async cnatPurge(@Req() req: VrxRequest) {
    req.audit = { resource: 'nat/cnat/sessions' };
    const out = await this.svc.cnatPurge();
    req.audit.after = { purged: true };
    return out;
  }
}
