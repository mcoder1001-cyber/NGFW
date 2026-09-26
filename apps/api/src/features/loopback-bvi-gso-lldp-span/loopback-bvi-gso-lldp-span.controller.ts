import { Controller, Get, Query } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import type { LldpNeighbor } from '@ngfw/proto';
import { isPlainObject } from '@ngfw/schema';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { DatastoreService } from '../../datastore/datastore.service.js';

type Json = Record<string, unknown>;

const NeighborOut = z.object({
  interface: z.string().describe('logical name of the LLDP-enabled interface'),
  swIfIndex: z.number().int(),
  heard: z.boolean().describe('a peer was heard since LLDP was enabled'),
  chassisId: z.string().describe('peer chassis id (MAC, address, text or hex); "" = none heard'),
  chassisIdSubtype: z
    .string()
    .describe('mac-address, network-address, interface-name, local, …; "" = none'),
  portId: z.string().describe('peer port id; "" = none heard'),
  portIdSubtype: z.string().describe('interface-name, mac-address, local, …; "" = none'),
  ttl: z.number().int().describe('time to live the peer advertised (s)'),
  lastHeardSecAgo: z
    .number()
    .describe('seconds since the peer was last heard (VPP clock, estimated); 0 = never'),
  lastSentSecAgo: z
    .number()
    .describe('seconds since this interface last sent an LLDPDU; 0 = never'),
  configured: z.boolean().describe('the running configuration enables LLDP on this interface'),
  portDescription: z
    .string()
    .nullable()
    .describe('the running port description; null when not configured'),
});
const NeighborsOut = z.object({
  retrievedAt: z.string().optional(),
  page: z.number().int(),
  pageSize: z.number().int(),
  total: z.number().int(),
  items: z.array(NeighborOut),
});
const NeighborsQuery = z.object({
  page: z.coerce.number().int().min(1).default(1),
  pageSize: z.coerce.number().int().min(1).max(1000).default(100),
});

/** The running `services.lldp.interfaces` by interface name (empty when LLDP is disabled). */
export function lldpInterfacesOf(doc: Json): Map<string, Json> {
  const services = doc['services'];
  const lldp = isPlainObject(services) ? services['lldp'] : undefined;
  const out = new Map<string, Json>();
  if (!isPlainObject(lldp) || lldp['enabled'] !== true || !Array.isArray(lldp['interfaces']))
    return out;
  for (const e of lldp['interfaces'] as unknown[]) {
    if (isPlainObject(e) && typeof e['interface'] === 'string') out.set(e['interface'], e);
  }
  return out;
}

function neighborJson(
  n: LldpNeighbor,
  configured: Map<string, Json>,
): z.output<typeof NeighborOut> {
  const cfg = configured.get(n.interface);
  const desc = cfg?.['portDescription'];
  return {
    interface: n.interface,
    swIfIndex: n.swIfIndex,
    heard: n.heard,
    chassisId: n.chassisId,
    chassisIdSubtype: n.chassisIdSubtype,
    portId: n.portId,
    portIdSubtype: n.portIdSubtype,
    ttl: n.ttl,
    lastHeardSecAgo: n.lastHeardSecAgo,
    lastSentSecAgo: n.lastSentSecAgo,
    configured: cfg !== undefined,
    portDescription: typeof desc === 'string' ? desc : null,
  };
}

/**
 * F-loopback-bvi-gso-lldp-span live state under `/api/v1/state/lldp/**` (00-CONTEXT rule 8: read-only), served from the
 * agent's LldpNeighbors RPC (docs/contracts/proto.md §11). LLDP, GSO, mirroring and nsim are configured through the
 * generic pointer routes (`/api/v1/config/services/lldp`, `/api/v1/config/interfaces/<if>/gso|mirror`,
 * `/api/v1/config/services/nsim`); mirror sessions and GSO appear in `/api/v1/state/interfaces` (`config`).
 */
@ApiTags('state')
@Controller('api/v1/state/lldp')
export class LoopbackBviGsoLldpSpanController {
  constructor(
    private readonly agent: AgentClient,
    private readonly ds: DatastoreService,
  ) {}

  @Get('neighbors')
  @Protected(400, 501, 502, 503)
  @ApiQuery({ name: 'page', required: false, schema: { type: 'integer', minimum: 1 } })
  @ApiQuery({
    name: 'pageSize',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 1000 },
  })
  @ApiOperation({
    summary:
      'LLDP table: every LLDP-enabled interface the agent can name with the peer heard on it (agent LldpNeighbors, server-side paged, ordered by interface), marked with the running configuration',
  })
  @ApiOkResponse({ schema: openapi(NeighborsOut, 'output') })
  async neighbors(@Query(new ZodPipe(NeighborsQuery)) q: z.output<typeof NeighborsQuery>) {
    const [r, running] = await Promise.all([
      this.agent.lldpNeighbors({ offset: (q.page - 1) * q.pageSize, limit: q.pageSize }),
      this.ds.getRunning(),
    ]);
    const configured = lldpInterfacesOf(running.doc);
    return {
      retrievedAt: r.retrievedAt?.toISOString(),
      page: q.page,
      pageSize: q.pageSize,
      total: r.total,
      items: r.neighbors.map((n) => neighborJson(n, configured)),
    };
  }
}
