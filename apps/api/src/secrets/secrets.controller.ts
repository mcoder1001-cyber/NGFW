import { Body, Controller, Delete, Get, HttpCode, Param, Post, Req } from '@nestjs/common';
import {
  ApiBody,
  ApiNoContentResponse,
  ApiOkResponse,
  ApiOperation,
  ApiTags,
} from '@nestjs/swagger';
import { SECRET_KINDS } from '@ngfw/schema';
import { z } from 'zod';
import type { VrxRequest } from '../common/principal.js';
import { Protected } from '../common/responses.js';
import { openapi, ZodPipe } from '../common/zod.js';
import { SecretsService } from './secrets.service.js';

const SecretBody = z.strictObject({
  kind: z.enum(SECRET_KINDS),
  name: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/),
  value: z.string().min(1).max(65536).describe('write-only; never returned'),
});
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
  @ApiOkResponse({ schema: openapi(z.array(SecretOut), 'output') })
  list() {
    return this.secrets.list();
  }

  @Post()
  @HttpCode(200)
  @Protected(400)
  @ApiOperation({
    summary: 'Create or replace a secret; returns the reference to put into the configuration',
  })
  @ApiBody({ schema: openapi(SecretBody) })
  @ApiOkResponse({ schema: openapi(z.object({ ref: z.string(), created: z.boolean() }), 'output') })
  async put(
    @Body(new ZodPipe(SecretBody)) body: z.output<typeof SecretBody>,
    @Req() req: VrxRequest,
  ) {
    const r = await this.secrets.put(body.kind, body.name, body.value);
    req.audit = { resource: `secret/${r.ref}`, after: { ref: r.ref, created: r.created } };
    return r;
  }

  @Delete(':kind/:name')
  @HttpCode(204)
  @Protected(404, 409)
  @ApiOperation({ summary: 'Delete a secret that nothing references' })
  @ApiNoContentResponse({ description: 'Deleted' })
  async delete(
    @Param('kind') kind: string,
    @Param('name') name: string,
    @Req() req: VrxRequest,
  ): Promise<void> {
    req.audit = { resource: `secret/${kind}/${name}` };
    await this.secrets.delete(kind, name);
  }
}
