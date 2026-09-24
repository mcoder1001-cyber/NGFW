import { status, type handleUnaryCall } from '@grpc/grpc-js';
import type {
  BondLacpPort,
  BondMemberStatus,
  BondStateRequest,
  BondStateResponse,
  BondStatus,
  DataplaneServer,
} from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';

type Json = Record<string, unknown>;

const obj = (v: unknown): Json =>
  v !== null && typeof v === 'object' && !Array.isArray(v) ? (v as Json) : {};

/** Test knob of the F-bonding fake: an agent without the RPC (answers UNIMPLEMENTED). */
export interface BondingFakeOptions {
  unimplemented?: boolean;
}

const FORCED: Record<string, string> = {
  'round-robin': 'round-robin',
  'active-backup': 'active-backup',
  broadcast: 'broadcast',
};

const lacpPort = (key: number, port: number, state: number, flags: string[]): BondLacpPort => ({
  systemPriority: 65535,
  system: key === 0 ? '00:00:00:00:00:00' : '02:fe:b0:00:00:01',
  key,
  portPriority: 255,
  portNumber: port,
  state,
  stateFlags: flags,
});

/**
 * The fake agent's view of the applied document (FakeAgent.current, protobuf JSON) as the real agent's BondState would
 * report it (docs/contracts/proto.md §11 "F-bonding"): no LACP partner, so LACP members stay defaulted/detached and
 * inactive; members of the other modes are active when they and the bond are enabled.
 */
export class BondingFake {
  constructor(
    private readonly agent: FakeAgent,
    readonly opts: BondingFakeOptions = {},
  ) {}

  state(): BondStatus[] {
    const ifs = obj(this.agent.current['interfaces']);
    let idx = 100;
    return Object.keys(ifs)
      .filter((n) => Object.keys(obj(obj(ifs[n])['bond'])).length > 0)
      .sort()
      .map((name) => {
        const itf = obj(ifs[name]);
        const b = obj(itf['bond']);
        const mode = String(b['mode'] ?? '');
        const up = itf['enabled'] === true;
        const members: BondMemberStatus[] = Object.keys(obj(b['members']))
          .sort()
          .map((m) => {
            const mc = obj(obj(b['members'])[m]);
            const mUp = obj(ifs[m])['enabled'] === true;
            const long = mc['longTimeout'] === true;
            return {
              interface: m,
              swIfIndex: ++idx,
              passive: mc['passive'] === true,
              longTimeout: long,
              weight: typeof mc['weight'] === 'number' ? mc['weight'] : 0,
              isLocalNuma: true,
              adminUp: mUp,
              linkUp: mUp,
              lacp:
                mode === 'lacp'
                  ? {
                      rxState: 'defaulted',
                      txState: 'transmit',
                      muxState: 'detached',
                      ptxState: long ? 'slow-periodic' : 'fast-periodic',
                      actor: lacpPort(
                        Number(name.replace('BondEthernet', '')),
                        idx,
                        long ? 0x45 : 0x47,
                        long
                          ? ['activity', 'aggregation', 'defaulted']
                          : ['activity', 'timeout', 'aggregation', 'defaulted'],
                      ),
                      partner: lacpPort(0, 0, 0, []),
                    }
                  : undefined,
            };
          });
        return {
          name,
          vppName: name,
          swIfIndex: ++idx,
          id: Number(name.replace('BondEthernet', '')),
          mode,
          loadBalance: FORCED[mode] ?? String(b['loadBalance'] ?? 'l2'),
          numaOnly: b['numaOnly'] === true,
          adminUp: up,
          linkUp: up && members.some((m) => m.linkUp),
          memberCount: members.length,
          activeMemberCount: mode === 'lacp' || !up ? 0 : members.filter((m) => m.adminUp).length,
          members,
        };
      });
  }

  /** The BondState handler, with the fake agent's owner check. */
  handlers(): Pick<DataplaneServer, 'bondState'> {
    const bondState: handleUnaryCall<BondStateRequest, BondStateResponse> = (call, cb) => {
      this.agent.calls.push({ method: 'BondState', request: call.request });
      if (this.opts.unimplemented) {
        return cb({ code: status.UNIMPLEMENTED, details: 'unknown method BondState' });
      }
      if (call.request.owner && call.request.owner !== this.agent.owner) {
        return cb({
          code: status.INVALID_ARGUMENT,
          details: `owner '${call.request.owner}' ≠ agent owner`,
        });
      }
      const want = new Set(call.request.names);
      cb(null, {
        bonds: this.state().filter((b) => want.size === 0 || want.has(b.name)),
        owner: this.agent.owner,
        retrievedAt: new Date(),
      });
    };
    return { bondState };
  }
}

/**
 * Give a running FakeAgent the F-bonding RPC (its own stub answers UNIMPLEMENTED, wave-A-hotspots P5): the handler is
 * layered over the fake's service implementation and its gRPC server is restarted on the same socket (the API's
 * channel reconnects by itself). Test code only.
 */
export async function installBondingFake(
  agent: FakeAgent,
  opts: BondingFakeOptions = {},
): Promise<BondingFake> {
  const fake = new BondingFake(agent, opts);
  const inner = agent as unknown as { impl: () => DataplaneServer; socketPath: string };
  const base = inner.impl.bind(agent);
  inner.impl = () => ({ ...base(), ...fake.handlers() });
  const socket = inner.socketPath;
  await agent.stop();
  await agent.start(socket);
  return fake;
}
