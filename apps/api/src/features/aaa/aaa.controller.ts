import { Body, Controller, HttpCode, Post, Req } from '@nestjs/common';
import { ApiBody, ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { MinRole } from '../../auth/decorators.js';
import type { VrxRequest } from '../../common/principal.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { AaaService } from './aaa.service.js';

const TestBody = z.strictObject({
  method: z.enum(['radius', 'ldap', 'tacacs']),
  username: z.string().min(1).max(255),
  password: z.string().min(1).max(1024),
});
const TestOut = z.object({
  method: z.string(),
  reachable: z.boolean(),
  authenticated: z.boolean(),
  groups: z.array(z.string()),
  role: z.enum(['admin', 'operator', 'readonly']).nullable(),
  detail: z.string(),
});

/**
 * F-aaa admin route: validate a configured external auth backend by authenticating a test credential against it
 * (no session is issued). RADIUS and LDAP (F-aaa-login); TACACS+ answers 501.
 */
@ApiTags('aaa')
@Controller('api/v1/actions/aaa')
export class AaaController {
  constructor(private readonly aaa: AaaService) {}

  @Post('test')
  @HttpCode(200)
  @MinRole('admin')
  @Protected(400, 403, 501)
  @ApiOperation({
    summary: 'Admin: test an external AAA backend with a credential (no session issued)',
  })
  @ApiBody({ schema: openapi(TestBody) })
  @ApiOkResponse({ schema: openapi(TestOut, 'output') })
  async test(@Body(new ZodPipe(TestBody)) body: z.output<typeof TestBody>, @Req() req: VrxRequest) {
    const r = await this.aaa.test(body.method, body.username, body.password);
    req.audit = {
      resource: 'actions/aaa/test',
      after: { method: body.method, reachable: r.reachable, authenticated: r.authenticated },
    };
    return r;
  }
}
