import { Controller, Get, Param, Query } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiParam, ApiQuery, ApiTags } from '@nestjs/swagger';
import {
  DesiredState,
  type InterfaceCounters,
  type InterfaceState,
  IssueSeverity,
  type StatsBatch,
  type ValidationIssue,
} from '@ngfw/proto';
import {
  canonicalPrefix,
  deepEqual,
  diff,
  isPlainObject,
  parsePointer,
  type Change,
} from '@ngfw/schema';
import { z } from 'zod';
import { AgentClient } from '../agent/agent.client.js';
import { SystemEventsService } from '../audit/system-events.service.js';
import { ProblemError, problems } from '../common/problem.js';
import { ApiOut, Protected } from '../common/responses.js';
import { openapi, ZodPipe } from '../common/zod.js';
import { safeText } from '../common/text.js';
import { CommitService } from '../commit/commit.service.js';
import { ValidationService } from '../commit/validation.service.js';
import { DatastoreService } from '../datastore/datastore.service.js';
import { redact } from '../datastore/documents.js';
import { RelayService } from '../telemetry/relay.service.js';

const startedAt = new Date();

const RoutesQuery = z.object({
  vrf: safeText(64).optional(),
  page: z.coerce.number().int().min(1).default(1),
  pageSize: z.coerce.number().int().min(1).max(1000).default(100),
});
const PageQuery = z.object({
  limit: z.coerce.number().int().min(1).max(500).default(100),
  offset: z.coerce.number().int().min(0).default(0),
});

const RouteOut = z.object({
  vrf: z.string(),
  prefix: z.string(),
  origin: z.enum(['connected', 'static']),
  nextHops: z.array(z.object({ address: z.string().optional(), interface: z.string().optional() })),
  distance: z.number().int().optional(),
});

const SystemOut = z.object({
  api: z.object({ version: z.string(), startedAt: z.string(), wsClients: z.number().int() }),
  agent: z
    .record(z.string(), z.unknown())
    .describe('HealthResponse plus reachable:true, or {reachable:false, error}'),
  runningRevision: z.number().int().nullable(),
  pendingCommit: z.record(z.string(), z.unknown()).nullable(),
  sync: z.object({
    state: z.string(),
    reason: z.string(),
    txnId: z.string().nullable(),
    since: z.string(),
  }),
});
const CountersOut = z.object({
  name: z.string(),
  swIfIndex: z.number().int(),
  rxPackets: z.string(),
  rxBytes: z.string(),
  txPackets: z.string(),
  txBytes: z.string(),
  drops: z.string(),
  errors: z.string(),
  punts: z.string(),
  rxMisses: z.string(),
});
const LiveStateOut = z
  .object({
    name: z.string(),
    vppName: z.string(),
    swIfIndex: z.number().int(),
    type: z.string(),
    adminUp: z.boolean(),
    linkUp: z.boolean(),
    mtu: z.number().int(),
    linkMtu: z.number().int(),
    mac: z.string(),
    ipv4: z.array(z.string()),
    ipv6: z.array(z.string()),
    vrf: z.string(),
    tableId: z.number().int(),
    parent: z.string(),
    vlanId: z.number().int(),
    innerVlanId: z.number().int(),
    managed: z.boolean(),
    linkSpeedKbps: z.string(),
    rxMode: z.string(),
    description: z.string(),
  })
  .describe('live state from the agent (InterfaceState RPC, dumped from VPP)');
