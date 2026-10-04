import { Controller, Get, Query, Inject } from '@nestjs/common';
import { ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import { AgentClient } from '../../agent/agent.client.js';
import { ApiOut, Protected } from '../../common/responses.js';
import { ProblemError } from '../../common/problem.js';
import { IsisRipStateOut, observed } from './state.js';

@ApiTags('state')
@Controller('api/v1/state/routing')
export class IsisRipController {
  constructor(@Inject(AgentClient) private readonly agent: AgentClient) {}
  @Get('isis/adjacencies')
  @ApiOperation({ summary: 'Bounded observed IS-IS adjacency state' })
  @Protected(400, 502, 503)
  @ApiOut(IsisRipStateOut)
  @ApiQuery({ name: 'offset', required: false, type: Number })
  @ApiQuery({ name: 'limit', required: false, type: Number })
  adjacencies(@Query('offset') offset?: string, @Query('limit') limit?: string) {
    return this.read('isisNeighbors', offset, limit);
  }
  @Get('isis/database')
  @ApiOperation({ summary: 'Paged bounded public IS-IS link-state database' })
  @Protected(400, 502, 503)
  @ApiOut(IsisRipStateOut)
  @ApiQuery({ name: 'offset', required: false, type: Number })
  @ApiQuery({ name: 'limit', required: false, type: Number })
  database(@Query('offset') offset?: string, @Query('limit') limit?: string) {
    return this.read('isisDatabase', offset, limit);
  }
  @Get('rip/peers')
  @ApiOperation({ summary: 'Bounded default-VRF RIPv2 or RIPng peer observations' })
  @Protected(400, 502, 503)
  @ApiOut(IsisRipStateOut)
  @ApiQuery({ name: 'version', required: false, enum: ['2', 'ng'] })
  @ApiQuery({ name: 'offset', required: false, type: Number })
  @ApiQuery({ name: 'limit', required: false, type: Number })
  peers(
    @Query('version') version?: string,
    @Query('offset') offset?: string,
    @Query('limit') limit?: string,
  ) {
    return this.read(this.ripReader(version, 'Status'), offset, limit);
  }
  @Get('rip/routes')
  @ApiOperation({ summary: 'Paged bounded FRR RIPv2 or RIPng routes; VPP FIB remains separate' })
  @Protected(400, 502, 503)
  @ApiOut(IsisRipStateOut)
  @ApiQuery({ name: 'version', required: false, enum: ['2', 'ng'] })
  @ApiQuery({ name: 'offset', required: false, type: Number })
  @ApiQuery({ name: 'limit', required: false, type: Number })
  routes(
    @Query('version') version?: string,
    @Query('offset') offset?: string,
    @Query('limit') limit?: string,
  ) {
    return this.read(this.ripReader(version, 'Routes'), offset, limit);
  }
  private ripReader(version: string | undefined, suffix: 'Status' | 'Routes') {
    if (version !== undefined && version !== '2' && version !== 'ng')
      throw new ProblemError(
        400,
        'invalid-version',
        'Invalid RIP version',
        'version must be 2 or ng',
      );
    return `${version === 'ng' ? 'ripng' : 'rip'}${suffix}`;
  }
  private async read(reader: string, rawOffset = '0', rawLimit = '100') {
    const offset = Number(rawOffset),
      limit = Number(rawLimit);
    if (
      !/^\d+$/.test(rawOffset) ||
      !/^\d+$/.test(rawLimit) ||
      !Number.isSafeInteger(offset) ||
      !Number.isSafeInteger(limit) ||
      limit < 1 ||
      limit > 100
    )
      throw new ProblemError(
        400,
        'invalid-query',
        'Invalid paging',
        'offset must be nonnegative and limit 1–100',
      );
    return IsisRipStateOut.parse(
      observed(
        await this.agent.routingState({ readers: [reader], ribPrefixes: [], ribVrf: '' }),
        reader,
        offset,
        limit,
      ),
    );
  }
}
