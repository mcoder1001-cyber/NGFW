import { MergePatchError } from '@ngfw/schema';
import { beforeEach, describe, expect, it } from 'vitest';
import { ProblemError } from '../common/problem.js';
import type { Principal } from '../common/principal.js';
import { ADMIN, OPERATOR, TEST_HASH, testEnv } from '../testing/fixtures.js';
import { MemoryConfigRepo } from '../testing/memory-repo.js';
import { DatastoreService } from './datastore.service.js';
import { parseDocument } from './documents.js';

function problemOf(e: unknown): { status: number; body: Record<string, unknown> } {
  expect(e).toBeInstanceOf(ProblemError);
  const p = e as ProblemError;
  return { status: p.getStatus(), body: p.body() };
}

async function rejects(p: Promise<unknown>) {
  try {
    await p;
  } catch (e) {
    return problemOf(e);
  }
  throw new Error('expected a rejection');
}

describe('DatastoreService', () => {
  let repo: MemoryConfigRepo;
  let ds: DatastoreService;

  beforeEach(() => {
    repo = new MemoryConfigRepo();
    repo.addUser('admin', 'admin', TEST_HASH);
    repo.addUser('op', 'operator');
    ds = new DatastoreService(repo, testEnv());
  });

  it('running before the first commit is the empty document with defaults (D-048)', async () => {
    const r = await ds.getRunning();
    expect(r.revision).toBeNull();
    expect(r.doc).toHaveProperty('interfaces', {});
    expect(await ds.getCandidate()).toEqual(r.doc);
    expect((await ds.diff()).changes).toEqual([]);
  });

  it('patch at a pointer takes the lock, fills defaults and shows up in the diff', async () => {
    const r = await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
    expect(r.before).toBeUndefined();
    expect(r.after).toMatchObject({ ipv4: ['10.1.0.1/24'], enabled: false, vrf: 'default' });
    const lock = await ds.lock();
    expect(lock).toMatchObject({ locked: true, owner: 'admin', ownerId: 1 });
    const d = await ds.diff();
    expect(d.changes).toEqual([
      {
        op: 'add',
        pointer: '/interfaces/loop1',
        to: expect.objectContaining({ ipv4: ['10.1.0.1/24'] }),
      },
    ]);
  });

  it('a second writer gets 409 with the lock owner', async () => {
    await ds.patchCandidate(ADMIN, '/system', { hostname: 'a' });
    const p = await rejects(ds.patchCandidate(OPERATOR, '/system', { hostname: 'b' }));
    expect(p.status).toBe(409);
    expect(p.body['lock']).toMatchObject({ owner: 'admin', locked: true });
    expect(String(p.body['detail'])).toContain("locked by 'admin'");
    expect((await rejects(ds.discard(OPERATOR))).status).toBe(409);
  });

  it('a stale lock can be taken over; the candidate is kept', async () => {
    await ds.patchCandidate(ADMIN, '/system', { hostname: 'a' });
    const later = new Date(Date.now() + 61_000);
    ds.now = () => later;
    await ds.patchCandidate(OPERATOR, '/system', { timezone: 'UTC' });
    expect(await ds.lock()).toMatchObject({ owner: 'op' });
    expect(await ds.getCandidate()).toMatchObject({ system: { hostname: 'a' } });
  });

  it('schema errors are 400 with pointers and leave the candidate untouched', async () => {
    const p = await rejects(ds.patchCandidate(ADMIN, '/interfaces/loop1', { mtu: 5, bogus: 1 }));
    expect(p.status).toBe(400);
    const pointers = (p.body['errors'] as { pointer: string }[]).map((e) => e.pointer);
    expect(pointers).toEqual(
      expect.arrayContaining(['/interfaces/loop1/mtu', '/interfaces/loop1/bogus']),
    );
    expect((await ds.lock()).locked).toBe(false);
  });

  it('prototype keys are MergePatchError (→ 400, D-070)', async () => {
    await expect(
      ds.patchCandidate(ADMIN, '/system', JSON.parse('{"__proto__": {"x": 1}}')),
    ).rejects.toBeInstanceOf(MergePatchError);
  });

  it('PUT replaces a node, DELETE removes it, deleting nothing is 404', async () => {
    await ds.putCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'], mtu: 1500 });
    await ds.putCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.2/24'] });
    expect(await ds.getCandidate()).toMatchObject({
      interfaces: { loop1: { ipv4: ['10.1.0.2/24'] } },
    });
    expect(
      ((await ds.getCandidate()) as { interfaces: { loop1: object } }).interfaces.loop1,
    ).not.toHaveProperty('mtu');
    await ds.deleteCandidate(ADMIN, '/interfaces/loop1');
    expect((await ds.diff()).changes).toEqual([]);
    expect((await rejects(ds.deleteCandidate(ADMIN, '/interfaces/loop9'))).status).toBe(404);
  });

  it('discard drops the candidate and releases the lock', async () => {
    await ds.patchCandidate(ADMIN, '/system', { hostname: 'a' });
    expect(await ds.discard(ADMIN)).toEqual({ discarded: true });
    expect((await ds.lock()).locked).toBe(false);
    expect((await ds.diff()).changes).toEqual([]);
  });

  it('operators may not change users or AAA (403), admins may', async () => {
    const users = [{ username: 'bob', role: 'operator' }];
    const p = await rejects(ds.putCandidate(OPERATOR, '/management/users', users));
    expect(p.status).toBe(403);
    expect(p.body['errors']).toEqual([{ pointer: '/management/users', message: 'admin only' }]);
    expect(
      (
        await rejects(
          ds.patchCandidate(OPERATOR, '/management/aaa', {
            tacacs: { servers: [{ address: '10.0.0.9', secretRef: 'psk/tac' }] },
          }),
        )
      ).status,
    ).toBe(403);
    await ds.putCandidate(ADMIN, '/management/users', users);
    // the operator may still edit everything else once the lock is free
    await ds.discard(ADMIN);
    await ds.patchCandidate(OPERATOR, '/system', { hostname: 'x' });
  });

  it('secret leaves never come out and survive a GET → PUT round-trip (D-046)', async () => {
    await ds.putCandidate(ADMIN, '/management/users', [
      { username: 'alice', role: 'admin', passwordHash: TEST_HASH },
    ]);
    const seen = (await ds.getCandidate()) as { management: { users: Record<string, unknown>[] } };
    expect(seen.management.users[0]).not.toHaveProperty('passwordHash');
    expect(JSON.stringify(await ds.diff())).not.toContain('VRX_TEST_HASH');
    // PUT the redacted document back (whole) plus an extra user before alice: matched by username, not index
    seen.management.users.unshift({ username: 'carol', role: 'readonly' });
    const r = await ds.importCandidate(ADMIN, seen);
    expect(JSON.stringify(r)).not.toContain('VRX_TEST_HASH');
    const stored = repo.state.candidate.payload as {
      management: { users: Record<string, unknown>[] };
    };
    expect(stored.management.users.find((u) => u['username'] === 'alice')).toHaveProperty(
      'passwordHash',
      TEST_HASH,
    );
    expect(stored.management.users.find((u) => u['username'] === 'carol')).not.toHaveProperty(
      'passwordHash',
    );
  });

  it('import replaces the whole candidate but rejects schema-invalid documents', async () => {
    await ds.importCandidate(ADMIN, { system: { hostname: 'imported' } });
    expect(await ds.getCandidate()).toMatchObject({
      system: { hostname: 'imported' },
      interfaces: {},
    });
    expect((await rejects(ds.importCandidate(ADMIN, { nope: {} }))).status).toBe(400);
  });
});

