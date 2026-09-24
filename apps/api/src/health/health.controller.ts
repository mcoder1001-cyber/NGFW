import { Controller, Get } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import { Public } from '../auth/decorators.js';

export interface HealthDto {
  status: 'ok';
  service: 'vrx-api';
  version: string;
  time: string;
}

@ApiTags('system')
@Controller('api/v1/health')
export class HealthController {
  /** Liveness only — public so service checks need no credentials; it reveals nothing about the device. */
  @Get()
  @Public()
  @ApiOperation({ summary: 'Liveness of the API process' })
  @ApiOkResponse({ description: 'Liveness of the API process (not of VPP).' })
  health(): HealthDto {
    return {
      status: 'ok',
      service: 'vrx-api',
      version: process.env['VRX_VERSION'] ?? '0.1.0-dev',
      time: new Date().toISOString(),
    };
  }
}