const InterfaceItemOut = z.object({
  name: z.string().describe('logical name; sub-interfaces are "<parent>.<id>"'),
  kind: z.enum(['interface', 'subinterface']),
  parent: z.string().nullable(),
  state: LiveStateOut.nullable().describe(
    'null when VPP has no such interface (or the agent is older than P08)',
  ),
  config: z
    .record(z.string(), z.unknown())
    .nullable()
    .describe(
      'what the agent retrieved as configured on the data plane (Retrieve), as before P08; null only on the rows P08 added (a live interface the agent does not manage, a configured one the data plane does not have yet, or one only in the candidate)',
    ),
  running: z
    .record(z.string(), z.unknown())
    .nullable()
    .describe('the running configuration of this (sub-)interface; null when it is not configured'),
  counters: CountersOut.nullable(),
  hasPendingChange: z
    .boolean()
    .describe('the candidate differs from running for this (sub-)interface'),
});
const InterfacesOut = z.object({
  retrievedAt: z.string().optional(),
  countersAt: z.string().optional(),
  items: z.array(InterfaceItemOut),
});
const InterfaceNameParam = z
  .string()
  .min(1)
  .max(80)
  .regex(/^[A-Za-z][A-Za-z0-9_./-]*$/, 'expected an interface name');
const DriftOut = z.object({
  subsystems: z.array(z.string()),
  changes: z.array(
    z.object({
      op: z.string(),
      pointer: z.string(),
      from: z.unknown().optional(),
      to: z.unknown().optional(),
    }),
  ),
  ignored: z.array(z.object({ pointer: z.string(), rule: z.string() })),
});
const EventsOut = z.object({
  total: z.number().int(),
  items: z.array(
    z.object({
      id: z.number().int(),
      ts: z.string(),
      severity: z.string(),
      subsystem: z.string(),
      code: z.string(),
      message: z.string(),
      data: z.unknown(),
    }),
  ),
});

type Json = Record<string, unknown>;

/**
 * `/api/v1/state/**` — live, read-only (00-CONTEXT rule 8). Everything comes from the agent (Retrieve, Health,
 * StreamStats); nothing is read from VPP here. Routes are what the agent retrieved from VPP for this owner
 * (connected + static) — the agent contract has no FIB/neighbor dump RPC yet (see P06-questions).
 */
@ApiTags('state')
@Controller('api/v1/state')
export class StateController {
  constructor(
    private readonly agent: AgentClient,
    private readonly relay: RelayService,
    private readonly commits: CommitService,
    private readonly ds: DatastoreService,
    private readonly sysEvents: SystemEventsService,
    private readonly validation: ValidationService,
  ) {}

  @Get('system')
  @Protected()
  @ApiOperation({ summary: 'API + agent health, pending commit, running revision' })
  @ApiOut(SystemOut)
  async system() {
    const [health, pending, running] = await Promise.all([
      this.agent.health().then(
        (h) => ({ reachable: true as const, ...h }),
        (e: Error) => ({ reachable: false as const, error: e.message }),
      ),
      this.commits.pendingInfo(),
      this.ds.getRunning(),
    ]);
    return {
      api: {
        version: process.env['VRX_VERSION'] ?? '0.1.0-dev',
        startedAt: startedAt.toISOString(),
        wsClients: this.relay.clientCount,
      },
      agent: health,
      runningRevision: running.revision?.id ?? null,
      pendingCommit: pending,
      sync: await this.commits.syncStatus(),
    };
  }

  /** One StatsBatch: the relay's latest when its stream runs, else a one-shot subscription. */
  private async statsSnapshot(timeoutMs = 2500): Promise<StatsBatch | undefined> {
    const last = this.relay.lastStats;
    if (last?.ts && Date.now() - last.ts.getTime() < 3000) return last;
    return new Promise((resolve) => {
      const call = this.agent.streamStats({
        intervalMs: 200,
        interfaces: [],
        includeWorkerCpu: false,
      });
      const done = (b?: StatsBatch) => {
        clearTimeout(t);
        call.removeAllListeners();
        call.on('error', () => undefined);
        call.cancel();
        resolve(b);
      };
      const t = setTimeout(() => done(), timeoutMs);
      call.on('data', (b: StatsBatch) => done(b));
      call.on('error', () => done());
    });
  }