describe('DatastoreService — candidate lock per API key (TD-2 #5, D-093)', () => {
  let repo: MemoryConfigRepo;
  let ds: DatastoreService;
  const KEY_A: Principal = {
    ...ADMIN,
    via: 'apikey',
    keyId: 'aaaaaaaa-0000-4000-8000-00000000000a',
    keyName: 'ci-a',
  };
  const KEY_B: Principal = {
    ...ADMIN,
    via: 'apikey',
    keyId: 'bbbbbbbb-0000-4000-8000-00000000000b',
    keyName: 'ci-b',
  };
  const ADMIN_TAB2: Principal = { ...ADMIN, sid: 'second-login-session' };

  beforeEach(() => {
    repo = new MemoryConfigRepo();
    repo.addUser('admin', 'admin', TEST_HASH);
    ds = new DatastoreService(repo, testEnv());
  });

  it('two keys of one user: the second gets 409 naming the first key; it cannot edit, discard or take it', async () => {
    await ds.patchCandidate(KEY_A, '/system', { hostname: 'from-a' });
    const lock = await ds.lock();
    expect(lock).toMatchObject({ locked: true, ownerId: 1, ownerKeyId: KEY_A.keyId });
    const p = await rejects(ds.patchCandidate(KEY_B, '/system', { hostname: 'from-b' }));
    expect(p.status).toBe(409);
    expect(p.body['detail']).toContain('API key');
    expect(p.body['lock']).toMatchObject({ ownerKeyId: KEY_A.keyId });
    expect((await rejects(ds.discard(KEY_B))).status).toBe(409);
    // the user's interactive session is a different owner too
    expect((await rejects(ds.patchCandidate(ADMIN, '/system', { hostname: 'ui' }))).status).toBe(
      409,
    );
    // A's work is intact and A continues
    await ds.patchCandidate(KEY_A, '/system', { timezone: 'UTC' });
    expect(await ds.getCandidate()).toMatchObject({
      system: { hostname: 'from-a', timezone: 'UTC' },
    });
    expect(await ds.discard(KEY_A)).toEqual({ discarded: true });
    await ds.patchCandidate(KEY_B, '/system', { hostname: 'from-b' });
    expect(await ds.lock()).toMatchObject({ ownerKeyId: KEY_B.keyId });
  });

  it('interactive sessions of one user still share the candidate (unchanged)', async () => {
    await ds.patchCandidate(ADMIN, '/system', { hostname: 'tab1' });
    await ds.patchCandidate(ADMIN_TAB2, '/system', { timezone: 'UTC' });
    expect(await ds.lock()).toMatchObject({ ownerId: 1, ownerKeyId: null });
    expect((await rejects(ds.patchCandidate(KEY_A, '/system', { hostname: 'k' }))).status).toBe(
      409,
    );
  });

  it('a stale key lock can be taken over like any other', async () => {
    await ds.patchCandidate(KEY_A, '/system', { hostname: 'from-a' });
    ds.now = () => new Date(Date.now() + 61_000);
    repo.state.candidate.updatedAt = new Date(Date.now() - 120_000);
    await ds.patchCandidate(KEY_B, '/system', { timezone: 'UTC' });
    expect(await ds.lock()).toMatchObject({ ownerKeyId: KEY_B.keyId });
  });
});

