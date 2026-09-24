import { Controller, Get, Query } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import { objectName } from '@ngfw/schema';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { redact } from '../../datastore/documents.js';
import { OBJECT_KINDS, USAGE_KINDS, whereUsed } from './usage.js';

const FqdnQuery = z.object({
  name: objectName.optional().describe('only this FQDN address object'),
});

const FqdnItemOut = z.object({
  name: z.string().describe('FQDN address object (key of objects.addresses)'),
  fqdn: z.string(),
  addresses: z
    .array(z.string())
    .describe('A and AAAA answers in use, IPv4 first; the last good answers after a failed refresh'),
  lastResolved: z.string().nullable().describe('null = never resolved (the object expands to nothing)'),
  nextRefresh: z.string().nullable(),
  error: z.string().describe('the latest attempt’s failure; empty when it succeeded'),
  failures: z.number().int().describe('consecutive failed attempts'),
});
const FqdnOut = z.object({
  retrievedAt: z.string().optional(),
  items: z.array(FqdnItemOut),
});

// A where-used query names anything a document can reference: an object, a tag, a zone or an interface.
const UsageQuery = z.object({
  name: z.string().min(1).max(80).regex(/^[A-Za-z0-9][A-Za-z0-9_./:-]*$/, 'expected an object or interface name'),
  source: z.enum(['running', 'candidate']).default('running'),
});
const UsageOut = z.object({
  name: z.string(),
  source: z.enum(['running', 'candidate']),
  definedAs: z.array(z.enum(OBJECT_KINDS)).describe('the object kinds that define this name'),
  usedBy: z.array(
    z.object({
      pointer: z.string().describe('JSON pointer of the referencing leaf'),
      container: z.string().describe('pointer of the group, rule, attachment or object holding it'),
      domain: z.enum(['objects', 'acl']),
      kind: z.enum(USAGE_KINDS),
    }),
  ),
});

/**
 * F-object-model state routes (`/api/v1/state/objects/**`, read-only, 00-CONTEXT rule 8): the agent's FQDN resolver
 * state (FqdnObjectState RPC) and where-used computed from the running (or candidate) document. Object configuration
 * itself goes through the generic pointer routes (`/api/v1/config/objects/<kind>/<name>`).
 */
@ApiTags('state')
@Controller('api/v1/state/objects')
export class ObjectModelController {
  constructor(
    private readonly agent: AgentClient,
    private readonly ds: DatastoreService,
  ) {}

  @Get('fqdn')
  @Protected(400, 501, 502, 503)
  @ApiQuery({ name: 'name', required: false, schema: { type: 'string' } })
  @ApiOperation({
    summary:
      'FQDN address objects as the agent resolves them: addresses in use, last resolution, next refresh, latest error',
  })
  @ApiOkResponse({ schema: openapi(FqdnOut, 'output') })
  async fqdn(@Query(new ZodPipe(FqdnQuery)) q: z.output<typeof FqdnQuery>) {
    const r = await this.agent.fqdnObjectState(q.name === undefined ? [] : [q.name]);
    return {
      retrievedAt: r.retrievedAt?.toISOString(),
      items: r.objects.map((o) => ({
        name: o.name,
        fqdn: o.fqdn,
        addresses: o.addresses,
        lastResolved: o.lastResolved?.toISOString() ?? null,
        nextRefresh: o.nextRefresh?.toISOString() ?? null,
        error: o.error,
        failures: o.failures,
      })),
    };
  }

  @Get('usage')
  @Protected(400)
  @ApiQuery({ name: 'name', required: true, schema: { type: 'string' } })
  @ApiQuery({
    name: 'source',
    required: false,
    schema: { type: 'string', enum: ['running', 'candidate'], default: 'running' },
  })
  @ApiOperation({
    summary:
      'Where-used of an object, tag, zone or interface name: group members, tags, ACL rules and attachments, zones',
  })
  @ApiOkResponse({ schema: openapi(UsageOut, 'output') })
  async usage(@Query(new ZodPipe(UsageQuery)) q: z.output<typeof UsageQuery>) {
    const doc = q.source === 'candidate' ? await this.ds.getCandidate() : (await this.ds.getRunning()).doc;
    return { ...whereUsed(redact(doc) as Record<string, unknown>, q.name), source: q.source };
  }
}
