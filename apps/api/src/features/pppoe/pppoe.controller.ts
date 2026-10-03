import { Controller, HttpCode, Param, Post, Req } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiParam, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import type { NgfwRequest } from '../../common/principal.js';
import { Protected } from '../../common/responses.js';
import { openapi, SafeParamPipe } from '../../common/zod.js';

const ReconnectOut = z.object({
  accepted: z.boolean(),
  message: z.string(),
});

/**
 * F-pppoe-client action route. The live session state is served by `/api/v1/state/interfaces` (the `pppoe` field on
 * each interface's live state). This route asks the agent to redial one client now; an agent without PPPoE support
 * answers 501. Configuration of the client goes through the generic pointer routes (`/api/v1/config/interfaces/…`).
 */
@ApiTags('pppoe')
@Controller('api/v1/actions/interfaces')
export class PppoeController {
  constructor(private readonly agent: AgentClient) {}

  @Post(':name/pppoe/reconnect')
  @HttpCode(200)
  @Protected(404, 501, 502, 503)
  @ApiParam({
    name: 'name',
    schema: { type: 'string' },
    description: 'the PPPoE client interface name',
  })
  @ApiOperation({
    summary:
      'Redial a PPPoE client now (ignores the hold-off); the session comes up asynchronously',
  })
  @ApiOkResponse({ schema: openapi(ReconnectOut, 'output') })
  async reconnect(@Param('name', new SafeParamPipe('name')) name: string, @Req() req: NgfwRequest) {
    const r = await this.agent.pppoeReconnect(name);
    req.audit = {
      resource: `actions/interfaces/${name}/pppoe/reconnect`,
      after: { accepted: r.accepted },
    };
    return { accepted: r.accepted, message: r.message };
  }
}
