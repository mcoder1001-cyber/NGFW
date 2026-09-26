import { Controller, Get, HttpCode, Param, Post, Query, Req } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiParam, ApiQuery, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import type { VrxRequest } from '../../common/principal.js';
import { ProblemError, problems } from '../../common/problem.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { joinPolicers } from './model.js';

/** A policer name as the agent knows it: a `services.qos.policers` key (objectName) or `shaper:<shapers key>`. */
const POLICER_NAME = /^(shaper:)?[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/;
const PolicerName = z
  .string()
  .regex(
    POLICER_NAME,
    'a policer name (letters, digits, `_`, `.`, `-`), optionally `shaper:<name>`',
  );

const PoliciesQuery = z.object({
  name: PolicerName.optional().describe(
    'only this policer (a qos.policers key, or shaper:<qos.shapers key>)',
  ),
});
const ResetParams = z.object({ name: PolicerName });

const CounterOut = z.object({
  packets: z.string().describe('64-bit counter as a decimal string'),
  bytes: z.string(),
});
const PolicerItemOut = z.object({
  name: z
    .string()
    .describe('services.qos.policers key, or the services.qos.shapers key for kind=shaper'),
  kind: z.enum(['policer', 'shaper']),
  vppName: z
    .string()
    .describe(
      'the name the agent gives it in VPP without the owner prefix (shaper:<name> for a shaper)',
    ),
  configured: z.boolean().describe('in the running configuration'),
  present: z.boolean().describe('VPP reports it for this agent'),
  index: z.number().int().nullable().describe('VPP pool index; null when not present'),
  type: z.string(),
  rateUnit: z.string(),
  cir: z.number().int(),
  eir: z.number().int(),
  cb: z.number().int(),
  eb: z.number().int(),
  bucket: z
    .object({
      current: z.number().int(),
      limit: z.number().int(),
      extendedCurrent: z.number().int(),
      extendedLimit: z.number().int(),
    })
    .nullable()
    .describe("VPP's token buckets (internal token units); null when not present"),
  conform: CounterOut,
  exceed: CounterOut,
  violate: CounterOut,
  attachments: z
    .array(z.object({ interface: z.string(), direction: z.enum(['input', 'output']) }))
    .describe('interfaces the running configuration attaches it to'),
});
const PoliciesOut = z.object({
  retrievedAt: z.string().optional(),
  countersError: z
    .string()
    .nullable()
    .describe('set when the agent could not read the stats segment (the counters are then 0)'),
  items: z.array(PolicerItemOut),
});
const ResetOut = z.object({
  name: z.string(),
  index: z.number().int(),
  resetAt: z.string().optional(),
});

/**
 * F-qos-flat (docs/user/services/qos-flat.md): live policer status and the policer reset action. The configuration
 * itself (`services.qos`) goes through the generic pointer routes (/api/v1/config/**); nothing here reads VPP — the
 * agent does (QosPolicerState / QosPolicerReset, docs/contracts/proto.md §11). The agent serialises its policer walks
 * (D-132): the UI polls this route no faster than every 30 s and has a Refresh button.
 */
@ApiTags('state')
@Controller('api/v1')
export class QosFlatController {
  constructor(
    private readonly agent: AgentClient,
    private readonly ds: DatastoreService,
  ) {}

  @Get('state/services/qos/policers')
  @Protected(400, 501, 502, 503)
  @ApiQuery({ name: 'name', required: false, schema: { type: 'string' } })
  @ApiOperation({
    summary:
      'QoS policers and shapers: what VPP reports (parameters, token buckets, conform/exceed/violate counters) joined with the running configuration (attachments)',
  })
  @ApiOkResponse({ schema: openapi(PoliciesOut, 'output') })
  async policers(@Query(new ZodPipe(PoliciesQuery)) q: z.output<typeof PoliciesQuery>) {
    const [live, running] = await Promise.all([
      this.agent.qosPolicerState({ names: q.name === undefined ? [] : [q.name] }),
      this.ds.getRunning(),
    ]);
    let items = joinPolicers(running.doc, live.policers);
    if (q.name !== undefined) items = items.filter((i) => i.vppName === q.name);
    return {
      retrievedAt: live.retrievedAt?.toISOString(),
      countersError: live.countersError === '' ? null : live.countersError,
      items,
    };
  }

  @Post('actions/qos/policers/:name/reset')
  @HttpCode(200)
  @Protected(400, 404, 501, 502, 503)
  @ApiParam({
    name: 'name',
    schema: { type: 'string', pattern: POLICER_NAME.source },
    description: 'a qos.policers key, or shaper:<qos.shapers key>',
  })
  @ApiOperation({
    summary:
      "Refill a policer's token buckets (VPP policer_reset); the counters are kept. Operator; audited",
  })
  @ApiOkResponse({ schema: openapi(ResetOut, 'output') })
  async reset(
    @Param(new ZodPipe(ResetParams)) p: z.output<typeof ResetParams>,
    @Req() req: VrxRequest,
  ) {
    req.audit = { resource: `qos/policers/${p.name}` };
    try {
      const r = await this.agent.qosPolicerReset({ name: p.name });
      req.audit = { resource: `qos/policers/${p.name}`, after: { reset: true, index: r.index } };
      return { name: r.name, index: r.index, resetAt: r.resetAt?.toISOString() };
    } catch (e) {
      if (e instanceof ProblemError && e.extra['grpcCode'] === 'NOT_FOUND') {
        throw problems.notFound(`no policer '${p.name}' of this agent in VPP`);
      }
      throw e;
    }
  }
}
