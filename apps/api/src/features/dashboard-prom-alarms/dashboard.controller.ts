import { Controller, Get, HttpCode, Param, Post, Query, Req } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiParam, ApiQuery, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { ProblemError, problems } from '../../common/problem.js';
import type { VrxRequest } from '../../common/principal.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { AgentClient } from '../../agent/agent.client.js';
import { AlarmsService } from './alarms.service.js';

const boolQ = z
  .enum(['true', 'false', '1', '0'])
  .transform((v) => v === 'true' || v === '1')
  .optional();
const AlarmsQuery = z.object({
  active: boolQ.describe('true = only active, false = only cleared, omitted = all'),
  page: z.coerce.number().int().min(1).default(1),
  pageSize: z.coerce.number().int().min(1).max(500).default(100),
});

const AlarmOut = z.object({
  id: z.number().int(),
  rule: z.string(),
  instance: z.string(),
  metric: z.string(),
  severity: z.string(),
  state: z.string(),
  value: z.string(),
  threshold: z.string(),
  message: z.string(),
  raisedAt: z.string(),
  clearedAt: z.string().nullable(),
  ackedAt: z.string().nullable(),
  ackedBy: z.string().nullable(),
});
const AlarmsOut = z.object({ total: z.number().int(), items: z.array(AlarmOut) });
const AckOut = z.object({ acked: z.boolean() });
const DashboardOut = z.object({
  alarms: z.object({
    active: z.number().int(),
    bySeverity: z.record(z.string(), z.number().int()),
  }),
  agent: z.object({ reachable: z.boolean() }),
});

/**
 * F-dashboard-prom-alarms read/action routes: the alarm table, ack, and a one-call dashboard summary. The Prometheus
 * exposition is the agent's `/metrics` (P05) and the external `management.prometheus` listener — not an API route.
 */
@ApiTags('dashboard')
@Controller('api/v1')
export class DashboardController {
  constructor(
    private readonly alarms: AlarmsService,
    private readonly agent: AgentClient,
  ) {}

  @Get('state/alarms')
  @Protected()
  @ApiQuery({ name: 'active', required: false, schema: { type: 'boolean' } })
  @ApiQuery({ name: 'page', required: false, schema: { type: 'integer', minimum: 1, default: 1 } })
  @ApiQuery({
    name: 'pageSize',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 500, default: 100 },
  })
  @ApiOperation({ summary: 'Active and historical alarms, newest first' })
  @ApiOkResponse({ schema: openapi(AlarmsOut, 'output') })
  listAlarms(@Query(new ZodPipe(AlarmsQuery)) q: z.output<typeof AlarmsQuery>) {
    return this.alarms.list({ page: q.page, pageSize: q.pageSize, ...(q.active === undefined ? {} : { active: q.active }) });
  }

  @Post('actions/alarms/:id/ack')
  @HttpCode(200)
  @Protected(404)
  @ApiParam({ name: 'id', schema: { type: 'integer' } })
  @ApiOperation({ summary: 'Acknowledge an alarm (records who and when; does not clear it)' })
  @ApiOkResponse({ schema: openapi(AckOut, 'output') })
  async ack(@Param('id') id: string, @Req() req: VrxRequest) {
    const n = Number(id);
    if (!Number.isInteger(n) || n <= 0) throw problems.badRequest('id must be a positive integer');
    const acked = await this.alarms.ack(n, req.principal!.username);
    if (!acked) throw problems.notFound(`alarm ${n} does not exist or is already acknowledged`);
    req.audit = { resource: `actions/alarms/${n}/ack`, after: { acked } };
    return { acked };
  }

  @Get('state/dashboard')
  @Protected()
  @ApiOperation({ summary: 'Dashboard summary tile: active alarm counts and agent reachability' })
  @ApiOkResponse({ schema: openapi(DashboardOut, 'output') })
  async dashboard() {
    const alarms = await this.alarms.summary();
    let reachable = true;
    try {
      await this.agent.health();
    } catch (e) {
      if (e instanceof ProblemError) reachable = false;
      else reachable = false;
    }
    return { alarms, agent: { reachable } };
  }
}
