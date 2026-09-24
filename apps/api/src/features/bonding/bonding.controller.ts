import { Controller, Get } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import type { BondLacpPort, BondStatus } from '@ngfw/proto';
import { deepEqual, isPlainObject } from '@ngfw/schema';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { ProblemError } from '../../common/problem.js';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';
import { DatastoreService } from '../../datastore/datastore.service.js';

type Json = Record<string, unknown>;

const LacpPortOut = z.object({
  systemPriority: z.number().int(),
  system: z.string().describe('system id MAC (all zero while no partner has been seen)'),
  key: z.number().int(),
  portPriority: z.number().int(),
  portNumber: z.number().int(),
  state: z.number().int().describe('LACP state octet (802.1AX)'),
  stateFlags: z
    .array(z.string())
    .describe(
      'set bits: activity, timeout, aggregation, synchronization, collecting, distributing, defaulted, expired',
    ),
});
const LacpOut = z.object({
  rxState: z
    .string()
    .describe('initialize | port-disabled | expired | lacp-disabled | defaulted | current'),
  txState: z.string(),
  muxState: z.string().describe('detached | waiting | attached | collecting-distributing'),
  ptxState: z.string().describe('no-periodic | fast-periodic | slow-periodic | periodic-tx'),
  actor: LacpPortOut,
  partner: LacpPortOut,
});
const LiveMemberOut = z.object({
  interface: z.string().describe('logical interface name'),
  swIfIndex: z.number().int(),
  passive: z.boolean(),
  longTimeout: z.boolean(),
  weight: z.number().int().describe('0 = never set'),
  isLocalNuma: z.boolean(),
  adminUp: z.boolean(),
  linkUp: z.boolean(),
  lacp: LacpOut.nullable().describe('LACP state; null for bonds in other modes'),
});
const LiveBondOut = z
  .object({
    vppName: z.string(),
    swIfIndex: z.number().int(),
    id: z.number().int(),
    mode: z.string().describe('lacp | xor | round-robin | active-backup | broadcast'),
    loadBalance: z
      .string()
      .describe(
        'l2 | l23 | l34, or the algorithm VPP forces: round-robin | active-backup | broadcast',
      ),
    numaOnly: z.boolean(),
    adminUp: z.boolean(),
    linkUp: z.boolean(),
    memberCount: z.number().int(),
    activeMemberCount: z.number().int(),
    members: z.array(LiveMemberOut),
  })
  .describe('live state from the agent (BondState RPC, dumped from VPP)');
const BondItemOut = z.object({
  name: z.string().describe('bond interface (BondEthernet<id>)'),
  state: LiveBondOut.nullable().describe(
    'null when VPP has no such bond (or the agent has no BondState RPC)',
  ),
  running: z
    .record(z.string(), z.unknown())
    .nullable()
    .describe(
      'interfaces.<name>.bond of the running configuration; null when it is not configured',
    ),
  candidate: z
    .record(z.string(), z.unknown())
    .nullable()
    .describe('interfaces.<name>.bond of the candidate; null when the candidate has none'),
  hasPendingChange: z
    .boolean()
    .describe(
      'the candidate differs from running for this bond (interfaces.<name>, bond leaf included)',
    ),
});
const BondsOut = z.object({
  retrievedAt: z.string().optional(),
  live: z
    .boolean()
    .describe('false when the agent does not implement BondState (live state unknown)'),
  items: z.array(BondItemOut),
});

type LiveBond = z.output<typeof LiveBondOut>;

/** `interfaces.<name>` entries of a document that carry a `bond` leaf. */
function bondedInterfaces(doc: Json): Map<string, Json> {
  const out = new Map<string, Json>();
  const ifs = doc['interfaces'];
  if (!isPlainObject(ifs)) return out;
  for (const [name, itf] of Object.entries(ifs)) {
    if (isPlainObject(itf) && isPlainObject(itf['bond'])) out.set(name, itf);
  }
  return out;
}

