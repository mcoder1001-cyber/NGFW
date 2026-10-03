import { Controller, Get } from '@nestjs/common';
import { ApiOperation, ApiTags } from '@nestjs/swagger';
import { AgentClient } from '../../agent/agent.client.js';
import { ApiOut, Protected } from '../../common/responses.js';
import { OspfStateOut, type OspfState } from './dto.js';
import { OSPF_NEIGHBORS_READER, ospfStateOut } from './state.js';

@ApiTags('state')
@Controller('api/v1/state')
export class OspfController {
  constructor(private readonly agent: AgentClient) {}

  @Get('ospf')
  @Protected(502, 503)
  @ApiOperation({ summary: 'Bounded observed OSPFv2 neighbors; unavailable readers are explicit' })
  @ApiOut(OspfStateOut)
  async state(): Promise<OspfState> {
    // Fixed reader allowlist; no caller-defined commands, RIB walk or configuration mutation.
    return OspfStateOut.parse(
      ospfStateOut(
        await this.agent.routingState({
          readers: [OSPF_NEIGHBORS_READER],
          ribPrefixes: [],
          ribVrf: '',
        }),
      ),
    );
  }
}
