import { Body, Controller, Get, HttpCode, Inject, Post, Query, Req } from '@nestjs/common';
import { ApiBody, ApiOkResponse, ApiOperation, ApiQuery, ApiTags } from '@nestjs/swagger';
import type { WireguardStateResponse } from '@ngfw/proto';
import { generateKeyPairSync } from 'node:crypto';
import { z } from 'zod';
import { AgentClient } from '../../agent/agent.client.js';
import { MinRole } from '../../auth/decorators.js';
import type { VrxRequest } from '../../common/principal.js';
import { Protected } from '../../common/responses.js';
import { openapi, ZodPipe } from '../../common/zod.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { SecretsService } from '../../secrets/secrets.service.js';

type Json = Record<string, unknown>;

const PeerStateOut = z.object({
  name: z
    .string()
    .nullable()
    .describe('peer name in the running configuration (null: not configured)'),
  publicKey: z.string(),
  peerIndex: z.number().int(),
  status: z.enum(['established', 'dead', 'down']),
  established: z.boolean(),
  dead: z.boolean(),
  endpoint: z
    .string()
    .nullable()
    .describe('the endpoint VPP currently sends to (learnt); null = none yet'),
  endpointPort: z.number().int(),
  lastHandshake: z
    .string()
    .nullable()
    .describe(
      'last time the agent saw the peer become established (VPP has no handshake timestamp)',
    ),
  persistentKeepaliveSec: z.number().int(),
  allowedIps: z.array(z.string()),
});
const InterfaceStateOut = z.object({
  name: z
    .string()
    .nullable()
    .describe('interface name in the running configuration (null: not configured)'),
  vppName: z.string(),
  instance: z.number().int(),
  swIfIndex: z.number().int(),
  publicKey: z.string().describe('the interface public key (the private key is never returned)'),
  listenAddress: z.string(),
  listenPort: z.number().int(),
  adminUp: z.boolean(),
  linkUp: z.boolean(),
  rxPackets: z.number(),
  rxBytes: z.number(),
  txPackets: z.number(),
  txBytes: z.number(),
  peers: z.array(PeerStateOut),
});
const WireguardStateOut = z.object({
  retrievedAt: z.string().nullable(),
  eventsActive: z
    .boolean()
    .describe('whether the agent watches peer events (live status, last handshake)'),
  interfaces: z.array(InterfaceStateOut),
});
const StateQuery = z.object({
  interface: z
    .string()
    .regex(/^wg[0-9]{1,10}$/)
    .optional()
    .describe('only this VPP interface (wg<instance>)'),
});
const KeypairBody = z.strictObject({
  name: z
    .string()
    .regex(/^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/)
    .describe('secret name: the private key is stored as key/<name>'),
});
const KeypairOut = z.object({
  ref: z.string().describe('the reference to put into privateKeyRef (key/<name>)'),
  publicKey: z.string().describe('the public key to give to the peers'),
  version: z.number().int(),
});

/** Base64 of the raw 32-byte X25519 keys of a fresh key pair (WireGuard's text form). The private key never leaves. */
export function wireguardKeypair(): { privateKey: string; publicKey: string } {
  const { privateKey, publicKey } = generateKeyPairSync('x25519');
  const priv = privateKey.export({ format: 'jwk' }).d;
  const pub = publicKey.export({ format: 'jwk' }).x;
  if (priv === undefined || pub === undefined) throw new Error('x25519 export failed');
  return {
    privateKey: Buffer.from(priv, 'base64url').toString('base64'),
    publicKey: Buffer.from(pub, 'base64url').toString('base64'),
  };
}

/** Configuration names by VPP identity: `wg<instance>` → interface name, `<wg>|<public key>` → peer name. */
export function wireguardNames(running: Json): {
  itf: Map<string, string>;
  peer: Map<string, string>;
} {
  const itf = new Map<string, string>();
  const peer = new Map<string, string>();
  const wg = ((running['vpn'] as Json | undefined)?.['wireguard'] as Json | undefined)?.[
    'interfaces'
  ];
  for (const [name, raw] of Object.entries((wg ?? {}) as Json)) {
    const w = raw as Json;
    if (typeof w['instance'] !== 'number') continue;
    const vpp = `wg${w['instance']}`;
    itf.set(vpp, name);
    for (const [pname, p] of Object.entries((w['peers'] ?? {}) as Json)) {
      const key = (p as Json)['publicKey'];
      if (typeof key === 'string') peer.set(`${vpp}|${key}`, pname);
    }
  }
  return { itf, peer };
}