function portJson(p: BondLacpPort | undefined): z.output<typeof LacpPortOut> {
  return {
    systemPriority: p?.systemPriority ?? 0,
    system: p?.system ?? '',
    key: p?.key ?? 0,
    portPriority: p?.portPriority ?? 0,
    portNumber: p?.portNumber ?? 0,
    state: p?.state ?? 0,
    stateFlags: p?.stateFlags ?? [],
  };
}

/** BondStatus as JSON with every field present (ts-proto's toJSON drops defaults such as false/0). */
function liveJson(s: BondStatus): LiveBond {
  return {
    vppName: s.vppName,
    swIfIndex: s.swIfIndex,
    id: s.id,
    mode: s.mode,
    loadBalance: s.loadBalance,
    numaOnly: s.numaOnly,
    adminUp: s.adminUp,
    linkUp: s.linkUp,
    memberCount: s.memberCount,
    activeMemberCount: s.activeMemberCount,
    members: s.members.map((m) => ({
      interface: m.interface,
      swIfIndex: m.swIfIndex,
      passive: m.passive,
      longTimeout: m.longTimeout,
      weight: m.weight,
      isLocalNuma: m.isLocalNuma,
      adminUp: m.adminUp,
      linkUp: m.linkUp,
      lacp: m.lacp
        ? {
            rxState: m.lacp.rxState,
            txState: m.lacp.txState,
            muxState: m.lacp.muxState,
            ptxState: m.lacp.ptxState,
            actor: portJson(m.lacp.actor),
            partner: portJson(m.lacp.partner),
          }
        : null,
    })),
  };
}

/**
 * F-bonding live state `GET /api/v1/state/interfaces/bonds` (00-CONTEXT rule 8: read-only), served from the agent's
 * BondState RPC (docs/contracts/proto.md §11 "F-bonding") and merged with the running and candidate configuration.
 * Bonds are configured through the generic pointer routes (`PATCH /api/v1/config/interfaces`).
 */
@ApiTags('state')
@Controller('api/v1/state/interfaces')
export class BondingController {
  constructor(
    private readonly agent: AgentClient,
    private readonly ds: DatastoreService,
  ) {}

  @Get('bonds')
  @Protected(502, 503)
  @ApiOperation({
    summary:
      'Bond interfaces: live state from VPP (agent BondState: mode, load balance, members with LACP actor/partner state, active member count) merged with the running configuration and pending candidate changes',
  })
  @ApiOkResponse({ schema: openapi(BondsOut, 'output') })
  async bonds() {
    const [st, running, candidate] = await Promise.all([
      this.live(),
      this.ds.getRunning(),
      this.ds.getCandidate(),
    ]);
    const run = bondedInterfaces(running.doc);
    const cand = bondedInterfaces(candidate);
    const live = new Map((st?.bonds ?? []).map((b) => [b.name, b]));
    const names = new Set([...run.keys(), ...cand.keys(), ...live.keys()]);
    const items = [...names]
      .sort((a, b) => a.localeCompare(b, undefined, { numeric: true }))
      .map((name) => {
        const s = live.get(name);
        const r = run.get(name);
        const c = cand.get(name);
        return {
          name,
          state: s ? liveJson(s) : null,
          running: (r?.['bond'] as Json | undefined) ?? null,
          candidate: (c?.['bond'] as Json | undefined) ?? null,
          hasPendingChange: !deepEqual(r ?? null, c ?? null),
        };
      });
    return { retrievedAt: st?.retrievedAt?.toISOString(), live: st !== undefined, items };
  }

  /** The agent's bond table; undefined when the agent predates the BondState RPC (501). */
  private async live() {
    try {
      return await this.agent.bondState();
    } catch (e) {
      if (e instanceof ProblemError && e.getStatus() === 501) return undefined;
      throw e;
    }
  }
}
