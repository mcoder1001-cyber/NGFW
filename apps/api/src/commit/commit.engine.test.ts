import {
  ApplyStatus,
  DesiredState,
  EventKind,
  IssueSeverity,
  type ApplyRequest,
  type Event,
} from '@ngfw/proto';
import { status as grpcStatus } from '@grpc/grpc-js';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { AgentClient } from '../agent/agent.client.js';
import type { AuditService } from '../audit/audit.service.js';
import type { SystemEventsService } from '../audit/system-events.service.js';
import type { TokensService } from '../auth/tokens.service.js';
import type { Env } from '../config.js';
import { ProblemError } from '../common/problem.js';
import type { Db } from '../db/db.js';
import { SecretsService } from '../secrets/secrets.service.js';
import { DatastoreService } from '../datastore/datastore.service.js';
import { emptyDocument } from '../datastore/documents.js';
import { Bus } from '../infra/bus.js';
import { FakeAgent } from '../testing/fake-agent.js';
import { ADMIN, TEST_HASH, testEnv } from '../testing/fixtures.js';
import { MemoryConfigRepo } from '../testing/memory-repo.js';
import { CommitService } from './commit.service.js';
import { ValidationService } from './validation.service.js';

/**
 * TD-10a (REVIEW-2026-09-24 2.1, 2.2, 2.4a, 2.5, ARCH-01): commit-engine correctness against the in-process fake agent
 * over real gRPC; persistence is the in-memory repo (the PostgreSQL paths are in test/e2e/td10a.e2e.test.ts).
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

function agentEvent(kind: EventKind, txnId = ''): Event {
  return {
    kind,
    txnId,
    seq: '1',
    message: '',
    attributes: {},
    ts: new Date(),
    interface: undefined,
    summary: undefined,
  };
}

const EMPTY_STATE = () =>
  DesiredState.toJSON(ValidationService.desiredState(emptyDocument())) as Record<string, unknown>;

const WARNING = {
  pointer: '/system',
  message: 'fyi: not enforced yet',
  severity: IssueSeverity.ISSUE_SEVERITY_WARNING,
  rule: 'agent.unsupported-field',
};

describe('TD-10a commit engine (fake agent over gRPC)', { timeout: 20_000 }, () => {
  const dir = mkdtempSync(join(tmpdir(), 'vrx-td10a-'));
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
  const extra: { svc: CommitService; agent?: AgentClient }[] = [];
  const events = { record: vi.fn(async (..._a: unknown[]) => undefined) };
  const tokens = { revokeUser: vi.fn(async () => ({ persisted: true, families: 0 })) };
  const audit = { write: vi.fn(async () => undefined) };

  /** Another CommitService on the same repo and bus: an API restart, or one with its own agent timeouts. */
  function service(e: Env = env, ownAgent = false): CommitService {
    const a = ownAgent ? new AgentClient(e) : agent;
    const svc = new CommitService(
      repo,
      new ValidationService(repo, a),
      a,
      events as unknown as SystemEventsService,
      bus,
      e,
      tokens as unknown as TokensService,
      audit as unknown as AuditService,
    );
    extra.push({ svc, ...(ownAgent ? { agent: a } : {}) });
    return svc;
  }
  const shortEnv = (ms: string) =>
    testEnv({ VRX_AGENT_SOCKET: socket, VRX_AGENT_OWNER: 'w1', VRX_AGENT_TIMEOUT_MS: ms });

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
    fake.reset(EMPTY_STATE());
    repo = new MemoryConfigRepo();
    repo.addUser('admin', 'admin', TEST_HASH);
    repo.addUser('op', 'operator');
    bus = new Bus();
    ds = new DatastoreService(repo, env);
    events.record.mockClear();
    commits = service();
  });
  afterEach(() => {
    for (const x of extra.splice(0)) {
      x.svc.onApplicationShutdown();
      x.agent?.close();
    }
  });

  const codes = () => events.record.mock.calls.map((c) => c[2]);

  // ------------------------------------------------------------------------------------------------ 2.1

  describe('2.1 a confirm is decided by Health.last_txn_id (confirmed ≠ reverted)', () => {
    it('a confirm whose answer is lost is looked up at once: 200 confirmed, revision saved', async () => {
      const c2 = service(shortEnv('300'), true);
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      const r = await c2.commit(ADMIN, { confirmSec: 30, comment: 'careful' });
      expect(r.status).toBe('pending');
      fake.confirmDelayMs = 1000; // the agent confirms, its answer arrives after the API gave up
      const c = await c2.confirm(ADMIN);
      expect(c).toMatchObject({
        status: 'confirmed',
        txnId: r.txnId,
        revision: { id: 1, txnId: r.txnId, comment: 'careful' },
      });
      expect(await c2.pendingInfo()).toBeNull();
      expect((await c2.syncStatus()).state).toBe('in-sync');
      expect((await ds.getRunning()).doc).toMatchObject({
        interfaces: { loop1: { ipv4: ['10.1.0.1/24'] } },
      });
    });

    it('a confirm retried after a lost answer (agent: FAILED_PRECONDITION) is confirmed, not "reverted"', async () => {
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      const r = await commits.commit(ADMIN, { confirmSec: 30 });
      fake.confirmPending(); // the first confirm reached the agent; its answer did not reach the API
      const c = await commits.confirm(ADMIN);
      expect(c).toMatchObject({ status: 'confirmed', txnId: r.txnId, revision: { id: 1 } });
      expect(repo.state.revisions.map((x) => x.txnId)).toEqual([r.txnId]);
      expect(await commits.pendingInfo()).toBeNull();
      expect(codes()).toContain('COMMIT_CONFIRMED');
      expect(codes()).not.toContain('CONFIRM_REVERTED');
    });

    it('the deadline watcher saves a transaction the agent confirmed instead of dropping it', async () => {
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      const r = await commits.commit(ADMIN, { confirmSec: 1, comment: 'watched' });
      fake.confirmPending();
      await vi.waitFor(async () => expect(await commits.pendingInfo()).toBeNull(), {
        timeout: 6000,
        interval: 200,
      });
      expect(repo.state.revisions.map((x) => [x.txnId, x.comment])).toEqual([[r.txnId, 'watched']]);
      expect((await ds.getRunning()).doc).toMatchObject({ interfaces: { loop1: {} } });
      expect(codes()).toContain('COMMIT_CONFIRMED');
      expect(codes()).not.toContain('CONFIRM_REVERTED');
    });

    it('a real revert is still dropped: confirm afterwards is 409 commit-reverted, no revision', async () => {
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      await commits.commit(ADMIN, { confirmSec: 30 });
      fake.revertNow();
      const p = await problem(commits.confirm(ADMIN));
      expect(p.status).toBe(409);
      expect(p.body['type']).toBe('https://vrx.dev/problems/commit-reverted');
      expect(await commits.pendingInfo()).toBeNull();
      expect(repo.state.revisions).toEqual([]);
      expect(fake.current['interfaces']).toBeUndefined();
    });
  });

  // ------------------------------------------------------------------------------------------------ 2.2

  describe('2.2 a confirmed rollback restores the secret versions of its target', () => {
    async function twoVersions() {
      expect(repo.putSecret('psk/tac')).toBe(1);
      await ds.patchCandidate(ADMIN, '/management/aaa', {
        tacacs: { servers: [{ address: '10.0.0.9', secretRef: 'psk/tac' }] },
      });
      const r1 = await commits.commit(ADMIN, {});
      expect(repo.putSecret('psk/tac')).toBe(2);
      await ds.patchCandidate(ADMIN, '/system', { hostname: 'after-rotation' });
      await commits.commit(ADMIN, {});
      return r1.revision!.id;
    }

    it('rollback with a confirm window, then confirm: version 1 active and pinned', async () => {
      const rev = await twoVersions();
      const r = await commits.rollback(ADMIN, rev, { confirmSec: 30 });
      expect(r.status).toBe('pending');
      // kept with the pending commit (config_pending.restore_secrets), not only in this process
      expect(repo.state.pending?.restoreSecrets).toEqual({ 'psk/tac': 1 });
      expect(repo.state.secretVersion.get('psk/tac')).toBe(2);
      await commits.confirm(ADMIN);
      expect(repo.state.secretVersion.get('psk/tac')).toBe(1);
      expect(repo.state.revisions.at(-1)).toMatchObject({
        kind: 'rollback',
        secretVersions: { 'psk/tac': 1 },
      });
    });

    it('also when the confirm is found through Health (watcher) by a restarted API', async () => {
      const rev = await twoVersions();
      await commits.rollback(ADMIN, rev, { confirmSec: 1 });
      commits.onApplicationShutdown(); // the API process that knew restoreSecrets in memory is gone
      fake.confirmPending();
      const c3 = service();
      await c3.resumePending();
      await vi.waitFor(async () => expect(await c3.pendingInfo()).toBeNull(), {
        timeout: 6000,
        interval: 200,
      });
      expect(repo.state.secretVersion.get('psk/tac')).toBe(1);
      expect(repo.state.revisions.at(-1)?.secretVersions).toEqual({ 'psk/tac': 1 });
    });
  });

  // ------------------------------------------------------------------------------------------------ 2.5

  describe('2.5 agent warnings reach every caller', () => {
    it('kept with the pending commit and returned by confirm', async () => {
      fake.dryRunIssues = () => [WARNING];
      await ds.patchCandidate(ADMIN, '/system', { hostname: 'warned' });
      const r = await commits.commit(ADMIN, { confirmSec: 30 });
      const want = [{ pointer: '/system', message: WARNING.message, rule: WARNING.rule }];
      expect(r.warnings).toEqual(want);
      expect(repo.state.pending?.warnings).toEqual(want);
      const c = await commits.confirm(ADMIN);
      expect(c.warnings).toEqual(want);
    });

    it('the 422 apply-failed problem carries them', async () => {
      fake.dryRunIssues = () => [WARNING];
      fake.nextApply = () => ({ status: ApplyStatus.APPLY_STATUS_ROLLED_BACK, message: 'no' });
      await ds.patchCandidate(ADMIN, '/system', { hostname: 'warned' });
      const p = await problem(commits.commit(ADMIN, {}));
      expect(p.status).toBe(422);
      expect(p.body['warnings']).toEqual([
        { pointer: '/system', message: WARNING.message, rule: WARNING.rule },
      ]);
    });

    it('the running-unknown problem carries them', async () => {
      const c2 = service(shortEnv('300'), true);
      fake.dryRunIssues = () => [WARNING];
      fake.applyDelayMs = 800;
      await ds.patchCandidate(ADMIN, '/system', { hostname: 'warned' });
      const p = await problem(c2.commit(ADMIN, {}));
      expect(p.body['type']).toBe('https://vrx.dev/problems/running-unknown');
      expect(p.body['warnings']).toEqual([
        { pointer: '/system', message: WARNING.message, rule: WARNING.rule },
      ]);
    });
  });

  // ------------------------------------------------------------------------------------------------ 2.4a

  describe('2.4a explicit server time budget, no queue', () => {
    it('a commit while another is in flight is 409 commit-busy at once', async () => {
      const slow = service(shortEnv('9000'), true);
      fake.applyDelayMs = 5000; // three attempts × the 1 s lock wait fit well inside
      await ds.patchCandidate(ADMIN, '/system', { hostname: 'first' });
      const first = slow.commit(ADMIN, {});
      await vi.waitFor(() => expect(applies(fake)).toHaveLength(1), {
        timeout: 3000,
        interval: 20,
      });
      for (const second of [
        () => slow.commit(ADMIN, {}),
        () => slow.confirm(ADMIN),
        () => slow.rollback(ADMIN, 1, {}),
      ]) {
        const t0 = Date.now();
        const p = await problem(second());
        expect(p.status).toBe(409);
        expect(p.body['type']).toBe('https://vrx.dev/problems/commit-busy');
        expect(Date.now() - t0).toBeLessThan(2000);
      }
      expect((await first).status).toBe('applied');
    });

    it('DryRun and Apply carry the budget deadlines even when VRX_AGENT_TIMEOUT_MS is larger', async () => {
      const big = service(shortEnv('120000'), true);
      await ds.patchCandidate(ADMIN, '/system', { hostname: 'budget' });
      const t0 = Date.now();
      await big.commit(ADMIN, {});
      const left = (m: string) => new Date(fake.deadlines[m]!).getTime() - t0;
      expect(left('DryRun')).toBeLessThanOrEqual(30_000 + 1000);
      expect(left('Apply')).toBeLessThanOrEqual(60_000 + 1000);
      expect(left('Apply')).toBeGreaterThan(30_000);
    });
  });

  // ------------------------------------------------------------------------------------------------ ARCH-01

  describe('ARCH-01 API half: Health.last_txn_id is compared with running', () => {
    /** The agent restarted from a stale persisted state (a failed state save answered APPLIED). */
    function staleAgent(): void {
      fake.current = EMPTY_STATE();
      fake.confirmed = EMPTY_STATE();
      fake.lastTxnId = 'stale-txn';
    }

    it('RECONCILE_DONE from an agent resync with another last_txn_id → running re-applied', async () => {
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      await commits.commit(ADMIN, {});
      staleAgent();
      bus.agentEvent(agentEvent(EventKind.EVENT_KIND_RECONCILE_DONE));
      await vi.waitFor(
        () =>
          expect(fake.current['interfaces']).toMatchObject({ loop1: { ipv4: ['10.1.0.1/24'] } }),
        { timeout: 6000, interval: 100 },
      );
      await vi.waitFor(async () => expect((await commits.syncStatus()).state).toBe('in-sync'), {
        timeout: 6000,
        interval: 100,
      });
      expect(fake.lastTxnId).not.toBe('stale-txn');
      expect(codes()).toContain('RUNNING_UNKNOWN');
      expect(repo.state.revisions).toHaveLength(1);
    });

    it('RECONCILE_DONE with a matching last_txn_id changes nothing', async () => {
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      await commits.commit(ADMIN, {});
      const n = applies(fake).length;
      bus.agentEvent(agentEvent(EventKind.EVENT_KIND_RECONCILE_DONE));
      await new Promise((r) => setTimeout(r, 800));
      expect(applies(fake)).toHaveLength(n);
      expect((await commits.syncStatus()).state).toBe('in-sync');
    });

    it('boot (resumeSync): an agent whose last_txn_id is not running’s is put back on running', async () => {
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      await commits.commit(ADMIN, {});
      commits.onApplicationShutdown();
      staleAgent();
      const c3 = service();
      await c3.resumeSync();
      await vi.waitFor(() => expect(fake.current['interfaces']).toMatchObject({ loop1: {} }), {
        timeout: 6000,
        interval: 100,
      });
    });

    it('boot: the re-apply txn of a reconcile (config_sync, newer than the revision) is what the agent must have', async () => {
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      fake.nextApply = () => ({ status: ApplyStatus.APPLY_STATUS_DEGRADED, message: 'x' });
      await problem(commits.commit(ADMIN, {}));
      await vi.waitFor(async () => expect((await commits.syncStatus()).state).toBe('in-sync'), {
        timeout: 8000,
        interval: 100,
      });
      const reapplied = fake.lastTxnId;
      expect(repo.state.sync.txnId).toBe(reapplied);
      commits.onApplicationShutdown();
      const n = applies(fake).length;
      const c3 = service();
      await c3.resumeSync();
      await new Promise((r) => setTimeout(r, 800));
      expect(applies(fake)).toHaveLength(n);
    });

    it('boot: a confirm the agent completed while the API was down is saved at once', async () => {
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      const r = await commits.commit(ADMIN, { confirmSec: 30 });
      commits.onApplicationShutdown();
      fake.confirmPending();
      const c3 = service();
      await c3.resumePending();
      await c3.resumeSync();
      await vi.waitFor(() => expect(repo.state.revisions.map((x) => x.txnId)).toEqual([r.txnId]), {
        timeout: 3000,
        interval: 100,
      });
      expect(await c3.pendingInfo()).toBeNull();
    });

    it('agent reconnect: an agent that comes back with another last_txn_id is put back on running', async () => {
      const own = new AgentClient(env);
      const c3 = new CommitService(
        repo,
        new ValidationService(repo, own),
        own,
        events as unknown as SystemEventsService,
        bus,
        env,
        tokens as unknown as TokensService,
        audit as unknown as AuditService,
      );
      extra.push({ svc: c3, agent: own });
      await c3.resumeSync(); // boot: agent up and in step (nothing committed yet)
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      await c3.commit(ADMIN, {});
      await fake.stop();
      staleAgent();
      await fake.start(socket);
      await vi.waitFor(() => expect(fake.current['interfaces']).toMatchObject({ loop1: {} }), {
        timeout: 15_000,
        interval: 200,
      });
    });
  });

  // ------------------------------------------------------------------------------------------------ TD-9 review add-on

  describe('TD-9 review M4 / L8', () => {
    it('M4: a ROLLED_BACK answer with an error and a warning → the 422 has errors=[the error], warnings=[the warning]', async () => {
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      fake.nextApply = (req) => ({
        status: ApplyStatus.APPLY_STATUS_ROLLED_BACK,
        message: 'vpp said no',
        validation: {
          txnId: req.txnId,
          ok: false,
          errors: [
            {
              pointer: '/interfaces/loop1',
              message: 'no such device',
              severity: IssueSeverity.ISSUE_SEVERITY_ERROR,
              rule: 'interfaces.exists',
            },
            {
              pointer: '/system',
              message: 'domain system is not implemented by this agent build',
              severity: IssueSeverity.ISSUE_SEVERITY_WARNING,
              rule: 'agent.unimplemented-domain',
            },
          ],
          plan: [],
          summary: undefined,
        },
      });
      const p = await problem(commits.commit(ADMIN, {}));
      expect(p.status).toBe(422);
      expect(p.body['errors']).toEqual([
        { pointer: '/interfaces/loop1', message: 'no such device', rule: 'interfaces.exists' },
      ]);
      expect(p.body['warnings']).toEqual([
        {
          pointer: '/system',
          message: 'domain system is not implemented by this agent build',
          rule: 'agent.unimplemented-domain',
        },
      ]);
    });

    it('L8: the reconcile waits while the agent still finishes the lost Apply, then saves it (no re-apply over it)', async () => {
      const c2 = service(shortEnv('300'), true);
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      fake.applyDelayMs = 800;
      const p = await problem(c2.commit(ADMIN, {}));
      expect(p.body['type']).toBe('https://vrx.dev/problems/running-unknown');
      const txn = fake.lastTxnId;
      // the agent is still running that Apply (its lock held): last_txn_id not yet updated
      fake.lastTxnId = 'previous';
      fake.reconcileInProgress = true;
      fake.applyDelayMs = 0;
      const n = applies(fake).length;
      await new Promise((r) => setTimeout(r, 1500));
      expect(applies(fake)).toHaveLength(n); // no re-apply of running queued behind the agent's Apply
      fake.lastTxnId = txn;
      fake.reconcileInProgress = false;
      await vi.waitFor(async () => expect((await c2.syncStatus()).state).toBe('in-sync'), {
        timeout: 8000,
        interval: 100,
      });
      expect(repo.state.revisions.map((x) => x.txnId)).toEqual([txn]);
      expect(applies(fake)).toHaveLength(n);
    });
  });

  // ------------------------------------------------------------------------------------------------ fix round 1

  describe('fix round 1 (TD-10a-review.md)', () => {
    it('M2: a secret delete during a commit is 409 commit-busy after the lock wait, not queued', async () => {
      const slow = service(shortEnv('9000'), true);
      const tx = vi.fn(async () => undefined);
      const secrets = new SecretsService(
        { transaction: tx } as unknown as Db,
        env,
        events as unknown as SystemEventsService,
        slow,
      );
      fake.applyDelayMs = 4000;
      await ds.patchCandidate(ADMIN, '/system', { hostname: 'busy' });
      const first = slow.commit(ADMIN, {});
      await vi.waitFor(() => expect(applies(fake)).toHaveLength(1), {
        timeout: 3000,
        interval: 20,
      });
      const t0 = Date.now();
      const p = await problem(secrets.delete('psk', 'x'));
      expect(p.status).toBe(409);
      expect(p.body['type']).toBe('https://vrx.dev/problems/commit-busy');
      expect(Date.now() - t0).toBeLessThan(2500);
      expect(tx).not.toHaveBeenCalled();
      expect((await first).status).toBe('applied');
    });

    it('L1: a second API process does not undo the first one’s lost-answer recovery (sync re-read from the DB)', async () => {
      const a = service(shortEnv('300'), true);
      const b = service(); // another API process on the same database
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      expect((await a.commit(ADMIN, {})).status).toBe('applied');
      await ds.patchCandidate(ADMIN, '/interfaces/loop2', { ipv4: ['10.2.0.1/24'] });
      fake.applyDelayMs = 800; // C is applied, its answer is lost: A marks sync UNKNOWN in the database
      const p = await problem(a.commit(ADMIN, {}));
      expect(p.body['type']).toBe('https://vrx.dev/problems/running-unknown');
      const lostTxn = fake.lastTxnId;
      fake.applyDelayMs = 0;
      const n = applies(fake).length;
      await b.checkAgentTxn('agent reconcile'); // B saw C's RECONCILE_DONE — not its own transaction
      expect((await b.syncStatus()).state).toBe('in-sync');
      await vi.waitFor(async () => expect((await a.syncStatus()).state).toBe('in-sync'), {
        timeout: 6000,
        interval: 100,
      });
      expect(repo.state.revisions.at(-1)?.txnId).toBe(lostTxn); // A's recovery saved C
      expect(applies(fake)).toHaveLength(n); // nobody re-applied an older running
    });

    it('L4: confirm with no answer and no Health → running-unknown; the reconcile finds it confirmed and saves it', async () => {
      const c2 = service(shortEnv('300'), true);
      await ds.patchCandidate(ADMIN, '/interfaces/loop1', { ipv4: ['10.1.0.1/24'] });
      const r = await c2.commit(ADMIN, { confirmSec: 30 });
      fake.confirmDelayMs = 600;
      const confirming = problem(c2.confirm(ADMIN));
      await vi.waitFor(() => expect(fake.lastTxnId).toBe(r.txnId), { timeout: 2000, interval: 10 });
      fake.failAllWith = grpcStatus.UNAVAILABLE; // Health unreachable while the confirm answer is lost
      const p = await confirming;
      expect(p.body['type']).toBe('https://vrx.dev/problems/running-unknown');
      expect(p.body['sync']).toMatchObject({ state: 'unknown' });
      expect(repo.state.revisions).toEqual([]);
      fake.failAllWith = undefined;
      await vi.waitFor(async () => expect((await c2.syncStatus()).state).toBe('in-sync'), {
        timeout: 8000,
        interval: 100,
      });
      expect(repo.state.revisions.map((x) => x.txnId)).toEqual([r.txnId]);
      expect(await c2.pendingInfo()).toBeNull();
      expect(codes()).toContain('COMMIT_CONFIRMED');
    });
  });
});
