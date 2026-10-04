import { Controller, Get, HttpCode, Inject, Post } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import {
  DataplaneConfig,
  type DataplaneStartupPreviewResponse,
  type DataplaneStartupStateResponse,
} from '@ngfw/proto';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { ProblemError } from '../../common/problem.js';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';
import { DatastoreService } from '../../datastore/datastore.service.js';

export const DataplaneStateOut = z
  .object({
    runtimeThreads: z.array(
      z.object({
        id: z.number(),
        name: z.string(),
        type: z.string(),
        cpuId: z.number(),
        core: z.number(),
        numaSocket: z.number(),
      }),
    ),
    loadedPlugins: z.string(),
    nicQueues: z.string(),
    runtimeMemory: z.string(),
    runtimeErrors: z.array(z.string()),
    startupPath: z.string().describe('installed VPP start-up file the agent read'),
    startupPresent: z.boolean(),
    workers: z.number().int().nullable().describe('cpu { workers N } of the installed file'),
    corelistWorkers: z.string().describe('cpu { corelist-workers … } of the installed file'),
    mainCore: z.number().int().nullable().describe('cpu { main-core N } of the installed file'),
    plugins: z.record(z.string(), z.boolean()).describe('plugin switches of the installed file'),
    onlineCpus: z.string().describe('online CPUs of the host (CPU-list notation)'),
    hugepagesTotalBytes: z.string().describe('uint64 as decimal string (D-039)'),
    hugepagesFreeBytes: z.string().describe('uint64 as decimal string (D-039)'),
    error: z.string().describe('what could not be read; empty = everything was read'),
    retrievedAt: z.string().nullable(),
  })
  .describe(
    'VPP start-up configuration as installed + host facts (DataplaneStartupState RPC). Read-only.',
  );

export const DataplanePreviewOut = z
  .object({
    rendered: z.string().describe('startup.conf rendered from the candidate `dataplane` domain'),
    startupPath: z.string(),
    diff: z.string().describe('unified diff installed → rendered; empty = identical'),
    changed: z.boolean().describe('applying would change the installed file (VPP restart)'),
    warnings: z.array(z.string()),
    sha256: z.string(),
    restartRequired: z
      .literal(true)
      .describe('these settings only take effect after a VPP restart'),
    applyAvailable: z
      .literal(false)
      .describe('installing the file (apply-startup.sh) is a manager step gated by TD-17'),
  })
  .describe('DataplaneStartupPreview RPC: nothing is written, VPP is never restarted');

const big = (v: unknown) => String(v ?? '0');

export function toDataplaneState(
  r: DataplaneStartupStateResponse,
): z.infer<typeof DataplaneStateOut> {
  return {
    runtimeThreads: r.runtimeThreads ?? [],
    loadedPlugins: r.loadedPlugins ?? '',
    nicQueues: r.nicQueues ?? '',
    runtimeMemory: r.runtimeMemory ?? '',
    runtimeErrors: r.runtimeErrors ?? [],
    startupPath: r.startupPath,
    startupPresent: r.startupPresent,
    workers: r.workers ?? null,
    corelistWorkers: r.corelistWorkers,
    mainCore: r.mainCore ?? null,
    plugins: { ...r.plugins },
    onlineCpus: r.onlineCpus,
    hugepagesTotalBytes: big(r.hugepagesTotalBytes),
    hugepagesFreeBytes: big(r.hugepagesFreeBytes),
    error: r.error,
    retrievedAt: r.retrievedAt ? new Date(r.retrievedAt).toISOString() : null,
  };
}

export function toDataplanePreview(
  r: DataplaneStartupPreviewResponse,
): z.infer<typeof DataplanePreviewOut> {
  return {
    rendered: r.rendered,
    startupPath: r.startupPath,
    diff: r.diff,
    changed: r.changed,
    warnings: [...r.warnings],
    sha256: r.sha256,
    restartRequired: true,
    applyAvailable: false,
  };
}

/**
 * F-dataplane-ui: read-only views of the VPP start-up configuration. Configuration goes through the generic pointer
 * routes (`/config/dataplane`). No route here writes startup.conf or restarts VPP (TD-17 gates apply-startup.sh).
 */
@ApiTags('state')
@Controller('api/v1')
export class DataplaneController {
  constructor(
    @Inject(AgentClient) private readonly agent: AgentClient,
    @Inject(DatastoreService) private readonly ds: DatastoreService,
  ) {}

  @Get('state/dataplane')
  @Protected(501, 502, 503)
  @ApiOperation({
    summary:
      'Observed VPP threads, plugins, RX queues and memory, alongside installed startup settings and host facts',
  })
  @ApiOkResponse({ schema: openapi(DataplaneStateOut, 'output') })
  async state() {
    return toDataplaneState(await this.agent.dataplaneStartupState());
  }

  @Post('actions/dataplane/preview')
  @HttpCode(200)
  @Protected(400, 409, 501, 502, 503)
  @ApiOperation({
    summary:
      'Render startup.conf for the candidate `dataplane` domain and diff it against the installed file (read-only)',
  })
  @ApiOkResponse({ schema: openapi(DataplanePreviewOut, 'output') })
  async preview() {
    const candidate = await this.ds.getCandidate();
    const dp = DataplaneConfig.fromJSON(candidate['dataplane'] ?? {});
    try {
      return toDataplanePreview(await this.agent.dataplaneStartupPreview({ dataplane: dp }));
    } catch (e) {
      if (e instanceof ProblemError && e.extra['grpcCode'] === 'INVALID_ARGUMENT') {
        throw new ProblemError(400, 'validation', 'Validation failed', e.detail, [
          { pointer: '/dataplane', message: e.detail ?? e.title, rule: 'dataplane.startup' },
        ]);
      }
      throw e;
    }
  }
}
