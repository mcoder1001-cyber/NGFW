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
  ApiOkResponse,
  ApiOperation,
  ApiTags,
} from '@nestjs/swagger';
import type { FastifyReply } from 'fastify';
import { z } from 'zod';
import { ENV, type Env } from '../config.js';
import { Protected, PublicDoc } from '../common/responses.js';
import { sourceIp, type VrxRequest } from '../common/principal.js';
import { openapi, ZodPipe } from '../common/zod.js';
import { ROLES } from '../db/schema.js';
import { AuthService, type LoginResult } from './auth.service.js';
import { MinRole, NoAudit, Public } from './decorators.js';

export const REFRESH_COOKIE = 'vrx_refresh';
const COOKIE_PATH = '/api/v1/auth';

const LoginBody = z.strictObject({
  username: z.string().min(1).max(64),
  password: z.string().min(1).max(1024),
});
const PasswordBody = z.strictObject({
  current: z.string().min(1).max(1024),
  password: z.string().min(12).max(1024),
});
const ApiKeyBody = z.strictObject({
  name: z.string().min(1).max(64),
  /** Role cap; the effective role is never above the owner's. */
  role: z.enum(ROLES).optional(),
  expiresInDays: z.number().int().min(1).max(3650).optional(),
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
    @Inject(ENV) private readonly env: Env,
  ) {}

  private setRefresh(reply: FastifyReply, r: LoginResult): Omit<LoginResult, 'refreshToken'> {
    void reply.setCookie(REFRESH_COOKIE, r.refreshToken, {
      httpOnly: true,
      sameSite: 'strict',
      secure: this.env.VRX_COOKIE_SECURE,
      path: COOKIE_PATH,
      maxAge: this.env.VRX_REFRESH_TTL_SEC,
    });
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
  @ApiOkResponse({ schema: openapi(SessionOut, 'output') })
  @PublicDoc(400, 401, 429)
  async login(
    @Body(new ZodPipe(LoginBody)) body: z.output<typeof LoginBody>,
    @Req() req: VrxRequest,
    @Res({ passthrough: true }) reply: FastifyReply,
  ) {
    return this.setRefresh(
      reply,
      await this.auth.login(body.username, body.password, sourceIp(req)),
    );
  }

  @Post('refresh')
  @Public()
  @NoAudit()
  @HttpCode(200)
  @ApiCookieAuth('refreshCookie')
  @ApiOperation({ summary: 'Rotate the refresh cookie and get a new access token' })
  @ApiOkResponse({ schema: openapi(SessionOut, 'output') })
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
  @ApiNoContentResponse({ description: 'Logged out (also when the cookie was already invalid)' })
  async logout(
    @Req() req: VrxRequest,
    @Res({ passthrough: true }) reply: FastifyReply,
  ): Promise<void> {
    await this.auth.logout(req.cookies[REFRESH_COOKIE]);
    void reply.clearCookie(REFRESH_COOKIE, { path: COOKIE_PATH });
  }

  @Get('me')
  @Protected()
  @ApiOperation({ summary: 'The authenticated user' })
  @ApiOkResponse({ schema: openapi(MeOut, 'output') })
  me(@Req() req: VrxRequest) {
    return this.auth.me(req.principal!);
  }

  /** Self-service password change — the one non-GET route open to readonly users (own credentials only). */
  @Post('password')
  @MinRole('readonly')
  @HttpCode(204)
  @Protected(400)
  @ApiOperation({ summary: 'Change the own password (argon2id)' })
  @ApiNoContentResponse({ description: 'Password changed' })
  @ApiBody({ schema: openapi(PasswordBody) })
  async password(
    @Body(new ZodPipe(PasswordBody)) body: z.output<typeof PasswordBody>,
    @Req() req: VrxRequest,
  ) {
    req.audit = { resource: `user/${req.principal!.username}`, after: { passwordChanged: true } };
    await this.auth.changePassword(req.principal!, body.current, body.password);
  }

  @Get('api-keys')
  @Protected()
  @ApiOperation({ summary: 'API keys of the authenticated user' })
  @ApiOkResponse({ schema: openapi(z.array(ApiKeyOut), 'output') })
  apiKeys(@Req() req: VrxRequest) {
    return this.auth.listApiKeys(req.principal!);
  }

  @Post('api-keys')
  @Protected(400)
  @ApiOperation({
    summary: 'Create an API key (`Authorization: ApiKey <key>`); the key is shown once',
  })
  @ApiBody({ schema: openapi(ApiKeyBody) })
  @ApiOkResponse({ schema: openapi(ApiKeyCreated, 'output') })
  async createApiKey(
    @Body(new ZodPipe(ApiKeyBody)) body: z.output<typeof ApiKeyBody>,
    @Req() req: VrxRequest,
  ) {
    const created = await this.auth.createApiKey(
      req.principal!,
      body.name,
      body.role,
      body.expiresInDays,
    );
    req.audit = {
      resource: `api-key/${created.id}`,
      after: { name: created.name, role: created.role },
    };
    return created;
  }

  @Delete('api-keys/:id')
  @HttpCode(204)
  @Protected(404)
  @ApiOperation({ summary: 'Delete an API key (own; admin: any)' })
  @ApiNoContentResponse({ description: 'Deleted' })
  async deleteApiKey(@Param('id') id: string, @Req() req: VrxRequest): Promise<void> {
    req.audit = { resource: `api-key/${id}` };
    await this.auth.deleteApiKey(req.principal!, id);
  }
}
