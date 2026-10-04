import { Body, Controller, HttpCode, Inject, Post, Req } from '@nestjs/common';
import { ApiBody, ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import { DataplaneConfig } from '@ngfw/proto';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { MinRole } from '../../auth/decorators.js';
import type { NgfwRequest } from '../../common/principal.js';
import { ProblemError } from '../../common/problem.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { ApprovalOut, ApplyOut, DataplaneApplyService } from './apply.service.js';

const ApproveIn = z.object({ sha256: z.string().regex(/^[0-9a-f]{64}$/) }).strict();
const ApplyIn = ApproveIn.extend({ token: z.string().regex(/^[0-9a-f]{64}$/) }).strict();

@ApiTags('dataplane')
@Controller('api/v1/actions/dataplane')
export class DataplaneApplyController {
  constructor(
    @Inject(AgentClient) private readonly agent: AgentClient,
    @Inject(DatastoreService) private readonly ds: DatastoreService,
    @Inject(DataplaneApplyService) private readonly executor: DataplaneApplyService,
  ) {}

  private async prepare(req: NgfwRequest, sha256: string) {
    const candidate = await this.ds.getCandidate();
    const dataplane = candidate['dataplane'] ?? {};
    const preview = await this.agent.dataplaneStartupPreview({
      dataplane: DataplaneConfig.fromJSON(dataplane),
    });
    if (!preview.changed || preview.sha256 !== sha256) {
      throw new ProblemError(
        409,
        'preview-changed',
        'Preview changed',
        'Preview the current configuration again.',
      );
    }
    req.audit = { resource: '/dataplane/startup', after: { sha256 } };
    return { actor: String(req.principal!.id), sha256, dataplane };
  }

  @Post('approve')
  @MinRole('admin')
  @HttpCode(200)
  @Protected(400, 409, 503)
  @ApiOperation({
    summary: 'Issue a root-owned single-use approval of the reviewed startup SHA (120 seconds)',
  })
  @ApiBody({ schema: openapi(ApproveIn) })
  @ApiOkResponse({ schema: openapi(ApprovalOut, 'output') })
  async approve(
    @Req() req: NgfwRequest,
    @Body(new ZodPipe(ApproveIn)) body: z.infer<typeof ApproveIn>,
  ) {
    return ApprovalOut.parse(
      await this.executor.send('/approve', await this.prepare(req, body.sha256)),
    );
  }

  @Post('apply')
  @MinRole('admin')
  @HttpCode(200)
  @Protected(400, 409, 503)
  @ApiOperation({
    summary: 'Consume the approval and request product-mode startup apply with rollback protection',
  })
  @ApiBody({ schema: openapi(ApplyIn) })
  @ApiOkResponse({ schema: openapi(ApplyOut, 'output') })
  async apply(@Req() req: NgfwRequest, @Body(new ZodPipe(ApplyIn)) body: z.infer<typeof ApplyIn>) {
    const prepared = await this.prepare(req, body.sha256);
    return ApplyOut.parse(await this.executor.send('/apply', { ...prepared, token: body.token }));
  }
}