describe('DatastoreService — control characters in documents (TD-2 #4, D-049)', () => {
  let ds: DatastoreService;
  beforeEach(() => {
    const repo = new MemoryConfigRepo();
    repo.addUser('admin', 'admin', TEST_HASH);
    ds = new DatastoreService(repo, testEnv());
  });

  it.each([
    [
      'ESC in a value',
      { interfaces: { loop1: { description: 'a\u001b]0;x\u0007' } } },
      '/interfaces/loop1/description',
    ],
    [
      'RLO in a value',
      { interfaces: { loop1: { description: 'a\u202eb' } } },
      '/interfaces/loop1/description',
    ],
    ['C1 CSI in a member name', { interfaces: { 'lo\u009b1': {} } }, '/interfaces/lo\\u{9b}1'],
  ])('%s → 400 with the pointer, nothing stored', async (_n, patch, pointer) => {
    const p = await rejects(ds.patchCandidate(ADMIN, '', patch));
    expect(p.status).toBe(400);
    expect(p.body['errors']).toEqual([expect.objectContaining({ pointer, rule: 'api.safe-text' })]);
    expect((await ds.lock()).locked).toBe(false);
  });

  it('LF and TAB stay allowed in documents (multi-line banners)', async () => {
    await ds.patchCandidate(ADMIN, '/system/banner', { motd: 'line 1\n\tline 2' });
    expect(await ds.getCandidate()).toMatchObject({
      system: { banner: { motd: 'line 1\n\tline 2' } },
    });
  });
});

