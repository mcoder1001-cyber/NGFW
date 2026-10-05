import { Controller, Get, HttpCode, Post, Req } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import { HaSyncOp } from '@ngfw/proto';
import { AgentClient } from '../../agent/agent.client.js';
import { MinRole } from '../../auth/decorators.js';
import type { NgfwRequest } from '../../common/principal.js';
import { ProblemError } from '../../common/problem.js';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';
import { syncState, syncResult, toState } from './dto.js';
@ApiTags('ha')
@Controller('api/v1')
export class HaStateSyncController {
  constructor(private readonly agent: AgentClient) {}
  @Get('state/ha/sync')
  @Protected()
  @ApiOperation({ summary: 'Observed NAT HA endpoints and explicit unsupported session kinds' })
  @ApiOkResponse({ schema: openapi(syncState, 'output') })
  async state() {
    return toState(await this.agent.haSyncState());
  }
  @Post('actions/ha/sync/resync')
  @HttpCode(200)
  @MinRole('admin')
  @Protected(400, 403, 409, 502, 503)
  @ApiOperation({
    summary: 'Globals owner only: wait up to 15 seconds for native NAT44-EI resync completion',
  })
  @ApiOkResponse({ schema: openapi(syncResult, 'output') })
  async resync(@Req() req: NgfwRequest) {
    req.audit = { resource: 'ha/state-sync/resync', before: { operation: 'resync' } };
    const result = await this.agent.runAction(
      { haSync: { op: HaSyncOp.HA_SYNC_OP_RESYNC } },
      20_000,
    );
    if (!result.done || result.done.exitCode !== 0)
      throw new ProblemError(
        502,
        'ha-resync-failed',
        'HA resync failed',
        result.done?.summary ?? 'Agent stream ended without completion',
      );
    req.audit.after = { operation: 'resync', completed: true };
    return { summary: result.done.summary };
  }
}
