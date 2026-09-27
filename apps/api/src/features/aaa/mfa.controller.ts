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
import { ApiBody, ApiNoContentResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import type { FastifyReply } from 'fastify';
import { z } from 'zod';
import { SessionOut } from '../../auth/auth.controller.js';
import { AuthService } from '../../auth/auth.service.js';
import { setSessionCookies } from '../../auth/cookies.js';
import { MinRole, NoAudit, Public } from '../../auth/decorators.js';
import { secureTransport } from '../../auth/transport.js';
import { sourceIp, type VrxRequest } from '../../common/principal.js';
import { ApiOut, Protected, PublicDoc } from '../../common/responses.js';
import { openapi, SafeParamPipe, ZodPipe } from '../../common/zod.js';
import { ENV, type Env } from '../../config.js';

const Challenge = z
  .string()
  .regex(/^[A-Za-z0-9_-]{43}$/)
  .describe('the `challenge` of the login answer');

const VerifyBody = z.union([
  z.strictObject({ challenge: Challenge, code: z.string().regex(/^[0-9]{6}$/) }),
  z.strictObject({
    challenge: Challenge,
    recoveryCode: z
      .string()
      .regex(/^[0-9a-fA-F]{10}$/)
      .describe('one of the recovery codes shown at enrolment (single use)'),
  }),
]);
/** D-159: the one-time enrolment token an administrator issued for this user. */
const EnrolToken = z
  .string()
  .regex(/^[A-Za-z0-9_-]{32}$/)
  .describe('the one-time MFA enrolment token issued by an administrator (write-only)');
const EnrollBody = z.strictObject({ challenge: Challenge, token: EnrolToken });
const SetupBody = z.strictObject({
  current: z
    .string()
    .min(1)
    .max(1024)
    .describe('the caller’s current password (step-up; write-only)'),
  token: EnrolToken,
});
const ActivateBody = z.strictObject({ code: z.string().regex(/^[0-9]{6}$/) });

const EnrolmentOut = z.object({
  secret: z.string().describe('base32 TOTP secret — shown once; add it to an authenticator app'),
  otpauthUri: z.string().describe('the same secret as an otpauth:// URI (for a QR code)'),
});
const VerifiedOut = SessionOut.extend({
  recoveryCodes: z
    .array(z.string())
    .optional()
    .describe('only when this step enabled a new factor: single-use recovery codes, shown once'),
});
const StatusOut = z.object({
  enrolled: z.boolean(),
  recoveryCodesLeft: z.number().int(),
  required: z.boolean().describe('management.aaa.mfa.required covers the caller’s role'),
});

/**
 * F-aaa-login: the TOTP second factor. The two login-step routes are public and authorised by the single-use login
 * challenge (no session exists yet); set-up, status and the admin reset need a session. Secrets and recovery codes
 * appear only in the one response that creates them — never in the audit rows (req.audit is set explicitly).
 */
@ApiTags('auth')
@Controller('api/v1/auth/mfa')
export class MfaController {
  constructor(
    private readonly auth: AuthService,
    @Inject(ENV) private readonly env: Env,
  ) {}

  @Post('verify')
  @Public()
  @NoAudit()
  @HttpCode(200)
  @ApiOperation({
    summary:
      'Second login step: a TOTP code or a recovery code for the login challenge; sets the refresh cookie',
  })
  @ApiBody({ schema: openapi(VerifyBody) })
  @ApiOut(VerifiedOut)
  @PublicDoc(400, 401, 429)
  async verify(
    @Body(new ZodPipe(VerifyBody)) body: z.output<typeof VerifyBody>,
    @Req() req: VrxRequest,
    @Res({ passthrough: true }) reply: FastifyReply,
  ) {
    const r = await this.auth.mfaVerify(
      body.challenge,
      'code' in body ? { code: body.code } : { recoveryCode: body.recoveryCode },
      sourceIp(req),
      secureTransport(req),
    );
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
      ...(r.recoveryCodes === undefined ? {} : { recoveryCodes: r.recoveryCodes }),
    };
  }

  @Post('enroll')
  @Public()
  @NoAudit()
  @HttpCode(200)
  @ApiOperation({
    summary:
      'Login-time enrolment: the MFA policy requires a factor this user has not set up (challenge with enrolled=false); the secret is shown once',
  })
  @ApiBody({ schema: openapi(EnrollBody) })
  @ApiOut(EnrolmentOut)
  @PublicDoc(400, 401)
  enroll(@Body(new ZodPipe(EnrollBody)) body: z.output<typeof EnrollBody>, @Req() req: VrxRequest) {
    return this.auth.mfaEnrollWithChallenge(
      body.challenge,
      body.token,
      sourceIp(req),
      secureTransport(req),
    );
  }

  @Get()
  @Protected()
  @ApiOperation({ summary: 'The caller’s second-factor state' })
  @ApiOut(StatusOut)
  status(@Req() req: VrxRequest) {
    return this.auth.mfaStatus(req.principal!);
  }

  @Post('setup')
  @MinRole('readonly')
  @HttpCode(200)
  @Protected(400, 403, 409, 429)
  @ApiOperation({
    summary:
      'Start a voluntary TOTP enrolment from a login session (current password required); the secret is shown once',
  })
  @ApiBody({ schema: openapi(SetupBody) })
  @ApiOut(EnrolmentOut)
  async setup(
    @Body(new ZodPipe(SetupBody)) body: z.output<typeof SetupBody>,
    @Req() req: VrxRequest,
  ) {
    req.audit = { resource: `user/${req.principal!.username}`, after: { step: 'setup' } };
    return this.auth.mfaSetup(req.principal!, body.current, body.token, secureTransport(req));
  }

  @Post('activate')
  @MinRole('readonly')
  @HttpCode(200)
  @Protected(400, 403, 429)
  @ApiOperation({
    summary:
      'Enable the factor being set up with its first code; the user’s other sessions end; recovery codes are shown once',
  })
  @ApiBody({ schema: openapi(ActivateBody) })
  @ApiOut(z.object({ recoveryCodes: z.array(z.string()) }))
  async activate(
    @Body(new ZodPipe(ActivateBody)) body: z.output<typeof ActivateBody>,
    @Req() req: VrxRequest,
  ) {
    req.audit = { resource: `user/${req.principal!.username}`, after: { step: 'activate' } };
    return this.auth.mfaActivate(req.principal!, body.code);
  }

  @Post('users/:name/enrolment-token')
  @MinRole('admin')
  @HttpCode(200)
  @Protected(404, 409)
  @ApiOperation({
    summary:
      'Admin (D-159): issue a one-time MFA enrolment token for a user (shown once, 24 h; replaces an open one)',
  })
  @ApiOut(z.object({ token: z.string(), expiresIn: z.number().int() }))
  async issueToken(
    @Param('name', new SafeParamPipe('name', 64)) name: string,
    @Req() req: VrxRequest,
  ) {
    req.audit = { resource: `user/${name}`, after: { mfaEnrolmentToken: 'issued' } };
    return this.auth.issueEnrolmentToken(name);
  }

  @Delete('users/:name/enrolment-token')
  @MinRole('admin')
  @HttpCode(204)
  @Protected(404)
  @ApiOperation({ summary: 'Admin (D-159): revoke a user’s open MFA enrolment token' })
  @ApiNoContentResponse({ description: 'Revoked' })
  async revokeToken(
    @Param('name', new SafeParamPipe('name', 64)) name: string,
    @Req() req: VrxRequest,
  ): Promise<void> {
    req.audit = { resource: `user/${name}`, after: { mfaEnrolmentToken: 'revoked' } };
    await this.auth.revokeEnrolmentToken(name);
  }

  @Delete('users/:name')
  @MinRole('admin')
  @HttpCode(204)
  @Protected(404)
  @ApiOperation({
    summary: 'Admin: remove a user’s second factor (lost device); their sessions end',
  })
  @ApiNoContentResponse({ description: 'Removed' })
  async reset(
    @Param('name', new SafeParamPipe('name', 64)) name: string,
    @Req() req: VrxRequest,
  ): Promise<void> {
    req.audit = { resource: `user/${name}`, after: { mfaReset: true } };
    const r = await this.auth.mfaReset(name);
    req.audit = {
      resource: `user/${name}`,
      after: { mfaReset: true, apiKeysMfaCleared: r.apiKeysMfaCleared },
    };
  }
}
