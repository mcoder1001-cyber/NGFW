import 'reflect-metadata';
import { RootConfig, SetupInputSchema, buildSetup } from '@ngfw/schema';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { AuthService } from '../../auth/auth.service.js';
import { verifyPassword } from '../../auth/password.js';
import type { NgfwRequest } from '../../common/principal.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { ADMIN, TEST_HASH, testEnv } from '../../testing/fixtures.js';
import { MemoryConfigRepo } from '../../testing/memory-repo.js';
import { ValidationService } from '../../commit/validation.service.js';
import { notAppliedChanges } from '../../commit/commit.service.js';
import { ProblemError, problems } from '../../common/problem.js';
import { SetupController } from './controller.js';
const input = SetupInputSchema.parse({
  language: 'en',
  timezone: 'UTC',
  ntp: ['pool.ntp.org'],
  hostname: 'box',
  wan: 'wan0',
  wanMode: 'dhcp',
  lan: 'lan0',
  lanAddress: '192.168.40.1/24',
  dhcp: true,
});
const request = () =>
  ({ principal: ADMIN, raw: { socket: {} }, ip: '127.0.0.1', headers: {} }) as NgfwRequest;
const password = 'NGFW_TEST_PSK_setup_new';
const current = 'NGFW_TEST_PSK_setup_old';
describe('setup staging and security', () => {
  let repo: MemoryConfigRepo;
  let ds: DatastoreService;
  let c: SetupController;
  const checked = vi.fn();
  beforeEach(async () => {
    checked.mockReset();
    checked.mockResolvedValue(undefined);
    repo = new MemoryConfigRepo();
    repo.addUser('admin', 'admin', TEST_HASH);
    ds = new DatastoreService(repo, testEnv());
    await repo.tx((tx) =>
      tx.insertRevision({
        authorId: 1,
        comment: 'factory',
        parentId: null,
        hash: 'factory',
        txnId: null,
        kind: 'seed',
        payload: RootConfig.parse({
          interfaces: { wan0: {}, lan0: {} },
          management: { users: [{ username: 'admin', role: 'admin' }] },
        }),
      }),
    );
    c = new SetupController(ds, { checkSetupPassword: checked } as unknown as AuthService);
  });
  const body = () => ({ input, baseRevision: 1, completedAt: new Date().toISOString() });
  it('preview reads stored credentials for validation and neither stages nor leaks hashes', async () => {
    const req = request();
    const result = await c.preview(body(), req);
    expect((await repo.candidate()).payload).toBeNull();
    expect(result.changes).toContainEqual({
      op: 'replace',
      pointer: '/management/users/username=admin/passwordHash',
      redacted: true,
    });
    expect(JSON.stringify(result)).not.toContain(TEST_HASH);
  });
  it('stages password and setup together without changing running or stored credentials', async () => {
    const req = request();
    const result = await c.stage({ ...body(), current, password }, req);
    expect(result).toEqual({ staged: true });
    expect(checked).toHaveBeenCalledWith(ADMIN, current, true);
    const candidate = RootConfig.parse((await repo.candidate()).payload);
    expect(await verifyPassword(candidate.management.users[0]!.passwordHash, password)).toBe(true);
    expect(candidate.system.setup.completed).toBe(true);
    expect(RootConfig.parse((await ds.getRunning()).doc).system.setup.completed).toBe(false);
    expect((await repo.userHashes()).get('admin')).toBe(TEST_HASH);
    expect(JSON.stringify(req.audit)).not.toContain(password);
    expect(JSON.stringify(await ds.getCandidate())).not.toContain('$argon2id$');
  });
  it('unsupported PPPoE cannot stage or mark setup complete', async () => {
    await expect(
      c.stage(
        {
          ...body(),
          input: { ...input, wanMode: 'pppoe' } as unknown as typeof input,
          current,
          password,
        },
        request(),
      ),
    ).rejects.toThrow();
    expect((await repo.candidate()).payload).toBeNull();
    expect(RootConfig.parse((await ds.getRunning()).doc).system.setup.completed).toBe(false);
  });
  it('preserves semantic issue pointers from preview', async () => {
    vi.spyOn(ds, 'validateSetupPreview').mockRejectedValue(
      problems.validation([{ pointer: '/interfaces/lan0/ipv4', message: 'overlap' }]),
    );
    const error = await c.preview(body(), request()).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ProblemError);
    expect((error as ProblemError).body()).toMatchObject({
      errors: [{ pointer: '/interfaces/lan0/ipv4', message: 'overlap' }],
    });
  });
  it('keeps completion metadata in the API and out of agent desired state', () => {
    const base = RootConfig.parse({});
    const next = RootConfig.parse({
      system: { setup: { completed: true, completedAt: new Date().toISOString() } },
    });
    expect(notAppliedChanges(base, next, ['system'])).toEqual([]);
    expect(ValidationService.desiredState(next).system).not.toHaveProperty('setup');
  });
  it('failed step-up stages nothing', async () => {
    checked.mockRejectedValue(new Error('refused'));
    await expect(c.stage({ ...body(), current, password }, request())).rejects.toThrow('refused');
    expect((await repo.candidate()).payload).toBeNull();
  });
  it('refuses a stale base and existing candidate instead of losing another edit', async () => {
    await expect(c.preview({ ...body(), baseRevision: 0 }, request())).rejects.toThrow('changed');
    await ds.patchCandidate(ADMIN, '/system/hostname', 'other');
    await expect(c.stage({ ...body(), current, password }, request())).rejects.toThrow('candidate');
    expect((await ds.getCandidate()).system).toMatchObject({
      hostname: 'other',
      setup: { completed: false },
    });
  });
  it('checks revision and candidate again inside staging transaction', async () => {
    const base = RootConfig.parse((await ds.getRunning()).doc);
    const proposed = buildSetup(base, input, new Date().toISOString());
    await expect(ds.stageSetup(ADMIN, proposed, 0)).rejects.toThrow('changed');
    await ds.patchCandidate(ADMIN, '/system/hostname', 'other');
    await expect(ds.stageSetup(ADMIN, proposed, 1)).rejects.toThrow('changed');
  });
  it('generic config routes cannot spoof the read-only completion marker', async () => {
    await expect(ds.patchCandidate(ADMIN, '/system/setup', { completed: true })).rejects.toThrow(
      'wizard',
    );
    await expect(
      ds.putCandidate(ADMIN, '/system/setup', {
        completed: true,
        completedAt: new Date().toISOString(),
      }),
    ).rejects.toThrow('wizard');
    expect((await repo.candidate()).payload).toBeNull();
  });
});
