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
import { objectName } from '@ngfw/schema';
import type { FastifyReply } from 'fastify';
import { z } from 'zod';
import type { VrxRequest } from '../../common/principal.js';
import { Protected } from '../../common/responses.js';
import { openapi, SafeParamPipe, ZodPipe } from '../../common/zod.js';
import { AclService } from './acl.service.js';
import { MAX_SEQUENCE } from './rules.js';

const SourceQ = z.enum(['running', 'candidate']);
const boolQ = z
  .enum(['true', 'false', '1', '0'])
  .transform((v) => v === 'true' || v === '1')
  .optional();

const RulesQuery = z.object({
  page: z.coerce.number().int().min(1).default(1),
  pageSize: z.coerce.number().int().min(1).max(1000).default(100),
  filter: z
    .string()
    .max(200)
    .optional()
    .describe('quick filter: every whitespace-separated term must occur'),
  source: SourceQ.default('candidate'),
  hitsOnly: boolQ.describe('only rules whose counters show hits'),
});
const ExportQuery = z.object({ list: objectName, source: SourceQ.default('running') });
const ImportQuery = z.object({
  list: objectName,
  mode: z.enum(['replace', 'append']).default('replace'),
  dryRun: boolQ.default(true).describe('true (default) = validate and preview only'),
});

const sequence = z.number().int().min(1).max(MAX_SEQUENCE);
const BulkBody = z.discriminatedUnion('op', [
  z.object({
    op: z.enum(['enable', 'disable', 'delete']),
    sequences: z.array(sequence).min(1).max(100_000),
  }),
  z.object({
    op: z.literal('move'),
    sequences: z.array(sequence).min(1).max(100_000),
    to: sequence,
  }),
  z.object({
    op: z.literal('renumber'),
    start: sequence.default(10),
    step: z.number().int().min(1).max(1_000_000).default(10),
  }),
]);

const LiveListOut = z.object({
  aclIndex: z.number().int(),
  vppRules: z.number().int().describe('rules of the VPP ACL (the expansion of the list)'),
  mappingKnown: z.boolean().describe('VPP rules are attributed to configuration rules'),
  configRules: z.number().int(),
  packets: z.number(),
  bytes: z.number(),
});
const PendingOut = z
  .enum(['added', 'changed', 'deleted'])
  .nullable()
  .describe('candidate vs running');
