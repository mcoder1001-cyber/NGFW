import { DesiredState } from '@ngfw/proto';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { AgentClient } from '../../agent/agent.client.js';
import type { AuditService } from '../../audit/audit.service.js';
import type { SystemEventsService } from '../../audit/system-events.service.js';
import type { TokensService } from '../../auth/tokens.service.js';
import { CommitService } from '../../commit/commit.service.js';
import { ValidationService } from '../../commit/validation.service.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { emptyDocument } from '../../datastore/documents.js';
import { Bus } from '../../infra/bus.js';
import { FakeAgent } from '../../testing/fake-agent.js';
import { ADMIN, TEST_HASH, testEnv } from '../../testing/fixtures.js';
import { MemoryConfigRepo } from '../../testing/memory-repo.js';

describe('HA external commit uses ordinary atomic engine', () => {
  const dir = mkdtempSync(join(tmpdir(), 'ngfw-ha-'));
  const socket = join(dir, 'a.sock');
  const env = testEnv({ NGFW_AGENT_SOCKET: socket, NGFW_AGENT_OWNER: 'w1' });
  let fake: FakeAgent,
    agent: AgentClient,
    repo: MemoryConfigRepo,
    commits: CommitService,
    ds: DatastoreService;
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
    fake.reset(
      DesiredState.toJSON(ValidationService.desiredState(emptyDocument())) as Record<
        string,
        unknown
      >,
    );
    repo = new MemoryConfigRepo();
    repo.addUser('admin', 'admin', TEST_HASH);
    const events = { record: vi.fn(async () => undefined) },
      tokens = { revokeUser: vi.fn(async () => ({ persisted: true, families: 0 })) },
      audit = { write: vi.fn(async () => undefined) };
    commits = new CommitService(
      repo,
      new ValidationService(repo, agent),
      agent,
      events as unknown as SystemEventsService,
      new Bus(),
      env,
      tokens as unknown as TokensService,
      audit as unknown as AuditService,
    );
    ds = new DatastoreService(repo, env);
  });
  afterEach(() => commits.onApplicationShutdown());
  it('applies validated external config and persists cluster-sync origin', async () => {
    const r = await commits.clusterCommit(
      async (local) => ({ ...local, interfaces: { loop123: { ipv4: ['192.0.2.1/24'] } } }),
      'node-a revision 5',
    );
    expect(r.revision).toMatchObject({
      kind: 'cluster-sync',
      comment: 'cluster sync from node-a revision 5',
    });
    expect((await repo.latestRevision())?.payload).toHaveProperty('interfaces.loop123');
    expect((await repo.candidate()).payload).toBeNull();
  });
  it('refuses local dirty edits before running an external callback', async () => {
    await ds.patchCandidate(ADMIN, '/interfaces/loop123', { ipv4: ['192.0.2.1/24'] });
    const change = vi.fn(async (local) => local);
    await expect(commits.clusterCommit(change, 'node-a')).rejects.toMatchObject({
      slug: 'candidate-dirty',
    });
    expect(change).not.toHaveBeenCalled();
    expect(await repo.latestRevision()).toBeNull();
  });
  it('refuses a pending confirmation and leaves pending transaction untouched', async () => {
    await ds.patchCandidate(ADMIN, '/interfaces/loop123', { ipv4: ['192.0.2.1/24'] });
    const pending = await commits.commit(ADMIN, { confirmSec: 60 });
    expect(pending.status).toBe('pending');
    await expect(commits.clusterCommit(async (local) => local, 'node-a')).rejects.toMatchObject({
      slug: 'commit-pending',
    });
    expect((await repo.pending())?.txnId).toBe(pending.txnId);
    expect(await repo.latestRevision()).toBeNull();
  });
  it('validates schema before applying and leaves running unchanged on invalid sync', async () => {
    await expect(
      commits.clusterCommit(
        async (local) => ({ ...local, ha: { vrrp: { bad: { vrId: 999 } } } }),
        'node-a',
      ),
    ).rejects.toBeDefined();
    expect(await repo.latestRevision()).toBeNull();
    expect(fake.calls.filter((c) => c.method === 'Apply')).toHaveLength(0);
  });
});
