import { expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { pppoeValidators } from './pppoe.js';

const config = () =>
  RootConfig.parse({
    interfaces: {
      eth0: {
        enabled: true,
        subinterfaces: {
          '100': { enabled: true, vlanId: 100 },
          '200': { enabled: true, vlanId: 200, innerVlanId: 300, ipv4: ['192.0.2.1/24'] },
        },
      },
      pppwan: {
        enabled: true,
        pppoe: { parent: 'eth0.100', username: 'u', passwordRef: 'password/isp' },
      },
    },
  });
it('accepts dedicated VLAN leaf while preserving unrelated addressed sibling', () => {
  expect(pppoeValidators[0]!.validate(config())).toEqual([]);
});
it('rejects addressed selected child, disabled root, and whole-port with children', () => {
  const a = config();
  a.interfaces.pppwan!.pppoe!.parent = 'eth0.200';
  expect(pppoeValidators[0]!.validate(a).length).toBeGreaterThan(0);
  const b = config();
  b.interfaces.eth0!.enabled = false;
  expect(pppoeValidators[0]!.validate(b).length).toBeGreaterThan(0);
  const c = config();
  c.interfaces.pppwan!.pppoe!.parent = 'eth0';
  expect(pppoeValidators[0]!.validate(c).length).toBeGreaterThan(0);
});
it('accepts explicit QinQ and rejects noncanonical or missing child', () => {
  const a = config();
  a.interfaces.eth0!.subinterfaces['100']!.innerVlanId = 20;
  expect(pppoeValidators[0]!.validate(a)).toEqual([]);
  for (const parent of ['eth0.0100', 'eth0.999']) {
    a.interfaces.pppwan!.pppoe!.parent = parent;
    expect(pppoeValidators[0]!.validate(a).length).toBeGreaterThan(0);
  }
});
