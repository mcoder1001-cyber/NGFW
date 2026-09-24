import { Controller, Get, Param, Query } from '@nestjs/common';
import { ApiOkResponse, ApiOperation, ApiParam, ApiQuery, ApiTags } from '@nestjs/swagger';
import { DesiredState, type DhcpLeasesResponse } from '@ngfw/proto';
import { objectName } from '@ngfw/schema';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { redact } from '../../datastore/documents.js';

type Json = Record<string, unknown>;

const LeasesQuery = z.object({
  server: objectName
    .optional()
    .describe('only leases of this Kea server (services.dhcp.servers key)'),
  family: z.enum(['ipv4', 'ipv6']).optional(),
  page: z.coerce.number().int().min(1).max(100_000).default(1),
  pageSize: z.coerce.number().int().min(1).max(1000).default(100),
  filter: z
    .string()
    .max(64)
    .regex(/^[\x20-\x7e]*$/, 'printable ASCII')
    .optional()
    .describe('case-insensitive substring of address, MAC, client id / DUID or hostname'),
});

const LeaseOut = z.object({
  family: z.enum(['ipv4', 'ipv6']),
  address: z.string(),
  hwAddress: z.string(),
  clientId: z.string(),
  duid: z.string(),
  hostname: z.string(),
  server: z.string().describe('empty when the subnet is not one of this agent’s'),
  subnet: z.string(),
  subnetId: z.number().int(),
  validLifetimeSec: z.number().int(),
  expiresAt: z.string().nullable(),
  state: z.string().describe('default, declined, expired-reclaimed or state-<n>'),
  leaseType: z.string().describe('DHCPv6 IA_NA / IA_PD; empty for DHCPv4'),
  prefixLen: z.number().int(),
});
const SubnetUsageOut = z.object({
  server: z.string(),
  subnet: z.string(),
  prefix: z.string(),
  subnetId: z.number().int(),
  total: z.string().describe('addresses in the pools (decimal string: DHCPv6 pools exceed 2^53)'),
  assigned: z.string(),
  declined: z.string(),
});
const ServerStatusOut = z.object({
  family: z.enum(['ipv4', 'ipv6']),
  running: z.boolean().describe('the daemon answers on its control socket'),
  active: z.boolean().describe('its configuration binds interfaces'),
  actionRequired: z
    .string()
    .describe('"start" while an active configuration waits for a stopped daemon (D-079)'),
  reloadSec: z.number().int(),
  subnets: z.array(SubnetUsageOut),
  error: z.string(),
});
const LeasesOut = z.object({
  retrievedAt: z.string().optional(),
  page: z.number().int(),
  pageSize: z.number().int(),
  total: z.number().int().describe('matching leases the agent read'),
  truncated: z
    .boolean()
    .describe('Kea holds more leases than the agent reads per call (100 000 per family)'),
  items: z.array(LeaseOut),
  servers: z.array(ServerStatusOut),
});

const RelayItemOut = z.object({
  name: z.string(),
  config: z
    .record(z.string(), z.unknown())
    .nullable()
    .describe('the running configuration of the relay'),
  retrieved: z
    .record(z.string(), z.unknown())
    .nullable()
    .describe(
      'what the agent retrieved (VPP dhcp proxy + the relay record); null = not on the data plane',
    ),
  state: z
    .enum(['applied', 'drift', 'missing', 'disabled', 'unmanaged'])
    .describe(
      'applied = retrieved equals running; unmanaged = on the data plane but not in running',
    ),
});
const RelaysOut = z.object({ retrievedAt: z.string().optional(), items: z.array(RelayItemOut) });

const InterfaceNameParam = z
  .string()
  .min(1)
  .max(80)
  .regex(/^[A-Za-z][A-Za-z0-9_./-]*$/, 'expected an interface name');

