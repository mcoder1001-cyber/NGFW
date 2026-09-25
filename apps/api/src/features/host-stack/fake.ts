import { status, type handleUnaryCall } from '@grpc/grpc-js';
import type { HostStackStateRequest, HostStackStateResponse } from '@ngfw/proto';

type Json = Record<string, unknown>;

/**
 * Fake of the agent's HostStackState RPC for the in-process FakeAgent (wave-A-hotspots P5): the session layer is
 * "on" when the applied document enables it; rules and namespaces are the applied document's, as a real agent on a
 * rule-table VPP would report them.
 */
export function hostStackFake(
  owner: string,
  applied: () => Json,
): handleUnaryCall<HostStackStateRequest, HostStackStateResponse> {
  return (call, cb) => {
    if (call.request.owner !== '' && call.request.owner !== owner) {
      return cb({
        code: status.INVALID_ARGUMENT,
        details: `owner ${call.request.owner} does not match ${owner}`,
      });
    }
    const hs = ((applied()['services'] as Json | undefined)?.['hostStack'] ?? {}) as Json;
    const rules = (hs['sessionRules'] ?? []) as Json[];
    const namespaces = Object.keys((hs['namespaces'] ?? {}) as Json).sort();
    cb(null, {
      sessionEnabled: hs['enabled'] === true,
      sessionDetail:
        hs['enabled'] === true ? '' : 'session layer disabled (session_rules_v2_dump refused)',
      namespaces,
      rules: rules.map((r) => ({
        tag: String(r['tag']),
        scope: String(r['scope'] ?? 'global'),
        transport: String(r['transport']),
        local: String(r['local']),
        localPort: Number(r['localPort'] ?? 0),
        remote: String(r['remote']),
        remotePort: Number(r['remotePort'] ?? 0),
        action: String(r['action']),
        appnsIndexes: [r['appNamespace'] ? namespaces.indexOf(String(r['appNamespace'])) + 1 : 0],
      })),
      ruleCountTotal: rules.length,
      retrievedAt: new Date(),
    });
  };
}
