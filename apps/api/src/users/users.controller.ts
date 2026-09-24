import { Body, Controller, HttpCode, Param, Post, Req } from '@nestjs/common';
import { ApiBody, ApiOkResponse, ApiOperation, ApiParam, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { MinRole } from '../auth/decorators.js';
import type { VrxRequest } from '../common/principal.js';
import { Protected } from '../common/responses.js';
import { openapi, SafeParamPipe, ZodPipe } from '../common/zod.js';
import { UsersService } from './users.service.js';

const SetPasswordBody = z.strictObject({
  password: z
    .string()
    .min(12)
    .max(1024)
    .describe('the new password (write-only; hashed with argon2id)'),
  current: z
    .string()
    .min(1)
    .max(1024)
    .optional()
    .describe('the caller’s current password — required when setting one’s own password'),
  keepApiKeys: z
    .boolean()
    .optional()
    .describe(
      'admin reset only: keep the target’s API keys (service users); by default an admin reset revokes them (D-097)',
    ),
});
const SetPasswordOut = z.object({
  self: z.boolean(),
  apiKeysRevoked: z
    .array(z.object({ id: z.string(), name: z.string() }))
    .describe('API keys of the target revoked by an admin reset'),
  discardedCandidate: z
    .boolean()
    .describe(
      'true when one of the revoked keys held the candidate lock: its staged (uncommitted) edits were discarded',
    ),
});

/**
 * `/api/v1/users` — user credentials that do not belong in the configuration document (TD-2 #1). The users
 * themselves (name, role, disabled) stay in `management.users` and are changed through the candidate.
 */
@ApiTags('users')
@Controller('api/v1/users')
export class UsersController {
  constructor(private readonly users: UsersService) {}

  /**
   * An admin sets any user's password; everybody (readonly too) sets their own with the current password. Audited
   * without the value; rate-limited per caller; ends the target's other sessions.
   */
  @Post(':name/password')
  @MinRole('readonly')
  @HttpCode(200)
  @Protected(400, 404, 429)
  @ApiParam({ name: 'name', description: 'username', schema: { type: 'string' } })
  @ApiOperation({
    summary:
      "Set a user's password (admin: any user; everyone: their own, with `current`). TLS only; argon2id server-side; ends the user's other sessions; an admin reset also revokes the user's API keys unless keepApiKeys",
  })
  @ApiBody({ schema: openapi(SetPasswordBody) })
  @ApiOkResponse({ schema: openapi(SetPasswordOut, 'output') })
  async setPassword(
    @Param('name', new SafeParamPipe('name', 64)) name: string,
    @Body(new ZodPipe(SetPasswordBody)) body: z.output<typeof SetPasswordBody>,
    @Req() req: VrxRequest,
  ): Promise<z.output<typeof SetPasswordOut>> {
    req.audit = { resource: `user/${name}` };
    const r = await this.users.setPassword(req.principal!, name, body, req);
    req.audit = {
      resource: `user/${name}`,
      after: {
        passwordSet: true,
        self: r.self,
        apiKeysRevoked: r.apiKeysRevoked,
        // verify V5: the opt-out is recorded explicitly, with what it kept; dropped staged edits are recorded too
        ...(r.apiKeysKept !== undefined ? { keepApiKeys: true, apiKeysKept: r.apiKeysKept } : {}),
        ...(r.discardedCandidate ? { discardedCandidate: true } : {}),
        // verify V3: Valkey failed after the commit (the reset holds through the database generation)
        ...(r.revocationPersisted ? {} : { revocationPersisted: false }),
      },
    };
    return {
      self: r.self,
      apiKeysRevoked: r.apiKeysRevoked,
      discardedCandidate: r.discardedCandidate,
    };
  }
}
