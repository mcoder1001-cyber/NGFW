import { Controller, Get, HttpCode, Param, Post, Query, Req } from '@nestjs/common';
import { ApiOperation, ApiParam, ApiQuery, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { MinRole } from '../../auth/decorators.js';
import { ApiOut, Protected } from '../../common/responses.js';
import { ZodPipe } from '../../common/zod.js';
import { ProblemError, problems } from '../../common/problem.js';
import type { NgfwRequest } from '../../common/principal.js';

const Profile = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/);
const SessionId = z.string().regex(/^[a-f0-9]{64}$/);
const Page = z.strictObject({
  profile: Profile,
  cursor: z
    .string()
    .max(256)
    .regex(/^[A-Za-z0-9_-]*$/)
    .default(''),
  limit: z.coerce.number().int().min(1).max(100).default(50),
});
const DisconnectQuery = z.strictObject({ profile: Profile });
const Decimal = z
  .string()
  .regex(/^(0|[1-9][0-9]{0,19})$/)
  .refine((value) => BigInt(value) <= 18446744073709551615n);
export const RemoteAccessCapabilitiesOut = z
  .strictObject({
    engine: z.literal('strongswan-ra'),
    operational: z.boolean(),
    supportedAuth: z.array(z.enum(['eap-mschapv2', 'eap-tls', 'eap-radius', 'pubkey'])).max(4),
    reason: z.enum(['', 'engine-unavailable', 'engine-not-ready', 'runtime-unavailable']),
    editableDisabledDrafts: z.boolean(),
  })
  .refine((value) =>
    value.operational ? value.reason === '' && value.supportedAuth.length > 0 : value.reason !== '',
  );
export const RemoteAccessSessionsOut = z.strictObject({
  items: z
    .array(
      z.strictObject({
        id: SessionId,
        profile: Profile,
        identity: z.string().max(1024),
        addresses: z.array(z.string().max(64)).max(16),
        establishedSeconds: Decimal,
        bytesIn: Decimal,
        bytesOut: Decimal,
      }),
    )
    .max(100),
  nextCursor: z
    .string()
    .max(256)
    .regex(/^[A-Za-z0-9_-]*$/),
});
export const RemoteAccessDisconnectOut = z.strictObject({ disconnected: z.boolean() });

function observed<T>(schema: z.ZodType<T>, value: unknown): T {
  const result = schema.safeParse(value);
  if (!result.success)
    throw new ProblemError(
      502,
      'remote-access-observation-invalid',
      'Invalid agent observation',
      'The agent returned an invalid remote-access observation',
    );
  return result.data;
}

@ApiTags('vpn')
@Controller('api/v1')
export class RaVpnController {
  constructor(private readonly agent: AgentClient) {}

  @Get('state/vpn/remote-access/capabilities')
  @Protected(501, 502, 503)
  @ApiOut(RemoteAccessCapabilitiesOut)
  @ApiOperation({ summary: 'Observed independent remote-access engine capabilities' })
  async capabilities() {
    return observed(RemoteAccessCapabilitiesOut, await this.agent.remoteAccessCapabilities());
  }

  @Get('state/vpn/remote-access/sessions')
  @Protected(400, 409, 501, 502, 503)
  @ApiOut(RemoteAccessSessionsOut)
  @ApiQuery({ name: 'profile', required: true, schema: { type: 'string', maxLength: 63 } })
  @ApiQuery({ name: 'cursor', required: false, schema: { type: 'string', maxLength: 256 } })
  @ApiQuery({
    name: 'limit',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 100 },
  })
  @ApiOperation({ summary: 'Owned observed remote-access sessions, bounded cursor page' })
  async sessions(@Query(new ZodPipe(Page)) page: z.output<typeof Page>) {
    const response = await this.agent.remoteAccessSessions(page);
    const result = observed(RemoteAccessSessionsOut, {
      items: response.sessions,
      nextCursor: response.nextCursor,
    });
    if (result.items.some((item) => item.profile !== page.profile))
      throw new ProblemError(
        502,
        'remote-access-observation-invalid',
        'Invalid agent observation',
        'The agent returned sessions outside the requested profile',
      );
    return result;
  }

  @Post('actions/vpn/remote-access/sessions/:id/disconnect')
  @HttpCode(200)
  @MinRole('admin')
  @Protected(400, 403, 409, 501, 502, 503)
  @ApiOut(RemoteAccessDisconnectOut)
  @ApiParam({ name: 'id', required: true, schema: { type: 'string', pattern: '^[a-f0-9]{64}$' } })
  @ApiQuery({ name: 'profile', required: true, schema: { type: 'string', maxLength: 63 } })
  @ApiOperation({ summary: 'Disconnect an owned remote-access session and verify removal' })
  async disconnect(
    @Param('id', new ZodPipe(SessionId)) id: string,
    @Query(new ZodPipe(DisconnectQuery)) query: z.output<typeof DisconnectQuery>,
    @Req() req: NgfwRequest,
  ) {
    req.audit = {
      resource: `vpn/remoteAccess/${query.profile}/sessions/${id}`,
      after: { action: 'disconnect' },
    };
    const result = observed(
      RemoteAccessDisconnectOut,
      await this.agent.remoteAccessDisconnect({ profile: query.profile, id }),
    );
    if (!result.disconnected)
      throw problems.conflict(
        'remote-access-removal-unverified',
        'The agent has not verified removal of this session',
      );
    return result;
  }
}
