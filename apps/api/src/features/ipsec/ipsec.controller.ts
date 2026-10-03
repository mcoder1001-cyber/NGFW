import { Controller, Get, Inject, Query } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import type { IpsecStateResponse } from '@ngfw/proto';
import { ipsecObjectName } from '@ngfw/schema';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';

/**
 * P11: live strongSwan/IPsec state (read-only; the agent's IpsecState RPC — charon's VICI
 * list-conns / list-sas / stats — never key material). Two views:
 *   GET /api/v1/state/ipsec/tunnels  the loaded connections with an up/connecting/down status
 *   GET /api/v1/state/ipsec/sas      the IKE_SAs and their CHILD_SAs (SPIs, algorithms, byte/packet
 *                                    counters, rekey timers) — the SA inspection drawer
 * Live SA changes arrive on the WebSocket topic `ipsec.events` (EVENT_KIND_IPSEC_SA_CHANGED, relayed by
 * telemetry/relay.service.ts). Configuration goes through the generic `/api/v1/config/**` routes. PSK tunnels are
 * refused at commit until PENDING-secret-channel is answered (the agent has no secret resolver); certificates are
 * F-pki's. int64 counters are rendered as JSON numbers (exact up to 2^53 bytes — about 9 PB per SA).
 */

const ChildSaOut = z.object({
  name: z.string(),
  uniqueId: z.string(),
  reqId: z.number().int(),
  state: z.string().describe('INSTALLED, REKEYING, DELETING, …'),
  mode: z.string(),
  protocol: z.string().describe('ESP or AH'),
  encap: z.boolean().describe('UDP-encapsulated (NAT-T)'),
  spiIn: z.string(),
  spiOut: z.string(),
  encrAlg: z.string(),
  encrKeysize: z.number().int().describe('bits; 0 when charon reports none'),
  integAlg: z.string(),
  dhGroup: z.string(),
  esn: z.boolean(),
  bytesIn: z.number(),
  packetsIn: z.number(),
  bytesOut: z.number(),
  packetsOut: z.number(),
  rekeySec: z.number().int().describe('seconds until scheduled rekey'),
  lifeSec: z.number().int(),
  installSec: z.number().int(),
  localTs: z.array(z.string()),
  remoteTs: z.array(z.string()),
  ifIdIn: z.string().describe('route-based (kernel-vpp) tunnel id; "" for policy-based'),
  ifIdOut: z.string(),
});
const IkeSaOut = z.object({
  name: z.string().describe('connection name'),
  tunnel: z.string().describe('document tunnel name'),
  uniqueId: z.string(),
  version: z.string(),
  state: z.string().describe('ESTABLISHED, CONNECTING, DELETING, …'),
  localHost: z.string(),
  localPort: z.number().int(),
  localId: z.string(),
  remoteHost: z.string(),
  remotePort: z.number().int(),
  remoteId: z.string(),
  initiator: z.boolean(),
  natAny: z.boolean(),
  encrAlg: z.string(),
  encrKeysize: z.number().int(),
  integAlg: z.string(),
  prfAlg: z.string(),
  dhGroup: z.string(),
  establishedSec: z.number().int(),
  rekeySec: z.number().int(),
  reauthSec: z.number().int(),
  children: z.array(ChildSaOut),
});
export const SasOut = z.object({
  retrievedAt: z.string().nullable(),
  eventsActive: z.boolean(),
  charonRestarted: z
    .boolean()
    .describe(
      'charon restarted since the last acknowledgement (orphaned SAs may exist until reconcile)',
    ),
  daemonVersion: z.string(),
  unlistedSas: z
    .number()
    .int()
    .describe('IKE_SAs charon counts that belong to no owned connection'),
  total: z.number().int().describe('IKE_SAs matching the query before offset/limit'),
  pendingAction: z
    .string()
    .describe('a charon action the last commit could not take itself ("" = none)'),
  sas: z.array(IkeSaOut),
});

const ChildConnOut = z.object({
  name: z.string(),
  mode: z.string(),
  rekeySec: z.number().int(),
  localTs: z.array(z.string()),
  remoteTs: z.array(z.string()),
});
const TunnelOut = z.object({
  tunnel: z.string().describe('document tunnel name'),
  conn: z.string().describe('charon connection (section) name'),
  status: z
    .enum(['up', 'connecting', 'down'])
    .describe('up = an IKE_SA is ESTABLISHED with an INSTALLED CHILD_SA'),
  version: z.string(),
  localAddrs: z.array(z.string()),
  remoteAddrs: z.array(z.string()),
  localId: z.string(),
  remoteId: z.string(),
  localAuth: z.string(),
  remoteAuth: z.string(),
  rekeySec: z.number().int(),
  reauthSec: z.number().int(),
  children: z.array(ChildConnOut),
});
const TunnelsOut = z.object({
  retrievedAt: z.string().nullable(),
  eventsActive: z.boolean(),
  charonRestarted: z.boolean(),
  daemonVersion: z.string(),
  pendingAction: z.string(),
  tunnels: z.array(TunnelOut),
});

const StateQuery = z.object({
  tunnel: ipsecObjectName.optional().describe('only this document tunnel'),
});
const SasQuery = StateQuery.extend({
  offset: z.coerce.number().int().min(0).max(1_000_000).default(0),
  limit: z.coerce.number().int().min(1).max(1000).default(1000),
});

