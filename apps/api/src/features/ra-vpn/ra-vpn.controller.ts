import { Controller, Get, Param, Post, Query } from '@nestjs/common';
import { ApiOperation, ApiParam, ApiQuery, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { MinRole } from '../../auth/decorators.js';
import { problems } from '../../common/problem.js';
import { ApiOut, Protected } from '../../common/responses.js';
import { ZodPipe } from '../../common/zod.js';

/** Release capability, not a probe of an unimplemented or retired daemon. */
export const RemoteAccessCapabilitiesOut = z.strictObject({
  supported: z.literal(false),
  engine: z.literal('vpp-ikev2'),
  reasonCode: z.literal('native-remote-access-unavailable'),
  authentication: z.literal(false),
  addressAssignment: z.literal(false),
  sessions: z.literal(false),
  disconnect: z.literal(false),
  disabledDrafts: z.literal(true),
});
const Page = z.strictObject({
  offset: z.coerce.number().int().min(0).max(1000000).default(0),
  limit: z.coerce.number().int().min(1).max(200).default(50),
});
const SessionId = z.string().min(1).max(128).regex(/^[A-Za-z0-9_.:-]+$/);
const unavailable = () => problems.notImplemented(
  'Remote-access EAP, virtual-address pools and sessions are unavailable on the approved native IKEv2 engine. No remote-access listener is running.',
);

@ApiTags('vpn')
@Controller('api/v1')
export class RaVpnController {
  @Get('state/vpn/remote-access/capabilities')
  @Protected()
  @ApiOut(RemoteAccessCapabilitiesOut)
  @ApiOperation({ summary: 'Remote-access VPN capability of this release; no operational support claimed' })
  capabilities(): z.output<typeof RemoteAccessCapabilitiesOut> {
    return RemoteAccessCapabilitiesOut.parse({
      supported: false, engine: 'vpp-ikev2', reasonCode: 'native-remote-access-unavailable',
      authentication: false, addressAssignment: false, sessions: false, disconnect: false, disabledDrafts: true,
    });
  }

  @Get('state/vpn/remote-access/sessions')
  @Protected(400, 501)
  @ApiQuery({ name: 'offset', required: false, schema: { type: 'integer', minimum: 0, maximum: 1000000 } })
  @ApiQuery({ name: 'limit', required: false, schema: { type: 'integer', minimum: 1, maximum: 200 } })
  @ApiOperation({ summary: 'Explicitly refuse unavailable remote-access session inventory' })
  sessions(@Query(new ZodPipe(Page)) _page: z.output<typeof Page>): never {
    throw unavailable();
  }

  @Post('actions/vpn/remote-access/sessions/:id/disconnect')
  @MinRole('admin')
  @Protected(400, 501)
  @ApiParam({ name: 'id', required: true, schema: { type: 'string', maxLength: 128 } })
  @ApiOperation({ summary: 'Explicitly refuse unavailable remote-access disconnect; no runtime mutation' })
  disconnect(@Param('id', new ZodPipe(SessionId)) _id: string): never {
    throw unavailable();
  }
}
