import { describe, expect, it } from 'vitest';
import { snmpStateFake } from './fake.js';
import { SnmpStateOut, toSnmpState } from './snmp.controller.js';
import type { SnmpStateResponse } from '@ngfw/proto';

function call(doc: Record<string, unknown>): Promise<SnmpStateResponse> {
  return new Promise((resolve, reject) =>
    snmpStateFake(() => doc)({} as never, (err, res) => (err ? reject(err) : resolve(res!))),
  );
}

describe('F-snmp state', () => {
  it('maps the RPC to the documented shape, credentials by name only', async () => {
    const doc = {
      services: {
        snmp: {
          enabled: true,
          sysName: 'vrx-a',
          communities: { ro: { secretRef: 'password/snmp-ro' } },
          v3Users: { noc: { authRef: 'password/noc-auth', privRef: 'password/noc-priv' } },
        },
      },
    };
    const out = toSnmpState(await call(doc));
    expect(SnmpStateOut.parse(out)).toEqual(out);
    expect(out).toMatchObject({
      configured: true,
      daemon: { reachable: true, sysName: 'vrx-a', credential: 'v3 user noc' },
      subagent: { registered: true },
    });
    expect(JSON.stringify(out)).not.toContain('password/');
  });

  it('reports a disabled agent and a disabled subagent', async () => {
    expect((await call({})).configured).toBe(false);
    const r = await call({
      services: { snmp: { enabled: true, communities: { ro: {} }, subagent: { enabled: false } } },
    });
    expect(r.subagentRegistered).toBe(false);
    expect(r.credential).toBe('v2c community');
  });
});
