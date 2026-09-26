import { Controller, Get, Inject } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import type { Srv6StateResponse } from '@ngfw/proto';
import { canonicalIp, canonicalPrefix } from '@ngfw/schema';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';
import { DatastoreService } from '../../datastore/datastore.service.js';

type Json = Record<string, unknown>;

const LocalSidOut = z.object({
  sid: z.string(),
  behavior: z.string().describe('end, end.x, end.t, end.dx2, end.dx4, end.dx6, end.dt4, end.dt6'),
  psp: z.boolean(),
  vrf: z.string().describe('VRF (IPv6 table) the SID is installed in'),
  table: z.number().int(),
  interface: z.string().nullable(),
  nextHop: z.string().nullable(),
  lookupVrf: z
    .string()
    .nullable()
    .describe('end.t / end.dt4 / end.dt6: the VRF the inner packet is looked up in'),
  lookupTable: z.number().int().nullable(),
  goodPackets: z.number().describe('packets processed by the SID'),
  goodBytes: z.number(),
  badPackets: z.number().describe('packets the SID dropped'),
  badBytes: z.number(),
  configured: z.boolean().describe('whether the running configuration names this SID'),
});
const SidListOut = z.object({ sids: z.array(z.string()), weight: z.number().int() });
const PolicyOut = z.object({
  bsid: z.string(),
  type: z.string().describe('default, spray or tef'),
  encap: z.boolean(),
  vrf: z.string().describe('VRF (IPv6 table) of the binding SID'),
  table: z.number().int(),
  encapSource: z.string().nullable().describe('outer source address (encapsulating policies)'),
  sidLists: z.array(SidListOut),
  configured: z.boolean(),
});
const SteeringOut = z.object({
  type: z.enum(['l3', 'l2']),
  trafficType: z.enum(['ipv4', 'ipv6', 'l2']),
  prefix: z.string().nullable(),
  vrf: z.string().nullable(),
  table: z.number().int().nullable(),
  interface: z.string().nullable(),
  bsid: z.string(),
  configured: z.boolean(),
});
export const Srv6StateOut = z.object({
  retrievedAt: z.string().nullable(),
  localSids: z.array(LocalSidOut),
  policies: z.array(PolicyOut),
  steering: z.array(SteeringOut),
});
export type Srv6StateOut = z.infer<typeof Srv6StateOut>;

const LOOKUP_BEHAVIORS = new Set(['end.t', 'end.dt4', 'end.dt6']);

const asObject = (v: unknown): Json =>
  v !== null && typeof v === 'object' && !Array.isArray(v) ? (v as Json) : {};

/** Table id → VRF name of the running configuration (`vrfs.<name>.id`); 0 is `default` unless a VRF names it. */
export function vrfNames(running: Json): (table: number) => string {
  const byId = new Map<number, string>();
  for (const [name, v] of Object.entries(asObject(running['vrfs']))) {
    const id = asObject(v)['id'];
    if (typeof id === 'number' && !byId.has(id)) byId.set(id, name);
  }
  return (table) => byId.get(table) ?? (table === 0 ? 'default' : String(table));
}

/** What `routing.srv6` of the running configuration names: SIDs, BSIDs and steering keys (canonical). */
export function srv6Configured(running: Json): {
  sids: Set<string>;
  bsids: Set<string>;
  steering: Set<string>;
} {
  const s = asObject(asObject(running['routing'])['srv6']);
  const canon = (a: string) => canonicalIp(a) ?? a;
  const steering = new Set<string>();
  for (const raw of Array.isArray(s['steering']) ? (s['steering'] as unknown[]) : []) {
    const st = asObject(raw);
    if (st['type'] === 'l2' && typeof st['interface'] === 'string')
      steering.add(`l2|${st['interface']}`);
    if (st['type'] === 'l3' && typeof st['prefix'] === 'string') {
      const vrf = typeof st['vrf'] === 'string' ? st['vrf'] : 'default';
      steering.add(`l3|${vrf}|${canonicalPrefix(st['prefix']) ?? st['prefix']}`);
    }
  }
  return {
    sids: new Set(Object.keys(asObject(s['localSids'])).map(canon)),
    bsids: new Set(Object.keys(asObject(s['policies'])).map(canon)),
    steering,
  };
}