  @Get('interfaces')
  @Protected(502, 503)
  @ApiOperation({
    summary:
      'Interfaces: live state from VPP (agent InterfaceState), what the agent retrieved (config), the running configuration, the latest counters and pending candidate changes',
  })
  @ApiOut(InterfacesOut)
  async interfaces() {
    // P08: merged view — live state (InterfaceState, dumped from VPP by the agent), what the agent retrieved as
    // configured (Retrieve → `config`, its pre-P08 meaning, D-105), the running configuration (`running`), the
    // latest counters and whether the candidate changes it.
    const [r, live, stats, running, candidate] = await Promise.all([
      this.agent.retrieve(['interfaces']),
      this.liveState(),
      this.statsSnapshot(),
      this.ds.getRunning(),
      this.ds.getCandidate(),
    ]);
    const actual = DesiredState.toJSON(r.desiredState ?? DesiredState.fromPartial({})) as Json;
    const counters = new Map((stats?.interfaceCounters ?? []).map((c) => [c.name, c]));
    const runIfs = flattenInterfaces(running.doc['interfaces']);
    const candIfs = flattenInterfaces(candidate['interfaces']);
    const actIfs = flattenInterfaces(actual['interfaces']);
    const liveBy = new Map((live ?? []).map((s) => [s.name, s]));
    const names = new Set<string>([
      ...runIfs.keys(),
      ...candIfs.keys(),
      ...actIfs.keys(),
      ...liveBy.keys(),
    ]);
    const items = [...names].sort().map((name) => {
      const st = liveBy.get(name);
      const cfg = runIfs.get(name) ?? candIfs.get(name) ?? actIfs.get(name);
      const parent = cfg?.parent ?? (st?.parent ? st.parent : null);
      const c = counters.get(st?.vppName ?? name);
      return {
        name,
        kind: parent ? ('subinterface' as const) : ('interface' as const),
        parent,
        state: st ? liveJson(st) : null,
        config: actIfs.get(name)?.value ?? null,
        running: runIfs.get(name)?.value ?? null,
        counters: c ? countersJson(c) : null,
        hasPendingChange: !deepEqual(
          runIfs.get(name)?.value ?? null,
          candIfs.get(name)?.value ?? null,
        ),
      };
    });
    return {
      retrievedAt: r.retrievedAt?.toISOString(),
      countersAt: stats?.ts?.toISOString(),
      items,
    };
  }

  /** The agent's live interface table; undefined when the agent predates the InterfaceState RPC (501). */
  private async liveState(): Promise<InterfaceState[] | undefined> {
    try {
      return (await this.agent.interfaceState()).interfaces;
    } catch (e) {
      if (e instanceof ProblemError && e.getStatus() === 501) return undefined;
      throw e;
    }
  }

  @Get('interfaces/:name/counters')
  @Protected(404, 502, 503)
  @ApiParam({
    name: 'name',
    schema: { type: 'string' },
    description: 'logical interface name (sub-interfaces "<parent>.<id>")',
  })
  @ApiOperation({
    summary:
      'Latest counters of one interface (absolute; rates come from consecutive samples or the WS iface.counters topic)',
  })
  @ApiOut(
    z.object({
      name: z.string(),
      vppName: z.string(),
      ts: z.string().optional(),
      counters: CountersOut,
    }),
  )
  async counters(@Param('name', new ZodPipe(InterfaceNameParam)) name: string) {
    const live = await this.liveState();
    const vppName = live?.find((s) => s.name === name)?.vppName ?? name;
    if (live && !live.some((s) => s.name === name)) {
      throw problems.notFound(`no interface '${name}' on the data plane`);
    }
    const stats = await this.statsSnapshot();
    const c = stats?.interfaceCounters.find((x) => x.name === vppName);
    if (!c) throw problems.notFound(`no counters for '${name}' (VPP name '${vppName}')`);
    return { name, vppName, ts: stats?.ts?.toISOString(), counters: countersJson(c) };
  }

