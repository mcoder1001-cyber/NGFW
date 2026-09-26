import { Controller, Get } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import type { RoutingStateResponse } from '@ngfw/proto';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';

/** One BGP neighbour's live session (vrx.v1.BgpNeighborState). */
export const BgpNeighborOut = z.object({
  address: z.string().describe('neighbour address (the routing.bgp.neighbors key)'),
  remoteAs: z.number().int(),
  state: z
    .string()
    .describe('FRR state: Idle, Connect, Active, OpenSent, OpenConfirm, Established, Idle (Admin)'),
  uptimeSec: z.number().int().describe('seconds in Established (0 otherwise)'),
  prefixesReceived: z.number().int(),
  prefixesSent: z.number().int(),
  flaps: z.number().int().describe('session drops since the daemon started'),
  established: z.number().int().describe('sessions established since the daemon started'),
  description: z.string().optional(),
  messagesReceived: z.number().int(),
  messagesSent: z.number().int(),
  afis: z.array(
    z.object({
      afi: z.string(),
      prefixesReceived: z.number().int(),
      prefixesSent: z.number().int(),
    }),
  ),
});

export const BgpStateOut = z.object({
  frrRunning: z.boolean().describe('FRR answered on its vty sockets'),
  frrVersion: z.string().optional(),
  error: z.string().optional().describe('why FRR (or part of it) could not be read'),
  retrievedAt: z.string().optional(),
  instances: z.array(
    z.object({
      vrf: z.string(),
      asn: z.number().int(),
      routerId: z.string(),
      neighbors: z.array(BgpNeighborOut),
    }),
  ),
  ribCounts: z
    .record(z.string(), z.number().int())
    .describe('FRR RIB routes per "<family>/<vrf>/<protocol>"'),
  lcpPairs: z
    .array(
      z.object({
        interface: z.string(),
        hostIfName: z.string(),
        hostIfType: z.string(),
        netns: z.string().optional(),
      }),
    )
    .describe('linux-cp pairs of this agent (VPP interface ↔ Linux interface FRR runs on)'),
});
export type BgpStateOut = z.infer<typeof BgpStateOut>;

/** vrx.v1.RoutingStateResponse → the `/state/bgp` body. */
export function bgpStateOut(r: RoutingStateResponse): BgpStateOut {
  return {
    frrRunning: r.frrRunning,
    ...(r.frrVersion ? { frrVersion: r.frrVersion } : {}),
    ...(r.error ? { error: r.error } : {}),
    ...(r.retrievedAt ? { retrievedAt: r.retrievedAt.toISOString() } : {}),
    instances: r.bgp.map((b) => ({
      vrf: b.vrf,
      asn: b.asn,
      routerId: b.routerId,
      neighbors: b.neighbors.map((n) => ({
        address: n.address,
        remoteAs: n.remoteAs,
        state: n.state,
        uptimeSec: Number(n.uptimeSec),
        prefixesReceived: n.prefixesReceived,
        prefixesSent: n.prefixesSent,
        flaps: n.flaps,
        established: n.established,
        ...(n.description ? { description: n.description } : {}),
        messagesReceived: Number(n.messagesReceived),
        messagesSent: Number(n.messagesSent),
        afis: n.afis.map((a) => ({
          afi: a.afi,
          prefixesReceived: a.prefixesReceived,
          prefixesSent: a.prefixesSent,
        })),
      })),
    })),
    ribCounts: { ...r.ribCounts },
    lcpPairs: r.lcpPairs.map((p) => ({
      interface: p.interface,
      hostIfName: p.hostIfName,
      hostIfType: p.hostIfType,
      ...(p.netns ? { netns: p.netns } : {}),
    })),
  };
}

/**
 * P12 live state: `GET /api/v1/state/bgp` — the BGP instances and neighbours (state, uptime, prefixes received/sent,
 * flaps), FRR's RIB counts and the linux-cp pairs, read by the agent's RoutingState RPC (scoped FRR commands only, never
 * the RIB). BGP, prefix lists and route maps are configured through the generic `/api/v1/config/routing/**` pointer
 * routes (candidate → diff → commit → rollback); neighbour changes stream on the `routing.events` topic.
 */
@ApiTags('state')
@Controller('api/v1/state')
export class BgpController {
  constructor(private readonly agent: AgentClient) {}

  @Get('bgp')
  @Protected(502, 503)
  @ApiOperation({
    summary:
      'Live BGP state (instances, neighbours, prefix counts, flaps), FRR RIB counts and linux-cp pairs',
  })
  @ApiOkResponse({ schema: openapi(BgpStateOut, 'output') })
  async state(): Promise<BgpStateOut> {
    return bgpStateOut(await this.agent.routingState({ readers: [], ribPrefixes: [], ribVrf: '' }));
  }
}
