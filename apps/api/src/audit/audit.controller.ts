import { Controller, Get, Query } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { MinRole } from '../auth/decorators.js';
import { Protected } from '../common/responses.js';
import { openapi, ZodPipe } from '../common/zod.js';
import { AuditService } from './audit.service.js';

const PageQuery = z.object({
  limit: z.coerce.number().int().min(1).max(500).default(100),
  offset: z.coerce.number().int().min(0).default(0),
});

const AuditPage = z.object({
  total: z.number().int(),
  items: z.array(
    z.object({
      id: z.number().int(),
      ts: z.string(),
      userId: z.number().int().nullable(),
      username: z.string().nullable(),
      sourceIp: z.string().nullable(),
      action: z.string(),
      resource: z.string().nullable(),
      before: z.unknown(),
      after: z.unknown(),
      result: z.enum(['success', 'failure']),
      status: z.number().int().nullable(),
    }),
  ),
});

@ApiTags('audit')
@Controller('api/v1/audit')
export class AuditController {
  constructor(private readonly audit: AuditService) {}

  @Get()
  @MinRole('admin')
  @Protected()
  @ApiQuery({ name: 'limit', required: false, schema: { type: 'integer' } })
  @ApiQuery({ name: 'offset', required: false, schema: { type: 'integer' } })
  @ApiOperation({ summary: 'Audit log, newest first (admin)' })
  @ApiOkResponse({ schema: openapi(AuditPage, 'output') })
  list(@Query(new ZodPipe(PageQuery)) q: z.output<typeof PageQuery>) {
    return this.audit.list(q.limit, q.offset);
  }
}
