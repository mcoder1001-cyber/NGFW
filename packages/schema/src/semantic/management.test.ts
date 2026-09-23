import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { managementValidators } from './management.js';

const run = (name: string, doc: RootConfigInput) =>
  managementValidators.find((v) => v.name === name)!.validate(RootConfig.parse(doc));

const HASH = '$vrx-test$VRX_TEST_HASH_x';
const KEY = 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFakeKeyForFixturesOnly0000000000000000000000';
const NO_ADMIN = {
  pointer: '/management/users',
  message: 'at least one enabled admin user with a password or an SSH key is required',
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
  it('is not satisfied by no users, non-admins, disabled admins or admins that cannot log in', () => {
    expect(run('management.admin-exists', {})).toEqual([NO_ADMIN]);
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
            radius: { servers: [{ address: '10.0.0.1', secretRef: 'aaa/r', vrf: 'mgmt' }] },
            tacacs: { servers: [{ address: '10.0.0.2', secretRef: 'aaa/t' }] },
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
            radius: { servers: [{ address: '10.0.0.1', secretRef: 'aaa/r', vrf: 'x' }] },
            tacacs: { servers: [{ address: '10.0.0.2', secretRef: 'aaa/t', vrf: 'y' }] },
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
