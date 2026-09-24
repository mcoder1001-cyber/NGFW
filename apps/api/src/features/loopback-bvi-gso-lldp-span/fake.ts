import { status, type handleUnaryCall, type sendUnaryData } from '@grpc/grpc-js';
import type {
  DataplaneServer,
  LldpNeighbor,
  LldpNeighborsRequest,
  LldpNeighborsResponse,
} from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';

type Json = Record<string, unknown>;

const obj = (v: unknown): Json =>
  v !== null && typeof v === 'object' && !Array.isArray(v) ? (v as Json) : {};

/** Test knobs of the F-loopback-bvi-gso-lldp-span fake: the peer heard per interface, an agent without the RPC. */
export interface LldpFakeOptions {
  heard?: Record<string, { chassisId: string; portId: string; ttl?: number; agoSec?: number }>;
  unimplemented?: boolean;
}

/**
 * The fake agent's view of the applied document (FakeAgent.current, protobuf JSON) as the real agent's LldpNeighbors
 * reports it (docs/contracts/proto.md §11): one entry per interface of an enabled `services.lldp`, ordered by name.
 */
export class LldpFake {
  constructor(
    private readonly agent: FakeAgent,
    readonly opts: LldpFakeOptions = {},
  ) {}

  table(): LldpNeighbor[] {
    const lldp = obj(obj(this.agent.current['services'])['lldp']);
    if (lldp['enabled'] !== true || !Array.isArray(lldp['interfaces'])) return [];
    const names = (lldp['interfaces'] as unknown[])
      .map((e) => obj(e)['interface'])
      .filter((n): n is string => typeof n === 'string')
      .sort();
    return names.map((name, i) => {
      const h = this.opts.heard?.[name];
      return {
        interface: name,
        swIfIndex: i + 1,
        heard: h !== undefined,
        chassisId: h?.chassisId ?? '',
        chassisIdSubtype: h ? 'mac-address' : '',
        portId: h?.portId ?? '',
        portIdSubtype: h ? 'interface-name' : '',
        ttl: h ? (h.ttl ?? 120) : 0,
        lastHeardSecAgo: h ? (h.agoSec ?? 5) : 0,
        lastSentSecAgo: 3,
      };
    });
  }

  /** The handler, with the fake agent's owner check. */
  handlers(): Pick<DataplaneServer, 'lldpNeighbors'> {
    const deny = (cb: sendUnaryData<never>, req: { owner: string }): boolean => {
      this.agent.calls.push({ method: 'LldpNeighbors', request: req });
      if (this.opts.unimplemented) {
        cb({ code: status.UNIMPLEMENTED, details: 'unknown method LldpNeighbors' });
        return true;
      }
      if (req.owner && req.owner !== this.agent.owner) {
        cb({ code: status.INVALID_ARGUMENT, details: `owner '${req.owner}' ≠ agent owner` });
        return true;
      }
      return false;
    };
    const lldpNeighbors: handleUnaryCall<LldpNeighborsRequest, LldpNeighborsResponse> = (
      call,
      cb,
    ) => {
      if (deny(cb, call.request)) return;
      const r = call.request;
      const limit = r.limit === 0 ? 100 : r.limit;
      if (limit > 1000) {
        return cb({ code: status.INVALID_ARGUMENT, details: `limit ${r.limit} exceeds 1000` });
      }
      const all = this.table();
      cb(null, {
        neighbors: all.slice(r.offset, r.offset + limit),
        total: all.length,
        owner: this.agent.owner,
        retrievedAt: new Date(),
      });
    };
    return { lldpNeighbors };
  }
}

/**
 * Give a running FakeAgent the LldpNeighbors RPC (its own stub answers UNIMPLEMENTED, wave-A-hotspots P5): the handler
 * is layered over the fake's service implementation and its gRPC server restarted on the same socket (the API's
 * channel reconnects by itself). Test code only.
 */
export async function installLldpFake(
  agent: FakeAgent,
  opts: LldpFakeOptions = {},
): Promise<LldpFake> {
  const fake = new LldpFake(agent, opts);
  const inner = agent as unknown as { impl: () => DataplaneServer; socketPath: string };
  const base = inner.impl.bind(agent);
  inner.impl = () => ({ ...base(), ...fake.handlers() });
  const socket = inner.socketPath;
  await agent.stop();
  await agent.start(socket);
  return fake;
}
