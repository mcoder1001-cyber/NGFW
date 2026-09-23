import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { systemValidators } from './system.js';

const run = (name: string, doc: RootConfigInput) =>
  systemValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

describe('system.vrf-exists', () => {
  it('resolves the implicit default VRF and declared VRFs', () => {
    expect(run('system.vrf-exists', {})).toEqual([]);
    expect(
      run('system.vrf-exists', { vrfs: { mgmt: { id: 1 } }, system: { dns: { vrf: 'mgmt' } } }),
    ).toEqual([]);
  });
  it('reports a DNS VRF that does not exist', () => {
    expect(run('system.vrf-exists', { system: { dns: { vrf: 'oob' } } })).toEqual([
      { pointer: '/system/dns/vrf', message: "VRF 'oob' does not exist" },
    ]);
  });
});

describe('system.dns-server-unique', () => {
  it('accepts distinct servers and reports duplicates in any spelling', () => {
    expect(
      run('system.dns-server-unique', { system: { dns: { servers: ['10.0.0.1', '2001:db8::1'] } } }),
    ).toEqual([]);
    expect(
      run('system.dns-server-unique', {
        system: { dns: { servers: ['2001:db8::53', '10.0.0.1', '2001:DB8:0:0::53'] } },
      }),
    ).toEqual([
      {
        pointer: '/system/dns/servers/2',
        message: 'name server 2001:DB8:0:0::53 is listed more than once (first defined at /system/dns/servers/0)',
      },
    ]);
  });
});
