import { Controller, Get } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import type { SnmpStateResponse } from '@ngfw/proto';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';

export const SnmpStateOut = z
  .object({
    configured: z.boolean().describe('an enabled services.snmp is applied'),
    daemon: z.object({
      reachable: z.boolean(),
      endpoint: z.string(),
      credential: z.string().describe('credential used for the read-back, by name only'),
      sysName: z.string(),
      sysDescr: z.string(),
      sysLocation: z.string(),
      sysContact: z.string(),
      sysUpTimeCentiseconds: z.string().describe('uint64 as decimal string (D-039)'),
      error: z.string(),
    }),
    engineId: z.string(),
    pendingAction: z
      .string()
      .describe('daemon action the agent waits for (restart/start, D-079); empty = none'),
    subagent: z.object({
      registered: z.boolean(),
      registrations: z.string(),
      error: z.string(),
    }),
  })
  .describe('snmpd renderer stage state (SnmpState RPC). No community or passphrase value, ever.');

export function toSnmpState(r: SnmpStateResponse): z.infer<typeof SnmpStateOut> {
  return {
    configured: r.configured,
    daemon: {
      reachable: r.reachable,
      endpoint: r.endpoint,
      credential: r.credential,
      sysName: r.sysName,
      sysDescr: r.sysDescr,
      sysLocation: r.sysLocation,
      sysContact: r.sysContact,
      sysUpTimeCentiseconds: r.sysUpTime,
      error: r.error,
    },
    engineId: r.engineId,
    pendingAction: r.pendingAction,
    subagent: {
      registered: r.subagentRegistered,
      registrations: r.subagentRegistrations,
      error: r.subagentError,
    },
  };
}

/** F-snmp: `GET /api/v1/state/snmp`. Configuration goes through the generic pointer routes (`/config/services/snmp`). */
@ApiTags('state')
@Controller('api/v1/state')
export class SnmpStateController {
  constructor(private readonly agent: AgentClient) {}

  @Get('snmp')
  @Protected()
  @ApiOperation({
    summary: 'SNMP agent state: daemon read-back, pending daemon action, VRX-MIB subagent',
  })
  @ApiOkResponse({ schema: openapi(SnmpStateOut, 'output') })
  async snmp() {
    return toSnmpState(await this.agent.snmpState());
  }
}
