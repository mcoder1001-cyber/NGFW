import { Controller, Get } from '@nestjs/common';
import { ApiOperation, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { ApiOut, Protected } from '../../common/responses.js';
import { HostMetricsService } from './host-metrics.service.js';

const Disk = z.object({ mount: z.string(), totalBytes: z.number(), usedBytes: z.number() });
export const HostOut = z
  .object({
    hostname: z.string(),
    uptimeSec: z.number().int(),
    cpu: z.object({
      cores: z.number().int(),
      model: z.string(),
      usagePct: z
        .number()
        .nullable()
        .describe('all cores over the last sample period; null until two samples exist'),
      load: z
        .tuple([z.number(), z.number(), z.number()])
        .describe('1, 5 and 15 minute load averages'),
    }),
    memory: z.object({ totalBytes: z.number(), usedBytes: z.number(), availableBytes: z.number() }),
    hugepages: z
      .object({ total: z.number().int(), free: z.number().int(), sizeBytes: z.number() })
      .nullable()
      .describe('reserved hugepages (the packet engine uses them); null when none are configured'),
    disks: z.array(Disk),
    history: z
      .array(z.object({ at: z.number(), cpuPct: z.number().nullable(), memUsedPct: z.number() }))
      .describe('one sample every 5 s, oldest first, up to 6 minutes'),
    sampledAt: z.string(),
  })
  .describe(
    'The appliance host: CPU, memory, disks and hugepages (read from the kernel by ngfw-api, D-154)',
  );

/** WEB-dashboard: `GET /api/v1/state/host`. */
@ApiTags('state')
@Controller('api/v1/state')
export class HostMetricsController {
  constructor(private readonly host: HostMetricsService) {}

  @Get('host')
  @Protected()
  @ApiOperation({
    summary: 'Appliance host resources: CPU, memory, disks, hugepages, with 6 minutes of history',
  })
  @ApiOut(HostOut)
  async hostState() {
    return this.host.current();
  }
}
