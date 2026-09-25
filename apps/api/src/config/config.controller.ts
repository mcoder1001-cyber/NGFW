import {
  Body,
  Controller,
  Delete,
  Get,
  Header,
  HttpCode,
  Param,
  Patch,
  Post,
  Put,
  Query,
  Req,
  Res,
} from '@nestjs/common';
import { ApiBody, ApiOkResponse, ApiOperation, ApiParam, ApiQuery, ApiTags } from '@nestjs/swagger';
import type { FastifyReply } from 'fastify';
import { z } from 'zod';
import { MinRole } from '../auth/decorators.js';
import { getAt } from '../common/json.js';
import { problems } from '../common/problem.js';
import type { VrxRequest } from '../common/principal.js';
import { ApiOut, Protected, type SameKeys } from '../common/responses.js';
import { openapi, ref, ZodPipe } from '../common/zod.js';
import { safeText } from '../common/text.js';
import { CommitService } from '../commit/commit.service.js';
import { DatastoreService, type EditResult } from '../datastore/datastore.service.js';
import type { Revision, RevisionMeta, SyncStatus } from '../datastore/repo.js';
import { markSecretChanges } from '../datastore/documents.js';
import { pointerFromUrl } from './path.js';

const PREFIX = '/api/v1/config';

const CommitQuery = z.object({
  confirm: z.coerce
    .number()
    .int()
    .min(1)
    .max(3600)
    .optional()
    .describe('auto-revert after this many seconds unless confirmed'),
  comment: safeText(1024).optional(),
});
const PageQuery = z.object({
  limit: z.coerce.number().int().min(1).max(500).default(50),
  offset: z.coerce.number().int().min(0).default(0),
});
const Rev = z.coerce.number().int().min(1);

const ChangeOut = z.object({
  op: z.enum(['add', 'remove', 'replace']),
  pointer: z.string(),
  from: z.unknown().optional(),
  to: z.unknown().optional(),
  redacted: z
    .literal(true)
    .optional()
    .describe(
      'a write-only (secret) leaf changed; no from/to — the value is never shown (TD-2 #6)',
    ),
});
const SecretChangeOut = z.object({
  op: z.enum(['add', 'remove', 'replace']),
  pointer: z.string(),
  redacted: z.literal(true),
});
const LockOut = z.object({
  locked: z.boolean(),
  owner: z.string().nullable(),
  ownerId: z.number().int().nullable(),
  ownerKeyId: z
    .string()
    .nullable()
    .describe('API key holding the lock (each key is its own owner); null = interactive sessions'),
  ownerKey: z.string().nullable().describe('name of that API key'),
  lockedAt: z.string().nullable(),
  lastActivity: z.string().nullable(),
  expiresAt: z.string().nullable(),
});
const Issue = z.object({ pointer: z.string(), message: z.string(), rule: z.string().optional() });
const PlanOut = z.object({
  key: z.string(),
  op: z.string(),
  pointer: z.string(),
  subsystem: z.string(),
});
const RevisionMetaOut = z.object({
  id: z.number().int(),
  createdAt: z.string(),
  authorId: z.number().int().nullable(),
  author: z.string().nullable(),
  comment: z.string(),
  parentId: z.number().int().nullable(),
  hash: z.string(),
  txnId: z.string().nullable(),
  kind: z.string(),
  secretChanges: z
    .array(SecretChangeOut)
    .describe('secret leaves this revision changed against its parent, without values (TD-2 #6)'),
});
const RevisionOut = RevisionMetaOut.extend({
  payload: z
    .record(z.string(), z.unknown())
    .describe('the configuration document (redacted, D-046)'),
  secretVersions: z
    .record(z.string(), z.number().int())
    .nullable()
    .optional()
    .describe(
      'secret versions pinned by this revision, `<kind>/<name>` → version (no values; review M2); a rollback re-activates them',
    ),
});
// ARCH-04 (TD-15): the documented DTOs and what the handlers return must not drift apart (compile-time).
const _revisionMetaKeys: SameKeys<RevisionMeta, typeof RevisionMetaOut> = true;
const _revisionKeys: SameKeys<Revision, typeof RevisionOut> = true;
void _revisionMetaKeys;
void _revisionKeys;
const SyncOut = z.object({
  state: z.enum(['in-sync', 'unknown', 'degraded']),
  reason: z.string(),
  txnId: z.string().nullable(),
  since: z.string(),
});
const _syncKeys: SameKeys<SyncStatus, typeof SyncOut> = true;
void _syncKeys;
const CommitOut = z.object({
  status: z.enum([
    'applied',
    'partially-applied',
    'not-applied',
    'pending',
    'unchanged',
    'confirmed',
  ]),
  txnId: z.string().optional(),
  revision: RevisionMetaOut.optional(),
  confirmDeadline: z.string().optional(),
  results: z.array(
    z.object({
      key: z.string(),
      op: z.string(),
      code: z.string(),
      message: z.string(),
      pointer: z.string(),
      subsystem: z.string(),
    }),
  ),
  summary: z.record(z.string(), z.number()).optional(),
  warnings: z.array(Issue),
  notApplied: z.array(z.string()),
  sync: SyncOut.optional(),
});
const EditOut = z.object({
  pointer: z.string(),
  before: z.unknown(),
  after: z.unknown(),
  discardedStaleCandidateOf: z.string().optional(),
  secretChanges: z.array(SecretChangeOut).optional(),
  ignoredSecrets: z
    .array(z.string())
    .optional()
    .describe('import: password hashes in the document that were ignored (D-097)'),
});
const PendingOut = z.object({
  pending: z
    .object({
      txnId: z.string(),
      author: z.number().int().nullable(),
      comment: z.string(),
      kind: z.string(),
      deadline: z.string(),
      createdAt: z.string(),
    })
    .nullable(),
});