const TargetOut = z.object({
  kind: z.string(),
  interface: z.string().optional(),
  zone: z.string().optional(),
});
const ListsOut = z.object({
  countersAvailable: z.boolean(),
  countersReason: z.string(),
  agentError: z
    .string()
    .nullable()
    .describe('why the live columns are empty (agent unavailable, older agent)'),
  retrievedAt: z.string().nullable(),
  lists: z.array(
    z.object({
      name: z.string(),
      description: z.string().nullable(),
      tags: z.array(z.string()),
      rules: z.number().int().describe('configuration rules (candidate)'),
      pending: PendingOut,
      attachments: z.array(
        z.object({
          target: TargetOut,
          direction: z.string(),
          sequence: z.number().int(),
          enabled: z.boolean(),
        }),
      ),
      live: LiveListOut.nullable().describe(
        'null = not in VPP (not applied) or the agent is unavailable',
      ),
    }),
  ),
  macip: z.array(
    z.object({
      name: z.string(),
      description: z.string().nullable(),
      rules: z.number().int(),
      pending: PendingOut,
      interfaces: z.array(z.string()),
      live: z.object({ aclIndex: z.number().int(), vppRules: z.number().int() }).nullable(),
    }),
  ),
});
const RulesOut = z.object({
  list: z.string(),
  source: SourceQ,
  page: z.number().int(),
  pageSize: z.number().int(),
  total: z.number().int().describe('rules matching the filter'),
  size: z.number().int().describe('rules of the list (its rules array length: PUT …/rules/<size> appends)'),
  applied: z.boolean().describe('the agent reported the list (it is in VPP)'),
  mappingKnown: z.boolean(),
  countersAvailable: z.boolean(),
  countersReason: z.string(),
  agentError: z.string().nullable(),
  items: z.array(
    z.object({
      index: z.number().int().describe('position in the document: /acl/lists/<list>/rules/<index>'),
      sequence: z.number().int(),
      rule: z.record(z.string(), z.unknown()).describe('the configuration rule (AclRule)'),
      pending: z.enum(['added', 'changed']).nullable(),
      live: z
        .object({
          status: z.enum(['applied', 'disabled', 'schedule-inactive', 'empty', 'unknown']),
          vppRules: z.number().int(),
          packets: z.number(),
          bytes: z.number(),
        })
        .nullable(),
    }),
  ),
});
const BoundOut = z.object({
  aclIndex: z.number().int(),
  name: z.string().nullable().describe('list name when the ACL is this agent’s'),
  tag: z.string(),
  foreign: z.boolean().describe('another owner’s ACL (preserved, D-066)'),
});
const AttachmentsOut = z.object({
  agentError: z.string().nullable(),
  retrievedAt: z.string().nullable(),
  interfaces: z.array(
    z.object({
      interface: z.string(),
      swIfIndex: z.number().int().nullable(),
      input: z.array(BoundOut),
      output: z.array(BoundOut),
      macip: BoundOut.nullable(),
      expected: z.object({
        input: z.array(z.string()),
        output: z.array(z.string()),
        macip: z.string().nullable(),
      }),
      inSync: z
        .boolean()
        .nullable()
        .describe('this agent’s part of VPP’s lists equals the running configuration'),
    }),
  ),
});
const IssueOut = z.object({
  line: z.number().int().optional(),
  column: z.string().optional(),
  message: z.string(),
});
const ImportOut = z.object({
  list: z.string(),
  mode: z.enum(['replace', 'append']),
  dryRun: z.boolean(),
  rows: z.number().int(),
  valid: z.number().int(),
  errorCount: z.number().int(),
  errors: z.array(IssueOut).describe('first 200'),
  warnings: z.array(IssueOut),
  existingRules: z.number().int(),
  preview: z.array(z.record(z.string(), z.unknown())).describe('first 20 parsed rules (dry run)'),
  imported: z.number().int(),
  total: z.number().int().describe('rules of the list afterwards (dry run: now)'),
});
const BulkOut = z.object({
  list: z.string(),
  op: z.string(),
  changed: z.number().int(),
  total: z.number().int(),
  before: z.number().int(),
});

/**
 * F-acl routes. State (read-only, 00-CONTEXT rule 8): the list of lists with live VPP status, one list's rules paged
 * server-side with hit counters per configuration rule (AclState), the interface bindings as VPP holds them (other
 * owners' ACLs included, D-066). Actions: CSV import into the candidate (dry run first, ≤ 100 000 rows, streamed),
 * CSV export, and bulk edits of a list's rules (enable/disable/delete/move/renumber) as one candidate edit. The ACL
 * configuration itself goes through the generic pointer routes (`/api/v1/config/acl/…`).
 */
@ApiTags('acl')
@Controller('api/v1')
export class AclController {
  constructor(private readonly acl: AclService) {}

  @Get('state/acl/lists')
  @Protected()
  @ApiOperation({
    summary: 'ACL and MACIP lists with pending marks and live VPP status (index, VPP rules, hits)',
  })
  @ApiOkResponse({ schema: openapi(ListsOut, 'output') })
  lists() {
    return this.acl.lists();
  }

