import { Body, Controller, HttpCode, Param, Post } from '@nestjs/common';
import { ApiBody, ApiOkResponse, ApiOperation, ApiParam, ApiTags } from '@nestjs/swagger';
import type { ActionRequest } from '@ngfw/proto';
import { ipAddress, pointerIssues } from '@ngfw/schema';
import { z } from 'zod';
import { AgentClient } from '../agent/agent.client.js';
import { ProblemError, problems } from '../common/problem.js';
import { Protected } from '../common/responses.js';
import { openapi, SafeParamPipe } from '../common/zod.js';

export const ACTIONS = [
  'ping',
  'traceroute',
  'capture',
  'clear-counters',
  'reboot',
  'shutdown',
  'upgrade',
] as const;

/** `POST /api/v1/actions/ping` body (vrx.v1.PingAction; the agent enforces what VPP's ping API can do). */
export const PingBody = z.strictObject({
  target: ipAddress.describe('IPv4 or IPv6 address (no name resolution)'),
  vrf: z
    .string()
    .min(1)
    .max(63)
    .optional()
    .describe('VRF; the data plane pings from `default` only'),
  count: z.number().int().min(1).max(100).optional().describe('echo requests (default 5)'),
  intervalMs: z.number().int().min(100).max(60_000).optional().describe('default 1000'),
  timeoutMs: z.number().int().min(100).max(60_000).optional(),
  size: z.number().int().min(1).max(9000).optional().describe('not supported by VPP’s ping API'),
  source: ipAddress.optional().describe('not supported by VPP’s ping API'),
});

/** `POST /api/v1/actions/traceroute` body (vrx.v1.TracerouteAction). */
export const TracerouteBody = z.strictObject({
  target: ipAddress,
  vrf: z.string().min(1).max(63).optional(),
  maxHops: z.number().int().min(1).max(64).optional(),
  probes: z.number().int().min(1).max(10).optional(),
  timeoutMs: z.number().int().min(100).max(60_000).optional(),
  source: ipAddress.optional(),
});

export const ActionOut = z.object({
  action: z.string(),
  lines: z.array(z.string()).describe('human-readable output lines, in order'),
  done: z
    .object({
      summary: z.string(),
      exitCode: z.number().int().describe('0 = success'),
      stats: z.record(z.string(), z.string()).describe('e.g. transmitted, received, loss_pct'),
    })
    .describe('the terminal message of the action'),
});

/** The agent names the PingAction/TracerouteAction field first in INVALID_ARGUMENT ("…: vrf: …"): body pointers. */
const AGENT_FIELDS: Record<string, string> = {
  target: '/target',
  vrf: '/vrf',
  source: '/source',
  size: '/size',
  count: '/count',
  interval_ms: '/intervalMs',
  timeout_ms: '/timeoutMs',
};

/** An agent INVALID_ARGUMENT (502 from agentProblem) becomes 400 with the pointer of the field the agent names. */
export function agentRejected(e: ProblemError): ProblemError {
  const detail = (e.detail ?? 'the agent rejected the action').replace(/^agent: /, '');
  const field = /invalid action argument: ([a-z_]+):/.exec(detail)?.[1];
  const pointer = field !== undefined ? AGENT_FIELDS[field] : undefined;
  return problems.badRequest(
    detail,
    pointer !== undefined ? [{ pointer, message: detail }] : undefined,
  );
}

/**
 * `/api/v1/actions/**` — imperative operations (docs/04). F-vrf-static-ecmp turned the P06 stub into the bridge to the
 * agent's Action RPC (wave-A-hotspots P3): the body becomes an ActionRequest and the agent's output stream is collected
 * into one JSON answer (`lines[]` + `done`). ping runs in the data plane (VPP's ping plugin: default VRF only);
 * traceroute answers 501 (no VPP API); capture/clear-counters/reboot/shutdown/upgrade stay 501 until their features
 * (other features serve their own static routes, e.g. `/actions/arp-flush`, which win over `:action`). Every call is
 * audited as a mutation (AuditInterceptor) and needs the operator role.
 */
@ApiTags('actions')
@Controller('api/v1/actions')
export class ActionsController {
  constructor(private readonly agent: AgentClient) {}

  @Post(':action')
  @HttpCode(200)
  @Protected(400, 404, 501, 502, 503)
  @ApiParam({ name: 'action', schema: { type: 'string', enum: [...ACTIONS] } })
  @ApiBody({
    required: false,
    schema: { oneOf: [openapi(PingBody), openapi(TracerouteBody)] },
    description: 'ping: PingBody; traceroute: TracerouteBody; the other actions take no body (501)',
  })
  @ApiOperation({
    summary:
      'Run an action in the data plane (ping; traceroute and the others answer 501) and return its output',
  })
  @ApiOkResponse({ schema: openapi(ActionOut, 'output') })
  async run(@Param('action', new SafeParamPipe('action', 64)) action: string, @Body() body: unknown) {
    if (!(ACTIONS as readonly string[]).includes(action))
      throw problems.notFound(`unknown action '${action}'`);
    let req: ActionRequest;
    if (action === 'ping') {
      const p = parse(PingBody, body);
      req = {
        ping: {
          target: p.target,
          vrf: p.vrf ?? '',
          count: p.count ?? 0,
          size: p.size ?? 0,
          intervalMs: p.intervalMs ?? 0,
          timeoutMs: p.timeoutMs ?? 0,
          source: p.source ?? '',
        },
      };
    } else if (action === 'traceroute') {
      const p = parse(TracerouteBody, body);
      req = {
        traceroute: {
          target: p.target,
          vrf: p.vrf ?? '',
          maxHops: p.maxHops ?? 0,
          probes: p.probes ?? 0,
          timeoutMs: p.timeoutMs ?? 0,
          source: p.source ?? '',
        },
      };
    } else {
      throw problems.notImplemented(
        `action '${action}' is not implemented yet (the agent's Action RPC runs ping; ${action} belongs to a later feature)`,
      );
    }
    try {
      const r = await this.agent.runAction(req);
      const done = r.done ?? {
        summary: 'the agent ended the action without a result',
        exitCode: 1,
        stats: {},
      };
      return {
        action,
        lines: r.lines,
        done: { summary: done.summary, exitCode: done.exitCode, stats: { ...done.stats } },
      };
    } catch (e) {
      if (e instanceof ProblemError && e.extra['grpcCode'] === 'INVALID_ARGUMENT')
        throw agentRejected(e);
      throw e;
    }
  }
}

function parse<T extends z.ZodType>(schema: T, body: unknown): z.output<T> {
  const r = schema.safeParse(body ?? {});
  if (!r.success) throw problems.badRequest('invalid request body', pointerIssues(r.error));
  return r.data;
}
