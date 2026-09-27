import { InvalidCredentialsError } from 'ldapts';
import { describe, expect, it } from 'vitest';
import { ldapAuthenticate, userFilterFor, type LdapClientLike, type LdapServer } from './ldap.js';

/** A fake directory behind the ldapts client seam: svc bind + one user `w1alice` in two groups. */
function fakeDirectory(opts: { down?: boolean; entries?: number } = {}) {
  const calls: string[] = [];
  const factory = (): LdapClientLike => ({
    startTLS: async () => {
      calls.push('starttls');
    },
    bind: async (dn: string, pw?: string) => {
      calls.push(`bind ${dn}`);
      if (opts.down) throw new Error('connect ECONNREFUSED');
      if (dn === 'cn=svc,dc=x' && pw === 'VRX_TEST_PSK_FAAA_LDAP') return;
      if (dn === 'uid=w1alice,dc=x' && pw === 'alice-pw') return;
      throw new InvalidCredentialsError();
    },
    search: async (_base, o) => {
      calls.push(`search ${o.filter}`);
      const n = opts.entries ?? (o.filter === '(uid=w1alice)' ? 1 : 0);
      return {
        searchEntries: Array.from({ length: n }, () => ({
          dn: 'uid=w1alice,dc=x',
          memberOf: ['cn=netadmins,dc=x', 'cn=all,dc=x'],
        })),
      };
    },
    unbind: async () => {
      calls.push('unbind');
    },
  });
  return { calls, factory };
}

const srv: LdapServer = {
  url: 'ldaps://127.0.0.1:1',
  bindDn: 'cn=svc,dc=x',
  bindPassword: 'VRX_TEST_PSK_FAAA_LDAP',
  baseDn: 'dc=x',
  userFilter: '(uid=%s)',
  groupAttr: 'memberOf',
  startTls: false,
  timeoutMs: 1000,
};

describe('ldapAuthenticate', () => {
  it('accepts with groups after the service bind, the search and the user bind', async () => {
    const d = fakeDirectory();
    const r = await ldapAuthenticate(srv, 'w1alice', 'alice-pw', d.factory);
    expect(r).toEqual({
      status: 'accept',
      dn: 'uid=w1alice,dc=x',
      groups: ['cn=netadmins,dc=x', 'cn=all,dc=x'],
    });
    expect(d.calls).toEqual([
      'bind cn=svc,dc=x',
      'search (uid=w1alice)',
      'bind uid=w1alice,dc=x',
      'unbind',
    ]);
  });
  it('rejects a wrong password, an unknown user and an ambiguous filter', async () => {
    expect((await ldapAuthenticate(srv, 'w1alice', 'nope', fakeDirectory().factory)).status).toBe(
      'reject',
    );
    expect((await ldapAuthenticate(srv, 'w1bob', 'x', fakeDirectory().factory)).status).toBe(
      'reject',
    );
    const amb = await ldapAuthenticate(
      srv,
      'w1alice',
      'alice-pw',
      fakeDirectory({ entries: 2 }).factory,
    );
    expect(amb).toEqual({ status: 'reject', message: 'user filter is ambiguous' });
  });
  it('refuses an empty password without touching the network (unauthenticated bind)', async () => {
    const d = fakeDirectory();
    expect((await ldapAuthenticate(srv, 'w1alice', '', d.factory)).status).toBe('reject');
    expect(d.calls).toEqual([]);
  });
  it('refuses a clear-text bind and uses StartTLS on ldap://', async () => {
    const d = fakeDirectory();
    const clear = await ldapAuthenticate(
      { ...srv, url: 'ldap://h' },
      'w1alice',
      'alice-pw',
      d.factory,
    );
    expect(clear.status).toBe('unreachable');
    expect(d.calls).toEqual([]);
    const st = await ldapAuthenticate(
      { ...srv, url: 'ldap://h', startTls: true },
      'w1alice',
      'alice-pw',
      d.factory,
    );
    expect(st.status).toBe('accept');
    expect(d.calls[0]).toBe('starttls');
  });
  it('reports a down server or a refused service bind as unreachable, without the bind password', async () => {
    const down = await ldapAuthenticate(
      srv,
      'w1alice',
      'alice-pw',
      fakeDirectory({ down: true }).factory,
    );
    expect(down.status).toBe('unreachable');
    const bad = await ldapAuthenticate(
      { ...srv, bindPassword: 'wrong' },
      'w1alice',
      'alice-pw',
      fakeDirectory().factory,
    );
    expect(bad).toEqual({ status: 'unreachable', error: 'ldap: service bind refused' });
    expect(JSON.stringify([down, bad])).not.toContain('VRX_TEST_PSK_FAAA_LDAP');
  });
  it('escapes the login name into the filter (no filter injection)', () => {
    expect(userFilterFor('(uid=%s)', 'a*)(uid=*')).toBe('(uid=a\\2a\\29\\28uid=\\2a)');
    expect(userFilterFor('(|(uid=%s)(mail=%s))', 'x')).toBe('(|(uid=x)(mail=x))');
  });
});