describe('DatastoreService — secret leaves in the diff (TD-2 #6, P07b review H1)', () => {
  let repo: MemoryConfigRepo;
  let ds: DatastoreService;
  const ALICE_OLD = '$vrx-test$ALICE_OLD';
  const ALICE_NEW = '$vrx-test$ALICE_NEW';

  beforeEach(() => {
    repo = new MemoryConfigRepo();
    repo.addUser('admin', 'admin', TEST_HASH);
    repo.addUser('alice', 'operator', ALICE_OLD, 'config');
    ds = new DatastoreService(repo, testEnv());
    // running (redacted, as stored) with both users
    const payload = parseDocument({
      management: {
        users: [
          { username: 'admin', role: 'admin' },
          { username: 'alice', role: 'operator' },
        ],
      },
    });
    repo.state.revisions.push({
      id: 1,
      createdAt: new Date(),
      authorId: 1,
      author: 'admin',
      comment: 'users',
      parentId: null,
      hash: 'h',
      txnId: null,
      kind: 'commit',
      payload,
      secretChanges: [],
    });
  });

  it('a hash-only edit is ONE redacted change, without any value', async () => {
    const r = await ds.patchCandidate(ADMIN, '/management/users/1', { passwordHash: ALICE_NEW });
    expect(r.secretChanges).toEqual([
      { op: 'replace', pointer: '/management/users/1/passwordHash', redacted: true },
    ]);
    const d = await ds.diff();
    expect(d.changes).toEqual([
      { op: 'replace', pointer: '/management/users/1/passwordHash', redacted: true },
    ]);
    expect(JSON.stringify(d)).not.toContain('ALICE');
  });

  it('re-sending the stored hash (or omitting it) is no change; reordering users is no secret change', async () => {
    await ds.patchCandidate(ADMIN, '/management/users/1', { passwordHash: ALICE_OLD });
    expect((await ds.diff()).changes).toEqual([]);
    await ds.putCandidate(ADMIN, '/management/users', [
      { username: 'alice', role: 'operator' },
      { username: 'admin', role: 'admin' },
    ]);
    const changes = (await ds.diff()).changes;
    expect(changes).toHaveLength(1);
    expect(changes[0]).toMatchObject({ op: 'replace', pointer: '/management/users' });
    expect(changes[0]).not.toHaveProperty('redacted');
  });

  it('a new user with a hash: the redacted array change plus a redacted add', async () => {
    await ds.putCandidate(ADMIN, '/management/users', [
      { username: 'admin', role: 'admin' },
      { username: 'alice', role: 'operator' },
      { username: 'bob', role: 'readonly', passwordHash: '$vrx-test$BOB' },
    ]);
    const changes = (await ds.diff()).changes;
    expect(changes.map((c) => [c.op, c.pointer, 'redacted' in c])).toEqual([
      ['replace', '/management/users', false],
      ['add', '/management/users/2/passwordHash', true],
    ]);
    expect(JSON.stringify(changes)).not.toContain('BOB');
  });
});
