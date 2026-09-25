import { describe, expect, it } from 'vitest';
import { TEST_HASH } from '../testing/fixtures.js';
import {
  markSecretChanges,
  privilegedChanges,
  replaceUserHash,
  secretChanges,
  stagedHashesOnly,
  withoutPasswordHashes,
} from './documents.js';

describe('privilegedChanges (review M1)', () => {
  it('flags users, AAA and every changed secret reference', () => {
    const a = {
      management: { users: [{ username: 'admin', role: 'admin' }], aaa: { order: ['local'] } },
      vpn: { ipsec: { tunnels: { t1: { presharedKeyRef: 'psk/one' } } } },
      routing: { bgp: { neighbors: { '10.0.0.2': { passwordRef: 'password/bgp' } } } },
    };
    expect(privilegedChanges(a, structuredClone(a))).toEqual([]);
    const b = structuredClone(a);
    b.vpn.ipsec.tunnels.t1.presharedKeyRef = 'psk/two';
    expect(privilegedChanges(a, b)).toEqual(['/vpn/ipsec/tunnels/t1/presharedKeyRef']);
    const c = structuredClone(a) as typeof a & {
      routing: { bgp: { neighbors: Record<string, object> } };
    };
    delete (c.routing.bgp.neighbors as Record<string, unknown>)['10.0.0.2'];
    expect(privilegedChanges(a, c)).toEqual(['/routing/bgp/neighbors/10.0.0.2/passwordRef']);
    const d = structuredClone(a);
    (d.management.users[0] as Record<string, unknown>)['passwordHash'] = TEST_HASH;
    d.management.aaa.order = ['local', 'radius'];
    expect(privilegedChanges(a, d)).toEqual(['/management/users', '/management/aaa']);
  });

  it('non-secret edits are not privileged', () => {
    const a = { system: { hostname: 'a' }, interfaces: { loop1: { ipv4: ['10.0.0.1/24'] } } };
    expect(privilegedChanges(a, { ...a, system: { hostname: 'b' } })).toEqual([]);
  });
});

describe('replaceUserHash (TD-2 #1)', () => {
  const doc = {
    management: {
      users: [
        { username: 'a', role: 'admin', passwordHash: '$vrx-test$old-a' },
        { username: 'b', role: 'operator' },
      ],
    },
  };
  it('replaces the hash a stored document carries for the user, leaves the input untouched', () => {
    const out = replaceUserHash(doc, 'a', '$vrx-test$new');
    expect(out).toMatchObject({
      management: { users: [{ username: 'a', passwordHash: '$vrx-test$new' }, { username: 'b' }] },
    });
    expect(doc.management.users[0]!.passwordHash).toBe('$vrx-test$old-a');
  });
  it('null when there is nothing to replace (no hash, unknown user, no users)', () => {
    expect(replaceUserHash(doc, 'b', 'h')).toBeNull();
    expect(replaceUserHash(doc, 'zz', 'h')).toBeNull();
    expect(replaceUserHash({}, 'a', 'h')).toBeNull();
  });
});

describe('secretChanges / markSecretChanges (TD-2 #6)', () => {
  const users = (a: string | undefined, b?: string) => ({
    management: {
      users: [
        { username: 'a', role: 'admin', ...(a ? { passwordHash: a } : {}) },
        ...(b !== undefined ? [{ username: 'b', role: 'operator', passwordHash: b }] : []),
      ],
    },
  });
  it('replace / add / remove by username, no values', () => {
    expect(secretChanges(users('h1'), users('h2'))).toEqual([
      { op: 'replace', pointer: '/management/users/0/passwordHash', redacted: true },
    ]);
    expect(secretChanges(users('h1'), users('h1', 'x'))).toEqual([
      { op: 'add', pointer: '/management/users/1/passwordHash', redacted: true },
    ]);
    expect(secretChanges(users('h1', 'x'), users('h1'))).toEqual([
      { op: 'remove', pointer: '/management/users/1/passwordHash', redacted: true },
    ]);
    expect(secretChanges(users('h1', 'x'), users('h1', 'x'))).toEqual([]);
  });
  it('marks the audit subtrees without values', () => {
    const m = markSecretChanges(
      '/management/users/0',
      { username: 'a', role: 'admin' },
      { username: 'a', role: 'admin' },
      [{ op: 'replace', pointer: '/management/users/0/passwordHash', redacted: true }],
    );
    expect(m).toEqual({
      before: { username: 'a', role: 'admin', passwordHash: '<redacted>' },
      after: { username: 'a', role: 'admin', passwordHash: '<redacted:changed>' },
    });
  });
});

describe('stagedHashesOnly / withoutPasswordHashes (D-097, review H1)', () => {
  const OTHER = `${TEST_HASH}x`;
  it('keeps only hashes the raw document staged; hydrated ones are dropped', () => {
    const hydrated = {
      management: {
        users: [
          { username: 'a', role: 'admin', passwordHash: TEST_HASH },
          { username: 'b', role: 'operator', passwordHash: TEST_HASH },
        ],
      },
    };
    const raw = {
      management: {
        users: [
          { username: 'a', role: 'admin' },
          { username: 'b', role: 'operator', passwordHash: OTHER },
        ],
      },
    };
    expect(stagedHashesOnly(hydrated, raw)).toEqual({
      management: {
        users: [
          { username: 'a', role: 'admin' },
          { username: 'b', role: 'operator', passwordHash: OTHER },
        ],
      },
    });
    // input untouched; a document without users is returned as is
    expect(hydrated.management.users[0]).toHaveProperty('passwordHash', TEST_HASH);
    expect(stagedHashesOnly({ system: {} }, raw)).toEqual({ system: {} });
  });

  it('import: strips every hash and lists the pointers', () => {
    const doc = {
      management: {
        users: [{ username: 'a', passwordHash: TEST_HASH }, { username: 'b' }],
      },
    };
    const r = withoutPasswordHashes(doc);
    expect(r.removed).toEqual(['/management/users/0/passwordHash']);
    expect(r.doc).toEqual({ management: { users: [{ username: 'a' }, { username: 'b' }] } });
    expect(doc.management.users[0]).toHaveProperty('passwordHash');
  });
});
