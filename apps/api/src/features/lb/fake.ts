import { status, type sendUnaryData } from '@grpc/grpc-js';
import type {
  DataplaneServer,
  LbFlushVipRequest,
  LbFlushVipResponse,
  LbStateRequest,
  LbStateResponse,
  LbVipState,
} from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';

type Json = Record<string, unknown>;

const obj = (v: unknown): Json =>
  v !== null && typeof v === 'object' && !Array.isArray(v) ? (v as Json) : {};

/** Test knobs of the F-lb fake. */
export interface LbFakeOptions {
  /** Extra "removed" lb_vip_dump entries (and their removed servers) per VIP name (V20). */
  removed?: Record<string, string[]>;
  /** VIPs the agent has no boot record of (not applied on this VPP instance). */
  notApplied?: Set<string>;
  /** VIPs whose flush the agent refuses (no server in use → FAILED_PRECONDITION). */
  notFlushable?: Set<string>;
  /** An agent older than F-lb: both RPCs answer UNIMPLEMENTED. */
  unimplemented?: boolean;
}

/**
 * The fake agent's LbState / LbFlushVip over the applied document (FakeAgent.current, protobuf JSON), as the real agent
 * reports them (docs/contracts/proto.md "F-lb"): one entry per configured VIP, sorted by name, every configured server
 * in use.
 */
export class LbFake {
  opts: LbFakeOptions = {};
  flushed: string[] = [];

  constructor(private readonly agent: FakeAgent) {}

  private vips(): Record<string, Json> {
    return obj(obj(obj(this.agent.current['services'])['lb'])['vips']) as Record<string, Json>;
  }

  state(names: string[]): LbVipState[] {
    const vips = this.vips();
    return Object.keys(vips)
      .filter((n) => names.length === 0 || names.includes(n))
      .sort()
      .map((name) => {
        const v = obj(vips[name]);
        const removed = this.opts.removed?.[name] ?? [];
        const applied = !(this.opts.notApplied?.has(name) ?? false);
        const servers = (Array.isArray(v['servers']) ? (v['servers'] as unknown[]) : []).map(
          (s) => ({
            address: String(obj(s)['address']),
            inUse: applied,
            inUseSince: 100,
          }),
        );
        return {
          name,
          prefix: String(v['prefix'] ?? ''),
          protocol: String(v['protocol'] ?? 'any'),
          port: Number(v['port'] ?? 0),
          applied,
          vppEntries: applied ? 1 + (removed.length > 0 ? 1 : 0) : 0,
          encap: applied ? String(v['encap'] ?? '') : '',
          dscp: Number(v['dscp'] ?? 0),
          targetPort: Number(v['targetPort'] ?? 0),
          servers: [
            ...servers,
            ...removed.map((address) => ({ address, inUse: false, inUseSince: 50 })),
          ],
        };
      });
  }

  /** The handlers, with the fake agent's call log. */
  handlers(): Pick<DataplaneServer, 'lbState' | 'lbFlushVip'> {
    const refuse = (cb: sendUnaryData<never>, method: string, req: unknown): boolean => {
      this.agent.calls.push({ method, request: req });
      if (this.opts.unimplemented) {
        cb({ code: status.UNIMPLEMENTED, details: `unknown method ${method}` });
        return true;
      }
      const owner = (req as { owner?: string }).owner;
      if (owner !== undefined && owner !== '' && owner !== this.agent.owner) {
        cb({ code: status.INVALID_ARGUMENT, details: `owner ${owner} does not match` });
        return true;
      }
      return false;
    };
    return {
      lbState: (call, cb: sendUnaryData<LbStateResponse>) => {
        const req: LbStateRequest = call.request;
        if (refuse(cb, 'LbState', req)) return;
        const vips = this.state(req.names);
        const removed = Object.values(this.opts.removed ?? {}).filter((r) => r.length > 0).length;
        cb(null, {
          vips,
          owner: this.agent.owner,
          retrievedAt: new Date(),
          totalVppVips: vips.filter((v) => v.applied).length + removed,
        });
      },
      lbFlushVip: (call, cb: sendUnaryData<LbFlushVipResponse>) => {
        const req: LbFlushVipRequest = call.request;
        if (refuse(cb, 'LbFlushVip', req)) return;
        const v = this.vips()[req.name];
        if (v === undefined) {
          cb({
            code: status.NOT_FOUND,
            details: `VIP "${req.name}" is not in this agent's configuration`,
          });
          return;
        }
        if (this.opts.notFlushable?.has(req.name) || this.opts.notApplied?.has(req.name)) {
          cb({
            code: status.FAILED_PRECONDITION,
            details: `VIP "${req.name}" has no application server in use`,
          });
          return;
        }
        const port = Number(v['port'] ?? 0);
        const key = `lb.vip/${String(v['prefix'])}/${String(v['protocol'] ?? 'any')}/${port}`;
        this.flushed.push(req.name);
        cb(null, { vip: key });
      },
    };
  }
}

const fakes = new WeakMap<FakeAgent, LbFake>();

/** The F-lb fake of `agent` (one per fake agent; tests set its knobs). */
export function lbFake(agent: FakeAgent): LbFake {
  let f = fakes.get(agent);
  if (f === undefined) {
    f = new LbFake(agent);
    fakes.set(agent, f);
  }
  return f;
}
