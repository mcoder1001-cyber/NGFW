import { MergePatchError } from '@ngfw/schema';
import { beforeEach, describe, expect, it } from 'vitest';
import { ProblemError } from '../common/problem.js';
import { ADMIN, OPERATOR, TEST_HASH, testEnv } from '../testing/fixtures.js';
import { MemoryConfigRepo } from '../testing/memory-repo.js';
import { DatastoreService } from './datastore.service.js';

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