const ClientOut = z.object({
  interface: z.string(),
  configured: z.boolean().describe('VPP runs a DHCPv4 client on the interface'),
  state: z.string().describe('DISCOVER, REQUEST or BOUND; empty when not configured'),
  address: z.string().describe('leased address (CIDR); empty until bound'),
  router: z.string(),
  dnsServers: z.array(z.string()),
  hostname: z.string(),
  mac: z.string(),
  config: z
    .record(z.string(), z.unknown())
    .nullable()
    .describe('interfaces.<name>.dhcpClient of the running configuration (null = not configured)'),
});

function leasesJson(r: DhcpLeasesResponse) {
  return {
    retrievedAt: r.retrievedAt?.toISOString(),
    page: r.page,
    pageSize: r.pageSize,
    total: r.total,
    truncated: r.truncated,
    items: r.leases.map((l) => ({
      family: l.family === 'ipv6' ? ('ipv6' as const) : ('ipv4' as const),
      address: l.address,
      hwAddress: l.hwAddress,
      clientId: l.clientId,
      duid: l.duid,
      hostname: l.hostname,
      server: l.server,
      subnet: l.subnet,
      subnetId: l.subnetId,
      validLifetimeSec: l.validLifetimeSec,
      expiresAt: l.expiresAt?.toISOString() ?? null,
      state: l.state,
      leaseType: l.leaseType,
      prefixLen: l.prefixLen,
    })),
    servers: r.servers.map((s) => ({
      family: s.family === 'ipv6' ? ('ipv6' as const) : ('ipv4' as const),
      running: s.running,
      active: s.active,
      actionRequired: s.actionRequired,
      reloadSec: Number(s.reloadSec),
      subnets: s.subnets.map((u) => ({
        server: u.server,
        subnet: u.subnet,
        prefix: u.prefix,
        subnetId: u.subnetId,
        total: String(u.total),
        assigned: String(u.assigned),
        declined: String(u.declined),
      })),
      error: s.error,
    })),
  };
}

