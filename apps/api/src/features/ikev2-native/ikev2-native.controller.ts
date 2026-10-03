import { Body, Controller, Get, Inject, Param, Post, Query } from '@nestjs/common';
import { ApiBody, ApiOkResponse, ApiOperation, ApiTags, ApiParam, ApiQuery } from '@nestjs/swagger';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { MinRole } from '../../auth/decorators.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { ipsecSasOf, SasOut } from '../ipsec/ipsec.controller.js';

const Tunnel = z
  .string()
  .min(1)
  .max(63)
  .regex(/^[A-Za-z0-9_.-]+$/);
const Operation = z.enum(['initiate', 'rekey', 'delete-sa']);
export const NativeIkeActionInput = z.strictObject({
  ikeSpi: z
    .string()
    .regex(/^[0-9]{1,20}$/)
    .refine((s) => BigInt(s) <= 18446744073709551615n)
    .default('0'),
  childSpi: z.int().min(0).max(4294967295).default(0),
});
const State = z.strictObject({ tunnel: Tunnel.optional() });

@ApiTags('vpn')
@Controller('api/v1')
export class Ikev2NativeController {
  constructor(@Inject(AgentClient) private readonly agent: AgentClient) {}

  @Get('state/ipsec/ikev2/sas')
  @Protected(400, 502, 503)
  @ApiQuery({ name: 'tunnel', required: false, schema: { type: 'string' } })
  @ApiOperation({ summary: 'Native route-based IPsec SAs, scoped to owned profiles; no keys' })
  @ApiOkResponse({ schema: openapi(SasOut, 'output') })
  async sas(@Query(new ZodPipe(State)) q: z.output<typeof State>) {
    return ipsecSasOf(await this.agent.ipsecState(q.tunnel ? [q.tunnel] : []));
  }

  @Post('actions/ipsec/ikev2/:tunnel/:operation')
  @ApiParam({ name: 'tunnel', required: true, schema: { type: 'string' } })
  @ApiParam({ name: 'operation', required: true, enum: ['initiate', 'rekey', 'delete-sa'] })
  @MinRole('admin')
  @Protected(400, 404, 409, 502, 503)
  @ApiBody({ schema: openapi(NativeIkeActionInput, 'input') })
  @ApiOkResponse({ description: 'Native IKE runtime action accepted' })
  @ApiOperation({ summary: 'Initiate, rekey an owned CHILD SA, or delete an owned IKE SA' })
  async action(
    @Param('tunnel', new ZodPipe(Tunnel)) tunnel: string,
    @Param('operation', new ZodPipe(Operation)) operation: z.output<typeof Operation>,
    @Body(new ZodPipe(NativeIkeActionInput)) body: z.output<typeof NativeIkeActionInput>,
  ) {
    return this.agent.runAction({
      ikev2: { tunnel, operation, ikeSpi: body.ikeSpi, childSpi: body.childSpi },
    });
  }
}
