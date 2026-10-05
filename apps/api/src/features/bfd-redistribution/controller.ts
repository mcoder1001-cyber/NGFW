import { Controller, Get } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';
const session = z.object({
  engine: z.string(),
  interface: z.string(),
  localAddress: z.string(),
  peerAddress: z.string(),
  state: z.string(),
  desiredMinTxUs: z.number(),
  requiredMinRxUs: z.number(),
  detectMultiplier: z.number(),
  lastFlap: z.string().nullable(),
  multihop: z.boolean(),
});
const edge = z.object({
  source: z.string(),
  target: z.string(),
  vrf: z.string(),
  routeMap: z.string(),
  metric: z.number().optional(),
  routeCount: z.string().nullable(),
  readOnly: z.boolean(),
});
const sessionsOut = z.object({
  retrievedAt: z.string().nullable(),
  agentError: z.string().nullable(),
  sessions: z.array(session),
});
const matrixOut = z.object({
  retrievedAt: z.string().nullable(),
  agentError: z.string().nullable(),
  edges: z.array(edge),
});
@ApiTags('bfd-redistribution')
@Controller('api/v1/state/routing')
export class BfdRedistributionController {
  constructor(private readonly agent: AgentClient) {}
  @Get('bfd/sessions')
  @Protected()
  @ApiOperation({ summary: 'Live VPP and FRR BFD sessions' })
  @ApiOkResponse({ schema: openapi(sessionsOut, 'output') })
  async sessions() {
    const r = await this.agent.bfdState();
    return {
      retrievedAt: r.retrievedAt?.toISOString() ?? null,
      agentError: r.error || null,
      sessions: r.sessions.map((s) => ({ ...s, lastFlap: s.lastFlap?.toISOString() ?? null })),
    };
  }
  @Get('redistribution')
  @Protected()
  @ApiOperation({ summary: 'Redistribution matrix with scoped protocol route counts' })
  @ApiOkResponse({ schema: openapi(matrixOut, 'output') })
  async matrix() {
    const r = await this.agent.redistributionMatrix();
    return {
      retrievedAt: r.retrievedAt?.toISOString() ?? null,
      agentError: r.error || null,
      edges: r.edges.map((e) => ({ ...e, routeCount: e.routeCount?.toString() ?? null })),
    };
  }
}
