import { Body, Controller, Get, HttpCode, Put, Req } from '@nestjs/common';
import { ApiBody, ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { MinRole } from '../../auth/decorators.js';
import type { VrxRequest } from '../../common/principal.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { LICENSE_FORMAT } from './format.js';
import { LicensingService } from './licensing.service.js';

/** The `.vrxlic` file content as uploaded (its JSON object, unchanged). */
const LicenseFileBody = z
  .object({
    format: z.literal(LICENSE_FORMAT),
    license: z.record(z.string(), z.unknown()),
    signature: z
      .string()
      .max(256)
      .describe('detached Ed25519 signature (base64); stored, never returned'),
  })
  .describe('the .vrxlic file content');

const LicenseStateOut = z.object({
  status: z.enum(['community', 'valid', 'grace', 'expired', 'invalid']),
  reason: z.string().optional(),
  licenseId: z.string().optional(),
  customer: z.string().optional().describe('customer name only'),
  issuedAt: z.string().optional(),
  notBefore: z.string().optional(),
  expiresAt: z.string().optional(),
  daysLeft: z.number().int(),
  graceDays: z.number().int(),
  bound: z.object({ machineId: z.boolean(), serial: z.boolean() }),
  entitlements: z.object({
    features: z.array(z.string()),
    limits: z.record(z.string(), z.number().int()),
  }),
});

/** F-licensing: offline licence upload (admin) and status (any role). */
@ApiTags('licensing')
@Controller('api/v1')
export class LicensingController {
  constructor(private readonly licensing: LicensingService) {}

  @Get('state/license')
  @Protected()
  @ApiOperation({ summary: 'Licence status, entitlements in force and days left (no signature)' })
  @ApiOkResponse({ schema: openapi(LicenseStateOut, 'output') })
  state() {
    return this.licensing.state();
  }

  @Put('system/license')
  @MinRole('admin')
  @HttpCode(200)
  @Protected(400)
  @ApiOperation({ summary: 'Upload a .vrxlic licence file (verified before it is stored)' })
  @ApiBody({ schema: openapi(LicenseFileBody) })
  @ApiOkResponse({ schema: openapi(LicenseStateOut, 'output') })
  async put(
    @Body(new ZodPipe(LicenseFileBody)) body: z.output<typeof LicenseFileBody>,
    @Req() req: VrxRequest,
  ) {
    req.audit = { resource: 'system/license' };
    const st = await this.licensing.install(JSON.stringify(body));
    req.audit.after = { licenseId: st.licenseId, status: st.status, expiresAt: st.expiresAt };
    return st;
  }
}
