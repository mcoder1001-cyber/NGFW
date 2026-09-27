import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { aaaValidators } from './aaa.js';

const run = (doc: RootConfigInput) => aaaValidators[0]!.validate(RootConfig.parse(doc));
const ldapSrv = (over: Record<string, unknown> = {}) => ({
  url: 'ldaps://ldap.example.net',
  bindDn: 'cn=svc,dc=x',
  bindPasswordRef: 'password/bind',
  baseDn: 'dc=x',
  ...over,
});

describe('F-aaa semantic rules', () => {
  it('accepts ldaps and ldap+startTls', () => {
    expect(
      run({ management: { aaa: { order: ['local', 'ldap'], ldap: { servers: [ldapSrv()] } } } }),
    ).toEqual([]);
    expect(
      run({
        management: {
          aaa: {
            order: ['local', 'ldap'],
            ldap: { servers: [ldapSrv({ url: 'ldap://ldap.example.net', startTls: true })] },
          },
        },
      }),
    ).toEqual([]);
  });
  it('refuses a clear-text ldap bind', () => {
    expect(
      run({
        management: {
          aaa: {
            order: ['local', 'ldap'],
            ldap: { servers: [ldapSrv({ url: 'ldap://ldap.example.net' })] },
          },
        },
      }),
    ).toEqual([
      expect.objectContaining({
        pointer: '/management/aaa/ldap/servers/0/url',
        message: expect.stringContaining('clear-text'),
      }),
    ]);
  });
  it('refuses a duplicate role-map group', () => {
    const r = run({
      management: {
        aaa: {
          roleMap: [
            { group: 'admins', role: 'admin' },
            { group: 'admins', role: 'operator' },
          ],
        },
      },
    });
    expect(r.some((i) => i.message.includes("'admins' is mapped twice"))).toBe(true);
  });
});
