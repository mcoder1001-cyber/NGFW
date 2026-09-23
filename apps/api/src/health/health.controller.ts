import { Controller, Get } from '@nestjs/common';
import { ApiOkResponse, ApiTags } from '@nestjs/swagger';

export interface HealthDto {
  status: 'ok';
  service: 'vrx-api';
  version: string;
  time: string;
}

@ApiTags('system')
@Controller('api/v1/health')
export class HealthController {
  @Get()
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
