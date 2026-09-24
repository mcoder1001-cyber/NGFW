import { Controller, HttpCode, Param, Post } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiParam, ApiTags } from '@nestjs/swagger';
import { problems } from '../common/problem.js';
import { Protected } from '../common/responses.js';
import { SafeParamPipe } from '../common/zod.js';

export const ACTIONS = [
  'ping',
  'traceroute',
  'capture',
  'clear-counters',
  'reboot',
  'shutdown',
  'upgrade',
] as const;

/**
 * `/api/v1/actions/**` — imperative operations (docs/04). P06 ships the routes, auth and audit; the agent's Action
 * RPC is UNIMPLEMENTED until P08/F-*, so every action answers 501 (P06 §5).
 */
@ApiTags('actions')
@Controller('api/v1/actions')
export class ActionsController {
  @Post(':action')
  @HttpCode(501)
  @Protected(400, 404, 501)
  @ApiParam({ name: 'action', schema: { type: 'string', enum: [...ACTIONS] } })
  @ApiOperation({ summary: 'Run an action (all 501 until the agent implements Action)' })
  @ApiOkResponse({
    description: 'Action output (reserved: every action answers 501 in this release)',
  })
  run(@Param('action', new SafeParamPipe('action', 64)) action: string): never {
    if (!(ACTIONS as readonly string[]).includes(action))
      throw problems.notFound(`unknown action '${action}'`);
    throw problems.notImplemented(
      `action '${action}' is not implemented yet (agent Action RPC, P08)`,
    );
  }
}
