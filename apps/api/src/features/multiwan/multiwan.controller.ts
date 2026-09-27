import { Controller, Get } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { ProblemError } from '../../common/problem.js';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';

const MemberOut = z.object({
  interface: z.string(),
  up: z.boolean(),
  lossPct: z.number().int(),
  latencyMs: z.number().int(),
  weight: z.number().int(),
  priority: z.number().int(),
  since: z.string().nullable(),
});
const GroupOut = z.object({
  name: z.string(),
  mode: z.string(),
  active: z
    .string()
    .describe('interface carrying the default route (failover); empty in balance or when all down'),
  members: z.array(MemberOut),
});
const WanStateOut = z.object({
  agentError: z
    .string()
    .nullable()
    .describe('why the live view is empty (agent unavailable or older agent)'),
  retrievedAt: z.string().nullable(),
  groups: z.array(GroupOut),
});

/**
 * F-multiwan state route: the live health of each WAN group's members (up/down, loss, latency, the active member in
 * failover). Read-only (00-CONTEXT rule 8). The groups themselves are configured through the generic pointer routes
 * (`/api/v1/config/routing/wanGroups`).
 */
@ApiTags('multiwan')
@Controller('api/v1/state')
export class MultiwanController {
  constructor(private readonly agent: AgentClient) {}

  @Get('wan')
  @Protected()
  @ApiOperation({ summary: 'Live multi-WAN group member health' })
  @ApiOkResponse({ schema: openapi(WanStateOut, 'output') })
  async wan() {
    try {
      const r = await this.agent.wanState();
      return {
        agentError: null,
        retrievedAt: r.retrievedAt?.toISOString() ?? null,
        groups: r.groups.map((g) => ({
          name: g.name,
          mode: g.mode,
          active: g.active,
          members: g.members.map((m) => ({
            interface: m.interface,
            up: m.up,
            lossPct: m.lossPct,
            latencyMs: m.latencyMs,
            weight: m.weight,
            priority: m.priority,
            since: m.since?.toISOString() ?? null,
          })),
        })),
      };
    } catch (e) {
      const msg =
        e instanceof ProblemError
          ? String(e.body().detail ?? e.body().title)
          : e instanceof Error
            ? e.message
            : String(e);
      return { agentError: msg, retrievedAt: null, groups: [] };
    }
  }
}
