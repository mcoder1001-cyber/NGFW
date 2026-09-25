import {
  Body,
  Controller,
  Delete,
  Get,
  HttpCode,
  Inject,
  Param,
  Post,
  Req,
  Res,
} from '@nestjs/common';
import {
  ApiBody,
  ApiCookieAuth,
  ApiNoContentResponse,
  ApiOperation,
  ApiResponse,
  ApiTags,
} from '@nestjs/swagger';
import type { FastifyReply } from 'fastify';
import { z } from 'zod';
import { ENV, type Env } from '../config.js';
import { ApiOut, Protected, PublicDoc } from '../common/responses.js';
import { sourceIp, type VrxRequest } from '../common/principal.js';
import { ProblemError } from '../common/problem.js';
import { openapi, ref, SafeParamPipe, ZodPipe } from '../common/zod.js';
import { safeText } from '../common/text.js';
import { ROLES } from '../db/schema.js';
import { AuthService, type LoginResult } from './auth.service.js';
import { UsersService } from '../users/users.service.js';
import { AuditUnavailableDoc } from '../audit/audit.interceptor.js';
import { CommitBusyDoc } from '../users/commit-busy.js';
import { clearSessionCookies, REFRESH_COOKIE, setSessionCookies } from './cookies.js';
import { MinRole, NoAudit, Public } from './decorators.js';
import { secureTransport } from './transport.js';

export { REFRESH_COOKIE } from './cookies.js';
const TLS_REQUIRED =
  '`tls-required` (D-100): the password was sent over plain HTTP from a remote peer — connect through https; the attempt is not counted as a failed login';

const LoginBody = z.strictObject({
  username: safeText(64).min(1),
  password: z.string().min(1).max(1024),
});
const PasswordBody = z.strictObject({
  current: z.string().min(1).max(1024),
  password: z.string().min(12).max(1024),
});
const ApiKeyBody = z.strictObject({
  name: safeText(64).min(1),
  /** Role cap; the effective role is never above the owner's. */
  role: z.enum(ROLES).optional(),
  expiresInDays: z.number().int().min(1).max(3650).optional(),
  /** D-100 (2), TD-4: step-up — the same shape as PasswordBody.current. */
  current: z
    .string()
    .min(1)
    .max(1024)
    .optional()
    .describe(
      'the caller’s current password (write-only) — required when the caller is a login (Bearer/JWT) session: TLS only, rate-limited per account together with password changes (429), a wrong one counts toward the login lockout. Not allowed with `Authorization: ApiKey` (400 `current-not-allowed-with-api-key`, never checked)',
    ),
});

const UserOut = z.object({ id: z.number().int(), username: z.string(), role: z.enum(ROLES) });
const SessionOut = z.object({
  accessToken: z.string(),
  tokenType: z.literal('Bearer'),
  expiresIn: z.number().int().describe('seconds'),
  user: UserOut,
});
const MeOut = z.object({
  id: z.number().int(),
  username: z.string(),
  role: z.enum(ROLES),
  lastLogin: z.string().nullable(),
  effectiveRole: z.enum(ROLES),
  via: z.enum(['jwt', 'apikey']),
});
const ApiKeyOut = z.object({
  id: z.string(),
  name: z.string(),
  role: z.enum(ROLES).nullable(),
  expiresAt: z.string().nullable(),
  lastUsed: z.string().nullable(),
  createdAt: z.string(),
});
const ApiKeyCreated = z.object({
  id: z.string(),
  name: z.string(),
  role: z.enum(ROLES),
  expiresAt: z.string().nullable(),
  key: z.string().describe('shown once — store it now'),
});

/**
 * Authentication (P06 §6). Login and refresh answer with the access token in the body and set the rotating refresh
 * token as an httpOnly, SameSite=Strict cookie scoped to /api/v1/auth. Credentials never reach the audit log.
 */
@ApiTags('auth')
@Controller('api/v1/auth')
export class AuthController {
  constructor(
    private readonly auth: AuthService,
    private readonly users: UsersService,
    @Inject(ENV) private readonly env: Env,
  ) {}

  /**
   * The refresh cookie (lifetime: the idle TTL capped by the session's end, TD-10b review 2.3d) and the docs cookie
   * (the access token for /api/docs, TD-10b review 2.3g) — auth/cookies.ts.
   */
  private setRefresh(
    reply: FastifyReply,
    r: LoginResult,
  ): Omit<LoginResult, 'refreshToken' | 'refreshMaxAge'> {
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
    return {
      accessToken: r.accessToken,
      tokenType: r.tokenType,
      expiresIn: r.expiresIn,
      user: r.user,
    };
  }

  @Post('login')
  @Public()
  @NoAudit()
  @HttpCode(200)
  @ApiOperation({ summary: 'Log in with a local user; sets the refresh cookie' })
  @ApiBody({ schema: openapi(LoginBody) })
  @ApiOut(SessionOut)
  @ApiResponse({
    status: 403,
    description: TLS_REQUIRED,
    content: { 'application/problem+json': { schema: ref('Problem') } },
  })
  @PublicDoc(400, 401, 429)
  async login(
    @Body(new ZodPipe(LoginBody)) body: z.output<typeof LoginBody>,
    @Req() req: VrxRequest,
    @Res({ passthrough: true }) reply: FastifyReply,
  ) {
    return this.setRefresh(
      reply,
      await this.auth.login(body.username, body.password, sourceIp(req), secureTransport(req)),
    );
  }

