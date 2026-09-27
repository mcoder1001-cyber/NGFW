import { Controller, Get, Query } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { ProblemError } from '../../common/problem.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';

const agentError = z.string().nullable().describe('why the live view is empty (agent unavailable or older agent)');

const NeighborOut = z.object({
  lsrId: z.string(),
  address: z.string(),
  state: z.string(),
  uptimeSec: z.number().int(),
});
const NeighborsOut = z.object({ agentError, retrievedAt: z.string().nullable(), neighbors: z.array(NeighborOut) });

const BindingOut = z.object({
  prefix: z.string(),
  localLabel: z.number().int(),
  peer: z.string(),
  remoteLabel: z.number().int(),
  inUse: z.boolean(),
});
const BindingsOut = z.object({
  agentError,
  retrievedAt: z.string().nullable(),
  total: z.number().int(),
  bindings: z.array(BindingOut),
});
const BindingsQuery = z.object({
  page: z.coerce.number().int().min(1).default(1),
  pageSize: z.coerce.number().int().min(1).max(500).default(100),
});

const SyncOut = z.object({
  agentError,
  retrievedAt: z.string().nullable(),
  lastSyncAt: z.string().nullable(),
  installed: z.number().int(),
  conflicts: z.number().int(),
  lastError: z.string(),
  source: z.string().describe('the sync source in use ("zebra-lfib" | "ldp-bindings" | "")'),
});

/**
 * F-mpls-ldp state routes: live LDP neighbours, the label bindings (paged), and the FRR→VPP sync status. Read-only
 * (00-CONTEXT rule 8); LDP is configured through the generic pointer routes (`/api/v1/config/routing/mpls/ldp`). An
 * agent without LDP (or an older one) answers 501, surfaced as `agentError` with empty data. The FRR/VPP sync itself
 * is the host follow-up; until then these reflect what the fake/real agent reports.
 */
@ApiTags('mpls-ldp')
@Controller('api/v1/state/routing/mpls/ldp')
export class MplsLdpController {
  constructor(private readonly agent: AgentClient) {}

  private err(e: unknown): string {
    if (e instanceof ProblemError && e.getStatus() === 501) return 'the agent does not support LDP yet';
    return e instanceof Error ? e.message : 'the agent is unavailable';
  }

  @Get('neighbors')
  @Protected()
  @ApiOperation({ summary: 'Live LDP neighbours' })
  @ApiOkResponse({ schema: openapi(NeighborsOut, 'output') })
  async neighbors() {
    try {
      const r = await this.agent.mplsLdpState();
      return {
        agentError: null,
        retrievedAt: r.retrievedAt?.toISOString() ?? null,
        neighbors: r.neighbors.map((n) => ({ lsrId: n.lsrId, address: n.address, state: n.state, uptimeSec: n.uptimeSec })),
      };
    } catch (e) {
      return { agentError: this.err(e), retrievedAt: null, neighbors: [] };
    }
  }

  @Get('bindings')
  @Protected()
  @ApiQuery({ name: 'page', required: false, schema: { type: 'integer', minimum: 1, default: 1 } })
  @ApiQuery({ name: 'pageSize', required: false, schema: { type: 'integer', minimum: 1, maximum: 500, default: 100 } })
  @ApiOperation({ summary: 'LDP label bindings (LIB), paged' })
  @ApiOkResponse({ schema: openapi(BindingsOut, 'output') })
  async bindings(@Query(new ZodPipe(BindingsQuery)) q: z.output<typeof BindingsQuery>) {
    try {
      const r = await this.agent.mplsLdpState();
      const all = r.bindings.map((b) => ({
        prefix: b.prefix,
        localLabel: b.localLabel,
        peer: b.peer,
        remoteLabel: b.remoteLabel,
        inUse: b.inUse,
      }));
      const start = (q.page - 1) * q.pageSize;
      return {
        agentError: null,
        retrievedAt: r.retrievedAt?.toISOString() ?? null,
        total: all.length,
        bindings: all.slice(start, start + q.pageSize),
      };
    } catch (e) {
      return { agentError: this.err(e), retrievedAt: null, total: 0, bindings: [] };
    }
  }

  @Get('sync')
  @Protected()
  @ApiOperation({ summary: 'FRR→VPP label sync status (last sync, installed routes, conflicts, source)' })
  @ApiOkResponse({ schema: openapi(SyncOut, 'output') })
  async sync() {
    try {
      const r = await this.agent.mplsLdpState();
      const s = r.sync;
      return {
        agentError: null,
        retrievedAt: r.retrievedAt?.toISOString() ?? null,
        lastSyncAt: s?.lastSyncAt?.toISOString() ?? null,
        installed: s?.installed ?? 0,
        conflicts: s?.conflicts ?? 0,
        lastError: s?.lastError ?? '',
        source: s?.source ?? '',
      };
    } catch (e) {
      return { agentError: this.err(e), retrievedAt: null, lastSyncAt: null, installed: 0, conflicts: 0, lastError: '', source: '' };
    }
  }
}
