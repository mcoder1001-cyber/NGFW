import { status, type handleUnaryCall } from '@grpc/grpc-js';
import type {
  FqdnObjectState,
  FqdnObjectStateRequest,
  FqdnObjectStateResponse,
} from '@ngfw/proto';
import type { FakeAgent } from '../../testing/fake-agent.js';

/**
 * The fake agent's FqdnObjectState (wave-A-hotspots P5): one entry per FQDN address object of the state the fake
 * holds (`current.objects.addresses`, i.e. what was applied), with the resolver answer a test set through
 * `setFakeFqdn` (unset = not resolved yet). Owner check, forced failures and "objects not implemented" behave like
 * the fake's other RPCs.
 */
const answers = new WeakMap<FakeAgent, Map<string, Partial<FqdnObjectState>>>();

type Json = Record<string, unknown>;

/** Set (or clear with `undefined`) what the fake resolver reports for FQDN object `name`. */
export function setFakeFqdn(agent: FakeAgent, name: string, st: Partial<FqdnObjectState> | undefined): void {
  let m = answers.get(agent);
  if (!m) answers.set(agent, (m = new Map()));
  if (st === undefined) m.delete(name);
  else m.set(name, st);
}

export function fqdnObjectState(
  agent: FakeAgent,
): handleUnaryCall<FqdnObjectStateRequest, FqdnObjectStateResponse> {
  return (call, cb) => {
    const req = call.request;
    agent.calls.push({ method: 'FqdnObjectState', request: req });
    if (agent.failAllWith !== undefined) {
      cb({ code: agent.failAllWith, details: `fake agent: forced ${status[agent.failAllWith]}` });
      return;
    }
    if (req.owner && req.owner !== agent.owner) {
      cb({ code: status.INVALID_ARGUMENT, details: `owner '${req.owner}' ≠ agent owner '${agent.owner}'` });
      return;
    }
    if (!agent.implemented.includes('objects')) {
      cb({ code: status.UNIMPLEMENTED, details: 'fake agent: objects not implemented' });
      return;
    }
    const addresses = ((agent.current['objects'] as Json | undefined)?.['addresses'] ?? {}) as Record<string, Json>;
    const set = answers.get(agent);
    const objects: FqdnObjectState[] = Object.entries(addresses)
      .filter(([name, a]) => a['type'] === 'fqdn' && (req.names.length === 0 || req.names.includes(name)))
      .sort(([a], [b]) => (a < b ? -1 : 1))
      .map(([name, a]) => ({
        name,
        fqdn: String(a['fqdn'] ?? ''),
        addresses: [],
        lastResolved: undefined,
        nextRefresh: undefined,
        error: '',
        failures: 0,
        ...set?.get(name),
      }));
    cb(null, { objects, owner: agent.owner, retrievedAt: new Date() });
  };
}
