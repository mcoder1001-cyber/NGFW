import { Controller, Get, Inject } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import type { TunnelStateResponse } from '@ngfw/proto';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';

const Counters = z.object({
  rxPackets: z.string(),
  rxBytes: z.string(),
  txPackets: z.string(),
  txBytes: z.string(),
  drops: z.string(),
  errors: z.string(),
});
export const TunnelsStateOut = z.object({
  retrievedAt: z.string().nullable(),
  items: z.array(
    z.object({
      name: z.string(),
      kind: z.string(),
      interface: z.string(),
      swIfIndex: z.number().int(),
      adminUp: z.boolean(),
      linkUp: z.boolean(),
      mtu: z.number().int(),
      deviceClass: z.string(),
      src: z.string().nullable(),
      dst: z.string().nullable(),
      underlayTableId: z.number().int().nullable(),
      ipv4TableId: z.number().int().nullable(),
      ipv6TableId: z.number().int().nullable(),
      counters: Counters.nullable(),
      notes: z.array(z.string()),
    }),
  ),
});

export function toTunnelsState(r: TunnelStateResponse): z.infer<typeof TunnelsStateOut> {
  return {
    retrievedAt: r.retrievedAt?.toISOString() ?? null,
    items: r.tunnels.map((t) => ({
      name: t.name,
      kind: t.kind,
      interface: t.interface,
      swIfIndex: t.swIfIndex,
      adminUp: t.adminUp,
      linkUp: t.linkUp,
      mtu: t.mtu,
      deviceClass: t.deviceClass,
      src: t.src ?? null,
      dst: t.dst ?? null,
      underlayTableId: t.underlayTableId ?? null,
      ipv4TableId: t.ipv4TableId ?? null,
      ipv6TableId: t.ipv6TableId ?? null,
      notes: [...t.notes],
      counters: t.counters
        ? {
            rxPackets: String(t.counters.rxPackets),
            rxBytes: String(t.counters.rxBytes),
            txPackets: String(t.counters.txPackets),
            txBytes: String(t.counters.txBytes),
            drops: String(t.counters.drops),
            errors: String(t.counters.errors),
          }
        : null,
    })),
  };
}

@ApiTags('state')
@Controller('api/v1/state')
export class TunnelsController {
  constructor(@Inject(AgentClient) private readonly agent: AgentClient) {}

  @Get('tunnels')
  @ApiOperation({
    summary: 'Read this owner’s live tunnel interfaces, endpoints, FIBs and counters',
  })
  @Protected()
  @ApiOkResponse({ schema: openapi(TunnelsStateOut, 'output') })
  async state() {
    return toTunnelsState(await this.agent.tunnelState());
  }
}