/** The agent's Srv6State joined with the running configuration's VRF names and keys (pure; unit-tested). */
export function srv6StateOf(st: Srv6StateResponse, running: Json): Srv6StateOut {
  const vrf = vrfNames(running);
  const cfg = srv6Configured(running);
  const canon = (a: string) => canonicalIp(a) ?? a;
  const orNull = (s: string) => (s === '' ? null : s);
  return {
    retrievedAt: st.retrievedAt?.toISOString() ?? null,
    localSids: st.localSids.map((l) => {
      const lookup = LOOKUP_BEHAVIORS.has(l.behavior);
      return {
        sid: l.sid,
        behavior: l.behavior,
        psp: l.psp,
        vrf: vrf(l.fibTable),
        table: l.fibTable,
        interface: orNull(l.interface),
        nextHop: orNull(l.nextHop),
        lookupVrf: lookup ? vrf(l.lookupTable) : null,
        lookupTable: lookup ? l.lookupTable : null,
        goodPackets: Number(l.goodPackets),
        goodBytes: Number(l.goodBytes),
        badPackets: Number(l.badPackets),
        badBytes: Number(l.badBytes),
        configured: cfg.sids.has(canon(l.sid)),
      };
    }),
    policies: st.policies.map((p) => ({
      bsid: p.bsid,
      type: p.type,
      encap: p.encap,
      vrf: vrf(p.fibTable),
      table: p.fibTable,
      encapSource: orNull(p.encapSource),
      sidLists: p.sidLists.map((l) => ({ sids: [...l.sids], weight: l.weight })),
      configured: cfg.bsids.has(canon(p.bsid)),
    })),
    steering: st.steering.map((s) => {
      if (s.trafficType === 'l2') {
        return {
          type: 'l2' as const,
          trafficType: 'l2' as const,
          prefix: null,
          vrf: null,
          table: null,
          interface: s.interface,
          bsid: s.bsid,
          configured: cfg.steering.has(`l2|${s.interface}`),
        };
      }
      const name = vrf(s.fibTable);
      return {
        type: 'l3' as const,
        trafficType: s.trafficType === 'ipv4' ? ('ipv4' as const) : ('ipv6' as const),
        prefix: s.prefix,
        vrf: name,
        table: s.fibTable,
        interface: null,
        bsid: s.bsid,
        configured: cfg.steering.has(`l3|${name}|${canonicalPrefix(s.prefix) ?? s.prefix}`),
      };
    }),
  };
}

/**
 * F-srv6: live SRv6 state (`GET /api/v1/state/srv6`) — the agent's Srv6State RPC (this owner's local SIDs with their
 * good/bad counters, policies with their segment lists, steering entries) joined with the running configuration's VRF
 * names and keys. Configuration goes through the generic `/api/v1/config/**` routes (`routing.srv6`). The state walks
 * VPP: clients refresh on demand or at most every 30 s (D-132). The encapsulation globals are write-only in VPP and
 * never appear here.
 */
@ApiTags('routing')
@Controller('api/v1')
export class Srv6Controller {
  constructor(
    @Inject(AgentClient) private readonly agent: AgentClient,
    @Inject(DatastoreService) private readonly ds: DatastoreService,
  ) {}

  @Get('state/srv6')
  @Protected(502, 503)
  @ApiOperation({
    summary:
      'SRv6 local SIDs (with good/bad counters), policies and steering of this agent, joined with the running VRF names',
  })
  @ApiOkResponse({ schema: openapi(Srv6StateOut, 'output') })
  async state() {
    const [st, running] = await Promise.all([this.agent.srv6State(), this.ds.getRunning()]);
    return srv6StateOf(st, running.doc as Json);
  }
}