  @Post('refresh')
  @Public()
  @NoAudit()
  @HttpCode(200)
  @ApiCookieAuth('refreshCookie')
  @ApiOperation({ summary: 'Rotate the refresh cookie and get a new access token' })
  @ApiOut(SessionOut)
  @PublicDoc(401)
  async refresh(@Req() req: VrxRequest, @Res({ passthrough: true }) reply: FastifyReply) {
    return this.setRefresh(
      reply,
      await this.auth.refresh(req.cookies[REFRESH_COOKIE], sourceIp(req)),
    );
  }

  @Post('logout')
  @Public()
  @NoAudit()
  @HttpCode(204)
  @ApiCookieAuth('refreshCookie')
  @ApiOperation({ summary: 'Revoke the refresh cookie (and its rotation family)' })
  @ApiNoContentResponse({
    description:
      'Logged out: the login session of the refresh cookie (or of a Bearer token sent along) ends — its refresh chain and its access tokens; the user’s other sessions stay. Also 204 when there was no valid session',
  })
  async logout(
    @Req() req: VrxRequest,
    @Res({ passthrough: true }) reply: FastifyReply,
  ): Promise<void> {
    // TD-10b (review 2.3c/2.3e): per-session revocation, audited in the service (the route is @NoAudit)
    await this.auth.logout(req.cookies[REFRESH_COOKIE], req.headers.authorization, sourceIp(req));
    clearSessionCookies(reply);
  }

  @Get('me')
  @Protected()
  @ApiOperation({ summary: 'The authenticated user' })
  @ApiOut(MeOut)
  me(@Req() req: VrxRequest) {
    return this.auth.me(req.principal!);
  }

  /** Self-service password change — the one non-GET route open to readonly users (own credentials only). */
  @Post('password')
  @MinRole('readonly')
  @HttpCode(204)
  @Protected(400, 429)
  @CommitBusyDoc()
  @AuditUnavailableDoc()
  @ApiOperation({
    summary:
      'Change the own password (argon2id); same as POST /api/v1/users/{name}/password for yourself',
  })
  @ApiNoContentResponse({ description: 'Password changed' })
  @ApiBody({ schema: openapi(PasswordBody) })
  async password(
    @Body(new ZodPipe(PasswordBody)) body: z.output<typeof PasswordBody>,
    @Req() req: VrxRequest,
  ) {
    // TD-2 #1: one implementation with POST /api/v1/users/{name}/password (serialised with commits, stored
    // candidate/pending hashes replaced, the other sessions end — this one survives)
    req.audit = { resource: `user/${req.principal!.username}` };
    await this.users.setPassword(
      req.principal!,
      req.principal!.username,
      { password: body.password, current: body.current },
      req,
    );
    req.audit = { resource: `user/${req.principal!.username}`, after: { passwordChanged: true } };
  }

  @Get('api-keys')
  @Protected()
  @ApiOperation({ summary: 'API keys of the authenticated user' })
  @ApiOut(z.array(ApiKeyOut))
  apiKeys(@Req() req: VrxRequest) {
    return this.auth.listApiKeys(req.principal!);
  }

  @Post('api-keys')
  @ApiResponse({
    status: 403,
    description:
      'Step-up (D-100): `tls-required` — a password in the body over plain HTTP from a remote peer; `forbidden` — wrong `current` (counts toward the login lockout); `locked` — the account is locked (no key is created)',
    content: { 'application/problem+json': { schema: ref('Problem') } },
  })
  @ApiResponse({
    status: 429,
    description:
      '`rate-limited`: the account’s per-minute budget of current-password checks (VRX_PASSWORD_RATE_PER_MIN, shared with password changes) is spent — nothing was checked',
    content: { 'application/problem+json': { schema: ref('Problem') } },
  })
  @Protected(400)
  @AuditUnavailableDoc()
  @ApiOperation({
    summary: 'Create an API key (`Authorization: ApiKey <key>`); the key is shown once',
  })
  @ApiBody({ schema: openapi(ApiKeyBody) })
  @ApiOut(ApiKeyCreated)
  async createApiKey(
    @Body(new ZodPipe(ApiKeyBody)) body: z.output<typeof ApiKeyBody>,
    @Req() req: VrxRequest,
  ) {
    const via = req.principal!.via;
    // D-100 (2): the audit row says which credential created the key — never the password
    req.audit = { after: { name: body.name, via } };
    let created: Awaited<ReturnType<AuthService['createApiKey']>>;
    try {
      created = await this.auth.createApiKey(
        req.principal!,
        body.name,
        body.role,
        body.expiresInDays,
        { current: body.current, secure: secureTransport(req) },
      );
    } catch (e) {
      // TD-4 review M1: a failed creation says why — the problem slug only (never the detail, never the password)
      if (e instanceof ProblemError)
        req.audit = { after: { name: body.name, via, reason: e.slug } };
      throw e;
    }
    req.audit = {
      resource: `api-key/${created.id}`,
      after: { name: created.name, role: created.role, via },
    };
    return created;
  }

  @Delete('api-keys/:id')
  @HttpCode(204)
  @Protected(400, 404)
  @AuditUnavailableDoc()
  @ApiOperation({ summary: 'Delete an API key (own; admin: any)' })
  @ApiNoContentResponse({ description: 'Deleted' })
  async deleteApiKey(
    @Param('id', new SafeParamPipe('id', 64)) id: string,
    @Req() req: VrxRequest,
  ): Promise<void> {
    req.audit = { resource: `api-key/${id}` };
    await this.auth.deleteApiKey(req.principal!, id);
  }
}
