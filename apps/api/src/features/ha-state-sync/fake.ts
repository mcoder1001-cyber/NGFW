import { status, type handleUnaryCall } from '@grpc/grpc-js';
import { HaSyncOp, type HaSyncStateRequest, type HaSyncStateResponse } from '@ngfw/proto';
import {
  registerActionHandler,
  type ActionHandler,
  type FakeAgent,
} from '../../testing/fake-agent.js';
const observations = new WeakMap<FakeAgent, HaSyncStateResponse>();
/** Explicit test observations; desired configuration alone never fabricates live endpoints. */
export function setHaSyncObservation(host: FakeAgent, value: HaSyncStateResponse): void {
  observations.set(host, value);
}
let host: FakeAgent | undefined;
const action: ActionHandler = (call) => {
  const observed = host && observations.get(host);
  if (!observed?.actionsAllowed) {
    call.emit('error', { code: status.PERMISSION_DENIED, details: 'not the globals owner' });
    return;
  }
  if (host?.failAllWith !== undefined) {
    call.emit('error', { code: host.failAllWith, details: 'fake failure' });
    return;
  }
  if (
    call.request.haSync?.op !== HaSyncOp.HA_SYNC_OP_RESYNC &&
    call.request.haSync?.op !== HaSyncOp.HA_SYNC_OP_FLUSH
  ) {
    call.emit('error', { code: status.INVALID_ARGUMENT, details: 'unknown HA sync operation' });
    return;
  }
  if (!observed.kinds.some((k) => k.kind === 'nat44-ei' && k.configured && k.active)) {
    call.emit('error', { code: status.FAILED_PRECONDITION, details: 'NAT44-EI HA is not active' });
    return;
  }
  if (call.request.haSync.op === HaSyncOp.HA_SYNC_OP_RESYNC) {
    observed.lastResync = new Date();
    observed.lastMissedCount = 0;
    observed.resyncCount = (BigInt(observed.resyncCount) + 1n).toString();
  }
  call.write({ done: { exitCode: 0, summary: 'fake native HA completion', stats: {} } });
  call.end();
};
export function haSyncFake(
  agent: FakeAgent,
): handleUnaryCall<HaSyncStateRequest, HaSyncStateResponse> {
  host = agent;
  registerActionHandler('haSync', action);
  return (call, cb) => {
    agent.calls.push({ method: 'HaSyncState', request: call.request });
    if (agent.failAllWith !== undefined) {
      cb({ code: agent.failAllWith, details: 'fake failure' });
      return;
    }
    if (call.request.owner && call.request.owner !== agent.owner) {
      cb({ code: status.PERMISSION_DENIED, details: 'owner mismatch' });
      return;
    }
    cb(
      null,
      observations.get(agent) ?? {
        owner: agent.owner,
        listener: undefined,
        failover: undefined,
        lastResync: undefined,
        kinds: [
          {
            kind: 'nat44-ei',
            supported: true,
            configured: false,
            active: false,
            reason: 'no injected native observation',
          },
          {
            kind: 'nat44-ed',
            supported: false,
            configured: false,
            active: false,
            reason: 'not supported by VPP (V2)',
          },
          {
            kind: 'acl',
            supported: false,
            configured: false,
            active: false,
            reason: 'not supported by VPP (V2)',
          },
          {
            kind: 'ipsec',
            supported: false,
            configured: false,
            active: false,
            reason: 'native IKEv2 re-key on failover',
          },
        ],
        resyncCount: '0',
        packetCountersAvailable: false,
        actionsAllowed: false,
        retrievedAt: new Date(),
        observationError: 'no injected native observation',
      },
    );
  };
}
