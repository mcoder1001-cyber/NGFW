import { ObservationOut, observations } from './observations.js';
import { Controller, Get, Query } from '@nestjs/common';
import { ApiOperation, ApiTags, ApiQuery } from '@nestjs/swagger';
import { ProblemError } from '../../common/problem.js';
import { AgentClient } from '../../agent/agent.client.js';
import { ApiOut, Protected } from '../../common/responses.js';
import { OspfStateOut, type OspfState } from './dto.js';
import { OSPF_NEIGHBORS_READER, OSPF6_NEIGHBORS_READER, ospfStateOut } from './state.js';

@ApiTags('state')
@Controller('api/v1/state')
export class OspfController {
  constructor(private readonly agent: AgentClient) {}

  @Get(['ospf', 'routing/ospf/neighbors'])
  @Protected(400, 502, 503)
  @ApiOperation({
    summary: 'Bounded observed OSPFv2/v3 neighbors; unavailable readers are explicit',
  })
  @ApiOut(OspfStateOut)
  @ApiQuery({ name: 'version', required: false, enum: ['2', '3'] })
  async state(@Query('version') version?: string): Promise<OspfState> {
    if (version !== undefined && version !== '2' && version !== '3')
      throw new ProblemError(
        400,
        'invalid-version',
        'Invalid OSPF version',
        'version must be 2 or 3',
      );
    const reader = version === '3' ? OSPF6_NEIGHBORS_READER : OSPF_NEIGHBORS_READER;
    // Fixed reader allowlist; no caller-defined commands, RIB walk or configuration mutation.
    return OspfStateOut.parse(
      ospfStateOut(
        await this.agent.routingState({
          readers: [reader],
          ribPrefixes: [],
          ribVrf: '',
        }),
        reader,
      ),
    );
  }
  @Get('routing/ospf/interfaces')
  @ApiOperation({ summary: 'Bounded observed OSPFv2/v3 interface state' })
  @Protected(400, 502, 503)
  @ApiOut(ObservationOut)
  @ApiQuery({ name: 'version', required: false, enum: ['2', '3'] })
  async interfaces(@Query('version') version?: string) {
    return this.observe(version, 'interfaces', '0', '100');
  }

  @Get('routing/ospf/database')
  @ApiOperation({ summary: 'Paged bounded public OSPFv2/v3 link-state database observations' })
  @Protected(400, 502, 503)
  @ApiOut(ObservationOut)
  @ApiQuery({ name: 'version', required: false, enum: ['2', '3'] })
  @ApiQuery({ name: 'offset', required: false, type: Number })
  @ApiQuery({ name: 'limit', required: false, type: Number })
  async database(
    @Query('version') version?: string,
    @Query('offset') offset?: string,
    @Query('limit') limit?: string,
  ) {
    return this.observe(version, 'database', offset ?? '0', limit ?? '100');
  }

  private async observe(
    version: string | undefined,
    type: 'interfaces' | 'database',
    rawOffset: string,
    rawLimit: string,
  ) {
    const offset = Number(rawOffset),
      limit = Number(rawLimit);
    if (
      (version !== undefined && version !== '2' && version !== '3') ||
      !/^\d+$/.test(rawOffset) ||
      !/^\d+$/.test(rawLimit) ||
      !Number.isSafeInteger(offset) ||
      offset < 0 ||
      !Number.isSafeInteger(limit) ||
      limit < 1 ||
      limit > 100
    )
      throw new ProblemError(
        400,
        'invalid-query',
        'Invalid observation query',
        'version must be 2 or 3; offset nonnegative and limit 1–100',
      );
    const reader = `${version === '3' ? 'ospf6' : 'ospf'}${type === 'interfaces' ? 'Interfaces' : 'Database'}`;
    return ObservationOut.parse(
      observations(
        await this.agent.routingState({ readers: [reader], ribPrefixes: [], ribVrf: '' }),
        reader,
        offset,
        limit,
      ),
    );
  }
}
