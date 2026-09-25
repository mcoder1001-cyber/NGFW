import { Controller, Get } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import type { IpfixStateResponse } from '@ngfw/proto';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';

const ExporterOut = z.object({
  name: z.string().describe('services.ipfix.exporters key when the agent knows it, else ""'),
  defaultExporter: z.boolean().describe('exporter 0 — the one flowprobe records use'),
  collector: z.string(),
  collectorPort: z.number().int(),
  sourceAddress: z.string(),
  vrf: z.string(),
  pathMtu: z.number().int(),
  templateIntervalSec: z.number().int(),
  udpChecksum: z.boolean(),
  statIndex: z
    .number()
    .int()
    .nullable()
    .describe('stats index; null when not known (agent restart)'),
});

export const IpfixStateOut = z.object({
  exporters: z.array(ExporterOut),
  flowprobe: z.object({
    params: z
      .object({
        recordL2: z.boolean(),
        recordL3: z.boolean(),
        recordL4: z.boolean(),
        activeTimerSec: z.number().int(),
        passiveTimerSec: z.number().int(),
      })
      .nullable()
      .describe('null while VPP has no record flag set'),
    interfaces: z.array(
      z.object({
        interface: z.string(),
        which: z.enum(['ip4', 'ip6', 'l2']),
        direction: z.enum(['rx', 'tx', 'both']),
      }),
    ),
  }),
  sflow: z.object({
    global: z
      .object({
        samplingN: z.number().int(),
        pollingIntervalSec: z.number().int(),
        headerBytes: z.number().int(),
        direction: z.string(),
        dropMonitoring: z.boolean(),
      })
      .nullable(),
    interfaces: z.array(z.object({ interface: z.string(), hwIfIndex: z.number().int() })),
    counters: z
      .array(
        z.object({ name: z.string(), value: z.string().describe('uint64 as a decimal string') }),
      )
      .describe('sFlow node counters (/err/sflow/*) from the stats segment, summed over workers'),
    exportsToCollectors: z
      .literal(false)
      .describe('VPP samples; export to collectors needs hsflowd, not shipped in this build'),
  }),
  globalsOwner: z
    .boolean()
    .describe('the agent sets exporter 0 / flowprobe / sFlow globals (D-071)'),
  notes: z.array(z.string()),
  retrievedAt: z.string().nullable(),
});

/** Maps the agent's IpfixStateResponse to the REST shape. */
export function ipfixStateOf(r: IpfixStateResponse): z.infer<typeof IpfixStateOut> {
  const which = (w: string) => (w === 'ip6' || w === 'l2' ? w : 'ip4');
  const dir = (d: string) => (d === 'rx' || d === 'tx' ? d : 'both');
  return {
    exporters: r.exporters.map((e) => ({
      name: e.name,
      defaultExporter: e.defaultExporter,
      collector: e.collector,
      collectorPort: e.collectorPort,
      sourceAddress: e.sourceAddress,
      vrf: e.vrf,
      pathMtu: e.pathMtu,
      templateIntervalSec: e.templateIntervalSec,
      udpChecksum: e.udpChecksum,
      statIndex: e.statIndex ?? null,
    })),
    flowprobe: {
      params: r.flowprobeParams
        ? {
            recordL2: r.flowprobeParams.recordL2,
            recordL3: r.flowprobeParams.recordL3,
            recordL4: r.flowprobeParams.recordL4,
            activeTimerSec: r.flowprobeParams.activeTimerSec,
            passiveTimerSec: r.flowprobeParams.passiveTimerSec,
          }
        : null,
      interfaces: r.flowprobeInterfaces.map((f) => ({
        interface: f.interface,
        which: which(f.which),
        direction: dir(f.direction),
      })),
    },
    sflow: {
      global: r.sflowGlobal
        ? {
            samplingN: r.sflowGlobal.samplingN,
            pollingIntervalSec: r.sflowGlobal.pollingIntervalSec,
            headerBytes: r.sflowGlobal.headerBytes,
            direction: r.sflowGlobal.direction,
            dropMonitoring: r.sflowGlobal.dropMonitoring,
          }
        : null,
      interfaces: r.sflowInterfaces.map((i) => ({
        interface: i.interface,
        hwIfIndex: i.hwIfIndex,
      })),
      counters: r.sflowCounters.map((c) => ({ name: c.name, value: String(c.value) })),
      exportsToCollectors: false,
    },
    globalsOwner: r.globalsOwner,
    notes: r.notes,
    retrievedAt: r.retrievedAt ? r.retrievedAt.toISOString() : null,
  };
}

/**
 * F-ipfix-sflow: `GET /api/v1/state/ipfix` — live flow-export state from the agent's IpfixState RPC. Configuration
 * goes through the generic pointer routes (`/api/v1/config/services/ipfix/...`); their semantic rules
 * (`services.ipfix-sflow-*`) answer 400 problem+json with a `pointer`.
 */
@ApiTags('state')
@Controller('api/v1/state')
export class IpfixSflowController {
  constructor(private readonly agent: AgentClient) {}

  @Get('ipfix')
  @Protected(501, 502, 503)
  @ApiOperation({
    summary:
      'IPFIX exporters, flowprobe and sFlow interfaces and sampling counters as the data plane has them (agent IpfixState)',
  })
  @ApiOkResponse({ schema: openapi(IpfixStateOut, 'output') })
  async ipfix() {
    return ipfixStateOf(await this.agent.ipfixState());
  }
}