  @Get('routes')
  @Protected(502, 503)
  @ApiQuery({ name: 'vrf', required: false, schema: openapi(safeText(64)) })
  @ApiQuery({ name: 'page', required: false, schema: { type: 'integer', minimum: 1 } })
  @ApiQuery({
    name: 'pageSize',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 1000 },
  })
  @ApiOperation({
    summary: 'Connected + static routes retrieved from VPP by the agent (server-side paged)',
  })
  @ApiOut(
    z.object({
      page: z.number().int(),
      pageSize: z.number().int(),
      total: z.number().int(),
      items: z.array(RouteOut),
    }),
  )
  async routes(@Query(new ZodPipe(RoutesQuery)) q: z.output<typeof RoutesQuery>) {
    const r = await this.agent.retrieve(['interfaces', 'routing']);
    const actual = DesiredState.toJSON(r.desiredState ?? DesiredState.fromPartial({})) as Json;
    const all = routesOf(actual).filter((x) => q.vrf === undefined || x.vrf === q.vrf);
    const start = (q.page - 1) * q.pageSize;
    return {
      page: q.page,
      pageSize: q.pageSize,
      total: all.length,
      items: all.slice(start, start + q.pageSize),
    };
  }

  @Get('neighbors')
  @Protected(501)
  @ApiOperation({
    summary: 'IP neighbours — needs an agent state RPC that the v1 contract does not have (501)',
  })
  @ApiOkResponse({ description: 'Neighbour table (reserved: answers 501 in this release)' })
  neighbors() {
    throw problems.notImplemented(
      'the agent contract (vrx.v1.Dataplane) has no neighbour dump yet; an additive state RPC is requested in P06-questions',
    );
  }

  @Get('drift')
  @Protected(502, 503)
  @ApiOperation({ summary: 'Running configuration vs what the agent retrieves (proto.md §5)' })
  @ApiOut(DriftOut)
  async drift() {
    const [running, r, impl] = await Promise.all([
      this.ds.getRunning(),
      this.agent.retrieve([]),
      this.validation.implemented(),
    ]);
    const desired = DesiredState.fromJSON(redact(running.doc));
    // the agent's own statement of what it does not manage (DryRun of running: no side effects, proto.md §3)
    const report = await this.agent.dryRun({
      txnId: `drift-${Date.now()}`,
      desiredState: desired,
      subsystems: impl.subsystems,
    });
    const expected = DesiredState.toJSON(desired) as Json;
    const actual = DesiredState.toJSON(r.desiredState ?? DesiredState.fromPartial({})) as Json;
    const pick = (d: Json) => Object.fromEntries(r.subsystems.map((s) => [s, d[s]]));
    return driftOf(diff(pick(expected), pick(actual)), report.errors, r.subsystems);
  }

  @Get('events')
  @Protected()
  @ApiQuery({ name: 'limit', required: false, schema: { type: 'integer' } })
  @ApiQuery({ name: 'offset', required: false, schema: { type: 'integer' } })
  @ApiOperation({
    summary: 'System events (commits, confirm reverts, agent degradation), newest first',
  })
  @ApiOut(EventsOut)
  events(@Query(new ZodPipe(PageQuery)) q: z.output<typeof PageQuery>) {
    return this.sysEvents.list(q.limit, q.offset);
  }
}

const COVERAGE_RULES = new Set(['agent.unsupported-field', 'agent.unimplemented-domain']);

function isEmptyContainer(v: unknown): boolean {
  return (Array.isArray(v) && v.length === 0) || (isPlainObject(v) && Object.keys(v).length === 0);
}

/**
 * Running-vs-actual drift restricted to what the agent manages (review H2), without per-domain knowledge in the API:
 * - fields the agent reported as `agent.unsupported-field` (field-level pointers) and domains it reported as
 *   `agent.unimplemented-domain` are not compared — Retrieve cannot return what the agent never applies;
 * - an empty object/list on one side and absence on the other is not drift (proto3 has no presence for them);
 * a domain-level `unsupported-field` note (e.g. `/routing`: protocols are FRR's) does not hide the whole domain.
 */
