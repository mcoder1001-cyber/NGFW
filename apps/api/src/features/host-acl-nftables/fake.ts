import { status, type handleUnaryCall } from '@grpc/grpc-js';
import type {
  HostAclChainState,
  HostAclRuleState,
  HostAclStateRequest,
  HostAclStateResponse,
} from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';

/**
 * The fake agent's HostAclState (wave-A-hotspots P5): a plausible table derived from the host lists and attachments the
 * fake holds (`current.acl`, i.e. what was applied) — mode `check`, table `vrx_<owner>`, one chain per enabled
 * attachment (`in_|out_|fwd_<list>`), one kernel rule per enabled host rule (comment `vrx:<list>:<seq>/0:00000000`),
 * counters from {@link setFakeHostAclCounters}. Owner check, forced failures and "acl not implemented" behave like the
 * fake's other RPCs. The real renderer (and its rule text) lives in the agent (renderers/nftables).
 */
const counters = new WeakMap<FakeAgent, Map<string, { packets: string; bytes: string }>>();

type Json = Record<string, unknown>;

/** Set (or clear with `undefined`) the counters the fake reports for host rule `<list>:<sequence>`. */
export function setFakeHostAclCounters(
  agent: FakeAgent,
  values: Record<string, { packets: number | string; bytes: number | string } | undefined>,
): void {
  let m = counters.get(agent);
  if (!m) counters.set(agent, (m = new Map()));
  for (const [key, v] of Object.entries(values)) {
    if (v === undefined) m.delete(key);
    else m.set(key, { packets: String(v.packets), bytes: String(v.bytes) });
  }
}

const HOOK_PREFIX: Record<string, string> = { input: 'in', output: 'out', forward: 'fwd' };

/** The table the fake reports for its current `acl` state (exported for unit tests). */
export function fakeHostAclTable(agent: FakeAgent): Omit<HostAclStateResponse, 'retrievedAt'> {
  const acl = (agent.current['acl'] ?? {}) as Json;
  const lists = (acl['host'] ?? {}) as Record<string, Json>;
  const attachments = (acl['hostAttachments'] ?? []) as Json[];
  const policy = String(((acl['hostSettings'] ?? {}) as Json)['defaultInput'] ?? 'accept');
  const set = counters.get(agent);
  const chains: HostAclChainState[] = attachments
    .filter((a) => a['enabled'] !== false && typeof a['list'] === 'string')
    .map((a) => {
      const list = String(a['list']);
      const hook = String(a['chain'] ?? 'input');
      const rules = ((lists[list]?.['rules'] ?? []) as Json[])
        .map((r, i) => ({ r, i }))
        .filter(({ r }) => r['enabled'] !== false)
        .sort((x, y) => Number(x.r['sequence']) - Number(y.r['sequence']))
        .map(({ r, i }): HostAclRuleState => {
          const sequence = Number(r['sequence'] ?? 0);
          const verdict = String(r['action'] ?? 'accept');
          const c = set?.get(`${list}:${sequence}`);
          return {
            kind: 'rule',
            list,
            sequence,
            pointer: `/acl/host/${list.replaceAll('~', '~0').replaceAll('/', '~1')}/rules/${i}`,
            text: verdict,
            verdict,
            comment: `vrx:${list}:${sequence}/0:00000000`,
            packets: c?.packets ?? '0',
            bytes: c?.bytes ?? '0',
          };
        });
      return {
        name: `${HOOK_PREFIX[hook] ?? hook}_${list}`,
        hook,
        priority: Number(a['priority'] ?? 0),
        policy: hook === 'input' ? policy : 'accept',
        list,
        rules,
      };
    });
  return {
    owner: agent.owner,
    table: `vrx_${agent.owner}`,
    mode: 'check',
    present: false,
    inSync: true,
    sets: [],
    chains,
  };
}

export function hostAclState(
  agent: FakeAgent,
): handleUnaryCall<HostAclStateRequest, HostAclStateResponse> {
  return (call, cb) => {
    const req = call.request;
    agent.calls.push({ method: 'HostAclState', request: req });
    if (agent.failAllWith !== undefined) {
      cb({ code: agent.failAllWith, details: `fake agent: forced ${status[agent.failAllWith]}` });
      return;
    }
    if (req.owner && req.owner !== agent.owner) {
      cb({
        code: status.INVALID_ARGUMENT,
        details: `owner '${req.owner}' ≠ agent owner '${agent.owner}'`,
      });
      return;
    }
    if (!agent.implemented.includes('acl')) {
      cb({ code: status.UNIMPLEMENTED, details: 'fake agent: acl not implemented' });
      return;
    }
    cb(null, { ...fakeHostAclTable(agent), retrievedAt: new Date() });
  };
}
