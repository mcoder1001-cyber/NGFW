import { status, type handleUnaryCall, type sendUnaryData } from '@grpc/grpc-js';
import type {
  BridgeDomainMac,
  BridgeDomainMacsRequest,
  BridgeDomainMacsResponse,
  BridgeDomainMember,
  BridgeDomainStateRequest,
  BridgeDomainStateResponse,
  BridgeDomainStatus,
  DataplaneServer,
} from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';

type Json = Record<string, unknown>;

const obj = (v: unknown): Json =>
  v !== null && typeof v === 'object' && !Array.isArray(v) ? (v as Json) : {};

/** Test knobs of the F-bridge-l2 fake: learned MACs per bridge-domain id, and an agent without the RPCs. */
export interface BridgeL2FakeOptions {
  learned?: Record<number, number>;
  unimplemented?: boolean;
}

interface Port {
  name: string;
  l2: Json;
}

function ports(current: Json): Port[] {
  const out: Port[] = [];
  for (const [name, v] of Object.entries(obj(current['interfaces']))) {
    const itf = obj(v);
    if (itf['l2']) out.push({ name, l2: obj(itf['l2']) });
    for (const [id, s] of Object.entries(obj(itf['subinterfaces']))) {
      const sub = obj(s);
      if (sub['l2']) out.push({ name: `${name}.${id}`, l2: obj(sub['l2']) });
    }
  }
  return out;
}

/** A deterministic fake MAC for learned entry n of bridge domain id. */
function learnedMac(id: number, n: number): string {
  const b = [0x02, 0xfe, (id >> 8) & 0xff, id & 0xff, (n >> 8) & 0xff, n & 0xff];
  return b.map((x) => x.toString(16).padStart(2, '0')).join(':');
}

/**
 * The fake agent's view of the applied document (FakeAgent.current, protobuf JSON) as the real agent's
 * BridgeDomainState / BridgeDomainMacs would report it (docs/contracts/proto.md §11).
 */
export class BridgeL2Fake {
  constructor(
    private readonly agent: FakeAgent,
    readonly opts: BridgeL2FakeOptions = {},
  ) {}

  private domains(): { name: string; cfg: Json }[] {
    const bds = obj(obj(obj(this.agent.current['routing'])['l2'])['bridgeDomains']);
    return Object.keys(bds)
      .sort()
      .map((name) => ({ name, cfg: obj(bds[name]) }));
  }

  state(): BridgeDomainStatus[] {
    const all = ports(this.agent.current);
    return this.domains().map(({ name, cfg }) => {
      const id = Number(cfg['id'] ?? 0);
      const members: BridgeDomainMember[] = all
        .filter((p) => p.l2['bridgeDomain'] === name)
        .map((p, i) => ({
          interface: p.name,
          swIfIndex: i + 1,
          portType: p.l2['bvi'] === true ? 'bvi' : p.l2['uuFwd'] === true ? 'uu-fwd' : 'normal',
          shg: Number(p.l2['shg'] ?? 0),
          tagRewrite: String(obj(p.l2['tagRewrite'])['op'] ?? ''),
        }));
      const bvi = members.find((m) => m.portType === 'bvi')?.interface ?? '';
      const statics = Array.isArray(cfg['staticMacs']) ? cfg['staticMacs'].length : 0;
      return {
        id,
        name,
        flood: cfg['flood'] !== false,
        uuFlood: cfg['uuFlood'] !== false,
        forward: cfg['forward'] !== false,
        learn: cfg['learn'] !== false,
        arpTerm: cfg['arpTerm'] === true,
        arpUfwd: false,
        macAgeMin: Number(cfg['macAgeMin'] ?? 0),
        bvi,
        uuFwd: members.find((m) => m.portType === 'uu-fwd')?.interface ?? '',
        members,
        learnedMacs: this.opts.learned?.[id] ?? 0,
        staticMacs: statics + (bvi ? 1 : 0),
      };
    });
  }

