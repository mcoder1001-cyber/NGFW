import type { handleUnaryCall } from '@grpc/grpc-js';
import type { SnmpStateRequest, SnmpStateResponse } from '@ngfw/proto';

type Json = Record<string, unknown>;

/**
 * F-snmp fake of the SnmpState RPC for the in-process fake agent (wired by one line in testing/fake-agent.ts, P5):
 * derived from the fake's applied document — `configured` = services.snmp.enabled, the daemon "answers" with the
 * configured sysName, the subagent is registered unless `subagent.enabled` is false. Credentials by name only (the
 * document holds refs, never values).
 */
export function snmpStateFake(
  current: () => Json,
): handleUnaryCall<SnmpStateRequest, SnmpStateResponse> {
  return (_call, cb) => {
    const snmp = ((current()['services'] as Json | undefined)?.['snmp'] ?? {}) as Json;
    const on = snmp['enabled'] === true;
    const users = Object.keys((snmp['v3Users'] as Json | undefined) ?? {}).sort();
    const communities = Object.keys((snmp['communities'] as Json | undefined) ?? {});
    const sub = snmp['subagent'] as Json | undefined;
    cb(null, {
      configured: on,
      reachable: on,
      endpoint: on ? '127.0.0.1:161' : '',
      credential: !on
        ? ''
        : users.length > 0
          ? `v3 user ${users[0]}`
          : communities.length > 0
            ? 'v2c community'
            : '',
      sysName: on ? String(snmp['sysName'] ?? '') : '',
      sysDescr: on ? 'Linux vrx (fake agent)' : '',
      sysLocation: on ? String(snmp['sysLocation'] ?? '') : '',
      sysContact: on ? String(snmp['sysContact'] ?? '') : '',
      sysUpTime: on ? '4200' : '0',
      error: '',
      engineId: String(snmp['engineId'] ?? ''),
      pendingAction: '',
      subagentRegistered: on && sub?.['enabled'] !== false,
      subagentRegistrations: on && sub?.['enabled'] !== false ? '1' : '0',
      subagentError: '',
    });
  };
}
