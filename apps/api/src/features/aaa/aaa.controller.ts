import { Body, Controller, HttpCode, Post, Req } from '@nestjs/common';
import {
  ApiBody,
  ApiNoContentResponse,
  ApiOkResponse,
  ApiOperation,
  ApiTags,
} from '@nestjs/swagger';
import { z } from 'zod';
import { AuditUnavailableDoc } from '../../audit/audit.interceptor.js';
import { MinRole } from '../../auth/decorators.js';
import type { VrxRequest } from '../../common/principal.js';
import { Protected } from '../../common/responses.js';
import { problems } from '../../common/problem.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { AaaService } from './aaa.service.js';
import { MfaService } from './mfa.service.js';

const TestBody = z.object({
  method: z.enum(['radius', 'ldap', 'tacacs']),
  username: z.string().min(1).max(255),
  password: z.string().min(1).max(1024),
});
const MfaResetBody = z.strictObject({ username: z.string().min(1).max(64) });
const TestOut = z.object({
  method: z.string(),
  reachable: z.boolean(),
  authenticated: z.boolean(),
  groups: z.array(z.string()),
  role: z.enum(['admin', 'operator', 'readonly']).nullable(),
  detail: z.string(),
});

/**
 * F-aaa admin routes: validate a configured external auth backend by authenticating a test credential against it (no
 * session is issued), and reset a user's second factor. The login-order walk and MFA enforcement themselves live in
 * `AuthService` (F-aaa-login).
 */
@ApiTags('aaa')
@Controller('api/v1/actions/aaa')
export class AaaController {
  constructor(
    private readonly aaa: AaaService,
    private readonly mfa: MfaService,
  ) {}

  @Post('test')
  @HttpCode(200)
  @MinRole('admin')
  @Protected(400, 403, 501)
  @ApiOperation({
    summary: 'Admin: test an external AAA backend with a credential (no session issued)',
  })
  @ApiOkResponse({ schema: openapi(TestOut, 'output') })
  async test(@Body(new ZodPipe(TestBody)) body: z.output<typeof TestBody>, @Req() req: VrxRequest) {
    const r = await this.aaa.test(body.method, body.username, body.password);
    req.audit = {
      resource: 'actions/aaa/test',
      after: { method: body.method, reachable: r.reachable, authenticated: r.authenticated },
    };
    return r;
  }

  /**
   * Admin: clear a user's second factor so they can enrol again — the way out of a lost authenticator with no recovery
   * code left. It grants nothing on its own: where the policy requires MFA, the user's next login asks for enrolment.
   */
  @Post('mfa/reset')
  @AuditUnavailableDoc()
  @HttpCode(204)
  @MinRole('admin')
  @Protected(403, 404)
  @ApiOperation({ summary: 'Admin: reset a user’s MFA enrolment' })
  @ApiNoContentResponse({ description: 'MFA cleared for that user' })
  @ApiBody({ schema: openapi(MfaResetBody) })
  async mfaReset(
    @Body(new ZodPipe(MfaResetBody)) body: z.output<typeof MfaResetBody>,
    @Req() req: VrxRequest,
  ) {
    req.audit = { resource: `user/${body.username}`, after: { mfa: 'reset' } };
    if (!(await this.mfa.resetFor(body.username)))
      throw problems.notFound(`no user '${body.username}'`);
  }
}
