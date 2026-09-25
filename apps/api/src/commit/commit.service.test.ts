import { status } from '@grpc/grpc-js';
import {
  DesiredState,
  ApplyOperation,
  ApplyStatus,
  IssueSeverity,
  ObjectResultCode,
  type ApplyRequest,
} from '@ngfw/proto';
import { ROOT_KEYS } from '@ngfw/schema';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { AgentClient } from '../agent/agent.client.js';
import type { AuditService } from '../audit/audit.service.js';
import type { SystemEventsService } from '../audit/system-events.service.js';
import type { TokensService } from '../auth/tokens.service.js';
import { ProblemError } from '../common/problem.js';
import { DatastoreService } from '../datastore/datastore.service.js';
import { emptyDocument } from '../datastore/documents.js';
import { Bus } from '../infra/bus.js';
import { FakeAgent } from '../testing/fake-agent.js';
import { ADMIN, OPERATOR, TEST_HASH, testEnv } from '../testing/fixtures.js';
import { MemoryConfigRepo } from '../testing/memory-repo.js';
import { CommitService } from './commit.service.js';
import { ValidationService } from './validation.service.js';

/**
 * Commit engine unit tests against the in-process fake agent over real gRPC on a unix socket (P05 not merged —
 * TASK ENVELOPE); persistence is the in-memory repo (the PostgreSQL path is covered by the e2e suite).
 */
async function problem(
  p: Promise<unknown>,
): Promise<{ status: number; body: Record<string, unknown> }> {
  try {
    await p;
  } catch (e) {
    expect(e).toBeInstanceOf(ProblemError);
    return { status: (e as ProblemError).getStatus(), body: (e as ProblemError).body() };
  }
  throw new Error('expected a problem');
}

const applies = (fake: FakeAgent) =>
  fake.calls.filter((c) => c.method === 'Apply').map((c) => c.request as ApplyRequest);

