import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { HaClusterSchema } from '../domains/ha.js';
import { vrrpConfigSyncValidators } from './vrrp-config-sync.js';
const doc = (address: string, priority = 255) =>
  RootConfig.parse({
    interfaces: { loop1: { ipv4: ['192.0.2.1/24'] } },
    ha: { vrrp: { lan: { interface: 'loop1', vrId: 1, addresses: [address], priority } } },
  });
describe('HA owner and sync boundary contracts', () => {
  it('priority255 requires actual interface ownership, not merely the same subnet', () => {
    expect(vrrpConfigSyncValidators[0]!.validate(doc('192.0.2.1'))).toEqual([]);
    expect(vrrpConfigSyncValidators[0]!.validate(doc('192.0.2.2'))).toMatchObject([
      { pointer: '/ha/vrrp/lan/priority' },
    ]);
    expect(vrrpConfigSyncValidators[0]!.validate(doc('192.0.2.2', 200))).toEqual([]);
  });
  it('rejects malformed TLS pins and unsafe exclusion paths', () => {
    const c = {
      nodeName: 'a',
      peers: [{ name: 'b', address: '192.0.2.2', certificatePin: 'a'.repeat(64) }],
      secretRef: 'key/cluster',
    };
    expect(HaClusterSchema.safeParse(c).success).toBe(true);
    expect(
      HaClusterSchema.safeParse({ ...c, peers: [{ ...c.peers[0], certificatePin: 'sha256/no' }] })
        .success,
    ).toBe(false);
    expect(
      HaClusterSchema.safeParse({ ...c, syncExclude: ['/constructor/prototype'] }).success,
    ).toBe(false);
    expect(
      HaClusterSchema.safeParse({ ...c, syncExclude: ['/interfaces/loop1/ipv4'] }).success,
    ).toBe(true);
  });
});
