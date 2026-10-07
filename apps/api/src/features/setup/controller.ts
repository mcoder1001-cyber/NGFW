import { Body, Controller, HttpCode, Post, Req } from '@nestjs/common';
import { ApiBody, ApiOperation, ApiTags } from '@nestjs/swagger';
import { RootConfig, SetupInputSchema, InterfaceSchema, buildSetup, diff } from '@ngfw/schema';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { AuditUnavailableDoc } from '../../audit/audit.interceptor.js';
import { AuthService } from '../../auth/auth.service.js';
import { hashPassword } from '../../auth/password.js';
import { secureTransport } from '../../auth/transport.js';
import { MinRole } from '../../auth/decorators.js';
import type { NgfwRequest } from '../../common/principal.js';
import { ProblemError, problems } from '../../common/problem.js';
import { ApiOut, Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { newPassword } from '../../users/password-policy.js';

const Preview = z.strictObject({
  input: SetupInputSchema,
  baseRevision: z.number().int().nonnegative(),
  completedAt: z.iso.datetime(),
});
const Stage = Preview.extend({
  current: z.string().min(1).max(1024).meta({ writeOnly: true }),
  password: newPassword().meta({ writeOnly: true }),
});
@ApiTags('setup')
@Controller('api/v1/config/setup')
export class SetupController {
  constructor(
    private readonly ds: DatastoreService,
    private readonly auth: AuthService,
    private readonly agent: AgentClient,
  ) {}

  private async proposed(body: z.infer<typeof Preview>) {
    const running = await this.ds.getRunning();
    if ((running.revision?.id ?? 0) !== body.baseRevision)
      throw problems.conflict('setup-stale', 'running configuration changed; reload setup');
    const at = Date.parse(body.completedAt);
    if (Math.abs(Date.now() - at) > 3600_000)
      throw problems.badRequest('setup summary expired; preview again');
    try {
      const base = RootConfig.parse(running.doc);
      const selected = [body.input.wan, body.input.lan];
      if (
        selected.some(
          (name) => name === 'local0' || base.interfaces[name]?.physical?.owner === 'host',
        )
      )
        throw problems.badRequest('Select dataplane interfaces');
      const source = structuredClone(base);
      const missing = selected.filter((name) => !source.interfaces[name]);
      if (missing.length > 0) {
        const live = await this.agent.interfaceState(missing);
        for (const name of missing) {
          const state = live.interfaces.find(
            (item) =>
              item.name === name &&
              !item.parent &&
              item.type !== 'sub-interface' &&
              item.swIfIndex !== 0,
          );
          if (!state) throw problems.badRequest('Select existing dataplane interfaces');
          source.interfaces[name] = InterfaceSchema.parse({ vrf: state.vrf });
        }
      }
      const doc = buildSetup(source, body.input, body.completedAt);
      await this.ds.validateSetupPreview(doc);
      return { base, doc };
    } catch (error) {
      if (error instanceof ProblemError) throw error;
      throw problems.badRequest(error instanceof Error ? error.message : 'invalid setup');
    }
  }

  @Post('preview')
  @HttpCode(200)
  @MinRole('admin')
  @Protected(400, 409)
  @ApiBody({ schema: openapi(Preview) })
  @ApiOperation({
    summary: 'Preview the exact setup diff without editing candidate or applying configuration',
  })
  @ApiOut(
    z.object({
      changes: z.array(
        z.object({
          op: z.enum(['add', 'remove', 'replace']),
          pointer: z.string(),
          from: z.unknown().optional(),
          to: z.unknown().optional(),
          redacted: z.boolean().optional(),
        }),
      ),
    }),
  )
  async preview(
    @Body(new ZodPipe(Preview)) body: z.infer<typeof Preview>,
    @Req() req: NgfwRequest,
  ) {
    const { base, doc } = await this.proposed(body);
    req.audit = { resource: 'setup/preview', after: { baseRevision: body.baseRevision } };
    return {
      changes: [
        ...diff(base, doc),
        {
          op: 'replace',
          pointer: `/management/users/username=${req.principal!.username}/passwordHash`,
          redacted: true,
        },
      ],
    };
  }

  @Post('stage')
  @HttpCode(200)
  @MinRole('admin')
  @Protected(400, 403, 409)
  @ApiBody({ schema: openapi(Stage) })
  @AuditUnavailableDoc()
  @ApiOperation({
    summary:
      'Stage setup and policy-checked own password together; use normal validate and confirmed commit',
  })
  @ApiOut(z.object({ staged: z.literal(true) }))
  async stage(@Body(new ZodPipe(Stage)) body: z.infer<typeof Stage>, @Req() req: NgfwRequest) {
    await this.auth.checkSetupPassword(req.principal!, body.current, secureTransport(req));
    const { doc } = await this.proposed(body);
    const candidate = await this.ds.diff();
    if (candidate.changes.length > 0)
      throw problems.conflict(
        'setup-candidate',
        'discard or commit existing candidate before setup',
      );
    const own = doc.management.users.find((u) => u.username === req.principal!.username);
    if (!own || own.role !== 'admin' || own.disabled)
      throw problems.forbidden('setup requires a configured local administrator');
    if (body.password === body.current)
      throw problems.badRequest('choose a different administrator password');
    own.passwordHash = await hashPassword(body.password);
    await this.ds.stageSetup(req.principal!, doc, body.baseRevision);
    req.audit = { resource: 'setup/stage', after: { staged: true, passwordStaged: true } };
    return { staged: true as const };
  }
}