describe('CommitService (fake agent over gRPC)', () => {
  const dir = mkdtempSync(join(tmpdir(), 'vrx-p06-'));
  const socket = join(dir, 'agent.sock');
  const env = testEnv({
    VRX_AGENT_SOCKET: socket,
    VRX_AGENT_OWNER: 'w1',
    VRX_AGENT_TIMEOUT_MS: '5000',
  });
  let fake: FakeAgent;
  let agent: AgentClient;
  let repo: MemoryConfigRepo;
  let ds: DatastoreService;
  let commits: CommitService;
  let bus: Bus;
  const events = { record: vi.fn(async () => undefined) };
  const tokens = { revokeUser: vi.fn(async () => ({ persisted: true, families: 0 })) };
  const audit = { write: vi.fn(async () => undefined) };

  beforeAll(async () => {
    fake = new FakeAgent({ owner: 'w1' });
    await fake.start(socket);
    agent = new AgentClient(env);
  });
  afterAll(async () => {
    agent.close();
    await fake.stop();
    rmSync(dir, { recursive: true, force: true });
  });

  beforeEach(() => {
    // the agent's actual state starts as the empty document (defaults only), so plans list real changes
    fake.reset(
      DesiredState.toJSON(ValidationService.desiredState(emptyDocument())) as Record<
        string,
        unknown
      >,
    );
    repo = new MemoryConfigRepo();
    repo.addUser('admin', 'admin', TEST_HASH);
    repo.addUser('op', 'operator');
    bus = new Bus();
    ds = new DatastoreService(repo, env);
    tokens.revokeUser.mockClear();
    audit.write.mockClear();
    commits = new CommitService(
      repo,
      new ValidationService(repo, agent),
      agent,
      events as unknown as SystemEventsService,
      bus,
      env,
      tokens as unknown as TokensService,
      audit as unknown as AuditService,
    );
  });
  afterEach(() => commits.onApplicationShutdown());

  it('applies the candidate, persists a revision and promotes it to running', async () => {
    await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'], enabled: true });
    const published: unknown[] = [];
    bus.onPublish((m) => published.push(m));
    const r = await commits.commit(ADMIN, { comment: 'first' });
    expect(r.status).toBe('applied');
    expect(r.revision).toMatchObject({
      id: 1,
      comment: 'first',
      kind: 'commit',
      parentId: null,
      author: 'admin',
    });
    expect(r.revision?.hash).toMatch(/^[0-9a-f]{64}$/);
    expect(r.results).toEqual([
      expect.objectContaining({
        key: 'interface/loop1',
        op: 'create',
        code: 'ok',
        pointer: '/interfaces/loop1',
      }),
    ]);
    const [req] = applies(fake);
    expect(req?.owner).toBe('w1');
    expect(req?.subsystems).toEqual([...ROOT_KEYS]);
    expect(req?.confirmTimeoutSec).toBe(0);
    expect(fake.current['interfaces']).toMatchObject({
      loop1: { ipv4: ['10.1.0.1/24'], enabled: true },
    });
    expect((await ds.getRunning()).doc).toMatchObject({
      interfaces: { loop1: { ipv4: ['10.1.0.1/24'] } },
    });
    expect((await ds.lock()).locked).toBe(false);
    expect((await ds.diff()).changes).toEqual([]);
    expect(published).toContainEqual({
      topic: 'commit.events',
      data: expect.objectContaining({ type: 'applied', revision: 1 }),
    });
  });

  it('commit without edits is a no-op (agent untouched)', async () => {
    expect((await commits.commit(ADMIN, {})).status).toBe('unchanged');
    expect(applies(fake)).toEqual([]);
  });

  it('semantic errors (overlapping IPs) are 400 with pointers before the agent is asked', async () => {
    await ds.patchCandidate(ADMIN, '/interfaces', {
      loop1: { ipv4: ['10.9.0.1/24'] },
      loop2: { ipv4: ['10.9.0.2/25'] },
    });
    const p = await problem(commits.commit(ADMIN, {}));
    expect(p.status).toBe(400);
    expect(p.body['tier']).toBe('semantic');
    const errors = p.body['errors'] as { pointer: string; message: string }[];
    expect(errors.map((e) => e.pointer)).toContain('/interfaces/loop2/ipv4/0');
    expect(errors[0]?.message).toMatch(/overlap/);
    expect(fake.calls.filter((c) => c.method === 'DryRun' || c.method === 'Apply')).toEqual([]);
    expect(repo.state.revisions).toEqual([]);
  });

  it('a reference to a secret that does not exist is a 400 (D-051)', async () => {
    await ds.patchCandidate(ADMIN, '/management/aaa', {
      tacacs: { servers: [{ address: '10.0.0.9', secretRef: 'psk/tacacs-1' }] },
    });
    const p = await problem(commits.commit(ADMIN, {}));
    expect(p.status).toBe(400);
    expect(p.body['errors']).toEqual([
      expect.objectContaining({
        pointer: '/management/aaa/tacacs/servers/0/secretRef',
        rule: 'secrets.ref-exists',
      }),
    ]);
    repo.state.secrets.add('psk/tacacs-1');
    expect((await commits.commit(ADMIN, {})).status).toBe('applied');
  });

  it('agent DryRun errors are tier-3 400s with the agent pointer', async () => {
    fake.dryRunIssues = () => [
      {
        pointer: '/interfaces/loop1',
        message: 'no such device',
        severity: IssueSeverity.ISSUE_SEVERITY_ERROR,
        rule: 'interfaces.exists',
      },
      {
        pointer: '/system',
        message: 'fyi',
        severity: IssueSeverity.ISSUE_SEVERITY_WARNING,
        rule: 'x',
      },
    ];
    await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
    const p = await problem(commits.commit(ADMIN, {}));
    expect(p.status).toBe(400);
    expect(p.body).toMatchObject({
      tier: 'agent',
      errors: [
        { pointer: '/interfaces/loop1', message: 'no such device', rule: 'interfaces.exists' },
      ],
      warnings: [{ pointer: '/system', message: 'fyi', rule: 'x' }],
    });
    expect(applies(fake)).toEqual([]);
  });

  it('ROLLED_BACK from the agent is 422 with per-object results; running and candidate stay', async () => {
    await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
    fake.nextApply = () => ({
      status: ApplyStatus.APPLY_STATUS_ROLLED_BACK,
      message: 'vpp said no',
      results: [
        {
          key: 'interface/loop1',
          op: ApplyOperation.APPLY_OPERATION_CREATE,
          code: ObjectResultCode.OBJECT_RESULT_CODE_FAILED,
          message: 'VNET_API_ERROR',
          pointer: '/interfaces/loop1',
          subsystem: 'interfaces',
        },
      ],
    });
    const p = await problem(commits.commit(ADMIN, {}));
    expect(p.status).toBe(422);
    expect(p.body).toMatchObject({
      applyStatus: 'rolled_back',
      type: 'https://vrx.dev/problems/apply-failed',
    });
    expect(p.body['results']).toEqual([
      expect.objectContaining({ key: 'interface/loop1', code: 'failed' }),
    ]);
    expect(p.body['errors']).toEqual([
      { pointer: '/interfaces/loop1', message: 'interface/loop1: VNET_API_ERROR' },
    ]);
    expect(repo.state.revisions).toEqual([]);
    expect((await ds.diff()).changes).toHaveLength(1);
    expect((await ds.lock()).owner).toBe('admin');
  });

  it('agent unavailable → 503; lock conflicts → 409', async () => {
    await ds.patchCandidate(ADMIN, '/system', { hostname: 'x' });
    expect((await problem(commits.commit(OPERATOR, {}))).status).toBe(409);
    fake.failAllWith = status.UNAVAILABLE;
    expect((await problem(commits.commit(ADMIN, {}))).status).toBe(503);
  });

  it('confirmed commit: pending until confirmed, then a revision', async () => {
    await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
    const r = await commits.commit(ADMIN, { confirmSec: 30, comment: 'careful' });
    expect(r.status).toBe('pending');
    expect(Date.parse(r.confirmDeadline!)).toBeGreaterThan(Date.now() + 20_000);
    expect(applies(fake)[0]?.confirmTimeoutSec).toBe(30);
    expect(repo.state.revisions).toEqual([]);
    expect(await commits.pendingInfo()).toMatchObject({ txnId: r.txnId, comment: 'careful' });
    // nothing else may be committed while one is pending
    expect((await problem(commits.commit(ADMIN, {}))).status).toBe(409);
    const c = await commits.confirm(ADMIN);
    expect(c).toMatchObject({
      status: 'confirmed',
      txnId: r.txnId,
      revision: { id: 1, comment: 'careful' },
    });
    expect(await commits.pendingInfo()).toBeNull();
    expect((await ds.lock()).locked).toBe(false);
    expect((await problem(commits.confirm(ADMIN))).status).toBe(409);
  });

  it('unconfirmed commit: the agent reverts and the API drops the pending commit (Health path)', async () => {
    await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
    await commits.commit(ADMIN, { confirmSec: 1 });
    expect(fake.current['interfaces']).toHaveProperty('loop1');
    await vi.waitFor(async () => expect(await commits.pendingInfo()).toBeNull(), {
      timeout: 6000,
      interval: 200,
    });
    expect(fake.current['interfaces']).toBeUndefined();
    expect(fake.current['system']).toBeDefined();
    expect(repo.state.revisions).toEqual([]);
    // the candidate is still there: fix and commit again
    expect((await ds.diff()).changes).toHaveLength(1);
    expect(events.record).toHaveBeenCalledWith(
      'warning',
      'commit',
      'CONFIRM_REVERTED',
      expect.any(String),
      expect.anything(),
    );
  });

  it('a CONFIRM_REVERTED agent event drops the pending commit at once', async () => {
    await ds.patchCandidate(ADMIN, '/system', { hostname: 'x' });
    const r = await commits.commit(ADMIN, { confirmSec: 60 });
    bus.agentEvent({
      kind: 6,
      txnId: r.txnId!,
      seq: '1',
      message: '',
      attributes: {},
      ts: new Date(),
      interface: undefined,
      summary: undefined,
    });
    await vi.waitFor(async () => expect(await commits.pendingInfo()).toBeNull());
  });

  it('rollback applies an old revision as a new revision', async () => {
    await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
    await commits.commit(ADMIN, {});
    await ds.patchCandidate(ADMIN, '/interfaces/loop2', { ipv4: ['10.2.0.1/24'] });
    await commits.commit(ADMIN, {});
    expect(Object.keys(fake.current['interfaces'] as object)).toEqual(['loop1', 'loop2']);
    const r = await commits.rollback(OPERATOR, 1, {});
    expect(r.revision).toMatchObject({
      id: 3,
      kind: 'rollback',
      parentId: 2,
      comment: 'rollback to revision 1',
    });
    expect(repo.state.revisions[2]?.payload).toEqual(repo.state.revisions[0]?.payload);
    expect(repo.state.revisions[2]?.hash).toBe(repo.state.revisions[0]?.hash);
    expect(Object.keys(fake.current['interfaces'] as object)).toEqual(['loop1']);
    expect((await problem(commits.rollback(ADMIN, 99, {}))).status).toBe(404);
    await ds.patchCandidate(ADMIN, '/system', { hostname: 'dirty' });
    expect((await problem(commits.rollback(ADMIN, 1, {}))).status).toBe(409);
  });

  it('management.users: hashes go to app_user, never into revisions; later commits still validate', async () => {
    await ds.putCandidate(ADMIN, '/management/users', [
      { username: 'alice', role: 'admin', passwordHash: TEST_HASH },
      { username: 'bob', role: 'readonly' },
    ]);
    const r1 = await commits.commit(ADMIN, {});
    expect(r1.status).toBe('applied');
    expect(JSON.stringify(repo.state.revisions)).not.toContain('VRX_TEST_HASH');
    expect(JSON.stringify(fake.calls)).not.toContain('VRX_TEST_HASH');
    expect(repo.state.users.get('alice')).toMatchObject({
      role: 'admin',
      hash: TEST_HASH,
      source: 'config',
    });
    expect(repo.state.users.get('bob')).toMatchObject({ role: 'readonly', hash: null });
    // the next commit starts from the redacted running document; hashes are hydrated from app_user for validation
    await ds.patchCandidate(ADMIN, '/system', { hostname: 'again' });
    expect((await commits.commit(ADMIN, {})).status).toBe('applied');
    // removing bob from the configuration deletes the config-sourced user; the bootstrap admin stays
    await ds.putCandidate(ADMIN, '/management/users', [{ username: 'alice', role: 'admin' }]);
    await commits.commit(ADMIN, {});
    expect(repo.state.users.has('bob')).toBe(false);
    expect(repo.state.users.get('alice')?.hash).toBe(TEST_HASH);
    expect(repo.state.users.has('admin')).toBe(true);
    // an operator may not roll the users back
    expect((await problem(commits.rollback(OPERATOR, 1, {}))).status).toBe(403);
  });

  it('D-102: a hash changed through the config API ends the user’s sessions after the promote, audited via: config', async () => {
    const OTHER = '$vrx-test$VRX_TEST_HASH_D102';
    await ds.putCandidate(ADMIN, '/management/users', [
      { username: 'admin', role: 'admin' },
      { username: 'alice', role: 'operator', passwordHash: TEST_HASH },
    ]);
    await commits.commit(ADMIN, {});
    // a NEW user with a hash is not a reset
    expect(tokens.revokeUser).not.toHaveBeenCalled();
    const alice = repo.state.users.get('alice')!;
    expect(alice.gen).toBe(0);
    // the same hash staged again is not a change either
    await ds.patchCandidate(ADMIN, '/management/users/1', { passwordHash: TEST_HASH });
    await ds.patchCandidate(ADMIN, '/system', { hostname: 'same-hash' });
    expect((await commits.commit(ADMIN, {})).status).toBe('applied');
    expect(tokens.revokeUser).not.toHaveBeenCalled();
    // a changed hash: generation bumped in the promote, sessions ended once it committed, one audit row
    await ds.patchCandidate(ADMIN, '/management/users/1', { passwordHash: OTHER });
    const r = await commits.commit(ADMIN, { comment: 'reset alice' });
    expect(r.status).toBe('applied');
    expect(repo.state.users.get('alice')).toMatchObject({ hash: OTHER, gen: 1 });
    expect(tokens.revokeUser).toHaveBeenCalledTimes(1);
    expect(tokens.revokeUser).toHaveBeenCalledWith(alice.id, 1);
    expect(audit.write).toHaveBeenCalledTimes(1);
    expect(audit.write).toHaveBeenCalledWith(
      expect.objectContaining({
        action: 'config.password-reset',
        resource: 'user/alice',
        userId: ADMIN.id,
        username: 'admin',
        result: 'success',
        after: expect.objectContaining({
          passwordSet: true,
          via: 'config',
          revision: r.revision!.id,
          txnId: r.txnId,
          apiKeysRevoked: [],
        }),
      }),
    );
    expect(JSON.stringify(audit.write.mock.calls)).not.toContain('VRX_TEST_HASH');
  });

  it('D-102: a confirmed commit resets at CONFIRM (when the hash reaches app_user), not while pending', async () => {
    await ds.putCandidate(ADMIN, '/management/users', [
      { username: 'admin', role: 'admin' },
      { username: 'carol', role: 'operator', passwordHash: TEST_HASH },
    ]);
    await commits.commit(ADMIN, {});
    await ds.patchCandidate(ADMIN, '/management/users/1', {
      passwordHash: '$vrx-test$VRX_TEST_HASH_D102C',
    });
    expect((await commits.commit(ADMIN, { confirmSec: 30 })).status).toBe('pending');
    expect(tokens.revokeUser).not.toHaveBeenCalled();
    expect(repo.state.users.get('carol')?.hash).toBe(TEST_HASH);
    await commits.confirm(ADMIN);
    expect(repo.state.users.get('carol')).toMatchObject({ gen: 1 });
    expect(tokens.revokeUser).toHaveBeenCalledWith(repo.state.users.get('carol')!.id, 1);
  });

  // ---------------------------------------------------------------- TD-4: D-100 (3) disable bumps the generation

  it('D-100 (3): disabling an existing user bumps the generation and ends the sessions after the promote, audited config.user-disabled', async () => {
    await ds.putCandidate(ADMIN, '/management/users', [
      { username: 'admin', role: 'admin' },
      { username: 'dave', role: 'operator', passwordHash: TEST_HASH },
      // a user CREATED disabled is not a disable
      { username: 'erin', role: 'operator', passwordHash: TEST_HASH, disabled: true },
    ]);
    await commits.commit(ADMIN, {});
    expect(tokens.revokeUser).not.toHaveBeenCalled();
    expect(repo.state.users.get('erin')).toMatchObject({ disabled: true, gen: 0 });
    const dave = repo.state.users.get('dave')!;
    await ds.patchCandidate(ADMIN, '/management/users/1', { disabled: true });
    const r = await commits.commit(ADMIN, { comment: 'disable dave' });
    expect(r.status).toBe('applied');
    // generation bumped; the hash and the lockout state are not touched (only a password change clears them)
    expect(repo.state.users.get('dave')).toMatchObject({ disabled: true, gen: 1, hash: TEST_HASH });
    expect(tokens.revokeUser).toHaveBeenCalledTimes(1);
    expect(tokens.revokeUser).toHaveBeenCalledWith(dave.id, 1);
    expect(audit.write).toHaveBeenCalledTimes(1);
    expect(audit.write).toHaveBeenCalledWith({
      userId: ADMIN.id,
      username: 'admin',
      sourceIp: null,
      action: 'config.user-disabled',
      resource: 'user/dave',
      after: { disabled: true, via: 'config', revision: r.revision!.id, txnId: r.txnId },
      result: 'success',
      status: null,
    });
    // the same flag staged again, and a re-enable, do not bump
    tokens.revokeUser.mockClear();
    audit.write.mockClear();
    await ds.patchCandidate(ADMIN, '/system', { hostname: 'still-disabled' });
    expect((await commits.commit(ADMIN, {})).status).toBe('applied');
    await ds.patchCandidate(ADMIN, '/management/users/1', { disabled: false });
    expect((await commits.commit(ADMIN, {})).status).toBe('applied');
    expect(repo.state.users.get('dave')).toMatchObject({ disabled: false, gen: 1 });
    expect(tokens.revokeUser).not.toHaveBeenCalled();
    expect(audit.write).not.toHaveBeenCalled();
    // disabling again after the re-enable is a new disable
    await ds.patchCandidate(ADMIN, '/management/users/1', { disabled: true });
    expect((await commits.commit(ADMIN, {})).status).toBe('applied');
    expect(tokens.revokeUser).toHaveBeenCalledWith(dave.id, 2);
  });

  it('D-100 (3): a hash change plus a disable in one commit bumps ONCE and writes two audit rows (the D-102 row unchanged)', async () => {
    const OTHER = '$vrx-test$VRX_TEST_HASH_TD4';
    await ds.putCandidate(ADMIN, '/management/users', [
      { username: 'admin', role: 'admin' },
      { username: 'frank', role: 'operator', passwordHash: TEST_HASH },
    ]);
    await commits.commit(ADMIN, {});
    const frank = repo.state.users.get('frank')!;
    await ds.patchCandidate(ADMIN, '/management/users/1', { passwordHash: OTHER, disabled: true });
    const r = await commits.commit(ADMIN, { comment: 'reset + disable frank' });
    expect(r.status).toBe('applied');
    expect(repo.state.users.get('frank')).toMatchObject({ hash: OTHER, disabled: true, gen: 1 });
    expect(tokens.revokeUser).toHaveBeenCalledTimes(1);
    expect(tokens.revokeUser).toHaveBeenCalledWith(frank.id, 1);
    expect(audit.write.mock.calls.map((c) => (c as unknown[])[0])).toEqual([
      {
        userId: ADMIN.id,
        username: 'admin',
        sourceIp: null,
        action: 'config.password-reset',
        resource: 'user/frank',
        after: {
          passwordSet: true,
          self: false,
          via: 'config',
          revision: r.revision!.id,
          txnId: r.txnId,
          apiKeysRevoked: [],
        },
        result: 'success',
        status: null,
      },
      {
        userId: ADMIN.id,
        username: 'admin',
        sourceIp: null,
        action: 'config.user-disabled',
        resource: 'user/frank',
        after: { disabled: true, via: 'config', revision: r.revision!.id, txnId: r.txnId },
        result: 'success',
        status: null,
      },
    ]);
    expect(JSON.stringify(audit.write.mock.calls)).not.toContain('VRX_TEST_HASH');
  });

  it('D-100 (3): a confirmed commit disables at CONFIRM, not while pending; Valkey failing is recorded on the row', async () => {
    await ds.putCandidate(ADMIN, '/management/users', [
      { username: 'admin', role: 'admin' },
      { username: 'gina', role: 'operator', passwordHash: TEST_HASH },
    ]);
    await commits.commit(ADMIN, {});
    await ds.patchCandidate(ADMIN, '/management/users/1', { disabled: true });
    expect((await commits.commit(ADMIN, { confirmSec: 30 })).status).toBe('pending');
    expect(tokens.revokeUser).not.toHaveBeenCalled();
    expect(repo.state.users.get('gina')).toMatchObject({ disabled: false });
    tokens.revokeUser.mockResolvedValueOnce({ persisted: false, families: 0 });
    await commits.confirm(ADMIN);
    expect(repo.state.users.get('gina')).toMatchObject({ disabled: true, gen: 1 });
    expect(tokens.revokeUser).toHaveBeenCalledWith(repo.state.users.get('gina')!.id, 1);
    expect(audit.write).toHaveBeenCalledWith(
      expect.objectContaining({
        action: 'config.user-disabled',
        resource: 'user/gina',
        after: expect.objectContaining({ disabled: true, revocationPersisted: false }),
      }),
    );
  });

  // ---------------------------------------------------------------- review fix round (P06-review.md)

  it('M1: an operator cannot commit admin-only changes staged by someone else (stale takeover discards them)', async () => {
    // admin stages a new admin user, then walks away
    await ds.putCandidate(ADMIN, '/management/users', [
      { username: 'admin', role: 'admin' },
      { username: 'evil', role: 'admin', passwordHash: TEST_HASH },
    ]);
    // before the lock is stale the operator is refused
    expect((await problem(ds.patchCandidate(OPERATOR, '/system', { hostname: 'x' }))).status).toBe(
      409,
    );
    // after the TTL the operator takes over: the privileged staged changes are NOT inherited
    ds.now = () => new Date(Date.now() + 61_000);
    const r = await ds.patchCandidate(OPERATOR, '/system', { hostname: 'op-host' });
    expect(r.discardedStaleCandidateOf).toBe('admin');
    const c = await commits.commit(OPERATOR, {});
    expect(c.status).toBe('applied');
    expect(repo.state.users.has('evil')).toBe(false);
    expect(JSON.stringify(repo.state.revisions.at(-1)?.payload)).not.toContain('evil');
  });

  it('M1: the commit itself is authorised on running → candidate, whoever staged it', async () => {
    // simulate a candidate that already contains a users change but is owned by the operator
    await ds.putCandidate(ADMIN, '/management/users', [
      { username: 'admin', role: 'admin' },
      { username: 'evil', role: 'admin', passwordHash: TEST_HASH },
    ]);
    repo.state.candidate.ownerId = 2; // op
    const p = await problem(commits.commit(OPERATOR, {}));
    expect(p.status).toBe(403);
    expect(p.body['errors']).toEqual([{ pointer: '/management/users', message: 'admin only' }]);
    expect((await problem(commits.validateCandidate(OPERATOR))).status).toBe(403);
    expect(applies(fake)).toEqual([]);
    repo.state.candidate.ownerId = 1;
    expect((await commits.commit(ADMIN, {})).status).toBe('applied');
  });

  it('M3: a lost Apply answer marks running UNKNOWN; reconcile finds the txn applied and saves the revision', async () => {
    const shortEnv = testEnv({
      VRX_AGENT_SOCKET: socket,
      VRX_AGENT_OWNER: 'w1',
      VRX_AGENT_TIMEOUT_MS: '300',
    });
    const shortAgent = new AgentClient(shortEnv);
    const c2 = new CommitService(
      repo,
      new ValidationService(repo, shortAgent),
      shortAgent,
      events as unknown as SystemEventsService,
      bus,
      shortEnv,
      tokens as unknown as TokensService,
      audit as unknown as AuditService,
    );
    try {
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      fake.applyDelayMs = 800;
      const p = await problem(c2.commit(ADMIN, {}));
      expect(p.status).toBe(504);
      expect(p.body).toMatchObject({
        type: 'https://vrx.dev/problems/running-unknown',
        sync: { state: 'unknown' },
      });
      expect(String(p.body['detail'])).not.toMatch(/running is unchanged/);
      fake.applyDelayMs = 0;
      await vi.waitFor(async () => expect((await c2.syncStatus()).state).toBe('in-sync'), {
        timeout: 8000,
        interval: 200,
      });
      expect(repo.state.revisions).toHaveLength(1);
      expect(repo.state.revisions[0]?.txnId).toBe(fake.lastTxnId);
      expect((await ds.getRunning()).doc).toMatchObject({
        interfaces: { loop1: { ipv4: ['10.1.0.1/24'] } },
      });
    } finally {
      c2.onApplicationShutdown();
      shortAgent.close();
    }
  });

  it('M3: DEGRADED is never "running unchanged": sync degraded, reconcile re-applies running', async () => {
    await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
    fake.nextApply = () => ({
      status: ApplyStatus.APPLY_STATUS_DEGRADED,
      message: 'rollback failed',
    });
    const p = await problem(commits.commit(ADMIN, {}));
    expect(p.status).toBe(422);
    expect(p.body).toMatchObject({ applyStatus: 'degraded', sync: { state: 'degraded' } });
    expect(String(p.body['detail'])).toMatch(/partially changed/);
    await vi.waitFor(async () => expect((await commits.syncStatus()).state).toBe('in-sync'), {
      timeout: 8000,
      interval: 200,
    });
    // the reconcile was a re-apply of running (empty document: no loopback)
    expect(fake.current['interfaces']).toBeUndefined();
    expect(repo.state.revisions).toEqual([]);
  });

  it('M3: revision save failure after APPLIED → UNKNOWN, then the revision is saved by the reconcile', async () => {
    await ds.patchCandidate(ADMIN, '/system', { hostname: 'db-down' });
    repo.failNextTx = 1;
    const p = await problem(commits.commit(ADMIN, {}));
    expect(p.status).toBe(500);
    expect(p.body).toMatchObject({
      type: 'https://vrx.dev/problems/running-unknown',
      sync: { state: 'unknown' },
    });
    await vi.waitFor(async () => expect((await commits.syncStatus()).state).toBe('in-sync'), {
      timeout: 8000,
      interval: 200,
    });
    expect((await ds.getRunning()).doc).toMatchObject({ system: { hostname: 'db-down' } });
  });

  it('M4: changes only in domains the agent does not implement → status not-applied', async () => {
    fake.implemented = ['interfaces', 'vrfs', 'routing'];
    await ds.patchCandidate(ADMIN, '/system', { hostname: 'not-enforced' });
    const r = await commits.commit(ADMIN, {});
    expect(r.status).toBe('not-applied');
    expect(r.notApplied).toEqual(['system']);
    await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
    await ds.patchCandidate(ADMIN, '/system', { hostname: 'both' });
    const r2 = await commits.commit(ADMIN, {});
    expect(r2).toMatchObject({ status: 'partially-applied', notApplied: ['system'] });
    // management.users is applied by the API itself
    await ds.putCandidate(ADMIN, '/management/users', [{ username: 'admin', role: 'admin' }]);
    expect(await commits.commit(ADMIN, {})).toMatchObject({ status: 'applied', notApplied: [] });
  });

  it('M2: revisions pin secret versions; rollback re-activates them', async () => {
    expect(repo.putSecret('psk/tac')).toBe(1);
    await ds.patchCandidate(ADMIN, '/management/aaa', {
      tacacs: { servers: [{ address: '10.0.0.9', secretRef: 'psk/tac' }] },
    });
    const r1 = await commits.commit(ADMIN, {});
    expect(repo.state.revisions.at(-1)?.secretVersions).toEqual({ 'psk/tac': 1 });
    expect(repo.putSecret('psk/tac')).toBe(2);
    await ds.patchCandidate(ADMIN, '/system', { hostname: 'after-rotation' });
    await commits.commit(ADMIN, {});
    expect(repo.state.revisions.at(-1)?.secretVersions).toEqual({ 'psk/tac': 2 });
    await commits.rollback(ADMIN, r1.revision!.id, {});
    expect(repo.state.secretVersion.get('psk/tac')).toBe(1);
    expect(repo.state.revisions.at(-1)?.secretVersions).toEqual({ 'psk/tac': 1 });
  });
});
