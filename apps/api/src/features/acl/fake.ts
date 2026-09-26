import { status, type handleUnaryCall } from '@grpc/grpc-js';
import {
  AclRuleStatus,
  type AclBoundAcl,
  type AclInterfaceState,
  type AclListState,
  type AclRuleState,
  type AclStateRequest,
  type AclStateResponse,
} from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';

/**
 * The fake agent's AclState (wave-A-hotspots P5): computed from the state the fake holds (`current.acl`, i.e. what was
 * applied). Each list gets an acl_index in name order; every enabled rule counts as one VPP rule (the real agent expands
 * objects; the fake does not). Tests set hits per sequence (`setFakeAclHits`), the counters flag
 * (`setFakeAclCounters`) and foreign ACLs on an interface (`setFakeAclForeign`). Owner check, forced failures and
 * "acl not implemented" behave like the fake's other RPCs.
 */
interface FakeAclState {
  hits: Map<string, Map<number, { packets: number; bytes: number }>>;
  counters: boolean;
  foreign: Map<string, { input: AclBoundAcl[]; output: AclBoundAcl[] }>;
}

const states = new WeakMap<FakeAgent, FakeAclState>();

function stateOf(agent: FakeAgent): FakeAclState {
  let s = states.get(agent);
  if (!s) states.set(agent, (s = { hits: new Map(), counters: false, foreign: new Map() }));
  return s;
}

export function setFakeAclHits(
  agent: FakeAgent,
  list: string,
  sequence: number,
  packets: number,
  bytes = packets * 100,
): void {
  const s = stateOf(agent);
  const m = s.hits.get(list) ?? new Map();
  m.set(sequence, { packets, bytes });
  s.hits.set(list, m);
}

export function setFakeAclCounters(agent: FakeAgent, on: boolean): void {
  stateOf(agent).counters = on;
}

export function setFakeAclForeign(
  agent: FakeAgent,
  iface: string,
  input: AclBoundAcl[],
  output: AclBoundAcl[] = [],
): void {
  stateOf(agent).foreign.set(iface, { input, output });
}

type Json = Record<string, unknown>;

export function aclState(agent: FakeAgent): handleUnaryCall<AclStateRequest, AclStateResponse> {
  return (call, cb) => {
    const req = call.request;
    agent.calls.push({ method: 'AclState', request: req });
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
    const limit = req.limit === 0 ? 100 : req.limit;
    if (limit > 1000 || (req.filter?.sequences.length ?? 0) > 1000) {
      cb({ code: status.INVALID_ARGUMENT, details: 'limit/sequences above 1000' });
      return;
    }
    const st = stateOf(agent);
    const acl = (agent.current['acl'] ?? {}) as Json;
    const lists = (acl['lists'] ?? {}) as Record<string, Json>;
    const names = Object.keys(lists).sort();
    const index = new Map(names.map((n, i) => [n, i]));
    const rulesOf = (n: string) =>
      (((lists[n]?.['rules'] as Json[] | undefined) ?? []) as Json[])
        .slice()
        .sort((a, b) => Number(a['sequence']) - Number(b['sequence']));
    const hits = (n: string, seq: number) => (st.counters ? st.hits.get(n)?.get(seq) : undefined);
    const summary = (n: string): AclListState => {
      const rules = rulesOf(n);
      let packets = 0;
      let bytes = 0;
      for (const r of rules) {
        const h = hits(n, Number(r['sequence']));
        packets += h?.packets ?? 0;
        bytes += h?.bytes ?? 0;
      }
      return {
        name: n,
        aclIndex: index.get(n)!,
        vppRules: rules.filter((r) => r['enabled'] !== false).length,
        mappingKnown: true,
        configRules: rules.length,
        packets: String(packets),
        bytes: String(bytes),
      };
    };
    const resp: AclStateResponse = {
      owner: agent.owner,
      retrievedAt: new Date(),
      countersAvailable: st.counters,
      countersReason: st.counters ? '' : 'per-rule counters are off in VPP (fake)',
      lists: [],
      rules: [],
      total: 0,
      interfaces: [],
      macipLists: [],
    };
    if (req.list) {
      if (!index.has(req.list)) {
        cb({ code: status.NOT_FOUND, details: `acl state: no such list in VPP: '${req.list}'` });
        return;
      }
      resp.lists = [summary(req.list)];
      const want = new Set(req.filter?.sequences ?? []);
      let first = 0;
      const all: AclRuleState[] = [];
      for (const r of rulesOf(req.list)) {
        const seq = Number(r['sequence']);
        const on = r['enabled'] !== false;
        const h = hits(req.list, seq);
        const rs: AclRuleState = {
          sequence: seq,
          status: on
            ? AclRuleStatus.ACL_RULE_STATUS_APPLIED
            : AclRuleStatus.ACL_RULE_STATUS_DISABLED,
          vppRules: on ? 1 : 0,
          firstVppRule: first,
          packets: String(h?.packets ?? 0),
          bytes: String(h?.bytes ?? 0),
        };
        if (on) first++;
        if (want.size > 0 && !want.has(seq)) continue;
        if (req.filter?.hitsOnly && (h?.packets ?? 0) === 0) continue;
        all.push(rs);
      }
      resp.total = all.length;
      resp.rules = all.slice(req.offset, req.offset + limit);
    } else {
      resp.lists = names.map(summary);
      resp.macipLists = Object.keys((acl['macip'] ?? {}) as Json)
        .sort()
        .map((n, i) => ({
          name: n,
          aclIndex: i,
          vppRules: 1,
          mappingKnown: true,
          configRules: 1,
          packets: '0',
          bytes: '0',
        }));
    }
    if (req.includeInterfaces) {
      const per = new Map<string, AclInterfaceState>();
      const get = (n: string) => {
        let s = per.get(n);
        if (!s)
          per.set(
            n,
            (s = {
              interface: n,
              swIfIndex: per.size + 1,
              input: [],
              output: [],
              macip: undefined,
            }),
          );
        return s;
      };
      for (const [n, f] of st.foreign) {
        get(n).input.push(...f.input);
        get(n).output.push(...f.output);
      }
      for (const a of (acl['attachments'] as Json[] | undefined) ?? []) {
        if (a['enabled'] === false) continue;
        const t = (a['target'] ?? {}) as Json;
        if (t['kind'] !== 'interface') continue;
        const name = String(a['list']);
        const b: AclBoundAcl = {
          aclIndex: index.get(name) ?? 0,
          name,
          tag: `${agent.owner}:${name}`,
          foreign: false,
        };
        (a['direction'] === 'out'
          ? get(String(t['interface'])).output
          : get(String(t['interface'])).input
        ).push(b);
      }
      resp.interfaces = [...per.values()];
    }
    cb(null, resp);
  };
}