const pathParam = ApiParam({
  name: 'path',
  description:
    'JSON pointer into the document without the leading slash, first segment a top-level key; `/` inside a name is `~1` (e.g. `interfaces/TenGigabitEthernet0~10~10/mtu`)',
  schema: { type: 'string' },
});

/**
 * `/api/v1/config/**` — transactional configuration (00-CONTEXT rule 8, docs/04 endpoint families). GET/PATCH/PUT/
 * DELETE on any pointer into the candidate work for every schema domain without new code (P06 §5). Every
 * document that leaves here is redacted (D-046/D-070).
 */
@ApiTags('config')
@Controller('api/v1/config')
export class ConfigController {
  constructor(
    private readonly ds: DatastoreService,
    private readonly commits: CommitService,
  ) {}

  private edited(req: VrxRequest, r: EditResult, resource = r.pointer || '/'): EditResult {
    // TD-2 #6: a changed secret leaf is marked in the audit row (never its value)
    const marked = markSecretChanges(r.pointer, r.before, r.after, r.secretChanges ?? []);
    req.audit = { resource, before: marked.before, after: marked.after };
    return r;
  }

  @Get()
  @Protected()
  @ApiOperation({ summary: 'Whole running configuration (redacted)' })
  @ApiOkResponse({ schema: ref('RootConfig') })
  async running(@Res({ passthrough: true }) reply: FastifyReply) {
    const r = await this.ds.getRunning();
    void reply.header('x-vrx-revision', String(r.revision?.id ?? 0));
    return r.doc;
  }

  @Get('candidate')
  @Protected()
  @ApiOperation({
    summary: 'Whole candidate configuration (redacted); equals running when nobody edits',
  })
  @ApiOkResponse({ schema: ref('RootConfig') })
  candidate() {
    return this.ds.getCandidate();
  }

  @Get('candidate/*')
  @Protected(404)
  @pathParam
  @ApiOperation({ summary: 'Node of the candidate at a JSON pointer' })
  @ApiOkResponse({ description: 'Any JSON value (the node)', schema: {} })
  async candidateAt(@Req() req: VrxRequest) {
    const pointer = pointerFromUrl(req.url, `${PREFIX}/candidate`);
    return this.found(getAt(await this.ds.getCandidate(), pointer), pointer);
  }

  @Get('diff')
  @Protected()
  @ApiOperation({
    summary: 'Structured candidate ↔ running diff (RFC 6902-style ops with JSON pointers)',
  })
  @ApiOut(z.object({ baseRevision: z.number().int().nullable(), changes: z.array(ChangeOut) }))
  diff() {
    return this.ds.diff();
  }

  @Get('lock')
  @Protected()
  @ApiOperation({ summary: 'Candidate lock (single writer)' })
  @ApiOut(LockOut)
  lock() {
    return this.ds.lock();
  }

