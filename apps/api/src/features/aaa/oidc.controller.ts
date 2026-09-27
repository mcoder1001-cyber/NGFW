import { Controller, Get, Inject, Query, Req, Res } from '@nestjs/common';
import { ApiOperation, ApiResponse, ApiTags } from '@nestjs/swagger';
import type { FastifyReply } from 'fastify';
import { z } from 'zod';
import { AuthService } from '../../auth/auth.service.js';
import { setSessionCookies } from '../../auth/cookies.js';
import { NoAudit, Public } from '../../auth/decorators.js';
import { sourceIp, type VrxRequest } from '../../common/principal.js';
import { ProblemError } from '../../common/problem.js';
import { ApiOut } from '../../common/responses.js';
import { ENV, type Env } from '../../config.js';

const MethodsOut = z.object({
  oidc: z
    .boolean()
    .describe('OpenID Connect single sign-on is offered (GET /api/v1/auth/oidc/start)'),
});

/**
 * F-aaa-login: browser single sign-on with OpenID Connect. Public by nature (no session exists yet): `start`
 * redirects to the IdP, `callback` comes back from it. The callback never renders secrets or tokens into a page:
 * with a session it sets the cookies and redirects to `/`; when the MFA policy applies it redirects to the login page
 * with the single-use challenge in the URL fragment (never sent to a server); on failure only an error slug.
 */
@ApiTags('auth')
@Controller('api/v1/auth')
export class OidcController {
  constructor(
    private readonly auth: AuthService,
    @Inject(ENV) private readonly env: Env,
  ) {}

  @Get('methods')
  @Public()
  @ApiOperation({ summary: 'Login methods the login page offers besides the password form' })
  @ApiOut(MethodsOut)
  async methods() {
    return { oidc: await this.auth.oidcEnabled() };
  }

  @Get('oidc/start')
  @Public()
  @NoAudit()
  @ApiOperation({ summary: 'Start an OpenID Connect login: 302 to the identity provider' })
  @ApiResponse({ status: 302, description: 'redirect to the IdP authorisation endpoint' })
  async start(@Req() req: VrxRequest, @Res() reply: FastifyReply) {
    try {
      const url = await this.auth.oidcStart(sourceIp(req));
      return reply.redirect(url, 302);
    } catch (e) {
      return reply.redirect(`/login#error=${slug(e)}`, 302);
    }
  }

  @Get('oidc/callback')
  @Public()
  @NoAudit()
  @ApiOperation({
    summary:
      'OpenID Connect redirect target: sets the session cookie (or hands over to the MFA step)',
  })
  @ApiResponse({ status: 302, description: 'redirect to the web UI' })
  async callback(
    @Query('code') code: string | undefined,
    @Query('state') state: string | undefined,
    @Req() req: VrxRequest,
    @Res() reply: FastifyReply,
  ) {
    try {
      const r = await this.auth.oidcCallback(
        typeof code === 'string' ? code : undefined,
        typeof state === 'string' ? state : undefined,
        sourceIp(req),
      );
      if ('mfaRequired' in r) {
        return reply.redirect(`/login#mfa=${r.challenge}&enrolled=${r.enrolled ? 1 : 0}`, 302);
      }
      setSessionCookies(
        reply,
        {
          refreshToken: r.refreshToken,
          accessToken: r.accessToken,
          refreshMaxAge: r.refreshMaxAge,
          accessMaxAge: r.expiresIn,
        },
        { secure: this.env.VRX_COOKIE_SECURE },
      );
      return reply.redirect('/', 302);
    } catch (e) {
      return reply.redirect(`/login#error=${slug(e)}`, 302);
    }
  }
}

/** Only a fixed slug reaches the browser (never an exception message). */
function slug(e: unknown): string {
  if (e instanceof ProblemError) {
    if (e.slug === 'no-role-mapping') return 'no-role-mapping';
    if (e.slug === 'rate-limited') return 'rate-limited';
    if (e.slug === 'not-found') return 'oidc-not-configured';
    if (e.slug === 'unavailable') return 'oidc-unavailable';
  }
  return 'oidc-failed';
}
