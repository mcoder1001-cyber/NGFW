import { Controller, Get } from '@nestjs/common';
import { ApiOperation, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { Public } from '../auth/decorators.js';
import { ApiOut } from '../common/responses.js';

/** The liveness answer — one Zod definition for the handler type and the OpenAPI response (TD-2 #3). */
export const HealthOut = z.object({
  status: z.literal('ok'),
  service: z.literal('vrx-api'),
  version: z.string(),
  time: z.string().describe('RFC 3339 timestamp of the answer'),
});
export type HealthDto = z.infer<typeof HealthOut>;

@ApiTags('system')
@Controller('api/v1/health')
export class HealthController {
  /** Liveness only — public so service checks need no credentials; it reveals nothing about the device. */
  @Get()
  @Public()
  @ApiOperation({ summary: 'Liveness of the API process' })
  @ApiOut(HealthOut, 'Liveness of the API process (not of VPP).')
  health(): HealthDto {
    return {
      status: 'ok',
      service: 'vrx-api',
      version: process.env['VRX_VERSION'] ?? '0.1.0-dev',
      time: new Date().toISOString(),
    };
  }
}