  @Delete('lock')
  @MinRole('admin')
  @Protected()
  @ApiOperation({ summary: 'Admin: break the lock of another user (the candidate is discarded)' })
  @ApiOut(LockOut)
  async breakLock(@Req() req: VrxRequest) {
    const before = await this.ds.breakLock();
    req.audit = { resource: 'lock', before };
    return before;
  }

  @Post('validate')
  @HttpCode(200)
  @Protected(400, 503)
  @ApiOperation({
    summary:
      'Three-tier validation of the candidate (schema → semantic → agent DryRun); nothing is applied',
  })
  @ApiOut(
    z.object({
      ok: z.literal(true),
      warnings: z.array(Issue),
      plan: z.array(PlanOut),
      notApplied: z.array(z.string()),
    }),
  )
  validate(@Req() req: VrxRequest) {
    return this.commits.validateCandidate(req.principal!);
  }

  @Post('commit')
  @HttpCode(200)
  @Protected(400, 409, 422, 502, 503)
  @ApiQuery({
    name: 'confirm',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 3600 },
  })
  @ApiQuery({ name: 'comment', required: false, schema: openapi(safeText(1024)) })
  @ApiOperation({
    summary:
      'Validate and apply the candidate; with ?confirm=<sec> the agent reverts unless confirmed',
  })
  @ApiOut(CommitOut)
  async commit(
    @Query(new ZodPipe(CommitQuery)) q: z.output<typeof CommitQuery>,
    @Req() req: VrxRequest,
  ) {
    const r = await this.commits.commit(req.principal!, {
      ...(q.confirm !== undefined ? { confirmSec: q.confirm } : {}),
      ...(q.comment !== undefined ? { comment: q.comment } : {}),
    });
    req.audit = {
      resource: 'commit',
      after: { status: r.status, txnId: r.txnId, revision: r.revision?.id },
    };
    return r;
  }

  @Post('commit/confirm')
  @HttpCode(200)
  @Protected(409, 502, 503)
  @ApiOperation({
    summary: 'Confirm the pending commit (cancels the auto-revert) and persist its revision',
  })
  @ApiOut(CommitOut)
  async confirm(@Req() req: VrxRequest) {
    const r = await this.commits.confirm(req.principal!);
    req.audit = { resource: 'commit/confirm', after: { txnId: r.txnId, revision: r.revision?.id } };
    return r;
  }

  @Get('commit/pending')
  @Protected()
  @ApiOperation({ summary: 'The commit waiting for confirmation, if any' })
  @ApiOut(PendingOut)
  async pending() {
    return { pending: await this.commits.pendingInfo() };
  }

  @Post('discard')
  @HttpCode(200)
  @Protected(409)
  @ApiOperation({ summary: 'Drop the candidate and release the lock' })
  @ApiOut(z.object({ discarded: z.boolean() }))
  async discard(@Req() req: VrxRequest) {
    const r = await this.ds.discard(req.principal!);
    req.audit = { resource: 'candidate', after: r };
    return r;
  }

  @Get('revisions')
  @Protected()
  @ApiQuery({
    name: 'limit',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 500 },
  })
  @ApiQuery({ name: 'offset', required: false, schema: { type: 'integer', minimum: 0 } })
  @ApiOperation({ summary: 'Revision history, newest first' })
  @ApiOut(z.object({ items: z.array(RevisionMetaOut), total: z.number().int() }))
  revisions(@Query(new ZodPipe(PageQuery)) q: z.output<typeof PageQuery>) {
    return this.ds.listRevisions(q.limit, q.offset);
  }

  @Get('revisions/:rev')
  @Protected(404)
  @ApiOperation({ summary: 'One revision with its (redacted) payload' })
  @ApiOut(RevisionOut)
  revision(@Param('rev', new ZodPipe(Rev)) rev: number) {
    return this.ds.getRevision(rev);
  }

  @Get('revisions/:rev/diff')
  @Protected(404)
  @ApiOperation({
    summary:
      'What revision {rev} changed against its parent (redacted; secret leaves as `redacted: true` entries)',
  })
  @ApiOut(
    z.object({
      revision: z.number().int(),
      parent: z.number().int().nullable(),
      changes: z.array(ChangeOut),
    }),
  )
  revisionDiff(@Param('rev', new ZodPipe(Rev)) rev: number) {
    return this.ds.revisionDiff(rev);
  }

  @Post('rollback/:rev')
  @HttpCode(200)
  @Protected(400, 404, 409, 422, 502, 503)
  @ApiQuery({
    name: 'confirm',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 3600 },
  })
  @ApiQuery({ name: 'comment', required: false, schema: openapi(safeText(1024)) })
  @ApiOperation({ summary: 'Apply an old revision as a new revision (payload = the old one)' })
  @ApiOut(CommitOut)
  async rollback(
    @Param('rev', new ZodPipe(Rev)) rev: number,
    @Query(new ZodPipe(CommitQuery)) q: z.output<typeof CommitQuery>,
    @Req() req: VrxRequest,
  ) {
    const r = await this.commits.rollback(req.principal!, rev, {
      ...(q.confirm !== undefined ? { confirmSec: q.confirm } : {}),
      ...(q.comment !== undefined ? { comment: q.comment } : {}),
    });
    req.audit = {
      resource: `rollback/${rev}`,
      after: { status: r.status, txnId: r.txnId, revision: r.revision?.id },
    };
    return r;
  }

  @Get('export')
  @Protected()
  @Header('content-disposition', 'attachment; filename="vrx-config.json"')
  @ApiOperation({ summary: 'Running configuration as a download (redacted)' })
  @ApiOkResponse({ schema: ref('RootConfig') })
  async export() {
    return (await this.ds.getRunning()).doc;
  }

  @Post('import')
  @HttpCode(200)
  @Protected(400, 409)
  @ApiOperation({ summary: 'Replace the candidate with a document (schema-checked; not applied)' })
  @ApiBody({ schema: ref('RootConfig') })
  @ApiOut(EditOut)
  async import(@Body() body: unknown, @Req() req: VrxRequest) {
    return this.edited(req, await this.ds.importCandidate(req.principal!, body), 'import');
  }

  // ---- whole document edits ----

  @Patch()
  @Protected(400, 403, 409)
  @ApiOperation({ summary: 'RFC 7386 merge patch of the whole candidate' })
  @ApiBody({ schema: { type: 'object' } })
  @ApiOut(EditOut)
  async patchRoot(@Body() body: unknown, @Req() req: VrxRequest) {
    return this.edited(req, await this.ds.patchCandidate(req.principal!, '', body));
  }

  // ---- generic pointer routes (last: static routes above win) ----

  @Get('*')
  @Protected(404)
  @pathParam
  @ApiOperation({ summary: 'Node of the running configuration at a JSON pointer' })
  @ApiOkResponse({ description: 'Any JSON value (the node)', schema: {} })
  async runningAt(@Req() req: VrxRequest) {
    const pointer = pointerFromUrl(req.url, PREFIX);
    return this.found(getAt((await this.ds.getRunning()).doc, pointer), pointer);
  }

  @Patch('*')
  @Protected(400, 403, 409)
  @pathParam
  @ApiOperation({ summary: 'RFC 7386 merge patch of the candidate node at a JSON pointer' })
  @ApiBody({ schema: {} })
  @ApiOut(EditOut)
  async patchAt(@Body() body: unknown, @Req() req: VrxRequest) {
    const pointer = pointerFromUrl(req.url, PREFIX);
    return this.edited(req, await this.ds.patchCandidate(req.principal!, pointer, body));
  }

  @Put('*')
  @Protected(400, 403, 409)
  @pathParam
  @ApiOperation({ summary: 'Replace the candidate node at a JSON pointer' })
  @ApiBody({ schema: {} })
  @ApiOut(EditOut)
  async putAt(@Body() body: unknown, @Req() req: VrxRequest) {
    const pointer = pointerFromUrl(req.url, PREFIX);
    if (body === undefined) throw problems.badRequest('PUT needs a JSON body');
    return this.edited(req, await this.ds.putCandidate(req.principal!, pointer, body));
  }

  @Delete('*')
  @Protected(400, 403, 404, 409)
  @pathParam
  @ApiOperation({ summary: 'Remove the candidate node at a JSON pointer' })
  @ApiOut(EditOut)
  async deleteAt(@Req() req: VrxRequest) {
    const pointer = pointerFromUrl(req.url, PREFIX);
    return this.edited(req, await this.ds.deleteCandidate(req.principal!, pointer));
  }

  private found(value: unknown, pointer: string): unknown {
    if (value === undefined) throw problems.notFound(`nothing at '${pointer}'`);
    return value;
  }
}
