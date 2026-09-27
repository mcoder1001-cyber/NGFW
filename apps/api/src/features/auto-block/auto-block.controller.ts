import { Body, Controller, Get, HttpCode, Post, Req } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { MinRole } from '../../auth/decorators.js';
import type { VrxRequest } from '../../common/principal.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { problems } from '../../common/problem.js';
import { AutoBlockService } from './auto-block.service.js';

const BlockedEntryOut = z.object({
  source: z.string(),
  reason: z.string(),
  hits: z.number().int(),
  offences: z.number().int(),
  origin: z.string(),
  note: z.string(),
  firstSeen: z.string(),
  blockedAt: z.string(),
  expiresAt: z.string(),
});
const ListOut = z.object({ items: z.array(BlockedEntryOut) });

const source = z.string().min(1).max(49);
const UnblockBody = z.object({ source });
const UnblockOut = z.object({ unblocked: z.boolean() });
const BlockBody = z.object({
  source,
  blockSec: z.number().int().min(1).max(30 * 86_400).optional(),
  note: z.string().max(255).default(''),
});

/**
 * F-bruteforce-block admin/state routes. The live blocked set is read-only for any authenticated user; unblocking and
 * blocking by hand are admin-only. The detector thresholds and the allow-list are configuration and are edited through
 * the generic config routes under `security.autoBlock` (an allow-listed source can never be blocked).
 */
@ApiTags('auto-block')
@Controller('api/v1')
export class AutoBlockController {
  constructor(private readonly svc: AutoBlockService) {}

  @Get('state/auto-block')
  @Protected()
  @ApiOperation({ summary: 'The live auto-block set (sources blocked now), newest first' })
  @ApiOkResponse({ schema: openapi(ListOut, 'output') })
  async list() {
    return { items: await this.svc.list() };
  }

  @Post('actions/auto-block/unblock')
  @HttpCode(200)
  @MinRole('admin')
  @Protected(400, 403, 404)
  @ApiOperation({ summary: 'Admin: remove a source from the auto-block set' })
  @ApiOkResponse({ schema: openapi(UnblockOut, 'output') })
  async unblock(@Body(new ZodPipe(UnblockBody)) body: z.output<typeof UnblockBody>, @Req() req: VrxRequest) {
    const unblocked = await this.svc.unblock(body.source, req.principal!);
    if (!unblocked) throw problems.notFound(`${body.source} is not blocked`);
    req.audit = { resource: 'actions/auto-block/unblock', after: { source: body.source } };
    return { unblocked };
  }

  @Post('actions/auto-block/block')
  @HttpCode(200)
  @MinRole('admin')
  @Protected(400, 403, 409)
  @ApiOperation({ summary: 'Admin: block a source by hand (refused for an allow-listed source)' })
  @ApiOkResponse({ schema: openapi(BlockedEntryOut, 'output') })
  async block(@Body(new ZodPipe(BlockBody)) body: z.output<typeof BlockBody>, @Req() req: VrxRequest) {
    const entry = await this.svc.manualBlock(body.source, body.blockSec, body.note, req.principal!);
    req.audit = {
      resource: 'actions/auto-block/block',
      after: { source: entry.source, expiresAt: entry.expiresAt },
    };
    return entry;
  }
}