  macs(id: number): BridgeDomainMac[] | undefined {
    const d = this.domains().find((x) => Number(x.cfg['id']) === id);
    if (!d) return undefined;
    const out: BridgeDomainMac[] = [];
    for (const e of Array.isArray(d.cfg['staticMacs']) ? (d.cfg['staticMacs'] as Json[]) : []) {
      out.push({
        mac: String(e['mac']).toLowerCase(),
        interface: String(e['interface']),
        swIfIndex: 1,
        static: true,
        filter: false,
        bvi: false,
      });
    }
    const n = this.opts.learned?.[id] ?? 0;
    for (let i = 0; i < n; i++) {
      out.push({
        mac: learnedMac(id, i),
        interface: 'host-learned',
        swIfIndex: 2,
        static: false,
        filter: false,
        bvi: false,
      });
    }
    return out.sort((a, b) => a.mac.localeCompare(b.mac));
  }

  /** The two handlers, with the fake agent's owner check. */
  handlers(): Pick<DataplaneServer, 'bridgeDomainState' | 'bridgeDomainMacs'> {
    const deny = (cb: sendUnaryData<never>, req: { owner: string }, method: string): boolean => {
      this.agent.calls.push({ method, request: req });
      if (this.opts.unimplemented) {
        cb({ code: status.UNIMPLEMENTED, details: `unknown method ${method}` });
        return true;
      }
      if (req.owner && req.owner !== this.agent.owner) {
        cb({ code: status.INVALID_ARGUMENT, details: `owner '${req.owner}' ≠ agent owner` });
        return true;
      }
      return false;
    };
    const bridgeDomainState: handleUnaryCall<
      BridgeDomainStateRequest,
      BridgeDomainStateResponse
    > = (call, cb) => {
      if (deny(cb, call.request, 'BridgeDomainState')) return;
      const want = new Set(call.request.ids);
      cb(null, {
        bridgeDomains: this.state().filter((b) => want.size === 0 || want.has(b.id)),
        owner: this.agent.owner,
        retrievedAt: new Date(),
      });
    };
    const bridgeDomainMacs: handleUnaryCall<BridgeDomainMacsRequest, BridgeDomainMacsResponse> = (
      call,
      cb,
    ) => {
      if (deny(cb, call.request, 'BridgeDomainMacs')) return;
      const r = call.request;
      const limit = r.limit === 0 ? 100 : r.limit;
      if (limit > 1000) {
        return cb({ code: status.INVALID_ARGUMENT, details: `limit ${r.limit} exceeds 1000` });
      }
      const all = this.macs(r.bdId);
      if (!all) {
        return cb({
          code: status.NOT_FOUND,
          details: `bridge domain ${r.bdId} is not this agent's`,
        });
      }
      cb(null, {
        macs: all.slice(r.offset, r.offset + limit),
        total: all.length,
        owner: this.agent.owner,
        retrievedAt: new Date(),
      });
    };
    return { bridgeDomainState, bridgeDomainMacs };
  }
}

/**
 * Give a running FakeAgent the F-bridge-l2 RPCs (its own stubs answer UNIMPLEMENTED, wave-A-hotspots P5): the
 * handlers are layered over the fake's service implementation and its gRPC server is restarted on the same socket
 * (the API's channel reconnects by itself). Test code only.
 */
export async function installBridgeL2Fake(
  agent: FakeAgent,
  opts: BridgeL2FakeOptions = {},
): Promise<BridgeL2Fake> {
  const fake = new BridgeL2Fake(agent, opts);
  const inner = agent as unknown as { impl: () => DataplaneServer; socketPath: string };
  const base = inner.impl.bind(agent);
  inner.impl = () => ({ ...base(), ...fake.handlers() });
  const socket = inner.socketPath;
  await agent.stop();
  await agent.start(socket);
  return fake;
}