export function driftOf(
  changes: Change[],
  report: readonly ValidationIssue[],
  subsystems: string[],
): { subsystems: string[]; changes: Change[]; ignored: { pointer: string; rule: string }[] } {
  const ignored = report
    .filter((i) => i.severity !== IssueSeverity.ISSUE_SEVERITY_ERROR && COVERAGE_RULES.has(i.rule))
    .map((i) => ({ pointer: i.pointer, rule: i.rule }));
  const skip = ignored
    .filter((i) => i.rule === 'agent.unimplemented-domain' || parsePointer(i.pointer).length >= 2)
    .map((i) => i.pointer);
  const kept = changes.filter((c) => {
    if (skip.some((p) => c.pointer === p || c.pointer.startsWith(p + '/'))) return false;
    if (c.op === 'remove' && isEmptyContainer(c.from)) return false;
    if (c.op === 'add' && isEmptyContainer(c.to)) return false;
    return true;
  });
  return { subsystems, changes: kept, ignored };
}

/** Connected prefixes of every interface/sub-interface address + static routes, sorted by VRF then prefix. */
export function routesOf(doc: Json): z.infer<typeof RouteOut>[] {
  const out: z.infer<typeof RouteOut>[] = [];
  const connected = (name: string, node: Json) => {
    const vrf = typeof node['vrf'] === 'string' ? node['vrf'] : 'default';
    for (const fam of ['ipv4', 'ipv6']) {
      for (const a of (node[fam] as string[] | undefined) ?? []) {
        const prefix = canonicalPrefix(a);
        if (prefix === undefined) continue;
        out.push({ vrf, prefix, origin: 'connected', nextHops: [{ interface: name }] });
      }
    }
  };
  for (const [name, node] of Object.entries((doc['interfaces'] ?? {}) as Json)) {
    if (!isPlainObject(node)) continue;
    connected(name, node);
    for (const [sub, s] of Object.entries((node['subinterfaces'] ?? {}) as Json)) {
      if (isPlainObject(s)) connected(`${name}.${sub}`, s);
    }
  }
  const routing = (doc['routing'] ?? {}) as Json;
  for (const r of (routing['static'] as Json[] | undefined) ?? []) {
    out.push({
      vrf: typeof r['vrf'] === 'string' ? r['vrf'] : 'default',
      prefix: String(r['prefix']),
      origin: 'static',
      nextHops: ((r['nextHops'] as Json[] | undefined) ?? []).map((h) => ({
        ...(typeof h['address'] === 'string' ? { address: h['address'] } : {}),
        ...(typeof h['interface'] === 'string' ? { interface: h['interface'] } : {}),
      })),
      ...(typeof r['distance'] === 'number' ? { distance: r['distance'] } : {}),
    });
  }
  return out.sort((a, b) => a.vrf.localeCompare(b.vrf) || a.prefix.localeCompare(b.prefix));
}

/** `interfaces` of a document → one entry per interface and per sub-interface ("<parent>.<id>"). */
function flattenInterfaces(v: unknown): Map<string, { value: Json; parent: string | null }> {
  const out = new Map<string, { value: Json; parent: string | null }>();
  if (!isPlainObject(v)) return out;
  for (const [name, itf] of Object.entries(v)) {
    if (!isPlainObject(itf)) continue;
    out.set(name, { value: itf, parent: null });
    const subs = itf['subinterfaces'];
    if (!isPlainObject(subs)) continue;
    for (const [id, sub] of Object.entries(subs)) {
      if (isPlainObject(sub)) out.set(`${name}.${id}`, { value: sub, parent: name });
    }
  }
  return out;
}

/** InterfaceState as JSON with every field present (ts-proto's toJSON drops defaults such as false/0). */
function liveJson(s: InterfaceState): Json {
  return { ...s, linkSpeedKbps: String(s.linkSpeedKbps) };
}

function countersJson(c: InterfaceCounters): Json {
  return { ...c };
}