function isObject(v: unknown): v is Json {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

/** A relay in protobuf JSON form (the canonical spelling both sides of the comparison use). */
function relayJson(relay: unknown): Json {
  const ds = DesiredState.toJSON(
    DesiredState.fromJSON({ services: { dhcp: { relays: { r: relay } } } }),
  ) as Json;
  const dhcp = (ds['services'] as Json | undefined)?.['dhcp'] as Json | undefined;
  const r = (dhcp?.['relays'] as Json | undefined)?.['r'];
  return isObject(r) ? r : {};
}

/**
 * F-kea-dhcp-relay state routes (read-only, 00-CONTEXT rule 8): Kea leases with daemon status and pool usage
 * (DhcpLeases RPC, paged by the agent), relays as the agent retrieves them (Retrieve of `services`), and the VPP
 * DHCPv4 client state of one interface (DhcpLeases with `interface`). Configuration goes through the generic pointer
 * routes (`/api/v1/config/services/dhcp/...`, `/api/v1/config/interfaces/<name>/dhcpClient`).
 */
@ApiTags('state')
@Controller('api/v1/state')
export class KeaDhcpRelayController {
  constructor(
    private readonly agent: AgentClient,
    private readonly ds: DatastoreService,
  ) {}

  @Get('dhcp/leases')
  @Protected(400, 501, 502, 503)
  @ApiQuery({ name: 'server', required: false, schema: { type: 'string' } })
  @ApiQuery({ name: 'family', required: false, schema: { type: 'string', enum: ['ipv4', 'ipv6'] } })
  @ApiQuery({ name: 'page', required: false, schema: { type: 'integer', minimum: 1, default: 1 } })
  @ApiQuery({
    name: 'pageSize',
    required: false,
    schema: { type: 'integer', minimum: 1, maximum: 1000, default: 100 },
  })
  @ApiQuery({ name: 'filter', required: false, schema: { type: 'string' } })
  @ApiOperation({
    summary:
      'Kea DHCP leases, one page (the agent pages lease4/6-get-page; never the whole lease file), with kea-dhcp4/6 status and per-subnet pool usage',
  })
  @ApiOkResponse({ schema: openapi(LeasesOut, 'output') })
  async leases(@Query(new ZodPipe(LeasesQuery)) q: z.output<typeof LeasesQuery>) {
    const r = await this.agent.dhcpLeases({
      server: q.server ?? '',
      family: q.family ?? '',
      page: q.page,
      pageSize: q.pageSize,
      filter: q.filter ?? '',
      interface: '',
    });
    return leasesJson(r);
  }

  @Get('dhcp/relays')
  @Protected(501, 502, 503)
  @ApiOperation({
    summary:
      'DHCP relays: the running configuration next to what the agent retrieves from VPP (dhcp proxy per client VRF)',
  })
  @ApiOkResponse({ schema: openapi(RelaysOut, 'output') })
  async relays() {
    const [running, r] = await Promise.all([
      this.ds.getRunning(),
      this.agent.retrieve(['services']),
    ]);
    const doc = redact(running.doc) as Json;
    const cfg = (((doc['services'] as Json | undefined)?.['dhcp'] as Json | undefined)?.[
      'relays'
    ] ?? {}) as Json;
    const got = r.desiredState?.services?.dhcp?.relays ?? {};
    const names = [...new Set([...Object.keys(cfg), ...Object.keys(got)])].sort();
    const items = names.map((name) => {
      const config = isObject(cfg[name]) ? cfg[name] : null;
      const actual = got[name];
      const retrieved = actual === undefined ? null : relayJson(actual);
      let state: z.infer<typeof RelayItemOut>['state'];
      if (config === null) state = 'unmanaged';
      else if (retrieved === null) state = config['enabled'] === false ? 'disabled' : 'missing';
      else if (JSON.stringify(sortKeys(relayJson(config))) === JSON.stringify(sortKeys(retrieved)))
        state = config['enabled'] === false ? 'disabled' : 'applied';
      else state = 'drift';
      return { name, config, retrieved, state };
    });
    return { retrievedAt: r.retrievedAt?.toISOString(), items };
  }

  @Get('interfaces/:name/dhcp-client')
  @Protected(400, 501, 502, 503)
  @ApiParam({ name: 'name', schema: { type: 'string' }, description: 'logical interface name' })
  @ApiOperation({
    summary: 'VPP DHCPv4 client of one interface: state and lease (dhcp_client_dump)',
  })
  @ApiOkResponse({ schema: openapi(ClientOut, 'output') })
  async client(@Param('name', new ZodPipe(InterfaceNameParam)) name: string) {
    const [r, running] = await Promise.all([
      this.agent.dhcpLeases({
        server: '',
        family: '',
        page: 0,
        pageSize: 0,
        filter: '',
        interface: name,
      }),
      this.ds.getRunning(),
    ]);
    const c = r.client;
    return {
      interface: name,
      configured: c?.configured ?? false,
      state: c?.state ?? '',
      address: c?.address ?? '',
      router: c?.router ?? '',
      dnsServers: c?.dnsServers ?? [],
      hostname: c?.hostname ?? '',
      mac: c?.mac ?? '',
      config: dhcpClientConfig(redact(running.doc) as Json, name),
    };
  }
}

/** `interfaces.<name>.dhcpClient` (or `interfaces.<parent>.subinterfaces.<id>.dhcpClient`) of a document. */
function dhcpClientConfig(doc: Json, name: string): Json | null {
  const ifs = (doc['interfaces'] ?? {}) as Json;
  const direct = ifs[name];
  if (isObject(direct)) return isObject(direct['dhcpClient']) ? direct['dhcpClient'] : null;
  const dot = name.lastIndexOf('.');
  if (dot > 0) {
    const parent = ifs[name.slice(0, dot)];
    const sub =
      isObject(parent) && isObject(parent['subinterfaces'])
        ? parent['subinterfaces'][name.slice(dot + 1)]
        : undefined;
    if (isObject(sub) && isObject(sub['dhcpClient'])) return sub['dhcpClient'];
  }
  return null;
}

function sortKeys(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(sortKeys);
  if (isObject(v))
    return Object.fromEntries(
      Object.keys(v)
        .sort()
        .map((k) => [k, sortKeys(v[k])]),
    );
  return v;
}
