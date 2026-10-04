import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { validateSemantics } from './index.js';
const NET = '49.0001.1921.6800.1001.00';
describe('IS-IS and RIPng semantic boundaries', () => {
  it('points to incompatible circuits and disabled families', () => {
    const config = RootConfig.parse({
      routing: {
        isis: {
          net: NET,
          level: 'level-1',
          interfaces: { loop0: { circuitType: 'level-2', ipv4: false, ipv6: false } },
        },
      },
    });
    const issues = validateSemantics(config);
    expect(issues.some((i) => i.pointer === '/routing/isis/interfaces/loop0/circuitType')).toBe(
      true,
    );
    expect(issues.some((i) => i.pointer === '/routing/isis/interfaces/loop0/ipv4')).toBe(true);
  });
  it('mirrors interface/VRF/map existence and detects canonical RIPng duplicate networks', () => {
    const config = RootConfig.parse({
      routing: {
        ripng: {
          vrf: 'missing',
          interfaces: { loop0: {} },
          redistribute: { static: { routeMap: 'missing' } },
          networks: ['2001:db8::/64', '2001:DB8::/64'],
        },
      },
    });
    const pointers = validateSemantics(config).map((i) => i.pointer);
    expect(pointers).toContain('/routing/ripng/vrf');
    expect(pointers).toContain('/routing/ripng/interfaces/loop0');
    expect(pointers).toContain('/routing/ripng/redistribute/static/routeMap');
    expect(pointers).toContain('/routing/ripng/networks/1');
  });
});
