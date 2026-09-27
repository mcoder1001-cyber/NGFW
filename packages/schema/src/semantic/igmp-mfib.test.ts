import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { igmpMfibValidators } from './igmp-mfib.js';

const run = (doc: RootConfigInput) => igmpMfibValidators[0]!.validate(RootConfig.parse(doc));
const ifaces = { eth0: { enabled: true }, eth1: { enabled: true } };

describe('F-igmp-mfib semantic rules', () => {
  it('accepts a well-formed multicast config', () => {
    expect(
      run({
        interfaces: ifaces,
        routing: {
          multicast: {
            igmp: { interfaces: { eth0: { mode: 'host', joins: [{ group: '239.1.1.1', sources: ['10.0.0.5'] }] } } },
            mroutes: [{ group: '239.2.2.2', source: '10.0.0.9', paths: [{ interface: 'eth0', flags: 'accept' }, { interface: 'eth1', flags: 'forward' }] }],
            pim: { interfaces: ['eth1'], rp: [{ address: '10.0.0.1', groups: ['239.0.0.0/8'] }] },
          },
        },
      }),
    ).toEqual([]);
  });

  it('rejects a link-local (224.0.0.0/24) group', () => {
    const issues = run({
      interfaces: ifaces,
      routing: { multicast: { igmp: { interfaces: { eth0: { mode: 'host', joins: [{ group: '224.0.0.5', sources: ['10.0.0.5'] }] } } } } },
    });
    expect(issues.some((i) => i.pointer.endsWith('/group'))).toBe(true);
  });

  it('rejects static joins on a router-mode interface', () => {
    const issues = run({
      interfaces: ifaces,
      routing: { multicast: { igmp: { interfaces: { eth0: { mode: 'router', joins: [{ group: '239.1.1.1', sources: ['10.0.0.5'] }] } } } } },
    });
    expect(issues.some((i) => i.message.includes('host-mode'))).toBe(true);
  });

  it('requires a source for an mroute in an SSM range', () => {
    const issues = run({
      interfaces: ifaces,
      routing: { multicast: { mroutes: [{ group: '232.1.1.1', paths: [{ interface: 'eth0', flags: 'accept' }] }] } },
    });
    expect(issues.some((i) => i.pointer.endsWith('/source'))).toBe(true);
  });

  it('flags unknown interfaces', () => {
    const issues = run({
      interfaces: { eth0: { enabled: true } },
      routing: { multicast: { pim: { interfaces: ['ghost0'], rp: [] } } },
    });
    expect(issues.some((i) => i.message.includes("'ghost0' does not exist"))).toBe(true);
  });
});
