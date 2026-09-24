import { Controller, Get } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiTags } from '@nestjs/swagger';
import { AgentClient } from '../../agent/agent.client.js';
import { Protected } from '../../common/responses.js';
import { openapi } from '../../common/zod.js';
import { HostAclStateOut, hostAclStateJson } from './model.js';

/**
 * F-host-acl-nftables state route (`/api/v1/state/host-acl`, read-only, 00-CONTEXT rule 8): the host firewall table the
 * agent rendered from `acl.host*` (HostAclState RPC) with per-rule counters. Host lists, attachments and settings are
 * configured through the generic pointer routes (`/api/v1/config/acl/host`, `/hostAttachments`, `/hostSettings`).
 */
@ApiTags('state')
@Controller('api/v1/state')
export class HostAclNftablesController {
  constructor(private readonly agent: AgentClient) {}

  @Get('host-acl')
  @Protected(501, 502, 503)
  @ApiOperation({
    summary:
      'Host firewall (nftables table inet vrx): sets, chains and rules with packet/byte counters, mode and in-sync flag',
  })
  @ApiOkResponse({ schema: openapi(HostAclStateOut, 'output') })
  async hostAcl() {
    return hostAclStateJson(await this.agent.hostAclState());
  }
}
