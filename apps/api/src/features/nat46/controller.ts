import { Controller, Get, Query } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import type { z } from 'zod';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { Nat46ClientOut, Nat46ClientQuery, Nat46Out } from './dto.js';
import { Nat46Service } from './service.js';

/**
 * NAT46 (F-nat46). Configuration (`nat.nat46`) goes through the generic `/api/v1/config/nat` pointer routes; NAT46 is
 * stateless, so this controller only reads the running configuration (`/state/**`, rule 8) — no sessions, no action.
 */
@ApiTags('nat')
@Controller('api/v1')
export class Nat46Controller {
  constructor(private readonly nat46: Nat46Service) {}

  @Get('state/nat/nat46')
  @Protected(503)
  @ApiOperation({
    summary:
      'NAT46 mappings of the running configuration with their VPP map domain names (stateless: nothing else to read)',
  })
  @ApiOkResponse({ schema: openapi(Nat46Out, 'output') })
  get() {
    return this.nat46.nat46();
  }

  @Get('state/nat/nat46/client')
  @Protected(400, 503)
  @ApiOperation({
    summary:
      'The IPv6 source address an IPv4 client appears as on the IPv6 side (RFC 6052: running clientPrefix + the IPv4 address)',
  })
  @ApiQuery({ name: 'ipv4', required: true, schema: { type: 'string', format: 'ipv4' } })
  @ApiOkResponse({ schema: openapi(Nat46ClientOut, 'output') })
  client(@Query(new ZodPipe(Nat46ClientQuery)) q: z.output<typeof Nat46ClientQuery>) {
    return this.nat46.client(q.ipv4);
  }
}
