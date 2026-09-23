import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { managementValidators } from './management.js';

const run = (name: string, doc: RootConfigInput) =>
  managementValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

const HASH = '$vrx-test$VRX_TEST_HASH_x';
const KEY = 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFakeKeyForFixturesOnly0000000000000000000000';
const NO_ADMIN = {
  pointer: '/management/users',
  message:
    'at least one enabled admin user with a password or an SSH key is required (removing, disabling or demoting the last one would lock everybody out)',
};

describe('management.admin-exists', () => {
  it('is satisfied by an enabled admin with a password or an SSH key', () => {
    expect(
      run('management.admin-exists', {
        management: { users: [{ username: 'a', role: 'admin', passwordHash: HASH }] },
      }),
    ).toEqual([]);
    expect(
      run('management.admin-exists', {
        management: { users: [{ username: 'a', role: 'admin', sshKeys: [KEY] }] },
      }),
    ).toEqual([]);
  });
  it('is silent on an empty user list — the API seeds the first admin (D-048)', () => {
    expect(run('management.admin-exists', {})).toEqual([]);
    expect(run('management.admin-exists', { management: { users: [] } })).toEqual([]);
  });
  it('is not satisfied by non-admins, disabled admins or admins that cannot log in', () => {
    expect(
      run('management.admin-exists', {
        management: { users: [{ username: 'o', role: 'operator', passwordHash: HASH }] },
      }),
    ).toEqual([NO_ADMIN]);
    expect(
      run('management.admin-exists', {
        management: {
          users: [{ username: 'a', role: 'admin', passwordHash: HASH, disabled: true }],
        },
      }),
    ).toEqual([NO_ADMIN]);
    expect(
      run('management.admin-exists', { management: { users: [{ username: 'a', role: 'admin' }] } }),
    ).toEqual([NO_ADMIN]);
  });
});

describe('management.username-unique', () => {
  it('reports repeated usernames at the later entries', () => {
    expect(
      run('management.username-unique', {
        management: {
          users: [
            { username: 'a', role: 'admin' },
            { username: 'b', role: 'admin' },
          ],
        },
      }),
    ).toEqual([]);
    expect(
      run('management.username-unique', {
        management: {
          users: [
            { username: 'a', role: 'admin' },
            { username: 'a', role: 'readonly' },
          ],
        },
      }),
    ).toEqual([
      { pointer: '/management/users/1/username', message: "username 'a' is already taken" },
    ]);
  });
});

describe('management.vrf-exists', () => {
  it('resolves default and declared VRFs for RADIUS, TACACS+ and syslog', () => {
    expect(
      run('management.vrf-exists', {
        vrfs: { mgmt: { id: 1 } },
        management: {
          aaa: {
            radius: { servers: [{ address: '10.0.0.1', secretRef: 'psk/r', vrf: 'mgmt' }] },
            tacacs: { servers: [{ address: '10.0.0.2', secretRef: 'psk/t' }] },
          },
          syslog: [{ address: '10.0.0.3', vrf: 'mgmt' }],
        },
      }),
    ).toEqual([]);
  });
  it('reports every unknown VRF with its pointer', () => {
    expect(
      run('management.vrf-exists', {
        management: {
          aaa: {
            radius: { servers: [{ address: '10.0.0.1', secretRef: 'psk/r', vrf: 'x' }] },
            tacacs: { servers: [{ address: '10.0.0.2', secretRef: 'psk/t', vrf: 'y' }] },
          },
          syslog: [{ address: '10.0.0.3', vrf: 'z' }],
        },
      }),
    ).toEqual([
      { pointer: '/management/aaa/radius/servers/0/vrf', message: "VRF 'x' does not exist" },
      { pointer: '/management/aaa/tacacs/servers/0/vrf', message: "VRF 'y' does not exist" },
      { pointer: '/management/syslog/0/vrf', message: "VRF 'z' does not exist" },
    ]);
  });
});

describe('management.server-unique (review M2)', () => {
  it('accepts distinct servers (a different port or transport is distinct)', () => {
    expect(
      run('management.server-unique', {
        management: {
          aaa: {
            radius: {
              servers: [
                { address: '10.0.0.1', secretRef: 'psk/r1' },
                { address: '10.0.0.1', authPort: 11812, secretRef: 'psk/r2' },
              ],
            },
            tacacs: { servers: [{ address: 'tac.lab', secretRef: 'psk/t1' }] },
          },
          syslog: [
            { address: '10.0.0.20' },
            { address: '10.0.0.20', protocol: 'tcp' },
            { address: '10.0.0.20', port: 1514 },
          ],
        },
      }),
    ).toEqual([]);
  });
  it('reports repeated RADIUS, TACACS+ and syslog servers in any spelling', () => {
    expect(
      run('management.server-unique', {
        management: {
          aaa: {
            radius: {
              servers: [
                { address: '2001:db8::1', secretRef: 'psk/r1' },
                { address: '2001:DB8:0::1', secretRef: 'psk/r2' },
              ],
            },
            tacacs: {
              servers: [
                { address: 'tac.lab', secretRef: 'psk/t1' },
                { address: 'TAC.lab', secretRef: 'psk/t2' },
              ],
            },
          },
          syslog: [{ address: 'log.lab' }, { address: 'log.lab' }],
        },
      }),
    ).toEqual([
      {
        pointer: '/management/aaa/radius/servers/1/address',
        message:
          'RADIUS server 2001:DB8:0::1 port 1812 is listed more than once (first defined at /management/aaa/radius/servers/0/address)',
      },
      {
        pointer: '/management/aaa/tacacs/servers/1/address',
        message:
          'TACACS+ server TAC.lab port 49 is listed more than once (first defined at /management/aaa/tacacs/servers/0/address)',
      },
      {
        pointer: '/management/syslog/1/address',
        message:
          'syslog collector udp://log.lab:514 is listed more than once (first defined at /management/syslog/0/address)',
      },
    ]);
  });
});
