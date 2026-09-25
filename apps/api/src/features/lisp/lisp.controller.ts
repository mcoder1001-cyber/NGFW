import { Controller, Get } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';

/** `GET /api/v1/state/lisp` body: the agent's LispState (F-lisp), VPP-wide like `show lisp …`. */
export const LispStateOut = z
  .object({
    enabled: z.boolean().describe('LISP control plane switch (VPP-global)'),
    gpeEnabled: z.boolean().describe('LISP-GPE data plane switch (VPP-global)'),
    pitr: z.string().describe('proxy-ITR locator set, "" when unset'),
    locatorSets: z.array(
      z.object({
        name: z.string(),
        locators: z.array(
          z.object({
            interface: z.string(),
            swIfIndex: z.number().int(),
            priority: z.number().int(),
            weight: z.number().int(),
          }),
        ),
      }),
    ),
    mappings: z
      .array(
        z.object({
          vni: z.number().int(),
          eid: z.string(),
          local: z.boolean(),
          locatorSet: z.string(),
          rlocs: z.array(z.string()),
          action: z.string(),
          authoritative: z.boolean(),
          ttl: z.number().int(),
        }),
      )
      .describe('local EIDs and the map-cache (static and learned remote mappings)'),
    adjacencies: z.array(z.object({ vni: z.number().int(), reid: z.string(), leid: z.string() })),
    eidTables: z.array(
      z.object({ vni: z.number().int(), dpTable: z.number().int(), isL2: z.boolean() }),
    ),
    mapResolvers: z.array(z.string()),
    mapServers: z.array(z.string()),
    gpeVnis: z
      .array(z.number().int())
      .describe('VNIs with LISP-GPE forwarding entries (their pairs cannot be read back, V13)'),
    retrievedAt: z.string().nullable(),
  })
  .describe('live LISP state from the agent (LispState RPC, dumped from VPP)');

@ApiTags('state')
@Controller('api/v1/state')
export class LispController {
  constructor(private readonly agent: AgentClient) {}

  @Get('lisp')
  @Protected(501, 502, 503)
  @ApiOperation({
    summary:
      'LISP / LISP-GPE: switches, locator sets, local EIDs and map-cache, adjacencies, EID-table maps, resolvers (agent LispState)',
  })
  @ApiOkResponse({ schema: openapi(LispStateOut, 'output') })
  async state(): Promise<z.infer<typeof LispStateOut>> {
    const s = await this.agent.lispState();
    return {
      enabled: s.enabled,
      gpeEnabled: s.gpeEnabled,
      pitr: s.pitr,
      locatorSets: s.locatorSets.map((ls) => ({
        name: ls.name,
        locators: ls.locators.map((l) => ({
          interface: l.interface,
          swIfIndex: l.swIfIndex,
          priority: l.priority,
          weight: l.weight,
        })),
      })),
      mappings: s.mappings.map((m) => ({
        vni: m.vni,
        eid: m.eid,
        local: m.local,
        locatorSet: m.locatorSet,
        rlocs: [...m.rlocs],
        action: m.action,
        authoritative: m.authoritative,
        ttl: m.ttl,
      })),
      adjacencies: s.adjacencies.map((a) => ({ vni: a.vni, reid: a.reid, leid: a.leid })),
      eidTables: s.eidTables.map((t) => ({ vni: t.vni, dpTable: t.dpTable, isL2: t.isL2 })),
      mapResolvers: [...s.mapResolvers],
      mapServers: [...s.mapServers],
      gpeVnis: [...s.gpeVnis],
      retrievedAt: s.retrievedAt ? s.retrievedAt.toISOString() : null,
    };
  }
}
