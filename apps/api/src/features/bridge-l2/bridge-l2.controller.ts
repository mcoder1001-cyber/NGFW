import { Controller, Get, Param, Query } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiParam, ApiQuery, ApiTags } from '@nestjs/swagger';
import type { BridgeDomainStatus } from '@ngfw/proto';
import { deepEqual, isPlainObject } from '@ngfw/schema';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { ProblemError, problems } from '../../common/problem.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { DatastoreService } from '../../datastore/datastore.service.js';

type Json = Record<string, unknown>;

const MemberOut = z.object({
  interface: z.string().describe('logical interface name'),
  swIfIndex: z.number().int(),
  portType: z.enum(['normal', 'bvi', 'uu-fwd']),
  shg: z.number().int(),
  tagRewrite: z.string().describe('VLAN tag rewrite in the configuration spelling ("" = none)'),
});
const LiveBridgeDomainOut = z
  .object({
    id: z.number().int(),
    flood: z.boolean(),
    uuFlood: z.boolean(),
    forward: z.boolean(),
    learn: z.boolean(),
    arpTerm: z.boolean(),
    arpUfwd: z.boolean(),
    macAgeMin: z.number().int(),
    bvi: z.string().describe('BVI interface ("" = none)'),
    uuFwd: z.string().describe('uu-fwd interface ("" = none)'),
    members: z.array(MemberOut),
    learnedMacs: z.number().int(),
    staticMacs: z.number().int().describe('static, filter and BVI L2 FIB entries'),
  })
  .describe('live state from the agent (BridgeDomainState RPC, dumped from VPP)');
const BridgeDomainItemOut = z.object({
  name: z
    .string()
    .describe('record name (routing.l2.bridgeDomains key; the id when VPP holds no name)'),
  id: z.number().int().nullable(),
  state: LiveBridgeDomainOut.nullable().describe('null when VPP has no such bridge domain'),
  running: z
    .record(z.string(), z.unknown())
    .nullable()
    .describe('the running configuration of the record; null when it is not configured'),
  hasPendingChange: z.boolean().describe('the candidate differs from running for this record'),
});
const BridgeDomainsOut = z.object({
  retrievedAt: z.string().optional(),
  items: z.array(BridgeDomainItemOut),
});
const MacOut = z.object({
  mac: z.string(),
  interface: z.string().describe('"" for a filter entry'),
  swIfIndex: z.number().int(),
  static: z.boolean(),
  filter: z.boolean(),
  bvi: z.boolean(),
});
const MacsOut = z.object({
  page: z.number().int(),
  pageSize: z.number().int(),
  total: z.number().int(),
  items: z.array(MacOut),
});
const MacsQuery = z.object({
  page: z.coerce.number().int().min(1).default(1),
  pageSize: z.coerce.number().int().min(1).max(1000).default(100),
});
const BdIdParam = z.coerce.number().int().min(1).max(16777215);

function bridgeDomainsOf(doc: Json): Record<string, Json> {
  const routing = doc['routing'];
  const l2 = isPlainObject(routing) ? routing['l2'] : undefined;
  const bds = isPlainObject(l2) ? l2['bridgeDomains'] : undefined;
  return isPlainObject(bds) ? (bds as Record<string, Json>) : {};
}

function liveJson(s: BridgeDomainStatus): z.output<typeof LiveBridgeDomainOut> {
  return {
    id: s.id,
    flood: s.flood,
    uuFlood: s.uuFlood,
    forward: s.forward,
    learn: s.learn,
    arpTerm: s.arpTerm,
    arpUfwd: s.arpUfwd,
    macAgeMin: s.macAgeMin,
    bvi: s.bvi,
    uuFwd: s.uuFwd,
    members: s.members.map((m) => ({
      interface: m.interface,
      swIfIndex: m.swIfIndex,
      portType: (['normal', 'bvi', 'uu-fwd'].includes(m.portType) ? m.portType : 'normal') as
        'normal' | 'bvi' | 'uu-fwd',
      shg: m.shg,
      tagRewrite: m.tagRewrite,
    })),
    learnedMacs: s.learnedMacs,
    staticMacs: s.staticMacs,
  };
}

