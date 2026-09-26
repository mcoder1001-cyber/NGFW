import { Body, Controller, Get, HttpCode, Post, Query } from '@nestjs/common';
import { ApiBody, ApiOkResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import type {
  DnsStateResponse,
  NtpStateResponse,
  SyslogEntriesResponse,
  SyslogStateResponse,
} from '@ngfw/proto';
import { SYSLOG_FACILITIES, SyslogSeverity } from '@ngfw/schema';
import type { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { MinRole } from '../../auth/decorators.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import {
  DnsLookupBody,
  DnsLookupOut,
  DnsStateOut,
  LogsOut,
  LogsQuery,
  NtpStateOut,
  SyslogStateOut,
} from './dto.js';

const iso = (d: Date | undefined): string | null => (d ? d.toISOString() : null);

/**
 * F-unbound-chrony-syslog (WBS D7.3, D7.4, D7.7): live state of the host services the agent renders — the Unbound
 * resolver, chronyd and the rsyslog export — the log explorer, and a DNS lookup through VPP's DNS cache. Configuration
 * goes through the generic pointer routes (`/api/v1/config/candidate/services/dns|ntp`, `/management/syslog`).
 * Everything here is read-only except the lookup (an action: operator, audited).
 */
@ApiTags('state')
@Controller('api/v1')
export class UnboundChronySyslogController {
  constructor(private readonly agent: AgentClient) {}

  @Get('state/dns')
  @Protected(501, 502, 503)
  @ApiOperation({
    summary:
      'Unbound resolver state (status, stats, forwards, local zones), pending daemon actions, VPP DNS cache as configured',
  })
  @ApiOkResponse({ schema: openapi(DnsStateOut, 'output') })
  async dns(): Promise<z.output<typeof DnsStateOut>> {
    return dnsJson(await this.agent.dnsState());
  }

  @Get('state/ntp')
  @Protected(501, 502, 503)
  @ApiOperation({
    summary: 'chronyd state: tracking, sources, source statistics, pending daemon actions',
  })
  @ApiOkResponse({ schema: openapi(NtpStateOut, 'output') })
  async ntp(): Promise<z.output<typeof NtpStateOut>> {
    return ntpJson(await this.agent.ntpState());
  }

  @Get('state/syslog')
  @Protected(501, 502, 503)
  @ApiOperation({
    summary: 'Remote-syslog export: per-target counters (rsyslog impstats), pending daemon actions',
  })
  @ApiOkResponse({ schema: openapi(SyslogStateOut, 'output') })
  async syslog(): Promise<z.output<typeof SyslogStateOut>> {
    return syslogJson(await this.agent.syslogState());
  }

  @Get('state/logs')
  // review M2: the whole host journal (auth/authpriv, every unit) — administrators only
  @MinRole('admin')
  @Protected(400, 501, 502, 503)
  @ApiOperation({
    summary:
      'Log explorer (admin only): one page of the local journal, newest first (bounded scan; filters by severity, facility, text)',
  })
  @ApiQuery({ name: 'since', required: false, schema: { type: 'string', format: 'date-time' } })
  @ApiQuery({
    name: 'severity',
    required: false,
    schema: { type: 'string', enum: [...SyslogSeverity.options] },
  })
  @ApiQuery({
    name: 'facility',
    required: false,
    schema: { type: 'string', enum: [...SYSLOG_FACILITIES] },
  })
  @ApiQuery({ name: 'q', required: false, schema: { type: 'string', maxLength: 128 } })
  @ApiQuery({
    name: 'page',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 5000 },
  })
  @ApiQuery({
    name: 'pageSize',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 500 },
  })
  @ApiOkResponse({ schema: openapi(LogsOut, 'output') })
  async logs(
    @Query(new ZodPipe(LogsQuery)) q: z.output<typeof LogsQuery>,
  ): Promise<z.output<typeof LogsOut>> {
    const r = await this.agent.syslogEntries({
      since: q.since === undefined ? undefined : new Date(q.since),
      severity: q.severity ?? '',
      facility: q.facility ?? '',
      query: q.q ?? '',
      page: q.page,
      pageSize: q.pageSize,
    });
    return logsJson(r);
  }

  @Post('actions/dns-lookup')
  @HttpCode(200)
  @Protected(400, 409, 501, 502, 503)
  @ApiOperation({
    summary:
      'Resolve a name through the VPP DNS cache (dns_resolve_name, with a deadline); 409 unless this agent (the globals owner) enabled the VPP DNS cache with an upstream',
  })
  @ApiBody({ schema: openapi(DnsLookupBody) })
  @ApiOkResponse({ schema: openapi(DnsLookupOut, 'output') })
  async dnsLookup(
    @Body(new ZodPipe(DnsLookupBody)) body: z.output<typeof DnsLookupBody>,
  ): Promise<z.output<typeof DnsLookupOut>> {
    const out = await this.agent.dnsLookup({ name: body.name, timeoutMs: body.timeoutMs ?? 0 });
    const addresses: z.output<typeof DnsLookupOut>['addresses'] = [];
    let summary = '';
    let ok = false;
    for (const o of out) {
      const m = /^(A|AAAA) (\S+)$/.exec(o.line ?? '');
      if (m) addresses.push({ type: m[1] as 'A' | 'AAAA', address: m[2]! });
      if (o.done) {
        summary = o.done.summary;
        ok = o.done.exitCode === 0;
      }
    }
    return { name: body.name, ok, addresses, summary };
  }
}

