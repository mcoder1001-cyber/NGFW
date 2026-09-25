import { SyslogFacility, SyslogSeverity } from '@ngfw/schema';
import { z } from 'zod';
import { safeText } from '../../common/text.js';

/**
 * F-unbound-chrony-syslog request/response shapes (Zod: validation and OpenAPI from one definition). Responses list
 * every field (the agent's proto3 zero values included), so the UI never guesses a default.
 */

const PendingAction = z.object({
  daemon: z.string(),
  unit: z.string(),
  action: z.string().describe('"start" | "restart"'),
  reason: z.string(),
});

export const DnsStateOut = z.object({
  retrievedAt: z.string().nullable(),
  running: z.boolean().describe('unbound answers on its control socket'),
  status: z.record(z.string(), z.string()),
  stats: z.record(z.string(), z.string()).describe('unbound-control stats_noreset'),
  forwards: z.array(
    z.object({
      zone: z.string(),
      kind: z.string(),
      flags: z.array(z.string()),
      addresses: z.array(z.string()),
    }),
  ),
  stubs: z.array(
    z.object({
      zone: z.string(),
      kind: z.string(),
      flags: z.array(z.string()),
      addresses: z.array(z.string()),
    }),
  ),
  localZones: z.array(z.object({ zone: z.string(), type: z.string() })),
  localData: z.array(z.string()),
  localDataTruncated: z.boolean(),
  pendingActions: z.array(PendingAction),
  vppCache: z
    .object({
      configured: z.boolean(),
      appliedByThisAgent: z.boolean().describe('the globals owner programs it (D-071)'),
      upstreams: z.array(z.string()),
      live: z
        .literal(false)
        .describe('VPP has no getter for its DNS cache (D-063): configured, never read back'),
    })
    .nullable(),
  configPath: z.string(),
  error: z.string(),
});

export const NtpStateOut = z.object({
  retrievedAt: z.string().nullable(),
  running: z.boolean(),
  tracking: z
    .object({
      refId: z.string(),
      refName: z.string(),
      stratum: z.number().int(),
      refTime: z.number(),
      systemTime: z.number(),
      lastOffset: z.number(),
      rmsOffset: z.number(),
      frequency: z.number(),
      residualFreq: z.number(),
      skew: z.number(),
      rootDelay: z.number(),
      rootDispersion: z.number(),
      updateInterval: z.number(),
      leap: z.string(),
    })
    .nullable(),
  sources: z.array(
    z.object({
      mode: z.string(),
      state: z
        .string()
        .describe(
          '"*" selected, "+" combined, "-" not combined, "?" unusable, "x" falseticker, "~" variable',
        ),
      name: z.string(),
      stratum: z.number().int(),
      poll: z.number().int(),
      reach: z.string(),
      lastRx: z.string(),
      offset: z.number(),
      measured: z.number(),
      error: z.number(),
    }),
  ),
  sourceStats: z.array(
    z.object({
      name: z.string(),
      np: z.number().int(),
      nr: z.number().int(),
      span: z.number().int(),
      frequency: z.number(),
      freqSkew: z.number(),
      offset: z.number(),
      stdDev: z.number(),
    }),
  ),
  serverStats: z.record(z.string(), z.string()),
  pendingActions: z.array(PendingAction),
  configPath: z.string(),
  error: z.string(),
});

export const SyslogStateOut = z.object({
  retrievedAt: z.string().nullable(),
  targets: z.array(
    z.object({
      index: z.number().int(),
      action: z.string(),
      target: z.string(),
      protocol: z.string(),
      reported: z.boolean(),
      processed: z.number().int(),
      failed: z.number().int(),
      suspended: z.number().int(),
      suspendedDuration: z.number().int(),
      resumed: z.number().int(),
      queueSize: z.number().int(),
      enqueued: z.number().int(),
      full: z.number().int(),
      discardedFull: z.number().int(),
      discardedNf: z.number().int(),
      maxQueueSize: z.number().int(),
    }),
  ),
  inputs: z.record(z.string(), z.number().int()),
  pendingActions: z.array(PendingAction),
  configPath: z.string(),
  error: z.string(),
});

export const LogsQuery = z.object({
  since: z.iso
    .datetime({ offset: true })
    .optional()
    .describe('oldest entry (default: one hour ago; at most 30 days)'),
  severity: SyslogSeverity.optional().describe('minimum severity'),
  facility: SyslogFacility.optional(),
  q: safeText(128)
    .optional()
    .describe('case-insensitive substring of the message or identifier (not a pattern)'),
  page: z.coerce.number().int().min(1).max(5000).default(1),
  pageSize: z.coerce.number().int().min(1).max(500).default(100),
});

export const LogsOut = z.object({
  items: z.array(
    z.object({
      time: z.string().nullable(),
      severity: z.string(),
      facility: z.string(),
      identifier: z.string(),
      pid: z.number().int(),
      hostname: z.string(),
      unit: z.string(),
      message: z.string(),
    }),
  ),
  page: z.number().int(),
  pageSize: z.number().int(),
  total: z.number().int().describe('matches within the scanned window'),
  scanned: z.number().int(),
  truncated: z
    .boolean()
    .describe('the scan stopped at its bound (5000 newest entries since `since`)'),
  source: z.string().describe('"journald"'),
});

export const DnsLookupBody = z.strictObject({
  name: z
    .string()
    .min(1)
    .max(253)
    .regex(/^[A-Za-z0-9._-]+$/, 'expected a DNS name (letters, digits, ".", "-", "_")'),
  timeoutMs: z.int().min(100).max(30000).optional().describe('deadline (default 5000)'),
});

export const DnsLookupOut = z.object({
  name: z.string(),
  ok: z.boolean(),
  addresses: z.array(z.object({ type: z.enum(['A', 'AAAA']), address: z.string() })),
  summary: z.string(),
});
