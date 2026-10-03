import { Body, Controller, Get, HttpCode, Param, Post, Query, Req, Res } from '@nestjs/common';
import {
  ApiBody,
  ApiConsumes,
  ApiOkResponse,
  ApiOperation,
  ApiParam,
  ApiProduces,
  ApiQuery,
  ApiTags,
} from '@nestjs/swagger';
import type { FastifyReply } from 'fastify';
import { z } from 'zod';
import { problems } from '../../common/problem.js';
import type { NgfwRequest } from '../../common/principal.js';
import { Protected } from '../../common/responses.js';
import { openapi, SafeParamPipe, ZodPipe } from '../../common/zod.js';
import { GlobalBlockingService } from './global-blocking.service.js';

const boolQ = z
  .enum(['true', 'false', '1', '0'])
  .transform((v) => v === 'true' || v === '1')
  .optional();
const DryRunQuery = z.object({
  dryRun: boolQ
    .default(true)
    .describe('true (default) = parse and preview only; false = stage into the candidate'),
});
const ExportQuery = z.object({ source: z.enum(['running', 'candidate']).default('running') });

const FetchStateOut = z.object({
  lastFetchAt: z.string(),
  lastResult: z.enum(['ok', 'unchanged', 'failed', 'deferred']),
  lastError: z.string().nullable(),
  etag: z.string().optional(),
  lastModified: z.string().optional(),
  entries: z.number().int().optional(),
});
const StatusOut = z.object({
  maxEntries: z.number().int(),
  totalEntries: z.number().int().describe('entries over all lists (candidate)'),
  countersError: z.string().nullable().describe('why hits are null (agent unavailable)'),
  lists: z.array(
    z.object({
      name: z.string(),
      description: z.string().nullable(),
      enabled: z.boolean(),
      pending: z.enum(['added', 'changed', 'deleted']).nullable().describe('candidate vs running'),
      source: z.object({
        kind: z.enum(['upload', 'url']),
        url: z.string().nullable(),
        refreshSec: z.number().int().nullable(),
      }),
      allInterfaces: z.boolean(),
      interfaces: z.array(z.string()),
      direction: z.string(),
      protectHost: z.boolean(),
      entries: z.number().int().describe('candidate'),
      runningEntries: z.number().int(),
      fetch: FetchStateOut.nullable().describe('the last download (URL sources)'),
      nextRefreshAt: z.string().nullable(),
      hits: z
        .object({
          dataplanePackets: z.number(),
          dataplaneBytes: z.number(),
          hostPackets: z.number().describe('drops of traffic to the box (protectHost)'),
        })
        .nullable(),
    }),
  ),
});
const PreviewOut = z.object({
  list: z.string(),
  dryRun: z.boolean(),
  lines: z.number().int(),
  entries: z
    .number()
    .int()
    .describe('valid entries after normalising, deduplicating and collapsing'),
  added: z.number().int(),
  removed: z.number().int(),
  unchanged: z.number().int(),
  normalised: z.number().int().describe('entries whose host bits were masked'),
  collapsed: z.number().int().describe('duplicates and entries covered by a wider one'),
  invalidCount: z.number().int(),
  invalid: z
    .array(z.object({ line: z.number().int(), text: z.string(), reason: z.string() }))
    .describe('first 200 invalid lines (skipped)'),
  addedSample: z.array(z.string()).describe('first 100'),
  removedSample: z.array(z.string()).describe('first 100'),
  staged: z.boolean().describe('the entries were written to the candidate'),
});

/**
 * F-global-blocking routes. Status (lists, last download, next refresh, hit counters), import of an uploaded file and
 * "Fetch now" from the list's server URL (both preview by default; `dryRun=false` stages the entries into the
 * candidate — the normal commit applies them), and export in the same text format. The list settings themselves go
 * through the generic pointer routes (`/api/v1/config/acl/globalBlocking/…`).
 */
@ApiTags('global-blocking')
@Controller('api/v1/security/global-blocking')
export class GlobalBlockingController {
  constructor(private readonly gb: GlobalBlockingService) {}

  @Get()
  @Protected()
  @ApiOperation({
    summary: 'Block lists with source, last download, next refresh and hit counters',
  })
  @ApiOkResponse({ schema: openapi(StatusOut, 'output') })
  status() {
    return this.gb.status();
  }

  @Post('lists/:name/import')
  @HttpCode(200)
  @Protected(400, 403, 404, 409)
  @ApiParam({ name: 'name', schema: { type: 'string' } })
  @ApiQuery({ name: 'dryRun', required: false, schema: { type: 'boolean', default: true } })
  @ApiConsumes('text/plain')
  @ApiBody({
    schema: { type: 'string', description: 'one address or prefix per line; # comments (≤ 8 MB)' },
  })
  @ApiOperation({
    summary: 'Preview an uploaded block-list file; with dryRun=false stage it into the candidate',
  })
  @ApiOkResponse({ schema: openapi(PreviewOut, 'output') })
  async import(
    @Param('name', new SafeParamPipe('name')) name: string,
    @Query(new ZodPipe(DryRunQuery)) q: z.output<typeof DryRunQuery>,
    @Body() body: unknown,
    @Req() req: NgfwRequest,
  ) {
    if (typeof body !== 'string')
      throw problems.badRequest('send the file as the request body with content-type text/plain');
    const p = await this.gb.importText(req.principal!, name, body, q.dryRun);
    if (p.staged)
      req.audit = {
        resource: `/acl/globalBlocking/lists/${name}/entries`,
        after: { entries: p.entries, added: p.added, removed: p.removed },
      };
    return p;
  }

  @Post('lists/:name/fetch')
  @HttpCode(200)
  @Protected(400, 403, 404, 409, 502)
  @ApiParam({ name: 'name', schema: { type: 'string' } })
  @ApiQuery({ name: 'dryRun', required: false, schema: { type: 'boolean', default: true } })
  @ApiOperation({
    summary: 'Download the list from its server URL now; preview, or stage with dryRun=false',
  })
  @ApiOkResponse({ schema: openapi(PreviewOut, 'output') })
  async fetch(
    @Param('name', new SafeParamPipe('name')) name: string,
    @Query(new ZodPipe(DryRunQuery)) q: z.output<typeof DryRunQuery>,
    @Req() req: NgfwRequest,
  ) {
    const p = await this.gb.fetchNow(req.principal!, name, q.dryRun);
    if (p.staged)
      req.audit = {
        resource: `/acl/globalBlocking/lists/${name}/entries`,
        after: { entries: p.entries, added: p.added, removed: p.removed },
      };
    return p;
  }

  @Get('lists/:name/export')
  @Protected(400, 404)
  @ApiParam({ name: 'name', schema: { type: 'string' } })
  @ApiQuery({
    name: 'source',
    required: false,
    schema: { type: 'string', enum: ['running', 'candidate'], default: 'running' },
  })
  @ApiProduces('text/plain')
  @ApiOperation({ summary: 'Export a block list in the import format (one entry per line)' })
  @ApiOkResponse({ content: { 'text/plain': { schema: { type: 'string' } } } })
  async export(
    @Param('name', new SafeParamPipe('name')) name: string,
    @Query(new ZodPipe(ExportQuery)) q: z.output<typeof ExportQuery>,
    @Res({ passthrough: true }) reply: FastifyReply,
  ) {
    const text = await this.gb.exportText(name, q.source);
    void reply
      .header('content-type', 'text/plain; charset=utf-8')
      .header('content-disposition', `attachment; filename="blocklist-${name}.txt"`);
    return text;
  }
}