export function dnsJson(r: DnsStateResponse): z.output<typeof DnsStateOut> {
  return {
    retrievedAt: iso(r.retrievedAt),
    running: r.running,
    status: r.status,
    stats: r.stats,
    forwards: r.forwards.map((z) => ({
      zone: z.zone,
      kind: z.kind,
      flags: z.flags,
      addresses: z.addresses,
    })),
    stubs: r.stubs.map((z) => ({
      zone: z.zone,
      kind: z.kind,
      flags: z.flags,
      addresses: z.addresses,
    })),
    localZones: r.localZones.map((z) => ({ zone: z.zone, type: z.type })),
    localData: r.localData,
    localDataTruncated: r.localDataTruncated,
    pendingActions: r.pendingActions.map((p) => ({ ...p })),
    vppCache: r.vppCache
      ? {
          configured: r.vppCache.configured,
          appliedByThisAgent: r.vppCache.appliedByThisAgent,
          upstreams: r.vppCache.upstreams,
          live: false,
        }
      : null,
    configPath: r.configPath,
    error: r.error,
  };
}

export function ntpJson(r: NtpStateResponse): z.output<typeof NtpStateOut> {
  return {
    retrievedAt: iso(r.retrievedAt),
    running: r.running,
    tracking: r.tracking ? { ...r.tracking } : null,
    sources: r.sources.map((s) => ({ ...s })),
    sourceStats: r.sourceStats.map((s) => ({ ...s })),
    serverStats: r.serverStats,
    pendingActions: r.pendingActions.map((p) => ({ ...p })),
    configPath: r.configPath,
    error: r.error,
  };
}

export function syslogJson(r: SyslogStateResponse): z.output<typeof SyslogStateOut> {
  const num = (v: number | bigint | string): number => Number(v);
  return {
    retrievedAt: iso(r.retrievedAt),
    targets: r.targets.map((t) => ({
      index: t.index,
      action: t.action,
      target: t.target,
      protocol: t.protocol,
      reported: t.reported,
      processed: num(t.processed),
      failed: num(t.failed),
      suspended: num(t.suspended),
      suspendedDuration: num(t.suspendedDuration),
      resumed: num(t.resumed),
      queueSize: num(t.queueSize),
      enqueued: num(t.enqueued),
      full: num(t.full),
      discardedFull: num(t.discardedFull),
      discardedNf: num(t.discardedNf),
      maxQueueSize: num(t.maxQueueSize),
    })),
    inputs: Object.fromEntries(Object.entries(r.inputs).map(([k, v]) => [k, num(v)])),
    pendingActions: r.pendingActions.map((p) => ({ ...p })),
    configPath: r.configPath,
    error: r.error,
  };
}

export function logsJson(r: SyslogEntriesResponse): z.output<typeof LogsOut> {
  return {
    items: r.entries.map((e) => ({
      time: iso(e.time),
      severity: e.severity,
      facility: e.facility,
      identifier: e.identifier,
      pid: e.pid,
      hostname: e.hostname,
      unit: e.unit,
      message: e.message,
    })),
    page: r.page,
    pageSize: r.pageSize,
    total: r.total,
    scanned: r.scanned,
    truncated: r.truncated,
    source: r.source,
  };
}
