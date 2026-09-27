import { Controller, Get } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { ProblemError } from '../../common/problem.js';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';

const GroupOut = z.object({
  interface: z.string(),
  group: z.string(),
  sources: z.array(z.string()),
});
const MrouteOut = z.object({
  vrf: z.string(),
  group: z.string(),
  source: z.string().describe('empty for a (*,G) route'),
  accept: z.string().describe('incoming (RPF) interface'),
  forward: z.array(z.string()),
  packets: z.string(),
  bytes: z.string(),
});
const PimNeighborOut = z.object({
  interface: z.string(),
  address: z.string(),
  uptimeSec: z.number().int(),
});
const agentError = z
  .string()
  .nullable()
  .describe('why the live view is empty (agent unavailable or older agent)');
const GroupsOut = z.object({ agentError, retrievedAt: z.string().nullable(), groups: z.array(GroupOut) });
const MroutesOut = z.object({ agentError, retrievedAt: z.string().nullable(), mroutes: z.array(MrouteOut) });
const PimNeighborsOut = z.object({ agentError, retrievedAt: z.string().nullable(), neighbors: z.array(PimNeighborOut) });

/**
 * F-igmp-mfib state routes: live IGMP group memberships, the VPP mFIB, and PIM neighbours. Read-only (00-CONTEXT rule
 * 8); the configuration goes through the generic pointer routes (`/api/v1/config/routing/multicast`). An agent without
 * multicast (or an older one) answers 501, surfaced here as `agentError` with an empty list.
 */
@ApiTags('multicast')
@Controller('api/v1/state/routing/multicast')
export class IgmpMfibController {
  constructor(private readonly agent: AgentClient) {}

  private async state() {
    return this.agent.multicastState();
  }

  private err(e: unknown): string {
    if (e instanceof ProblemError && e.getStatus() === 501) return 'the agent does not support multicast yet';
    return e instanceof Error ? e.message : 'the agent is unavailable';
  }

  @Get('groups')
  @Protected()
  @ApiOperation({ summary: 'Live IGMP group memberships (router-mode interfaces)' })
  @ApiOkResponse({ schema: openapi(GroupsOut, 'output') })
  async groups() {
    try {
      const r = await this.state();
      return {
        agentError: null,
        retrievedAt: r.retrievedAt?.toISOString() ?? null,
        groups: r.groups.map((g) => ({ interface: g.interface, group: g.group, sources: g.sources })),
      };
    } catch (e) {
      return { agentError: this.err(e), retrievedAt: null, groups: [] };
    }
  }

  @Get('mroutes')
  @Protected()
  @ApiOperation({ summary: 'The live VPP mFIB (static and PIM-learned routes)' })
  @ApiOkResponse({ schema: openapi(MroutesOut, 'output') })
  async mroutes() {
    try {
      const r = await this.state();
      return {
        agentError: null,
        retrievedAt: r.retrievedAt?.toISOString() ?? null,
        mroutes: r.mroutes.map((m) => ({
          vrf: m.vrf,
          group: m.group,
          source: m.source,
          accept: m.accept,
          forward: m.forward,
          packets: m.packets,
          bytes: m.bytes,
        })),
      };
    } catch (e) {
      return { agentError: this.err(e), retrievedAt: null, mroutes: [] };
    }
  }

  @Get('pim-neighbors')
  @Protected()
  @ApiOperation({ summary: 'PIM neighbours (from FRR pimd)' })
  @ApiOkResponse({ schema: openapi(PimNeighborsOut, 'output') })
  async pimNeighbors() {
    try {
      const r = await this.state();
      return {
        agentError: null,
        retrievedAt: r.retrievedAt?.toISOString() ?? null,
        neighbors: r.pimNeighbors.map((n) => ({ interface: n.interface, address: n.address, uptimeSec: n.uptimeSec })),
      };
    } catch (e) {
      return { agentError: this.err(e), retrievedAt: null, neighbors: [] };
    }
  }
}