/**
 * F-bridge-l2 live state under `/api/v1/state/l2/**` (00-CONTEXT rule 8: read-only). Everything comes from the agent's
 * BridgeDomainState / BridgeDomainMacs RPCs (docs/contracts/proto.md §11); configuration goes through the generic
 * pointer routes (`/api/v1/config/routing/l2/…`, `/api/v1/config/interfaces/<if>/l2`).
 */
@ApiTags('state')
@Controller('api/v1/state/l2')
export class BridgeL2Controller {
  constructor(
    private readonly agent: AgentClient,
    private readonly ds: DatastoreService,
  ) {}

  @Get('bridge-domains')
  @Protected(501, 502, 503)
  @ApiOperation({
    summary:
      'Bridge domains: live state from VPP (agent BridgeDomainState: members, BVI, flags, learned-MAC count) merged with the running configuration and pending candidate changes',
  })
  @ApiOkResponse({ schema: openapi(BridgeDomainsOut, 'output') })
  async bridgeDomains() {
    const [st, running, candidate] = await Promise.all([
      this.agent.bridgeDomainState(),
      this.ds.getRunning(),
      this.ds.getCandidate(),
    ]);
    const run = bridgeDomainsOf(running.doc);
    const cand = bridgeDomainsOf(candidate);
    const live = new Map<string, BridgeDomainStatus>();
    for (const bd of st.bridgeDomains) live.set(bd.name || String(bd.id), bd);
    const names = new Set([...Object.keys(run), ...Object.keys(cand), ...live.keys()]);
    const items = [...names].sort().map((name) => {
      const s = live.get(name);
      const cfg = run[name] ?? cand[name];
      const id = typeof cfg?.['id'] === 'number' ? cfg['id'] : (s?.id ?? null);
      return {
        name,
        id,
        state: s ? liveJson(s) : null,
        running: run[name] ?? null,
        hasPendingChange: !deepEqual(run[name] ?? null, cand[name] ?? null),
      };
    });
    return { retrievedAt: st.retrievedAt?.toISOString(), items };
  }

  @Get('bridge-domains/:id/macs')
  @Protected(400, 404, 501, 502, 503)
  @ApiParam({ name: 'id', schema: { type: 'integer', minimum: 1, maximum: 16777215 } })
  @ApiQuery({ name: 'page', required: false, schema: { type: 'integer', minimum: 1 } })
  @ApiQuery({
    name: 'pageSize',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 1000 },
  })
  @ApiOperation({
    summary:
      'L2 FIB of one bridge domain (agent BridgeDomainMacs, server-side paged, ordered by MAC)',
  })
  @ApiOkResponse({ schema: openapi(MacsOut, 'output') })
  async bridgeDomainMacs(
    @Param('id', new ZodPipe(BdIdParam)) id: number,
    @Query(new ZodPipe(MacsQuery)) q: z.output<typeof MacsQuery>,
  ) {
    try {
      const r = await this.agent.bridgeDomainMacs({
        bdId: id,
        offset: (q.page - 1) * q.pageSize,
        limit: q.pageSize,
      });
      return {
        page: q.page,
        pageSize: q.pageSize,
        total: r.total,
        items: r.macs.map((m) => ({
          mac: m.mac,
          interface: m.interface,
          swIfIndex: m.swIfIndex,
          static: m.static,
          filter: m.filter,
          bvi: m.bvi,
        })),
      };
    } catch (e) {
      // the agent answers NOT_FOUND for a bridge domain that is not its own (keyed on the gRPC code, review #10)
      if (e instanceof ProblemError && e.extra['grpcCode'] === 'NOT_FOUND') {
        throw problems.notFound(`bridge domain ${id} does not exist on the data plane`);
      }
      throw e;
    }
  }
}
