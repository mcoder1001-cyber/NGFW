import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { multiwanValidators } from './multiwan.js';

const run = (doc: RootConfigInput) => multiwanValidators[0]!.validate(RootConfig.parse(doc));

const group = (over: Record<string, unknown> = {}) => ({
  name: 'wan',
  mode: 'failover',
  members: [{ interface: 'eth0', nextHop: 'dhcp' }],
  monitors: [{ type: 'icmp', target: '1.1.1.1' }],
  ...over,
});

describe('F-multiwan semantic rules', () => {
  it('accepts members whose interfaces exist', () => {
    expect(
      run({
        interfaces: { eth0: { enabled: true }, eth1: { enabled: true } },
        routing: {
          wanGroups: [
            group({
              members: [
                { interface: 'eth0', nextHop: 'dhcp' },
                { interface: 'eth1', nextHop: 'dhcp' },
              ],
            }),
          ],
        },
      }),
    ).toEqual([]);
  });
  it('rejects an unknown member interface', () => {
    expect(
      run({
        interfaces: { eth0: { enabled: true } },
        routing: { wanGroups: [group({ members: [{ interface: 'ghost0', nextHop: 'dhcp' }] })] },
      }),
    ).toEqual([
      expect.objectContaining({
        pointer: '/routing/wanGroups/0/members/0/interface',
        message: expect.stringContaining("'ghost0' does not exist"),
      }),
    ]);
  });
  it('rejects duplicate group names', () => {
    const r = run({
      interfaces: { eth0: { enabled: true } },
      routing: { wanGroups: [group(), group()] },
    });
    expect(r.some((i) => i.message.includes("'wan' is defined twice"))).toBe(true);
  });
});
