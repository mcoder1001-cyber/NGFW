import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { systemValidators } from './system.js';

const run = (name: string, doc: RootConfigInput) =>
  systemValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

describe('system.vrf-exists', () => {
  it('resolves the implicit default VRF and declared VRFs', () => {
    expect(run('system.vrf-exists', {})).toEqual([]);
    expect(run('system.vrf-exists', { vrfs: { mgmt: { id: 1 } }, system: { ntp: { vrf: 'mgmt' }, dns: { vrf: 'mgmt' } } })).toEqual([]);
  });
  it('reports NTP and DNS VRFs that do not exist', () => {
    expect(run('system.vrf-exists', { system: { ntp: { vrf: 'mgmt' }, dns: { vrf: 'oob' } } })).toEqual([
      { pointer: '/system/ntp/vrf', message: "VRF 'mgmt' does not exist" },
      { pointer: '/system/dns/vrf', message: "VRF 'oob' does not exist" },
    ]);
  });
});

describe('system.ntp-server-unique', () => {
  it('accepts distinct servers and reports duplicates case-insensitively', () => {
    expect(run('system.ntp-server-unique', { system: { ntp: { servers: [{ address: 'a.ntp' }, { address: 'b.ntp' }] } } })).toEqual([]);
    expect(
      run('system.ntp-server-unique', {
        system: { ntp: { servers: [{ address: 'pool.ntp.org' }, { address: '10.0.0.1' }, { address: 'POOL.ntp.org' }] } },
      }),
    ).toEqual([
      { pointer: '/system/ntp/servers/2/address', message: "NTP server 'POOL.ntp.org' is listed more than once" },
    ]);
  });
});
