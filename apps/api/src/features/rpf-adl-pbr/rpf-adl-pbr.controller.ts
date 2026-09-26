import { Controller, Get, Query } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import { DesiredState } from '@ngfw/proto';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { pbrState, PbrStateOut, PbrStateQuery } from './pbr-state.js';

/**
 * F-rpf-adl-pbr state routes (`/api/v1/state/**`, read-only, 00-CONTEXT rule 8). Configuration (uRPF and ADL on
 * interfaces, `routing.pbr`, `services.autoSdl`) goes through the generic config pointer routes; this controller only
 * adds the live policy-based routing view.
 */
@ApiTags('state')
@Controller('api/v1/state')
export class RpfAdlPbrController {
  constructor(
    private readonly agent: AgentClient,
    private readonly ds: DatastoreService,
  ) {}

  @Get('pbr')
  @Protected(502, 503)
  @ApiQuery({ name: 'page', required: false, schema: { type: 'integer', minimum: 1 } })
  @ApiQuery({
    name: 'pageSize',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 1000 },
  })
  @ApiOperation({
    summary:
      'Policy-based routing: running policies and attachments vs what the agent retrieves from VPP (attachments paged)',
  })
  @ApiOkResponse({ schema: openapi(PbrStateOut, 'output') })
  async pbr(@Query(new ZodPipe(PbrStateQuery)) q: z.output<typeof PbrStateQuery>) {
    const [running, candidate, r] = await Promise.all([
      this.ds.getRunning(),
      this.ds.getCandidate(),
      this.agent.retrieve(['routing']),
    ]);
    const actual = DesiredState.toJSON(r.desiredState ?? DesiredState.fromPartial({})) as Record<
      string,
      unknown
    >;
    return pbrState(running.doc, candidate, actual, r.retrievedAt, q);
  }
}