/** The IKE_SAs and CHILD_SAs of the agent's IpsecState, mapped to the API shape (pure; unit-tested). */
export function ipsecSasOf(st: IpsecStateResponse): z.infer<typeof SasOut> {
  return {
    retrievedAt: st.retrievedAt?.toISOString() ?? null,
    eventsActive: st.eventsActive,
    charonRestarted: st.charonRestarted,
    daemonVersion: st.daemonVersion,
    unlistedSas: Number(st.unlistedSas),
    total: st.total,
    pendingAction: st.pendingAction,
    sas: st.sas.map((sa) => ({
      name: sa.name,
      tunnel: sa.tunnel,
      uniqueId: sa.uniqueId,
      version: sa.version,
      state: sa.state,
      localHost: sa.localHost,
      localPort: sa.localPort,
      localId: sa.localId,
      remoteHost: sa.remoteHost,
      remotePort: sa.remotePort,
      remoteId: sa.remoteId,
      initiator: sa.initiator,
      natAny: sa.natAny,
      encrAlg: sa.encrAlg,
      encrKeysize: sa.encrKeysize,
      integAlg: sa.integAlg,
      prfAlg: sa.prfAlg,
      dhGroup: sa.dhGroup,
      establishedSec: Number(sa.establishedSec),
      rekeySec: Number(sa.rekeySec),
      reauthSec: Number(sa.reauthSec),
      children: sa.children.map((ch) => ({
        name: ch.name,
        uniqueId: ch.uniqueId,
        reqId: ch.reqId,
        state: ch.state,
        mode: ch.mode,
        protocol: ch.protocol,
        encap: ch.encap,
        spiIn: ch.spiIn,
        spiOut: ch.spiOut,
        encrAlg: ch.encrAlg,
        encrKeysize: ch.encrKeysize,
        integAlg: ch.integAlg,
        dhGroup: ch.dhGroup,
        esn: ch.esn,
        bytesIn: Number(ch.bytesIn),
        packetsIn: Number(ch.packetsIn),
        bytesOut: Number(ch.bytesOut),
        packetsOut: Number(ch.packetsOut),
        rekeySec: Number(ch.rekeySec),
        lifeSec: Number(ch.lifeSec),
        installSec: Number(ch.installSec),
        localTs: ch.localTs,
        remoteTs: ch.remoteTs,
        ifIdIn: ch.ifIdIn,
        ifIdOut: ch.ifIdOut,
      })),
    })),
  };
}

/** Per-connection status from the live IKE_SAs: up (an ESTABLISHED SA with an INSTALLED child),
 * connecting (an SA that is not yet ESTABLISHED), else down. */
function connStatus(sas: IpsecStateResponse['sas'], conn: string): 'up' | 'connecting' | 'down' {
  let connecting = false;
  for (const sa of sas) {
    if (sa.name !== conn) continue;
    if (sa.state === 'ESTABLISHED' && sa.children.some((c) => c.state === 'INSTALLED')) return 'up';
    connecting = true;
  }
  return connecting ? 'connecting' : 'down';
}

/** The loaded connections of the agent's IpsecState with an up/connecting/down status (pure). */
export function ipsecTunnelsOf(st: IpsecStateResponse): z.infer<typeof TunnelsOut> {
  return {
    retrievedAt: st.retrievedAt?.toISOString() ?? null,
    eventsActive: st.eventsActive,
    charonRestarted: st.charonRestarted,
    daemonVersion: st.daemonVersion,
    pendingAction: st.pendingAction,
    tunnels: st.conns.map((c) => ({
      tunnel: c.tunnel,
      conn: c.name,
      status: connStatus(st.sas, c.name),
      version: c.version,
      localAddrs: c.localAddrs,
      remoteAddrs: c.remoteAddrs,
      localId: c.localId,
      remoteId: c.remoteId,
      localAuth: c.localAuth,
      remoteAuth: c.remoteAuth,
      rekeySec: Number(c.rekeySec),
      reauthSec: Number(c.reauthSec),
      children: c.children.map((ch) => ({
        name: ch.name,
        mode: ch.mode,
        rekeySec: Number(ch.rekeySec),
        localTs: ch.localTs,
        remoteTs: ch.remoteTs,
      })),
    })),
  };
}

@ApiTags('vpn')
@Controller('api/v1')
export class IpsecController {
  constructor(@Inject(AgentClient) private readonly agent: AgentClient) {}

  @Get('state/ipsec/tunnels')
  @Protected(400, 501, 502, 503)
  @ApiQuery({ name: 'tunnel', required: false, schema: { type: 'string' } })
  @ApiOperation({
    summary: 'IPsec tunnels: the loaded strongSwan connections with an up/connecting/down status',
  })
  @ApiOkResponse({ schema: openapi(TunnelsOut, 'output') })
  async tunnels(@Query(new ZodPipe(StateQuery)) q: z.output<typeof StateQuery>) {
    const st = await this.agent.ipsecState(q.tunnel === undefined ? [] : [q.tunnel]);
    return ipsecTunnelsOf(st);
  }

  @Get('state/ipsec/sas')
  @Protected(400, 501, 502, 503)
  @ApiQuery({ name: 'tunnel', required: false, schema: { type: 'string' } })
  @ApiQuery({ name: 'offset', required: false, schema: { type: 'integer', minimum: 0 } })
  @ApiQuery({
    name: 'limit',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 1000 },
  })
  @ApiOperation({
    summary:
      'IPsec security associations: IKE_SAs and their CHILD_SAs with SPIs, algorithms, byte/packet counters and rekey timers (no keys)',
  })
  @ApiOkResponse({ schema: openapi(SasOut, 'output') })
  async sas(@Query(new ZodPipe(SasQuery)) q: z.output<typeof SasQuery>) {
    const st = await this.agent.ipsecState(
      q.tunnel === undefined ? [] : [q.tunnel],
      q.offset,
      q.limit,
    );
    return ipsecSasOf(st);
  }
}