  @Get('state/acl/lists/:name/rules')
  @Protected(400, 404)
  @ApiParam({ name: 'name', schema: { type: 'string' } })
  @ApiQuery({ name: 'page', required: false, schema: { type: 'integer', minimum: 1, default: 1 } })
  @ApiQuery({
    name: 'pageSize',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 1000, default: 100 },
  })
  @ApiQuery({ name: 'filter', required: false, schema: { type: 'string' } })
  @ApiQuery({
    name: 'source',
    required: false,
    schema: { type: 'string', enum: ['running', 'candidate'], default: 'candidate' },
  })
  @ApiQuery({ name: 'hitsOnly', required: false, schema: { type: 'boolean' } })
  @ApiOperation({
    summary:
      'One list’s rules in sequence order, paged and searched server-side, with per-rule hits',
  })
  @ApiOkResponse({ schema: openapi(RulesOut, 'output') })
  rules(
    @Param('name', new SafeParamPipe('name')) name: string,
    @Query(new ZodPipe(RulesQuery)) q: z.output<typeof RulesQuery>,
  ) {
    return this.acl.rules(name, { ...q, hitsOnly: q.hitsOnly ?? false });
  }

  @Get('state/acl/attachments')
  @Protected()
  @ApiOperation({
    summary:
      'ACLs bound per interface as VPP holds them (other owners’ included) vs the running config',
  })
  @ApiOkResponse({ schema: openapi(AttachmentsOut, 'output') })
  attachments() {
    return this.acl.attachments();
  }

  @Get('actions/acl/export.csv')
  @Protected(400, 404)
  @ApiQuery({ name: 'list', required: true, schema: { type: 'string' } })
  @ApiQuery({
    name: 'source',
    required: false,
    schema: { type: 'string', enum: ['running', 'candidate'], default: 'running' },
  })
  @ApiProduces('text/csv')
  @ApiOperation({ summary: 'Export a list’s rules as CSV (docs/user/firewall/acl.md format)' })
  @ApiOkResponse({ content: { 'text/csv': { schema: { type: 'string' } } } })
  async exportCsv(
    @Query(new ZodPipe(ExportQuery)) q: z.output<typeof ExportQuery>,
    @Res({ passthrough: true }) reply: FastifyReply,
  ) {
    const stream = await this.acl.exportStream(q.list, q.source);
    void reply
      .header('content-type', 'text/csv; charset=utf-8')
      .header('content-disposition', `attachment; filename="acl-${q.list}.csv"`);
    return stream;
  }

  @Post('actions/acl/import')
  @HttpCode(200)
  @Protected(400, 403, 404, 409)
  @ApiQuery({ name: 'list', required: true, schema: { type: 'string' } })
  @ApiQuery({
    name: 'mode',
    required: false,
    schema: { type: 'string', enum: ['replace', 'append'], default: 'replace' },
  })
  @ApiQuery({ name: 'dryRun', required: false, schema: { type: 'boolean', default: true } })
  @ApiConsumes('text/csv')
  @ApiBody({ schema: { type: 'string', description: 'CSV (header row + up to 100 000 rules)' } })
  @ApiOperation({
    summary: 'Import CSV rules into a list of the candidate (dry run by default; streamed)',
  })
  @ApiOkResponse({ schema: openapi(ImportOut, 'output') })
  async importCsv(
    @Query(new ZodPipe(ImportQuery)) q: z.output<typeof ImportQuery>,
    @Body() body: unknown,
    @Req() req: VrxRequest,
  ) {
    const r = await this.acl.importCsv(req.principal!, q.list, body, {
      mode: q.mode,
      dryRun: q.dryRun,
    });
    if (!q.dryRun) {
      req.audit = {
        resource: `/acl/lists/${q.list}/rules`,
        before: { rules: r.existingRules },
        after: { rules: r.total, imported: r.imported, mode: q.mode },
      };
    }
    return r;
  }

  @Post('actions/acl/lists/:name/rules/bulk')
  @HttpCode(200)
  @Protected(400, 403, 404, 409)
  @ApiParam({ name: 'name', schema: { type: 'string' } })
  @ApiBody({ schema: openapi(BulkBody) })
  @ApiOperation({
    summary:
      'Bulk edit of a list’s rules in the candidate: enable, disable, delete, move to sequence, renumber',
  })
  @ApiOkResponse({ schema: openapi(BulkOut, 'output') })
  async bulk(
    @Param('name', new SafeParamPipe('name')) name: string,
    @Body(new ZodPipe(BulkBody)) body: z.output<typeof BulkBody>,
    @Req() req: VrxRequest,
  ) {
    const r = await this.acl.bulk(req.principal!, name, body);
    req.audit = {
      resource: `/acl/lists/${name}/rules`,
      before: { rules: r.before },
      after: {
        rules: r.total,
        op: body.op,
        changed: r.changed,
        ...('sequences' in body
          ? { sequences: body.sequences.slice(0, 50), count: body.sequences.length }
          : {}),
        ...('to' in body ? { to: body.to } : {}),
      },
    };
    return r;
  }
}
