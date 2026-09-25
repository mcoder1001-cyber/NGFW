import { Body, Controller, Delete, Get, HttpCode, Param, Post, Query, Req } from '@nestjs/common';
import { ApiBody, ApiNoContentResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import { SECRET_KINDS } from '@ngfw/schema';
import { z } from 'zod';
import { MinRole } from '../auth/decorators.js';
import type { VrxRequest } from '../common/principal.js';
import { ApiOut, Protected } from '../common/responses.js';
import { openapi, SafeParamPipe, ZodPipe } from '../common/zod.js';
import { SecretsService } from './secrets.service.js';

const SecretBody = z.strictObject({
  kind: z.enum(SECRET_KINDS),
  name: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/),
  value: z.string().min(1).max(65536).describe('write-only; never returned'),
});
const ReplaceQuery = z.object({ replace: z.enum(['true', 'false']).optional() });
const SecretOut = z.object({
  ref: z.string(),
  kind: z.enum(SECRET_KINDS),
  name: z.string(),
  createdAt: z.string(),
});

/** `/api/v1/secrets` — the store behind `<kind>/<name>` references (D-051). Values go in, never come out. */
@ApiTags('secrets')
@Controller('api/v1/secrets')
export class SecretsController {
  constructor(private readonly secrets: SecretsService) {}

  @Get()
  @Protected()
  @ApiOperation({ summary: 'Secret references (no values)' })
  @ApiOut(z.array(SecretOut))
  list() {
    return this.secrets.list();
  }

  @Post()
  @MinRole('admin')
  @ApiQuery({
    name: 'replace',
    required: false,
    schema: { type: 'string', enum: ['true', 'false'] },
  })
  @HttpCode(200)
  @Protected(400, 409)
  @ApiOperation({
    summary: 'Create or replace a secret; returns the reference to put into the configuration',
  })
  @ApiBody({ schema: openapi(SecretBody) })
  @ApiOut(z.object({ ref: z.string(), created: z.boolean(), version: z.number().int() }))
  async put(
    @Body(new ZodPipe(SecretBody)) body: z.output<typeof SecretBody>,
    @Query(new ZodPipe(ReplaceQuery)) q: z.output<typeof ReplaceQuery>,
    @Req() req: VrxRequest,
  ) {
    const r = await this.secrets.put(body.kind, body.name, body.value, {
      replace: q.replace === 'true',
      userId: req.principal!.id,
    });
    req.audit = {
      resource: `secret/${r.ref}`,
      after: { ref: r.ref, created: r.created, version: r.version },
    };
    return r;
  }

  @Delete(':kind/:name')
  @MinRole('admin')
  @HttpCode(204)
  @Protected(400, 404, 409)
  @ApiOperation({ summary: 'Delete a secret that nothing references' })
  @ApiNoContentResponse({ description: 'Deleted' })
  async delete(
    @Param('kind', new SafeParamPipe('kind', 16)) kind: string,
    @Param('name', new SafeParamPipe('name', 64)) name: string,
    @Req() req: VrxRequest,
  ): Promise<void> {
    req.audit = { resource: `secret/${kind}/${name}` };
    await this.secrets.delete(kind, name);
  }
}
