import { Body, Controller, Delete, Get, HttpCode, Param, Post, Req, Res } from '@nestjs/common';
import {
  ApiBody,
  ApiOkResponse,
  ApiOperation,
  ApiParam,
  ApiProduces,
  ApiTags,
} from '@nestjs/swagger';
import type { FastifyReply } from 'fastify';
import type { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { AuditService } from '../../audit/audit.service.js';
import { MinRole } from '../../auth/decorators.js';
import { sourceIp, type NgfwRequest } from '../../common/principal.js';
import { ProblemError, problems } from '../../common/problem.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import {
  CaptureBody,
  CaptureStarted,
  CapturesOut,
  pointerOf,
  toCaptureAction,
  toCaptures,
} from './dto.js';

const ID_RE = /^[A-Za-z0-9._-]{1,100}$/;

function checkId(id: string): string {
  if (!ID_RE.test(id) || id.startsWith('.')) throw problems.notFound(`no capture ${id}`);
  return id;
}

/**
 * F-capture-trace (WBS D8.2): pcap capture through the agent's `capture` action. The agent keeps the files (moved out
 * of VPP's /tmp, 0600, retention caps); the API lists, downloads (admin, audited) and deletes them. Trace and the
 * packet generator are reported unavailable with the agent's reason (D-128/TD-20, V18) — never faked.
 */
@ApiTags('tools')
@Controller('api/v1')
export class CaptureTraceController {
  constructor(
    private readonly agent: AgentClient,
    private readonly audit: AuditService,
  ) {}

  @Post('actions/capture')
  @HttpCode(202)
  @Protected(400, 409, 501, 502, 503)
  @ApiOperation({
    summary:
      'Start a pcap capture (one per VPP; 409 `capture-busy` when one runs). Returns its id; poll GET /state/captures',
  })
  @ApiBody({ schema: openapi(CaptureBody) })
  @ApiOkResponse({ schema: openapi(CaptureStarted, 'output') })
  async start(
    @Body(new ZodPipe(CaptureBody)) body: z.output<typeof CaptureBody>,
    @Req() req: NgfwRequest,
  ) {
    req.audit = { resource: `captures/${body.interface}`, before: body };
    try {
      const id = await this.agent.startCapture(toCaptureAction(body));
      req.audit.after = { id };
      return { id };
    } catch (e) {
      if (e instanceof ProblemError && e.extra['grpcCode'] === 'ABORTED') {
        throw problems.conflict('capture-busy', e.detail ?? 'a capture is already running');
      }
      if (e instanceof ProblemError && e.extra['grpcCode'] === 'INVALID_ARGUMENT') {
        const pointer = pointerOf(e.detail ?? '');
        throw problems.badRequest(e.detail ?? 'invalid capture', [
          { pointer, message: e.detail ?? e.title, rule: 'capture' },
        ]);
      }
      throw e;
    }
  }

  @Get('state/captures')
  @Protected(501, 502, 503)
  @ApiOperation({
    summary:
      'Captures kept by the agent (and the running one), retention caps, trace / PG availability',
  })
  @ApiOkResponse({ schema: openapi(CapturesOut, 'output') })
  async list() {
    return toCaptures(await this.agent.captureList());
  }

  @Get('state/captures/:id/file')
  @MinRole('admin')
  @Protected(403, 404, 409, 501, 502, 503)
  @ApiParam({ name: 'id', schema: { type: 'string' } })
  @ApiProduces('application/vnd.tcpdump.pcap')
  @ApiOperation({ summary: 'Download one capture as a pcap file (admin; audited)' })
  @ApiOkResponse({
    content: { 'application/vnd.tcpdump.pcap': { schema: { type: 'string', format: 'binary' } } },
  })
  async file(
    @Param('id') id: string,
    @Req() req: NgfwRequest,
    @Res({ passthrough: true }) reply: FastifyReply,
  ) {
    checkId(id);
    const entry = (
      result: 'success' | 'failure',
      status: number,
      after?: Record<string, unknown>,
    ) =>
      this.audit.write({
        userId: req.principal?.id ?? null,
        username: req.principal?.username ?? null,
        sourceIp: sourceIp(req),
        action: `GET ${req.routeOptions.url ?? req.url}`,
        resource: `captures/${id}`,
        after,
        result,
        status,
      });
    let data: Buffer;
    try {
      data = await this.agent.captureRead(id);
    } catch (e) {
      const err =
        e instanceof ProblemError && e.detail?.includes('no such capture')
          ? problems.notFound(`no capture ${id}`)
          : e;
      await entry('failure', err instanceof ProblemError ? err.getStatus() : 500);
      throw err;
    }
    await entry('success', 200, { bytes: data.length });
    void reply
      .header('content-type', 'application/vnd.tcpdump.pcap')
      .header('content-disposition', `attachment; filename="${id}.pcap"`);
    return data;
  }

  @Delete('state/captures/:id')
  @MinRole('admin')
  @HttpCode(204)
  @Protected(403, 404, 409, 501, 502, 503)
  @ApiParam({ name: 'id', schema: { type: 'string' } })
  @ApiOperation({ summary: 'Delete one kept capture file (409 while it is running)' })
  async remove(@Param('id') id: string, @Req() req: NgfwRequest) {
    checkId(id);
    req.audit = { resource: `captures/${id}` };
    try {
      const r = await this.agent.captureDelete(id);
      req.audit.after = { deleted: true, bytes: String(r.size) };
    } catch (e) {
      if (e instanceof ProblemError && e.detail?.includes('no such capture')) {
        throw problems.notFound(`no capture ${id}`);
      }
      throw e;
    }
  }
}