/** The agent's WireguardState joined with the running configuration's names (pure; unit-tested). */
export function wireguardStateOf(
  st: WireguardStateResponse,
  running: Json,
): z.infer<typeof WireguardStateOut> {
  const names = wireguardNames(running);
  return {
    retrievedAt: st.retrievedAt?.toISOString() ?? null,
    eventsActive: st.eventsActive,
    interfaces: st.interfaces.map((i) => ({
      name: names.itf.get(i.name) ?? null,
      vppName: i.name,
      instance: i.instance,
      swIfIndex: i.swIfIndex,
      publicKey: i.publicKey,
      listenAddress: i.listenAddress,
      listenPort: i.listenPort,
      adminUp: i.adminUp,
      linkUp: i.linkUp,
      rxPackets: Number(i.rxPackets),
      rxBytes: Number(i.rxBytes),
      txPackets: Number(i.txPackets),
      txBytes: Number(i.txBytes),
      peers: i.peers.map((p) => ({
        name: names.peer.get(`${i.name}|${p.publicKey}`) ?? null,
        publicKey: p.publicKey,
        peerIndex: p.peerIndex,
        status: p.established ? 'established' : p.dead ? 'dead' : 'down',
        established: p.established,
        dead: p.dead,
        endpoint: p.endpoint === '' ? null : p.endpoint,
        endpointPort: p.endpointPort,
        lastHandshake: p.lastHandshake?.toISOString() ?? null,
        persistentKeepaliveSec: p.persistentKeepaliveSec,
        allowedIps: p.allowedIps,
      })),
    })),
  };
}

/**
 * F-wireguard: live WireGuard state (`GET /api/v1/state/vpn/wireguard`, the agent's WireguardState RPC joined with the
 * running configuration's names; live status changes arrive on the WebSocket topic `wireguard.events`) and the
 * key-pair helper (`POST /api/v1/actions/vpn/wireguard/keypair`: the private key is stored through the secrets
 * service as `key/<name>` and never returned). Configuration goes through the generic `/api/v1/config/**` routes.
 * The state walks VPP: clients refresh on demand or at most every 30 s (D-132).
 */
@ApiTags('vpn')
@Controller('api/v1')
export class WireguardController {
  constructor(
    @Inject(AgentClient) private readonly agent: AgentClient,
    @Inject(DatastoreService) private readonly ds: DatastoreService,
    @Inject(SecretsService) private readonly secrets: SecretsService,
  ) {}

  @Get('state/vpn/wireguard')
  @Protected(400, 502, 503)
  @ApiQuery({
    name: 'interface',
    required: false,
    schema: { type: 'string', pattern: '^wg[0-9]{1,10}$' },
  })
  @ApiOperation({
    summary:
      'WireGuard interfaces and peers: handshake state, learnt endpoint, last handshake, interface counters (no keys but public ones)',
  })
  @ApiOkResponse({ schema: openapi(WireguardStateOut, 'output') })
  async state(@Query(new ZodPipe(StateQuery)) q: z.output<typeof StateQuery>) {
    const [st, running] = await Promise.all([
      this.agent.wireguardState(q.interface === undefined ? [] : [q.interface]),
      this.ds.getRunning(),
    ]);
    return wireguardStateOf(st, running.doc as Json);
  }

  @Post('actions/vpn/wireguard/keypair')
  @MinRole('admin')
  @HttpCode(200)
  @Protected(400, 409)
  @ApiOperation({
    summary:
      'Generate a WireGuard key pair: the private key is stored as secret key/<name> (never returned); returns the reference and the public key',
  })
  @ApiBody({ schema: openapi(KeypairBody) })
  @ApiOkResponse({ schema: openapi(KeypairOut, 'output') })
  async keypair(
    @Body(new ZodPipe(KeypairBody)) body: z.output<typeof KeypairBody>,
    @Req() req: VrxRequest,
  ) {
    const kp = wireguardKeypair();
    const r = await this.secrets.put('key', body.name, kp.privateKey, {
      replace: false,
      userId: req.principal!.id,
    });
    req.audit = {
      resource: `secret/${r.ref}`,
      after: { ref: r.ref, publicKey: kp.publicKey, version: r.version },
    };
    return { ref: r.ref, publicKey: kp.publicKey, version: r.version };
  }
}
