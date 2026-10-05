import { describe, expect, it } from 'vitest';
import { RootConfig } from '../../index.js';
import { HaNatListenerSchema } from './ha-state-sync.js';
import { haStateSyncValidators } from '../../semantic/ha-state-sync.js';

const cluster = {
  enabled: true,
  nodeName: 'a',
  peers: [{ name: 'b', address: '10.18.2.2' }],
  secretRef: 'key/ha',
  interface: 'loop18',
  stateSync: {
    nat: true,
    natListener: { address: '10.18.1.1', port: 21830, pathMtu: 1500 },
    natFailover: { address: '10.18.2.2', port: 21831, sessionRefreshSec: 10 },
  },
};
describe('HA state-sync additive contract', () => {
  it('rejects non-unicast listener and invalid MTU', () => {
    for (const address of ['::1', '0.0.0.0', '127.0.0.1', '224.0.0.1'])
      expect(HaNatListenerSchema.safeParse({ address, port: 8750 }).success).toBe(false);
    expect(HaNatListenerSchema.safeParse({ address: '10.18.1.1', port: 0 }).success).toBe(false);
    expect(
      HaNatListenerSchema.safeParse({ address: '10.18.1.1', port: 8750, pathMtu: 1 }).success,
    ).toBe(false);
  });
  it('requires enabled cluster with precise pointers but ED gaps are warnings in agent', () => {
    const config = RootConfig.parse({ ha: { cluster: { ...cluster, enabled: false } } });
    expect(haStateSyncValidators[0]!.validate(config)).toContainEqual({
      pointer: '/ha/cluster/stateSync/nat',
      message: 'State synchronisation requires an enabled HA cluster',
    });
    const ed = RootConfig.parse({
      nat: { mode: 'ed' },
      ha: { cluster: { ...cluster, stateSync: { nat: true, acl: true, ipsec: true } } },
    });
    expect(haStateSyncValidators[0]!.validate(ed)).toEqual([]);
  });
  it('requires explicit EI endpoints and dedicated interface', () => {
    const config = RootConfig.parse({
      nat: { mode: 'ei' },
      ha: { cluster: { ...cluster, interface: undefined, stateSync: { nat: true } } },
    });
    expect(haStateSyncValidators[0]!.validate(config).map((i) => i.pointer)).toEqual([
      '/ha/cluster/stateSync/natListener',
      '/ha/cluster/stateSync/natFailover',
      '/ha/cluster/interface',
    ]);
  });
});
