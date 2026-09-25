import { Controller, Get } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import type { HostStackStateResponse } from '@ngfw/proto';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';

export const HostStackRuleOut = z.object({
  tag: z.string(),
  scope: z.string(),
  transport: z.string(),
  local: z.string(),
  localPort: z.number().int(),
  remote: z.string(),
  remotePort: z.number().int(),
  action: z.string(),
  appnsIndexes: z.array(z.number().int()),
});

export const HostStackStateOut = z.object({
  sessionEnabled: z
    .boolean()
    .describe('the session layer answers the rules dump (VPP has no getter)'),
  sessionDetail: z.string().describe('why the layer is off or unknown; empty when on'),
  namespaces: z.array(z.string()).describe('app namespaces this agent applied on the running VPP'),
  ruleCount: z.number().int().describe("this owner's session rules"),
  ruleCountTotal: z.number().int().describe('every session rule in VPP (all owners)'),
  rules: z.array(HostStackRuleOut),
  retrievedAt: z.string().nullable(),
});
export type HostStackStateOut = z.infer<typeof HostStackStateOut>;

/** Maps the agent's HostStackStateResponse to the REST shape. */
export function toHostStackState(r: HostStackStateResponse): HostStackStateOut {
  return {
    sessionEnabled: r.sessionEnabled,
    sessionDetail: r.sessionDetail,
    namespaces: [...r.namespaces],
    ruleCount: r.rules.length,
    ruleCountTotal: r.ruleCountTotal,
    rules: r.rules.map((x) => ({
      tag: x.tag,
      scope: x.scope,
      transport: x.transport,
      local: x.local,
      localPort: x.localPort,
      remote: x.remote,
      remotePort: x.remotePort,
      action: x.action,
      appnsIndexes: [...x.appnsIndexes],
    })),
    retrievedAt: r.retrievedAt ? r.retrievedAt.toISOString() : null,
  };
}

/**
 * `GET /api/v1/state/host-stack` (F-host-stack): live, read-only view of VPP's host stack from the agent's
 * HostStackState RPC. Configuration goes through the generic pointer routes (`/api/v1/config/services/hostStack`).
 */
@ApiTags('state')
@Controller('api/v1/state')
export class HostStackController {
  constructor(private readonly agent: AgentClient) {}

  @Get('host-stack')
  @Protected(501, 502, 503)
  @ApiOperation({
    summary: 'Host stack: session layer on/off, applied app namespaces, session rules (read-only)',
  })
  @ApiOkResponse({ schema: openapi(HostStackStateOut, 'output') })
  async state(): Promise<HostStackStateOut> {
    return toHostStackState(await this.agent.hostStackState());
  }
}
