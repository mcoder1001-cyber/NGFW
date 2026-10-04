import { Body, Controller, Get, Headers, Post, Req } from '@nestjs/common';
import { ApiBody, ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import type { FastifyRequest } from 'fastify';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { Public } from '../../auth/decorators.js';
import { requestProtocol } from '../../common/principal.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { ClusterSyncService } from './service.js';
const runtime = z.object({
  name: z.string(),
  engine: z.string(),
  state: z.string(),
  currentPriority: z.number(),
  masterAdvertisementIntervalMs: z.number(),
  error: z.string(),
});
const vrrp = z.object({ retrievedAt: z.string(), routers: z.array(runtime) });
const cluster = z.object({
  nodeName: z.string(),
  enabled: z.boolean(),
  revision: z.number().nullable(),
  members: z.array(
    z.object({
      name: z.string(),
      role: z.string(),
      address: z.string(),
      revision: z.number().nullable(),
      sourceRevision: z.number().nullable(),
      error: z.string(),
      syncedAt: z.string().nullable(),
      lag: z.number().nullable(),
    }),
  ),
});
const envelope = z.strictObject({
  origin: z.string().min(1).max(253),
  revision: z.number().int().positive(),
  timestamp: z.number().int().positive(),
  nonce: z.string().uuid(),
  document: z.record(z.string(), z.unknown()),
});
@ApiTags('ha')
@Controller('api/v1')
export class VrrpConfigSyncController {
  constructor(
    private readonly agent: AgentClient,
    private readonly sync: ClusterSyncService,
  ) {}
  @Get('state/ha/vrrp')
  @Protected()
  @ApiOperation({ summary: 'Observed owner-scoped VRRP roles' })
  @ApiOkResponse({ schema: openapi(vrrp, 'output') })
  async vrrp() {
    const result = await this.agent.vrrpState();
    return { retrievedAt: result.retrievedAt?.toISOString() ?? '', routers: result.routers };
  }
  @Get('state/ha/cluster')
  @Protected()
  @ApiOperation({ summary: 'Cluster peers and last successful configuration sync' })
  @ApiOkResponse({ schema: openapi(cluster, 'output') })
  state() {
    return this.sync.state();
  }
  @Post('actions/ha/sync')
  @Protected()
  @ApiOperation({ summary: 'Force sync the confirmed running configuration to cluster peers' })
  @ApiOkResponse({ schema: openapi(cluster, 'output') })
  force() {
    return this.sync.force();
  }
  @Post('actions/ha/receive')
  @Public()
  @ApiOperation({ summary: 'HTTPS cluster-key authenticated peer delivery' })
  @ApiBody({ schema: openapi(envelope, 'input') })
  receive(
    @Body(new ZodPipe(envelope)) body: z.infer<typeof envelope>,
    @Headers('x-ngfw-cluster-signature') mac: string,
    @Req() req: FastifyRequest,
  ) {
    return this.sync.receive(body, mac, requestProtocol(req) === 'https');
  }
}
