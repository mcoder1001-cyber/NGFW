import {
  Body,
  Controller,
  Get,
  HttpCode,
  Param,
  Post,
  Put,
  Req,
  Res,
  StreamableFile,
} from '@nestjs/common';
import { ApiBody, ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import { UpgradeOp } from '@ngfw/proto';
import { z } from 'zod';
import { createWriteStream } from 'node:fs';
import { mkdir, realpath, unlink } from 'node:fs/promises';
import { randomUUID } from 'node:crypto';
import { Transform, type Readable } from 'node:stream';
import { pipeline } from 'node:stream/promises';
import type { FastifyReply } from 'fastify';
import { AgentClient } from '../../agent/agent.client.js';
import { MinRole } from '../../auth/decorators.js';
import type { NgfwRequest } from '../../common/principal.js';
import { problems } from '../../common/problem.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { BackupRestoreService } from './backup-restore.service.js';
import { renderTemplate } from './templates.js';
import { BackupScheduleService } from './schedule.js';
import { MAX_ARCHIVE } from './archive.js';

const BackupBody = z.strictObject({
  passphrase: z.string().min(12).max(1024),
  revisions: z.number().int().min(1).max(1000).default(100),
});
const RestoreBody = z.strictObject({
  passphrase: z.string().min(12).max(1024),
  archive: z
    .string()
    .max(Math.ceil(MAX_ARCHIVE / 3) * 4)
    .regex(/^[A-Za-z0-9+/]*={0,2}$/),
});
const TemplateBody = z.strictObject({
  description: z.string().max(1024).default(''),
  parameters: z
    .record(
      z.string(),
      z.strictObject({
        type: z.enum(['string', 'number', 'boolean']),
        required: z.boolean().default(true),
      }),
    )
    .default({}),
  patch: z.record(z.string(), z.unknown()),
});
const ApplyBody = z.strictObject({
  parameters: z.record(z.string(), z.union([z.string(), z.number(), z.boolean()])),
});
const UpgradeBody = z.strictObject({
  op: z.enum(['status', 'stage', 'activate', 'confirm', 'rollback']),
  bundle: z.string().max(256).optional(),
});
const upgradeOps = {
  status: UpgradeOp.UPGRADE_OP_STATUS,
  stage: UpgradeOp.UPGRADE_OP_STAGE,
  activate: UpgradeOp.UPGRADE_OP_ACTIVATE,
  confirm: UpgradeOp.UPGRADE_OP_CONFIRM,
  rollback: UpgradeOp.UPGRADE_OP_ROLLBACK,
};
const StagedOut = z.object({ staged: z.boolean() });
const StagedDiffOut = StagedOut.extend({
  diff: z.object({
    baseRevision: z.number().nullable(),
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
});

@ApiTags('backup-restore')
@Controller('api/v1')
@MinRole('admin')
export class BackupRestoreController {
  constructor(
    private readonly backup: BackupRestoreService,
    private readonly ds: DatastoreService,
    private readonly agent: AgentClient,
    private readonly schedule: BackupScheduleService,
  ) {}
  @Post('actions/backup')
  @HttpCode(200)
  @Protected(400, 403, 409)
  @ApiOperation({ summary: 'Download a passphrase-encrypted full configuration backup' })
  @ApiBody({ schema: openapi(BackupBody) })
  @ApiOkResponse({ schema: { type: 'string', format: 'binary' } })
  async download(
    @Body(new ZodPipe(BackupBody)) body: z.output<typeof BackupBody>,
    @Req() req: NgfwRequest,
    @Res({ passthrough: true }) reply: FastifyReply,
  ) {
    const bytes = await this.backup.backup(req.principal!, body.passphrase, body.revisions);
    req.audit = { resource: 'actions/backup', after: { revisions: body.revisions } };
    void reply.header(
      'content-disposition',
      `attachment; filename="ngfw-backup-${Date.now()}.ngfwbackup"`,
    );
    return new StreamableFile(bytes, { type: 'application/octet-stream' });
  }
  @Post('actions/restore')
  @HttpCode(200)
  @Protected(400, 403, 409)
  @ApiOperation({
    summary: 'Authenticate a backup and stage document and inactive secret versions',
  })
  @ApiBody({ schema: openapi(RestoreBody) })
  @ApiOkResponse({ schema: openapi(StagedDiffOut) })
  async restore(
    @Body(new ZodPipe(RestoreBody)) body: z.output<typeof RestoreBody>,
    @Req() req: NgfwRequest,
  ) {
    const result = await this.backup.restore(
      req.principal!,
      Buffer.from(body.archive, 'base64'),
      body.passphrase,
    );
    req.audit = { resource: 'actions/restore', after: { staged: true } };
    return result;
  }
  @Get('config-templates')
  @Protected(403)
  @ApiOperation({ summary: 'List candidate configuration templates' })
  @ApiOkResponse({
    schema: {
      type: 'object',
      properties: {
        items: { type: 'object', additionalProperties: openapi(TemplateBody, 'output') },
      },
    },
  })
  async templates() {
    const doc = await this.ds.getCandidate();
    const stored =
      (
        doc['management'] as {
          templates?: Record<
            string,
            { description: string; parameters: unknown; patchJson: string }
          >;
        }
      )?.templates ?? {};
    return {
      items: Object.fromEntries(
        Object.entries(stored).map(([name, t]) => [
          name,
          { description: t.description, parameters: t.parameters, patch: JSON.parse(t.patchJson) },
        ]),
      ),
    };
  }
  @Get('config-templates/:name')
  @Protected(400, 403, 404)
  @ApiOperation({ summary: 'Read a candidate configuration template' })
  @ApiOkResponse({ schema: openapi(TemplateBody, 'output') })
  async template(@Param('name') name: string) {
    this.name(name);
    const all = await this.templates();
    const out = (all.items as Record<string, unknown>)[name];
    if (!out) throw problems.notFound('template does not exist');
    return TemplateBody.parse(out);
  }
  @Put('config-templates/:name')
  @Protected(400, 403, 409)
  @ApiOperation({ summary: 'Stage a configuration template in the candidate' })
  @ApiBody({ schema: openapi(TemplateBody) })
  @ApiOkResponse({ schema: openapi(StagedOut) })
  async putTemplate(
    @Param('name') name: string,
    @Body(new ZodPipe(TemplateBody)) body: z.output<typeof TemplateBody>,
    @Req() req: NgfwRequest,
  ) {
    this.name(name);
    renderTemplate(
      body,
      Object.fromEntries(
        Object.entries(body.parameters).map(([key, spec]) => [
          key,
          spec.type === 'string' ? 'validation' : spec.type === 'number' ? 1 : true,
        ]),
      ),
    );
    await this.ds.putCandidate(req.principal!, `/management/templates/${name}`, {
      description: body.description,
      parameters: body.parameters,
      patchJson: JSON.stringify(body.patch),
    });
    req.audit = { resource: `config-templates/${name}`, after: { staged: true } };
    return { staged: true };
  }
  @Post('config-templates/:name/apply')
  @HttpCode(200)
  @Protected(400, 403, 404, 409)
  @ApiOperation({ summary: 'Render typed parameters and merge template patch into candidate' })
  @ApiBody({ schema: openapi(ApplyBody) })
  @ApiOkResponse({ schema: openapi(StagedDiffOut) })
  async apply(
    @Param('name') name: string,
    @Body(new ZodPipe(ApplyBody)) body: z.output<typeof ApplyBody>,
    @Req() req: NgfwRequest,
  ) {
    const template = await this.template(name);
    const patch = renderTemplate(template, body.parameters);
    await this.ds.patchCandidate(req.principal!, '', patch);
    req.audit = { resource: `config-templates/${name}/apply`, after: { staged: true } };
    return { staged: true, diff: await this.ds.diff() };
  }
  @Get('state/backup')
  @Protected(403)
  @ApiOperation({ summary: 'Read recent scheduled export outcomes' })
  @ApiOkResponse({
    schema: openapi(
      z.object({
        runs: z.array(
          z.object({
            at: z.string(),
            result: z.string(),
            filename: z.string().optional(),
            error: z.string().optional(),
          }),
        ),
      }),
      'output',
    ),
  })
  async runs() {
    return { runs: await this.schedule.history() };
  }
  @Post('actions/support-bundle')
  @HttpCode(200)
  @Protected(403, 503)
  @ApiOperation({ summary: 'Download a redacted read-only support summary' })
  @ApiOkResponse({ schema: { type: 'string', format: 'binary' } })
  async support(@Req() req: NgfwRequest, @Res({ passthrough: true }) reply: FastifyReply) {
    const bytes = await this.backup.support();
    req.audit = { resource: 'actions/support-bundle', after: { collected: true } };
    void reply.header('content-disposition', 'attachment; filename="ngfw-support.json"');
    return new StreamableFile(bytes, { type: 'application/json' });
  }
  @Post('actions/upgrade')
  @HttpCode(200)
  @Protected(400, 403, 503)
  @ApiOperation({ summary: 'Run an allow-listed appliance upgrade operation through the agent' })
  @ApiBody({ schema: openapi(UpgradeBody) })
  @ApiOkResponse({
    schema: openapi(
      z.object({
        lines: z.array(z.string()),
        done: z
          .object({
            summary: z.string(),
            exitCode: z.number(),
            stats: z.record(z.string(), z.string()),
          })
          .optional(),
      }),
      'output',
    ),
  })
  async upgrade(
    @Body(new ZodPipe(UpgradeBody)) body: z.output<typeof UpgradeBody>,
    @Req() req: NgfwRequest,
  ) {
    if (body.op === 'stage' ? !body.bundle : body.bundle !== undefined)
      throw problems.badRequest('only stage requires a bundle');
    req.audit = { resource: 'actions/upgrade', after: { op: body.op } };
    return this.agent.runAction(
      { upgrade: { op: upgradeOps[body.op], bundle: body.bundle ?? '' } },
      body.op === 'stage' ? 900000 : 60000,
    );
  }
  @Post('actions/upgrade-upload')
  @HttpCode(200)
  @Protected(400, 403, 409)
  @ApiOperation({
    summary: 'Stream a signed update bundle to the dedicated updates directory (maximum 8 GiB)',
  })
  @ApiBody({ schema: { type: 'string', format: 'binary' } })
  @ApiOkResponse({ schema: openapi(z.object({ bundle: z.string() }), 'output') })
  async upload(@Body() body: Readable, @Req() req: NgfwRequest) {
    if (!/^ngfw-update-\d+\.\d+\.\d+\.tar$/.test(String(req.headers['x-ngfw-filename'] ?? '')))
      throw problems.badRequest('invalid upgrade filename');
    const directory = process.env['NGFW_UPDATES_DIR'] ?? '/data/updates';
    await mkdir(directory, { recursive: true, mode: 0o700 });
    if ((await realpath(directory)) !== directory)
      throw problems.unavailable('updates directory must not contain symlinks');
    const path = `${directory}/ngfw-update-${randomUUID()}.tar`;
    let size = 0;
    const bound = new Transform({
      transform(chunk: Buffer, _encoding, callback) {
        size += chunk.length;
        callback(
          size > 8 * 1024 * 1024 * 1024 ? problems.badRequest('update exceeds 8 GiB') : null,
          chunk,
        );
      },
    });
    try {
      await pipeline(body, bound, createWriteStream(path, { flags: 'wx', mode: 0o600 }));
      if (size === 0) throw problems.badRequest('empty update');
    } catch (e) {
      await unlink(path).catch(() => undefined);
      throw e;
    }
    req.audit = { resource: 'actions/upgrade-upload', after: { bytes: size } };
    return { bundle: path };
  }
  private name(name: string): void {
    if (
      !/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$/.test(name) ||
      ['constructor', 'prototype', '__proto__'].includes(name)
    )
      throw problems.badRequest('invalid template name', [
        { pointer: '/name', message: 'safe name required' },
      ]);
  }
}
